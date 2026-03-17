"""
BaseAgent — generic agent kernel with intrinsic tools and capability dispatch.

Key concepts:
    - **2-state lifecycle**: SLEEPING (waiting for inbox) and ACTIVE (processing).
    - **Persistent LLM session**: each agent keeps its chat session across messages.
    - **2-layer tool dispatch**: intrinsics (built-in) + capability handlers.
    - **Opaque context**: the host app can pass any context object — the agent
      stores it but never introspects it.
    - **4 optional services**: LLM, FileIO, Mail, Logging —
      missing service auto-disables the intrinsics it backs.
"""

from __future__ import annotations

import json
import os
import queue
from collections import deque
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Any, Callable

from .config import AgentConfig
from .state import AgentState
from .workdir import WorkingDir
from .message import Message, _make_message, MSG_REQUEST, MSG_USER_INPUT
from .intrinsics import ALL_INTRINSICS
from .prompt import SystemPromptManager
from .llm import (
    ChatSession,
    FunctionSchema,
    LLMResponse,
    LLMService,
    ToolCall,
)
from .llm_utils import (
    send_with_timeout,
    send_with_timeout_stream,
    track_llm_usage,
    _is_stale_interaction_error,
)
from .logging import get_logger
from .loop_guard import LoopGuard
from .prompt import build_system_prompt
from .token_counter import count_tokens, count_tool_tokens
from .tool_timing import ToolTimer, stamp_tool_result
from .types import UnknownToolError

logger = get_logger()


# ---------------------------------------------------------------------------
# BaseAgent
# ---------------------------------------------------------------------------


