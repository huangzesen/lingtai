"""
BaseAgent — generic research agent with intrinsic tools and MCP tool dispatch.

Key concepts:
    - **2-state lifecycle**: SLEEPING (waiting for inbox) and ACTIVE (processing).
    - **Persistent LLM session**: each agent keeps its chat session across messages.
    - **2-layer tool dispatch**: intrinsics (built-in) + MCP handlers (domain tools).
    - **Opaque context**: the host app can pass any context object — the agent
      stores it but never introspects it.
"""

from __future__ import annotations

import enum
import json
import queue
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable
from uuid import uuid4

from .config import AgentConfig
from .intrinsics import ALL_INTRINSICS
from .intrinsics.manage_system_prompt import SystemPromptManager
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
from .types import (
    MCPTool,
    UnknownToolError,
    AgentNotConnectedError,
    EVENT_TOOL_CALL,
    EVENT_TOOL_RESULT,
    EVENT_TEXT_DELTA,
    EVENT_AGENT_STATE,
    EVENT_COMPACTION,
    EVENT_DEBUG,
)

logger = get_logger()


# ---------------------------------------------------------------------------
# AgentState
# ---------------------------------------------------------------------------


class AgentState(enum.Enum):
    """Lifecycle state of an agent.

    SLEEPING --(inbox message)---> ACTIVE
    ACTIVE   --(all done)--------> SLEEPING
    """

    ACTIVE = "active"
    SLEEPING = "sleeping"


# ---------------------------------------------------------------------------
# Message types and Message dataclass
# ---------------------------------------------------------------------------

MSG_REQUEST = "request"
MSG_CANCEL = "cancel"
MSG_USER_INPUT = "user_input"


@dataclass
class Message:
    """A message delivered to an agent's inbox.

    Attributes:
        id:        Unique message ID (auto-generated if not provided).
        type:      One of MSG_REQUEST, MSG_CANCEL, MSG_USER_INPUT.
        sender:    Agent ID, "user", etc.
        content:   Payload — str for requests, dict for structured data.
        reply_to:  Links back to original message.
        timestamp: ``time.monotonic()`` when created.
        _reply_event: Internal Event for callers waiting on a result.
        _reply_value: Internal slot for the agent's response.
    """

    type: str
    sender: str
    content: Any
    id: str = field(default_factory=lambda: f"msg_{uuid4().hex[:12]}")
    reply_to: str | None = None
    timestamp: float = field(default_factory=time.monotonic)
    _reply_event: threading.Event | None = field(default=None, repr=False)
    _reply_value: Any = field(default=None, repr=False)


def _make_message(
    type: str,
    sender: str,
    content: Any,
    *,
    reply_to: str | None = None,
    reply_event: threading.Event | None = None,
) -> Message:
    return Message(
        id=f"msg_{uuid4().hex[:12]}",
        type=type,
        sender=sender,
        content=content,
        reply_to=reply_to,
        _reply_event=reply_event,
    )


# ---------------------------------------------------------------------------
# MIME types for vision
# ---------------------------------------------------------------------------

_MIME_BY_EXT: dict[str, str] = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".webp": "image/webp",
}


# ---------------------------------------------------------------------------
# BaseAgent
# ---------------------------------------------------------------------------