class BaseAgent:
    """Generic research agent with intrinsic tools and MCP tool dispatch.

    Services (all optional):
        - ``service`` (LLMService): The brain — thinking, generating text.
        - ``file_io`` (FileIOService): File access — backs read/edit/write/glob/grep.
        - ``mail_service`` (MailService): Message transport — backs mail intrinsic.

    Missing service = intrinsics backed by it are auto-disabled.

    Subclasses customize behavior via:
        - ``_pre_request(msg)`` — transform message before LLM send
        - ``_post_request(msg, result)`` — side effects after LLM responds
        - ``_handle_message(msg)`` — message routing (must call super for processing)
        - ``_get_guard_limits()`` — per-agent loop guard limits
        - ``_PARALLEL_SAFE_TOOLS`` — set of tool names safe for concurrent execution
    """

    agent_type: str = ""

    # Tools safe for concurrent execution
    _PARALLEL_SAFE_TOOLS: set[str] = set()

    # Inbox polling interval (seconds)
    _inbox_timeout: float = 1.0

    def __init__(
        self,
        agent_id: str,
        service: LLMService,
        *,
        file_io: Any | None = None,
        mail_service: Any | None = None,
        config: AgentConfig | None = None,
        base_dir: str | Path,
        context: Any = None,
        admin: bool = False,
        streaming: bool = False,
        logging_service: Any | None = None,
        role: str = "",
        ltm: str = "",
    ):
        self.agent_id = agent_id
        self.service = service
        self._config = config or AgentConfig()
        self._context = context
        self._admin = admin
        self._cancel_event = threading.Event()
        self._cancel_mail: dict | None = None
        self._started_at: str = ""
        self._uptime_anchor: float | None = None  # set in start(), None means not started
        self._streaming = streaming

        # Base directory (shared root) and working directory (per-agent)
        self._base_dir = Path(base_dir)
        if not self._base_dir.is_dir():
            raise FileNotFoundError(f"base_dir does not exist: {self._base_dir}")
        self._workdir = WorkingDir(base_dir=base_dir, agent_id=agent_id)
        self._working_dir = self._workdir.path

        # LoggingService: auto-create in working dir if not provided
        if logging_service is not None:
            self._log_service = logging_service
        else:
            from .services.logging import JSONLLoggingService
            log_dir = self._working_dir / "logs"
            log_dir.mkdir(exist_ok=True)
            self._log_service = JSONLLoggingService(log_dir / "events.jsonl")

        # Acquire working directory lock
        self._workdir.acquire_lock()

        # --- Wire services ---
        # FileIOService: auto-create LocalFileIOService for backward compat
        if file_io is not None:
            self._file_io = file_io
        else:
            from .services.file_io import LocalFileIOService
            self._file_io = LocalFileIOService(root=self._working_dir)

        # MailService: None means mail intrinsic disabled
        self._mail_service = mail_service

        # Read manifest for resume (before prompt manager, so role can be restored)
        manifest_role, manifest_ltm = self._workdir.read_manifest()
        if not role and manifest_role:
            role = manifest_role

        # LTM and role file paths — renamed to covenant/memory
        system_dir = self._working_dir / "system"
        memory_file = system_dir / "memory.md"
        covenant_file = system_dir / "covenant.md"

        # If constructor ltm is provided and memory file doesn't exist, write it
        if ltm and not memory_file.is_file():
            system_dir.mkdir(exist_ok=True)
            memory_file.write_text(ltm)
        # If manifest has ltm and file doesn't exist, migrate
        elif manifest_ltm and not memory_file.is_file():
            system_dir.mkdir(exist_ok=True)
            memory_file.write_text(manifest_ltm)

        # If constructor role is provided and covenant file doesn't exist, write it
        if role and not covenant_file.is_file():
            system_dir.mkdir(exist_ok=True)
            covenant_file.write_text(role)

        # Auto-load memory from file into prompt manager
        loaded_memory = ""
        if memory_file.is_file():
            loaded_memory = memory_file.read_text()

        # System prompt manager
        self._prompt_manager = SystemPromptManager()
        if role:
            self._prompt_manager.write_section("covenant", role, protected=True)
        if loaded_memory.strip():
            self._prompt_manager.write_section("memory", loaded_memory)

        # Write manifest (without memory — it now lives in system/memory.md)
        from datetime import datetime, timezone
        manifest_data = {
            "agent_id": self.agent_id,
            "started_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "role": self._prompt_manager.read_section("covenant") or "",
        }
        if self._mail_service is not None and self._mail_service.address:
            manifest_data["address"] = self._mail_service.address
        self._workdir.write_manifest(manifest_data)

        # Mail FIFO queue — incoming messages consumed by read
        self._mail_queue: deque[dict] = deque()
        self._mail_queue_lock = threading.Lock()
        self._mail_arrived = threading.Event()  # set when normal mail arrives; clock wait uses this

        # MCP tool handlers
        self._mcp_handlers: dict[str, Callable[[dict], dict]] = {}
        self._mcp_schemas: list[FunctionSchema] = []


        # --- Wire intrinsic tools ---
        self._intrinsics: dict[str, Callable[[dict], dict]] = {}
        self._wire_intrinsics()

        # Inbox
        self.inbox: queue.Queue[Message] = queue.Queue()

        # Lifecycle
        self._shutdown = threading.Event()
        self._thread: threading.Thread | None = None
        self._idle = threading.Event()
        self._idle.set()
        self._state = AgentState.SLEEPING
        self._sealed = False

        # Persistent LLM session
        self._chat: ChatSession | None = None
        self._interaction_id: str | None = None
        self._guard: LoopGuard | None = None

        # Token tracking
        self._total_input_tokens = 0
        self._total_output_tokens = 0
        self._total_thinking_tokens = 0
        self._total_cached_tokens = 0
        self._api_calls = 0
        self._last_tool_context = "send_message"
        self._system_prompt_tokens = 0
        self._tools_tokens = 0
        self._token_decomp_dirty = True
        self._latest_input_tokens = 0

        # Streaming state
        self._text_already_streamed = False
        self._intermediate_text_streamed = False
        self._message_seq = 0

        # Timeout pool for LLM calls
        self._timeout_pool = ThreadPoolExecutor(max_workers=1)

    # ------------------------------------------------------------------
    # Intrinsic wiring
    # ------------------------------------------------------------------

    def _wire_intrinsics(self) -> None:
        """Wire kernel intrinsic tool handlers."""
        for name, info in ALL_INTRINSICS.items():
            handle_fn = info["handle"]
            self._intrinsics[name] = lambda args, fn=handle_fn: fn(self, args)

    # ------------------------------------------------------------------
    # Properties
    # ------------------------------------------------------------------

    @property
    def is_idle(self) -> bool:
        return self._idle.is_set()

    @property
    def state(self) -> AgentState:
        return self._state

    @property
    def working_dir(self) -> Path:
        """The agent's working directory."""
        return self._workdir.path

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        """Start the agent's main loop thread."""
        self._sealed = True
        if self._thread and self._thread.is_alive():
            return
        self._shutdown.clear()

        # Initialize git repo in working directory (first start only)
        self._workdir.init_git()

        # Capture startup time for uptime tracking
        from datetime import datetime, timezone
        self._started_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        self._uptime_anchor = time.monotonic()

        # Start MailService listener if configured
        if self._mail_service is not None:
            try:
                self._mail_service.listen(on_message=lambda payload: self._on_mail_received(payload))
            except RuntimeError:
                pass  # Already listening or no listen_port — that's fine

        self._thread = threading.Thread(
            target=self._run_loop,
            daemon=True,
            name=f"agent-{self.agent_id}",
        )
        self._thread.start()

    def stop(self, timeout: float = 5.0) -> None:
        """Signal shutdown and wait for the agent thread to exit."""
        self._log("agent_stop")
        self._shutdown.set()
        if self._thread:
            self._thread.join(timeout=timeout)
        self._timeout_pool.shutdown(wait=False)

        # Stop MailService if configured
        if self._mail_service is not None:
            try:
                self._mail_service.stop()
            except Exception:
                pass

        # Close LoggingService if configured
        if self._log_service is not None:
            try:
                self._log_service.close()
            except Exception:
                pass

        # Persist memory from prompt manager to file
        memory_content = self._prompt_manager.read_section("memory") or ""
        memory_file = self._working_dir / "system" / "memory.md"
        if memory_file.is_file() or memory_content:
            memory_file.parent.mkdir(exist_ok=True)
            memory_file.write_text(memory_content)

        # Persist final state and release lock
        from datetime import datetime, timezone
        manifest_data = {
            "agent_id": self.agent_id,
            "started_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "role": self._prompt_manager.read_section("covenant") or "",
        }
        if self._mail_service is not None and self._mail_service.address:
            manifest_data["address"] = self._mail_service.address
        self._workdir.write_manifest(manifest_data)
        self._workdir.release_lock()

    def _on_mail_received(self, payload: dict) -> None:
        """Callback for MailService — routes by mail type.

        Cancel-type emails bypass the queue and set the cancel event directly.
        Normal emails are delegated to ``_on_normal_mail`` (which capabilities
        like email can replace).

        This method is never replaced — it is the stable entry point for all
        incoming mail.
        """
        mail_type = payload.get("type", "normal")

        if mail_type == "cancel":
            self._cancel_mail = payload
            self._cancel_event.set()
            self._log(
                "cancel_received",
                sender=payload.get("from", "unknown"),
                subject=payload.get("subject", ""),
            )
            return

        self._on_normal_mail(payload)

    def _on_normal_mail(self, payload: dict) -> None:
        """Handle a normal mail — enqueue in FIFO and notify agent.

        Capabilities (e.g. email) replace this method to provide richer
        mail handling (mailbox, notifications, etc.).
        """
        from datetime import datetime, timezone

        sender = payload.get("from", "unknown")
        subject = payload.get("subject", "")
        message = payload.get("message", "")
        timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

        entry = {
            "from": sender,
            "to": payload.get("to", ""),
            "subject": subject,
            "message": message,
            "time": timestamp,
        }
        with self._mail_queue_lock:
            self._mail_queue.append(entry)
        self._mail_arrived.set()

        # Notify agent with full content inline
        notification = (
            f'[Mail from {sender}]\n'
            f'Subject: {subject}\n'
            f'{message}'
        )
        self._log("mail_received", sender=sender, subject=subject, message=message)
        msg = _make_message(MSG_REQUEST, sender, notification)
        self.inbox.put(msg)

    def _set_state(self, new_state: AgentState, reason: str = "") -> None:
        """Transition to a new state, keeping _idle in sync."""
        old = self._state
        if old == new_state:
            return
        self._state = new_state
        if new_state == AgentState.SLEEPING:
            self._idle.set()
        else:
            self._idle.clear()
        self._log("agent_state", old=old.value, new=new_state.value, reason=reason)

    def _log(self, event_type: str, **fields) -> None:
        """Write a structured event to the logging service, if configured."""
        if self._log_service:
            self._log_service.log({
                "type": event_type,
                "agent_id": self.agent_id,
                "ts": time.time(),
                **fields,
            })

    # ------------------------------------------------------------------
    # Main loop (final — do not override)
    # ------------------------------------------------------------------

    def _run_loop(self) -> None:
        """Wait for messages, process them. Agent persists between messages."""
        while not self._shutdown.is_set():
            try:
                msg = self.inbox.get(timeout=self._inbox_timeout)
            except queue.Empty:
                continue
            self._set_state(AgentState.ACTIVE, reason=f"received {msg.type}")
            try:
                self._handle_message(msg)
            except Exception as e:
                err_desc = str(e) or repr(e)
                logger.error(
                    f"[{self.agent_id}] Unhandled error in message handler: {err_desc}",
                    exc_info=True,
                )
                self._log("error", source="message_handler", message=err_desc)
                if msg._reply_event:
                    msg._reply_value = {
                        "text": f"Internal error: {err_desc}",
                        "failed": True,
                        "errors": [err_desc],
                    }
                    msg._reply_event.set()
            finally:
                self._set_state(AgentState.SLEEPING, reason="all done")

    # ------------------------------------------------------------------
    # Message handling
    # ------------------------------------------------------------------

    def _handle_message(self, msg: Message) -> None:
        """Route message by type. Subclasses may override for routing."""
        if msg.type in (MSG_REQUEST, MSG_USER_INPUT):
            self._handle_request(msg)
        else:
            logger.warning(f"[{self.agent_id}] Unknown message type: {msg.type}")

    def _handle_request(self, msg: Message) -> None:
        """Send request to LLM, process response with tool calls."""
        from datetime import datetime, timezone

        max_calls, dup_free, dup_hard = self._get_guard_limits()
        self._guard = LoopGuard(
            max_total_calls=max_calls,
            dup_free_passes=dup_free,
            dup_hard_block=dup_hard,
        )
        content = self._pre_request(msg)
        current_time = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        content = f"[Current time: {current_time}]\n\n{content}"
        response = self._llm_send(content)
        result = self._process_response(response)
        self._post_request(msg, result)
        self._deliver_result(msg, result)

    def _get_guard_limits(self) -> tuple[int, int, int]:
        """Return (max_total_calls, dup_free_passes, dup_hard_block).

        Uses config.max_turns as the basis.
        """
        max_turns = self._config.max_turns
        return (max_turns, 2, 8)

    # ------------------------------------------------------------------
    # Response processing
    # ------------------------------------------------------------------

    def _process_response(self, response: LLMResponse) -> dict:
        """Handle tool calls and collect text output.

        Returns a result dict: {"text": ..., "failed": ..., "errors": [...]}.
        """
        guard = self._guard or LoopGuard(max_total_calls=self._config.max_turns)
        collected_text_parts: list[str] = []
        collected_errors: list[str] = []

        while True:
            if response.text:
                collected_text_parts.append(response.text)
                self._log("diary", text=response.text)
                if response.tool_calls:
                    self._intermediate_text_streamed = False

            if response.thoughts:
                for thought in response.thoughts:
                    self._log("thinking", text=thought)

            if not response.tool_calls:
                break

            if self._cancel_event.is_set():
                return self._handle_cancel_diary()

            stop_reason = guard.check_limit(len(response.tool_calls))
            if stop_reason:
                break

            # Check for invalid tool names
            invalid_reason = guard.check_invalid_tool_limit()
            if invalid_reason:
                break

            all_parallel_safe = (
                len(response.tool_calls) > 1
                and self._PARALLEL_SAFE_TOOLS
                and all(
                    tc.name in self._PARALLEL_SAFE_TOOLS
                    for tc in response.tool_calls
                )
            )

            if all_parallel_safe:
                tool_results, intercepted, intercept_text = (
                    self._execute_tools_parallel(
                        response.tool_calls, guard, collected_errors,
                    )
                )
            else:
                tool_results, intercepted, intercept_text = (
                    self._execute_tools_sequential(
                        response.tool_calls, guard, collected_errors,
                    )
                )

            if intercepted:
                if tool_results and self._chat:
                    self._chat.commit_tool_results(tool_results)
                return {
                    "text": intercept_text,
                    "failed": False,
                    "errors": [],
                }

            guard.record_calls(len(response.tool_calls))

            # Break on repeated identical errors
            if (
                len(collected_errors) >= 2
                and collected_errors[-1] == collected_errors[-2]
            ):
                logger.warning(
                    "[%s] Same error repeated, breaking early: %s",
                    self.agent_id,
                    collected_errors[-1],
                )
                break

            response = self._llm_send(tool_results)

        final_text = "\n".join(collected_text_parts)
        has_errors = bool(collected_errors)
        no_useful_output = not final_text.strip()
        return {
            "text": final_text,
            "failed": has_errors and no_useful_output,
            "errors": collected_errors,
        }

    def _handle_cancel_diary(self) -> dict:
        """Handle cancellation triggered by a cancel email.

        Sends one final LLM call asking the agent to write a diary entry
        summarizing its work, then returns the diary text as the response.
        """
        cancel_mail = self._cancel_mail
        self._cancel_event.clear()

        diary_text = ""
        if cancel_mail and self._chat:
            sender = cancel_mail.get("from", "unknown")
            subject = cancel_mail.get("subject", "")
            message = cancel_mail.get("message", "")

            prompt = (
                f"[CANCELLED] You have been stopped by a cancel email.\n"
                f"From: {sender}\n"
                f"Subject: {subject}\n"
                f"Message: {message}\n\n"
                f"Write a brief diary entry summarizing what you were working on "
                f"and where you left off, so you can resume later."
            )
            try:
                response = self._chat.send(prompt)
                diary_text = response.text or ""
                self._log("cancel_diary", text=diary_text)
            except Exception as exc:
                logger.warning(
                    "[%s] Diary LLM call failed during cancel: %s",
                    self.agent_id, exc,
                )
                diary_text = (
                    f"[Cancelled by {sender}] "
                    f"Diary generation failed: {exc}"
                )

        self._cancel_mail = None
        return {
            "text": diary_text,
            "failed": False,
            "errors": [],
        }

    # ------------------------------------------------------------------
    # Tool dispatch — 2-layer
    # ------------------------------------------------------------------

    def _dispatch_tool(self, tc: ToolCall) -> dict:
        """Dispatch a tool call to the appropriate handler.

        Layer 1: intrinsics (built-in tools)
        Layer 2: MCP handlers (domain tools)

        Raises UnknownToolError if the tool name is not found.
        """
        if tc.name in self._intrinsics:
            return self._intrinsics[tc.name](tc.args or {})
        elif tc.name in self._mcp_handlers:
            return self._mcp_handlers[tc.name](tc.args or {})
        else:
            raise UnknownToolError(tc.name)

    def _execute_single_tool(
        self,
        tc: ToolCall,
        guard: LoopGuard,
        collected_errors: list[str],
    ) -> tuple[Any, bool, str]:
        """Execute a single tool call.

        Returns (result_msg, intercepted, intercept_text).
        """
        tc_id = getattr(tc, "id", None)
        args = dict(tc.args) if tc.args else {}
        reasoning = args.pop("reasoning", None)
        args.pop("commentary", None)
        args.pop("_sync", None)

        # Log reasoning as diary entry
        if reasoning:
            self._log("tool_reasoning", tool=tc.name, reasoning=reasoning)
            args["_reasoning"] = reasoning  # preserve for handlers that need it (e.g. delegate)

        verdict = guard.record_tool_call(tc.name, args)
        if verdict.blocked:
            result = {
                "status": "blocked",
                "_duplicate_warning": verdict.warning,
                "message": f"Execution skipped — duplicate call #{verdict.count}",
            }
            msg = self.service.make_tool_result(
                tc.name, result, tool_call_id=tc_id,
                provider=self._config.provider,
            )
            self._log("tool_result", tool_name=tc.name, status="blocked", elapsed_ms=0)
            return msg, False, ""

        self._log("tool_call", tool_name=tc.name, tool_args=args)
        timer = ToolTimer()
        try:
            # Check for unknown tool first
            if tc.name not in self._intrinsics and tc.name not in self._mcp_handlers:
                guard.record_invalid_tool(tc.name)
                raise UnknownToolError(tc.name)

            with timer:
                result = self._dispatch_tool(
                    ToolCall(name=tc.name, args=args, id=tc_id)
                )

            if isinstance(result, dict):
                stamp_tool_result(result, timer.elapsed_ms)

            status = result.get("status", "success") if isinstance(result, dict) else "success"
            self._log("tool_result", tool_name=tc.name, status=status, elapsed_ms=timer.elapsed_ms)

            if verdict.warning and isinstance(result, dict):
                result["_duplicate_warning"] = verdict.warning

            # Check for intercept sentinel
            if isinstance(result, dict) and result.get("intercept"):
                intercept_text = result.get("text", "")
                result_msg = self.service.make_tool_result(
                    tc.name, result, tool_call_id=tc_id,
                    provider=self._config.provider,
                )
                return result_msg, True, intercept_text

            result_msg = self.service.make_tool_result(
                tc.name, result, tool_call_id=tc_id,
                provider=self._config.provider,
            )

            if isinstance(result, dict) and result.get("status") == "error":
                err_msg = result.get("message", "unknown error")
                collected_errors.append(f"{tc.name}: {err_msg}")

            # Run interception hook
            intercept = self._on_tool_result_hook(tc.name, args, result)
            if intercept is not None:
                return result_msg, True, intercept

            return result_msg, False, ""

        except Exception as e:
            err_result = {"status": "error", "message": str(e)}
            stamp_tool_result(err_result, timer.elapsed_ms)
            result_msg = self.service.make_tool_result(
                tc.name, err_result, tool_call_id=tc_id,
                provider=self._config.provider,
            )
            collected_errors.append(f"{tc.name}: {e}")
            self._log("error", source=tc.name, message=str(e))
            return result_msg, False, ""

    def _execute_tools_sequential(
        self,
        tool_calls: list[ToolCall],
        guard: LoopGuard,
        collected_errors: list[str],
    ) -> tuple[list, bool, str]:
        """Run tool calls one at a time on the agent thread.

        Returns (tool_results, intercepted, intercept_text).
        """
        tool_results = []
        for tc in tool_calls:
            if self._cancel_event.is_set():
                return [], False, ""
            result_msg, intercepted, intercept_text = self._execute_single_tool(
                tc, guard, collected_errors,
            )
            if result_msg is not None:
                tool_results.append(result_msg)
            if intercepted:
                return tool_results, True, intercept_text
        return tool_results, False, ""

    def _execute_tools_parallel(
        self,
        tool_calls: list[ToolCall],
        guard: LoopGuard,
        collected_errors: list[str],
    ) -> tuple[list, bool, str]:
        """Run multiple tool calls concurrently via ThreadPoolExecutor.

        Returns (tool_results, intercepted, intercept_text).
        """
        # Phase 1: Pre-check duplicates (sequential — guard not thread-safe)
        to_execute: list[tuple[int, ToolCall, dict]] = []
        tool_results: list[tuple[int, Any]] = []

        for i, tc in enumerate(tool_calls):
            tc_id = getattr(tc, "id", None)
            args = dict(tc.args) if tc.args else {}
            reasoning = args.pop("reasoning", None)
            args.pop("commentary", None)
            args.pop("_sync", None)

            if reasoning:
                self._log("tool_reasoning", tool=tc.name, reasoning=reasoning)
                args["_reasoning"] = reasoning  # preserve for handlers that need it (e.g. delegate)

            verdict = guard.record_tool_call(tc.name, args)
            if verdict.blocked:
                result = {
                    "status": "blocked",
                    "_duplicate_warning": verdict.warning,
                    "message": f"Execution skipped — duplicate call #{verdict.count}",
                }
                tool_results.append((i, self.service.make_tool_result(
                    tc.name, result, tool_call_id=tc_id,
                    provider=self._config.provider,
                )))
            else:
                to_execute.append((i, tc, args))

        if not to_execute:
            tool_results.sort(key=lambda x: x[0])
            return [r for _, r in tool_results], False, ""

        # Phase 2: Execute in parallel
        results_map: dict[int, Any] = {}
        errors_map: dict[int, str] = {}

        def _run_one(index: int, tc: ToolCall, args: dict):
            timer = ToolTimer()
            with timer:
                result = self._dispatch_tool(
                    ToolCall(name=tc.name, args=args, id=tc.id)
                )
            if isinstance(result, dict):
                stamp_tool_result(result, timer.elapsed_ms)
            return index, result

        pool = ThreadPoolExecutor(max_workers=len(to_execute))
        try:
            futures = {
                pool.submit(_run_one, i, tc, args): i
                for i, tc, args in to_execute
            }
            for future in as_completed(futures, timeout=300.0):
                if self._cancel_event.is_set():
                    pool.shutdown(wait=False, cancel_futures=True)
                    return [], False, ""
                try:
                    idx, result = future.result()
                    results_map[idx] = result
                except Exception as e:
                    idx = futures[future]
                    errors_map[idx] = str(e)
        except TimeoutError:
            for future, idx in futures.items():
                if idx not in results_map and idx not in errors_map:
                    errors_map[idx] = "Timed out"
        finally:
            pool.shutdown(wait=False, cancel_futures=True)

        # Phase 3: Build result messages (sequential)
        for i, tc, args in to_execute:
            tc_id = getattr(tc, "id", None)
            if i in results_map:
                result = results_map[i]
                tool_results.append((i, self.service.make_tool_result(
                    tc.name, result, tool_call_id=tc_id,
                    provider=self._config.provider,
                )))
                if isinstance(result, dict) and result.get("status") == "error":
                    err_msg = result.get("message", "unknown error")
                    collected_errors.append(f"{tc.name}: {err_msg}")
                # Check intercept
                if isinstance(result, dict) and result.get("intercept"):
                    tool_results.sort(key=lambda x: x[0])
                    return (
                        [r for _, r in tool_results],
                        True,
                        result.get("text", ""),
                    )
            elif i in errors_map:
                err_msg = errors_map[i]
                err_result = {"status": "error", "message": err_msg}
                tool_results.append((i, self.service.make_tool_result(
                    tc.name, err_result, tool_call_id=tc_id,
                    provider=self._config.provider,
                )))
                collected_errors.append(f"{tc.name}: {err_msg}")

        tool_results.sort(key=lambda x: x[0])
        return [r for _, r in tool_results], False, ""

    # ------------------------------------------------------------------
    # LLM communication
    # ------------------------------------------------------------------

    def _build_system_prompt(self) -> str:
        """Build the system prompt from base + sections."""
        return build_system_prompt(prompt_manager=self._prompt_manager)

    def _build_tool_schemas(self) -> list[FunctionSchema]:
        """Build the complete tool schema list for the LLM.

        Every tool gets a 'reasoning' parameter injected — the agent must
        explain why it's calling this tool. Reasoning is logged as part of
        the agent's diary and stripped before the handler runs.
        """
        reasoning_prop = {
            "reasoning": {
                "type": "string",
                "description": "Brief explanation of why you are calling this tool (recorded in your diary).",
            },
        }

        schemas = []

        # Intrinsic schemas
        for name in self._intrinsics:
            info = ALL_INTRINSICS.get(name)
            if info:
                params = dict(info["schema"])
                props = dict(params.get("properties", {}))
                props.update(reasoning_prop)
                params["properties"] = props
                schemas.append(
                    FunctionSchema(
                        name=name,
                        description=info["description"],
                        parameters=params,
                    )
                )

        # Capability + MCP schemas — inject reasoning into each
        for s in self._mcp_schemas:
            params = dict(s.parameters)
            props = dict(params.get("properties", {}))
            props.update(reasoning_prop)
            params["properties"] = props
            schemas.append(
                FunctionSchema(
                    name=s.name,
                    description=s.description,
                    parameters=params,
                )
            )

        return schemas

    def _ensure_session(self) -> ChatSession:
        """Ensure a persistent LLM session exists, creating one if needed."""
        if self._chat is None:
            self._chat = self.service.create_session(
                system_prompt=self._build_system_prompt(),
                tools=self._build_tool_schemas() or None,
                model=self._config.model or self.service.model,
                thinking="high",
                agent_type=self.agent_id,
                tracked=True,
                interaction_id=self._interaction_id,
                provider=self._config.provider,
            )
        return self._chat

    def _llm_send(self, message: Any) -> LLMResponse:
        """Send a message to the LLM, reusing the persistent chat session."""
        self._ensure_session()

        self._check_and_compact()

        self._log("llm_call", model=self._config.model or self.service.model or "unknown")

        retry_timeout = self._config.retry_timeout

        try:
            if self._streaming:
                response = self._llm_send_streaming(message, retry_timeout)
            else:
                response = send_with_timeout(
                    chat=self._chat,
                    message=message,
                    timeout_pool=self._timeout_pool,
                    retry_timeout=retry_timeout,
                    agent_name=self.agent_id,
                    logger=logger,
                    on_reset=self._on_reset,
                )
        except Exception as exc:
            # Handle stale Interactions API session
            if self._interaction_id and _is_stale_interaction_error(exc):
                self._interaction_id = None
                self._chat = self.service.create_session(
                    system_prompt=self._build_system_prompt(),
                    tools=self._build_tool_schemas() or None,
                    model=self._config.model or self.service.model,
                    thinking="high",
                    agent_type=self.agent_id,
                    tracked=True,
                    provider=self._config.provider,
                )
                if self._streaming:
                    response = self._llm_send_streaming(message, retry_timeout)
                else:
                    response = send_with_timeout(
                        chat=self._chat,
                        message=message,
                        timeout_pool=self._timeout_pool,
                        retry_timeout=retry_timeout,
                        agent_name=self.agent_id,
                        logger=logger,
                    )
            else:
                raise

        self._track_usage(response)
        # Preserve interaction ID for session reuse
        if hasattr(self._chat, "interaction_id") and self._chat.interaction_id:
            self._interaction_id = self._chat.interaction_id
        return response

    def _llm_send_streaming(
        self, message: Any, retry_timeout: float
    ) -> LLMResponse:
        """Streaming LLM send via send_stream."""
        self._message_seq += 1

        response = send_with_timeout_stream(
            chat=self._chat,
            message=message,
            timeout_pool=self._timeout_pool,
            retry_timeout=retry_timeout,
            agent_name=self.agent_id,
            logger=logger,
            on_reset=self._on_reset,
        )

        if response.text:
            if response.tool_calls:
                self._intermediate_text_streamed = True
            else:
                self._text_already_streamed = True

        return response

    def _on_reset(self, chat, failed_message):
        """Rollback reset: new chat, drop failed turn, inject context."""
        from .llm.interface import ToolResultBlock, ToolCallBlock

        iface = chat.interface

        # Summarize tool calls from last assistant turn
        parts = []
        last_asst = iface.last_assistant_entry()
        if last_asst:
            for block in last_asst.content:
                if isinstance(block, ToolCallBlock):
                    args_str = ", ".join(
                        f"{k}={repr(v)[:80]}" for k, v in block.args.items()
                    )
                    parts.append(f"- {block.name}({args_str})")
        tool_summary = "\n".join(parts) if parts else "(no tool calls found)"

        # Drop failed turn
        iface.drop_trailing(lambda e: e.role == "assistant")
        iface.drop_trailing(
            lambda e: e.role == "user"
            and all(isinstance(b, ToolResultBlock) for b in e.content)
        )

        self._chat = self.service.create_session(
            system_prompt=self._build_system_prompt(),
            tools=self._build_tool_schemas() or None,
            model=self._config.model or self.service.model,
            thinking="high",
            tracked=False,
            provider=self._config.provider,
            interface=iface,
        )
        self._log("llm_reset", entries_kept=len(iface.entries))

        rollback_msg = (
            "Your previous response was lost due to a server error. "
            "Here is what happened:\n\n"
            f"You called these tools:\n{tool_summary}\n\n"
            "Data already fetched is still available in memory. "
            "Please continue based on these results."
        )
        return self._chat, rollback_msg

    # ------------------------------------------------------------------
    # Compaction
    # ------------------------------------------------------------------

    def _check_and_compact(self) -> None:
        """Check context usage and compact messages if nearing the limit."""
        if self._chat is None:
            return

        from .llm.service import COMPACTION_PROMPT

        agent_prompt = self._chat.interface.current_system_prompt or ""
        ctx_window = self._chat.context_window()
        target_tokens = int(ctx_window * 0.2) if ctx_window > 0 else 2048

        def summarizer(text: str) -> str:
            prompt_parts = [COMPACTION_PROMPT]
            prompt_parts.append(
                f"\nTarget summary length: ~{target_tokens} tokens "
                f"(20% of {ctx_window} token context window).\n"
            )
            if agent_prompt:
                prompt_parts.append(
                    f"\nThe agent's role:\n{agent_prompt}\n\n"
                    "Do your best to help this agent based on its role.\n"
                )
            prompt_parts.append(f"\nConversation history:\n{text}")
            response = self.service.generate(
                "".join(prompt_parts),
                temperature=0.1,
                max_output_tokens=target_tokens,
            )
            return response.text.strip() if response and response.text else ""

        new_chat = self.service.check_and_compact(
            self._chat,
            summarizer=summarizer,
            threshold=0.8,
            provider=self._config.provider,
        )
        if new_chat is not None:
            before_tokens = self._chat.interface.estimate_context_tokens()
            after_tokens = new_chat.interface.estimate_context_tokens()
            self._chat = new_chat
            self._interaction_id = None
            self._log("compaction", before_tokens=before_tokens, after_tokens=after_tokens)

    # ------------------------------------------------------------------
    # Token tracking
    # ------------------------------------------------------------------

    def _update_token_decomposition(self) -> None:
        """Recompute cached system prompt and tools token counts."""
        self._system_prompt_tokens = count_tokens(self._build_system_prompt())
        self._tools_tokens = count_tool_tokens(self._build_tool_schemas())
        self._token_decomp_dirty = False

    def _track_usage(self, response: LLMResponse) -> None:
        """Accumulate token usage from an LLMResponse."""
        if self._token_decomp_dirty:
            self._update_token_decomposition()
        token_state = {
            "input": self._total_input_tokens,
            "output": self._total_output_tokens,
            "thinking": self._total_thinking_tokens,
            "cached": self._total_cached_tokens,
            "api_calls": self._api_calls,
        }
        track_llm_usage(
            response=response,
            token_state=token_state,
            agent_name=self.agent_id,
            last_tool_context=self._last_tool_context,
            system_tokens=self._system_prompt_tokens,
            tools_tokens=self._tools_tokens,
        )
        self._total_input_tokens = token_state["input"]
        self._total_output_tokens = token_state["output"]
        self._total_thinking_tokens = token_state["thinking"]
        self._total_cached_tokens = token_state["cached"]
        self._api_calls = token_state["api_calls"]
        if response.usage:
            self._latest_input_tokens = response.usage.input_tokens
            self._log(
                "llm_response",
                input_tokens=response.usage.input_tokens,
                output_tokens=response.usage.output_tokens,
                thinking_tokens=response.usage.thinking_tokens,
                cached_tokens=response.usage.cached_tokens,
            )

    def get_token_usage(self) -> dict:
        """Return token usage summary."""
        return {
            "input_tokens": self._total_input_tokens,
            "output_tokens": self._total_output_tokens,
            "thinking_tokens": self._total_thinking_tokens,
            "cached_tokens": self._total_cached_tokens,
            "total_tokens": (
                self._total_input_tokens
                + self._total_output_tokens
                + self._total_thinking_tokens
            ),
            "api_calls": self._api_calls,
            "ctx_system_tokens": self._system_prompt_tokens,
            "ctx_tools_tokens": self._tools_tokens,
            "ctx_history_tokens": max(
                0,
                self._latest_input_tokens
                - self._system_prompt_tokens
                - self._tools_tokens,
            ),
            "ctx_total_tokens": self._latest_input_tokens,
        }

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def mail(self, address: str, message: str, subject: str = "") -> dict:
        """Send a message to another agent (public API). Requires MailService."""
        return self._intrinsics["mail"]({"action": "send", "address": address, "message": message, "subject": subject})

    def add_tool(
        self,
        name: str,
        *,
        schema: dict | None = None,
        handler: Callable[[dict], dict] | None = None,
        description: str = "",
    ) -> None:
        """Register a dynamic tool."""
        if self._sealed:
            raise RuntimeError("Cannot modify tools after start()")
        if handler is not None:
            self._mcp_handlers[name] = handler
        if schema is not None:
            # Remove any existing schema with same name
            self._mcp_schemas = [s for s in self._mcp_schemas if s.name != name]
            self._mcp_schemas.append(
                FunctionSchema(
                    name=name,
                    description=description,
                    parameters=schema,
                )
            )
        # Update the live session's tools if one exists
        if self._chat is not None:
            self._chat.update_tools(self._build_tool_schemas())
        self._token_decomp_dirty = True

    def remove_tool(self, name: str) -> None:
        """Unregister a dynamic tool."""
        if self._sealed:
            raise RuntimeError("Cannot modify tools after start()")
        self._mcp_handlers.pop(name, None)
        self._mcp_schemas = [s for s in self._mcp_schemas if s.name != name]
        if self._chat is not None:
            self._chat.update_tools(self._build_tool_schemas())
        self._token_decomp_dirty = True

    def override_intrinsic(self, name: str) -> Callable[[dict], dict]:
        """Remove an intrinsic and return its handler for delegation.

        Called by capabilities that upgrade an intrinsic (email → mail,
        anima → system). Must be called before start() (tool surface sealed).

        Returns the original handler so the capability can delegate to it.
        """
        if self._sealed:
            raise RuntimeError("Cannot modify tools after start()")
        handler = self._intrinsics.pop(name)  # raises KeyError if missing
        self._token_decomp_dirty = True
        return handler

    def update_system_prompt(
        self, section: str, content: str, *, protected: bool = False
    ) -> None:
        """Update a named section of the system prompt.

        Args:
            section: Section name.
            content: Section content.
            protected: If True, the LLM cannot overwrite this section.
        """
        self._prompt_manager.write_section(section, content, protected=protected)
        self._token_decomp_dirty = True
        # Update the live session's system prompt if one exists
        if self._chat is not None:
            self._chat.update_system_prompt(self._build_system_prompt())

    def send(
        self,
        content: str | dict,
        sender: str = "user",
        wait: bool = True,
        timeout: float = 300.0,
    ) -> dict | None:
        """Send a message to the agent.

        Args:
            content: Message content.
            sender: Message sender.
            wait: If True, block until result. If False, fire-and-forget.
            timeout: Max time to wait for result (only if wait=True).

        Returns:
            If wait=True: result dict {"text": ..., "failed": ..., "errors": [...]}.
            If wait=False: None.
        """
        reply_event = threading.Event() if wait else None
        msg = _make_message(MSG_REQUEST, sender, content, reply_event=reply_event)
        self.inbox.put(msg)

        if not wait:
            return None

        if not reply_event.wait(timeout=timeout):
            return {
                "text": f"Timeout after {timeout}s waiting for {self.agent_id}",
                "failed": True,
                "errors": ["timeout"],
            }
        if msg._reply_value is None:
            return {"text": "", "failed": True, "errors": ["no reply"]}
        return msg._reply_value

    # ------------------------------------------------------------------
    # Session persistence
    # ------------------------------------------------------------------

    def get_chat_state(self) -> dict:
        """Serialize current chat session for persistence."""
        if self._chat is None:
            return {}
        try:
            return {"messages": self._chat.interface.to_dict()}
        except Exception:
            return {}

    def restore_chat(self, state: dict) -> None:
        """Restore or create a chat session from saved state."""
        messages = state.get("messages")
        if messages:
            try:
                self._chat = self.service.resume_session(state)
                return
            except Exception as e:
                logger.warning(
                    f"[{self.agent_id}] Failed to resume session: {e}. Starting fresh.",
                    exc_info=True,
                )
        self._ensure_session()

    def restore_token_state(self, state: dict) -> None:
        """Restore cumulative token counters from a saved session."""
        self._total_input_tokens = state.get("input_tokens", 0)
        self._total_output_tokens = state.get("output_tokens", 0)
        self._total_thinking_tokens = state.get("thinking_tokens", 0)
        self._total_cached_tokens = state.get("cached_tokens", 0)
        self._api_calls = state.get("api_calls", 0)

    # ------------------------------------------------------------------
    # Status / introspection
    # ------------------------------------------------------------------

    def status(self) -> dict:
        """Return agent status for monitoring."""
        return {
            "agent_id": self.agent_id,
            "agent_type": self.agent_type,
            "state": self._state.value,
            "idle": self.is_idle,
            "queue_depth": self.inbox.qsize(),
            "tokens": self.get_token_usage(),
        }

    # ------------------------------------------------------------------
    # Hooks (overridable by subclasses)
    # ------------------------------------------------------------------

    def _pre_request(self, msg: Message) -> str:
        """Transform message content before sending to LLM.

        Returns the content string to send.
        """
        return msg.content if isinstance(msg.content, str) else json.dumps(msg.content)

    def _post_request(self, msg: Message, result: dict) -> None:
        """Called after _process_response, before _deliver_result.

        Override in subclasses for post-processing.
        """

    def _on_tool_result_hook(
        self, tool_name: str, tool_args: dict, result: dict
    ) -> str | None:
        """Hook called after each tool execution.

        If this returns a non-None string, the current request processing
        returns immediately with that string as the result text.
        """
        return None

    # ------------------------------------------------------------------
    # Result delivery
    # ------------------------------------------------------------------

    def _deliver_result(self, msg: Message, result: dict) -> None:
        """Deliver result to a waiting caller."""
        if msg._reply_event:
            msg._reply_value = result
            msg._reply_event.set()