class BaseAgent:
    """Generic research agent with intrinsic tools and MCP tool dispatch.

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
        config: AgentConfig | None = None,
        mcp_tools: list[MCPTool] | None = None,
        working_dir: str | Path | None = None,
        on_event: Callable[[str, dict], None] | None = None,
        context: Any = None,
        enabled_intrinsics: set[str] | None = None,
        disabled_intrinsics: set[str] | None = None,
        cancel_event: threading.Event | None = None,
        streaming: bool = False,
    ):
        if enabled_intrinsics is not None and disabled_intrinsics is not None:
            raise ValueError(
                "Cannot specify both enabled_intrinsics and disabled_intrinsics"
            )

        self.agent_id = agent_id
        self.service = service
        self._config = config or AgentConfig()
        self._context = context
        self._cancel_event = cancel_event
        self._streaming = streaming
        self.on_event = on_event

        # Working directory for file intrinsics
        self._working_dir = Path(working_dir) if working_dir else Path.cwd()

        # System prompt manager
        self._prompt_manager = SystemPromptManager()

        # Agent connections (for talk intrinsic)
        self._connections: dict[str, BaseAgent] = {}

        # MCP tool handlers
        self._mcp_handlers: dict[str, Callable[[dict], dict]] = {}
        self._mcp_schemas: list[FunctionSchema] = []
        if mcp_tools:
            for tool in mcp_tools:
                self._mcp_handlers[tool.name] = tool.handler
                self._mcp_schemas.append(
                    FunctionSchema(
                        name=tool.name,
                        description=tool.description,
                        parameters=tool.schema,
                    )
                )

        # --- Wire intrinsic tools ---
        self._intrinsics: dict[str, Callable[[dict], dict]] = {}
        self._wire_intrinsics(enabled_intrinsics, disabled_intrinsics)

        # Inbox
        self.inbox: queue.Queue[Message] = queue.Queue()

        # Lifecycle
        self._shutdown = threading.Event()
        self._thread: threading.Thread | None = None
        self._idle = threading.Event()
        self._idle.set()
        self._state = AgentState.SLEEPING

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

    def _wire_intrinsics(
        self,
        enabled: set[str] | None,
        disabled: set[str] | None,
    ) -> None:
        """Wire intrinsic tool handlers based on enabled/disabled sets."""
        # File intrinsics — thin wrappers that resolve working_dir
        from .intrinsics.read import handle_read
        from .intrinsics.edit import handle_edit
        from .intrinsics.write import handle_write
        from .intrinsics.glob import handle_glob
        from .intrinsics.grep import handle_grep

        file_intrinsics = {
            "read": ("file_path", handle_read),
            "edit": ("file_path", handle_edit),
            "write": ("file_path", handle_write),
            "glob": ("path", handle_glob),
            "grep": ("path", handle_grep),
        }

        # Agent-state intrinsics (bound methods)
        state_intrinsics = {
            "talk": self._handle_talk,
            "vision": self._handle_vision,
            "web_search": self._handle_web_search,
        }

        all_names = set(file_intrinsics.keys()) | set(state_intrinsics.keys())

        # Determine which intrinsics to enable
        if enabled is not None:
            active_names = enabled & all_names
        elif disabled is not None:
            active_names = all_names - disabled
        else:
            active_names = all_names

        # Wire file intrinsics
        for name, (path_key, handler) in file_intrinsics.items():
            if name in active_names:
                # Create closure with captured variables
                self._intrinsics[name] = self._make_file_handler(path_key, handler)

        # Wire state intrinsics
        for name, handler in state_intrinsics.items():
            if name in active_names:
                self._intrinsics[name] = handler

    def _make_file_handler(
        self, path_key: str, handler: Callable[[dict], dict]
    ) -> Callable[[dict], dict]:
        """Create a file intrinsic handler that resolves paths relative to working_dir."""

        def _handler(args: dict) -> dict:
            args = dict(args)  # don't mutate caller's dict
            path_val = args.get(path_key)
            if path_val and not Path(path_val).is_absolute():
                args[path_key] = str(self._working_dir / path_val)
            return handler(args)

        return _handler

    # ------------------------------------------------------------------
    # Intrinsic handlers (agent-state intrinsics)
    # ------------------------------------------------------------------

    def _handle_talk(self, args: dict) -> dict:
        """Send a message to a connected agent."""
        action = args.get("action", "send")
        target_id = args.get("target_id", "")
        message = args.get("message", "")
        timeout = args.get("timeout", 120)

        if target_id not in self._connections:
            raise AgentNotConnectedError(target_id)

        target = self._connections[target_id]
        msg = _make_message(MSG_REQUEST, self.agent_id, message)

        if action == "send_and_wait":
            reply_event = threading.Event()
            msg._reply_event = reply_event
            target.inbox.put(msg)
            if not reply_event.wait(timeout=timeout):
                return {"status": "timeout", "message": f"No reply from {target_id} within {timeout}s"}
            return {"status": "ok", "reply": msg._reply_value}
        else:
            target.inbox.put(msg)
            return {"status": "ok", "message": f"Sent to {target_id}"}

    def _handle_vision(self, args: dict) -> dict:
        """Analyze an image file using the model's vision capability."""
        image_path = args.get("image_path", "")
        question = args.get("question", "Describe what you see in this image.")

        if not image_path:
            return {"status": "error", "message": "Provide image_path"}

        path = Path(image_path)
        if not path.is_absolute():
            path = self._working_dir / path

        if not path.is_file():
            return {"status": "error", "message": f"Image file not found: {path}"}

        image_bytes = path.read_bytes()
        mime = _MIME_BY_EXT.get(path.suffix.lower(), "image/png")

        response = self.service.generate_vision(question, image_bytes, mime_type=mime)
        if not response.text:
            return {
                "status": "error",
                "message": "Vision analysis returned no response — vision provider may not be configured.",
            }
        return {"status": "ok", "analysis": response.text}

    def _handle_web_search(self, args: dict) -> dict:
        """Search the web for information."""
        query = args.get("query")
        if not query:
            return {"status": "error", "message": "Missing required parameter: query"}
        resp = self.service.web_search(query)
        if not resp.text:
            return {
                "status": "error",
                "message": "Web search returned no results. The web search provider may not be configured.",
            }
        return {"status": "ok", "results": resp.text}

    # ------------------------------------------------------------------
    # Properties
    # ------------------------------------------------------------------

    @property
    def is_idle(self) -> bool:
        return self._idle.is_set()

    @property
    def state(self) -> AgentState:
        return self._state

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        """Start the agent's main loop thread."""
        if self._thread and self._thread.is_alive():
            return
        self._shutdown.clear()
        self._thread = threading.Thread(
            target=self._run_loop,
            daemon=True,
            name=f"agent-{self.agent_id}",
        )
        self._thread.start()

    def stop(self, timeout: float = 5.0) -> None:
        """Signal shutdown and wait for the agent thread to exit."""
        self._shutdown.set()
        if self._thread:
            self._thread.join(timeout=timeout)
        self._timeout_pool.shutdown(wait=False)

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
        suffix = f" ({reason})" if reason else ""
        self._emit_event(
            EVENT_AGENT_STATE,
            {
                "old": old.value,
                "new": new_state.value,
                "reason": reason,
                "msg": f"[{self.agent_id}] {old.value} -> {new_state.value}{suffix}",
            },
        )

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
                self._emit_event(
                    EVENT_DEBUG,
                    {"level": "error", "msg": f"[{self.agent_id}] Unhandled error: {err_desc}"},
                )
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
        if msg.type == MSG_CANCEL:
            self._handle_cancel(msg)
        elif msg.type in (MSG_REQUEST, MSG_USER_INPUT):
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

    def _handle_cancel(self, msg: Message) -> None:
        """Cancel active tools. Agent stays alive."""
        if self._cancel_event:
            self._cancel_event.set()

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
                if response.tool_calls:
                    if not (self._streaming and self._intermediate_text_streamed):
                        self._emit_event(
                            EVENT_TEXT_DELTA,
                            {"text": response.text + "\n\n", "commentary": True},
                        )
                    self._intermediate_text_streamed = False

            if not response.tool_calls:
                break

            if self._cancel_event and self._cancel_event.is_set():
                return {
                    "text": "Interrupted by user.",
                    "failed": True,
                    "errors": ["Interrupted"],
                }

            stop_reason = guard.check_limit(len(response.tool_calls))
            if stop_reason:
                self._emit_event(
                    EVENT_DEBUG,
                    {"level": "debug", "msg": f"[{self.agent_id}] Stopping: {stop_reason}"},
                )
                break

            # Check for invalid tool names
            invalid_reason = guard.check_invalid_tool_limit()
            if invalid_reason:
                self._emit_event(
                    EVENT_DEBUG,
                    {"level": "warning", "msg": f"[{self.agent_id}] Stopping: {invalid_reason}"},
                )
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
        commentary = args.pop("commentary", None)
        if commentary:
            self._emit_event(
                EVENT_TEXT_DELTA,
                {"text": commentary + "\n\n", "commentary": True},
            )
        args.pop("_sync", None)

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
            return msg, False, ""

        # Emit tool_call event
        self._emit_event(
            EVENT_TOOL_CALL,
            {"tool_name": tc.name, "tool_args": args},
        )

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

            # Emit tool_result event
            status = result.get("status", "success") if isinstance(result, dict) else "success"
            self._emit_event(
                EVENT_TOOL_RESULT,
                {
                    "tool_name": tc.name,
                    "tool_result": result,
                    "status": status,
                },
            )

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
            self._emit_event(
                EVENT_DEBUG,
                {
                    "level": "error",
                    "msg": f"[{self.agent_id}] {tc.name} FAILED: {e}",
                    "tool_name": tc.name,
                    "error": str(e),
                },
            )
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
            commentary = args.pop("commentary", None)
            args.pop("_sync", None)

            if commentary:
                self._emit_event(
                    EVENT_TEXT_DELTA,
                    {"text": commentary + "\n\n", "commentary": True},
                )

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
        self._emit_event(
            EVENT_DEBUG,
            {"level": "debug", "msg": f"[{self.agent_id}] Parallel: {len(to_execute)} tools concurrently"},
        )

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
                if self._cancel_event and self._cancel_event.is_set():
                    break
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
        """Build the system prompt from intrinsics + sections + MCP tools."""
        mcp_tools = [
            MCPTool(
                name=name,
                schema=self._mcp_handlers.get(name, lambda a: {}).__doc__ or "",
                description=schema.description,
                handler=self._mcp_handlers[name],
            )
            for name, schema in zip(
                [s.name for s in self._mcp_schemas],
                self._mcp_schemas,
            )
            if name in self._mcp_handlers
        ]
        return build_system_prompt(
            prompt_manager=self._prompt_manager,
            intrinsic_names=list(self._intrinsics.keys()),
            mcp_tools=mcp_tools,
        )

    def _build_tool_schemas(self) -> list[FunctionSchema]:
        """Build the complete tool schema list for the LLM."""
        schemas = []

        # Intrinsic schemas
        for name in self._intrinsics:
            info = ALL_INTRINSICS.get(name)
            if info:
                schemas.append(
                    FunctionSchema(
                        name=name,
                        description=info["description"],
                        parameters=info["schema"],
                    )
                )

        # MCP schemas
        schemas.extend(self._mcp_schemas)

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

        retry_timeout = self._config.retry_timeout

        try:
            if self._streaming:
                response = self._llm_send_streaming(message, retry_timeout)
            else:
                response = send_with_timeout(
                    chat=self._chat,
                    message=message,
                    timeout_pool=self._timeout_pool,
                    cancel_event=self._cancel_event,
                    retry_timeout=retry_timeout,
                    agent_name=self.agent_id,
                    logger=logger,
                    on_reset=self._on_reset,
                    on_event=self._emit_event_kw,
                )
        except Exception as exc:
            # Handle stale Interactions API session
            if self._interaction_id and _is_stale_interaction_error(exc):
                self._emit_event(
                    EVENT_DEBUG,
                    {"level": "warning", "msg": f"[{self.agent_id}] Stale interaction — starting fresh session"},
                )
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
                        cancel_event=self._cancel_event,
                        retry_timeout=retry_timeout,
                        agent_name=self.agent_id,
                        logger=logger,
                        on_event=self._emit_event_kw,
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
        """Streaming LLM send — emits TEXT_DELTA events as tokens arrive."""
        self._message_seq += 1
        seq = self._message_seq
        first_chunk = True

        def _on_chunk(text_delta: str) -> None:
            nonlocal first_chunk
            data: dict = {"text": text_delta, "streaming": True}
            if first_chunk:
                data["message_seq"] = seq
                first_chunk = False
            self._emit_event(EVENT_TEXT_DELTA, data)

        response = send_with_timeout_stream(
            chat=self._chat,
            message=message,
            timeout_pool=self._timeout_pool,
            cancel_event=self._cancel_event,
            retry_timeout=retry_timeout,
            agent_name=self.agent_id,
            logger=logger,
            on_chunk=_on_chunk,
            on_reset=self._on_reset,
            on_event=self._emit_event_kw,
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

        self._emit_event(
            EVENT_DEBUG,
            {"level": "warning", "msg": f"[{self.agent_id}] Session rollback — new chat ({len(iface.entries)} entries kept)"},
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
                tracked=False,
            )
            return response.text.strip() if response and response.text else ""

        new_chat = self.service.check_and_compact(
            self._chat,
            summarizer=summarizer,
            threshold=0.8,
            provider=self._config.provider,
        )
        if new_chat is not None:
            old_tokens = self._chat.interface.estimate_context_tokens()
            new_tokens = new_chat.interface.estimate_context_tokens()
            self._chat = new_chat
            self._interaction_id = None
            self._emit_event(
                EVENT_COMPACTION,
                {
                    "before_tokens": old_tokens,
                    "after_tokens": new_tokens,
                    "context_window": self._chat.context_window(),
                },
            )

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
            on_event=self._emit_event_kw,
        )
        self._total_input_tokens = token_state["input"]
        self._total_output_tokens = token_state["output"]
        self._total_thinking_tokens = token_state["thinking"]
        self._total_cached_tokens = token_state["cached"]
        self._api_calls = token_state["api_calls"]
        if response.usage:
            self._latest_input_tokens = response.usage.input_tokens

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
    # Event emission
    # ------------------------------------------------------------------

    def _emit_event(self, event_type: str, payload: dict) -> None:
        """Emit an event via the on_event callback, injecting agent_id."""
        if self.on_event:
            self.on_event(event_type, {**payload, "agent_id": self.agent_id})

    def _emit_event_kw(self, event_type: str, payload: dict) -> None:
        """Emit event — same signature as on_event callback.

        Used as the on_event parameter for llm_utils functions.
        """
        self._emit_event(event_type, payload)

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def connect(self, agent_id: str, agent: BaseAgent) -> None:
        """Register a connected agent for the talk intrinsic."""
        self._connections[agent_id] = agent

    def disconnect(self, agent_id: str) -> None:
        """Remove a connected agent."""
        self._connections.pop(agent_id, None)

    def talk(self, target_id: str, message: str, *, wait: bool = False, timeout: float = 120) -> dict:
        """Send a message to a connected agent (public API).

        Raises AgentNotConnectedError if the target is not connected.
        """
        return self._handle_talk({
            "action": "send_and_wait" if wait else "send",
            "target_id": target_id,
            "message": message,
            "timeout": timeout,
        })

    def add_tool(
        self,
        name: str,
        *,
        schema: dict | None = None,
        handler: Callable[[dict], dict] | None = None,
        description: str = "",
    ) -> None:
        """Register a dynamic MCP tool."""
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
        """Unregister a dynamic MCP tool."""
        self._mcp_handlers.pop(name, None)
        self._mcp_schemas = [s for s in self._mcp_schemas if s.name != name]
        if self._chat is not None:
            self._chat.update_tools(self._build_tool_schemas())
        self._token_decomp_dirty = True

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

    # File access methods (convenience wrappers for programmatic use)

    def read_file(self, path: str) -> dict:
        """Read a file using the read intrinsic."""
        handler = self._intrinsics.get("read")
        if handler is None:
            return {"error": "read intrinsic is not enabled"}
        return handler({"file_path": path})

    def write_file(self, path: str, content: str) -> dict:
        """Write a file using the write intrinsic."""
        handler = self._intrinsics.get("write")
        if handler is None:
            return {"error": "write intrinsic is not enabled"}
        return handler({"file_path": path, "content": content})

    def edit_file(self, path: str, old_string: str, new_string: str) -> dict:
        """Edit a file using the edit intrinsic."""
        handler = self._intrinsics.get("edit")
        if handler is None:
            return {"error": "edit intrinsic is not enabled"}
        return handler({
            "file_path": path,
            "old_string": old_string,
            "new_string": new_string,
        })

    def glob(self, pattern: str, path: str = ".") -> dict:
        """Find files matching a glob pattern."""
        handler = self._intrinsics.get("glob")
        if handler is None:
            from .intrinsics.glob import handle_glob
            return handle_glob({"pattern": pattern, "path": path})
        return handler({"pattern": pattern, "path": path})

    def grep(self, pattern: str, path: str = ".", file_glob: str = "*") -> dict:
        """Search file contents by regex pattern."""
        handler = self._intrinsics.get("grep")
        if handler is None:
            from .intrinsics.grep import handle_grep
            return handle_grep({"pattern": pattern, "path": path, "glob": file_glob})
        return handler({"pattern": pattern, "path": path, "glob": file_glob})

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
