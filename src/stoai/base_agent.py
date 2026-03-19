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
import queue
import threading
import time
from pathlib import Path
from typing import Any, Callable

from .config import AgentConfig
from .state import AgentState
from .workdir import WorkingDir
from .message import Message, _make_message, MSG_REQUEST, MSG_USER_INPUT
from .intrinsics import ALL_INTRINSICS
from .prompt import SystemPromptManager
from .llm import (
    FunctionSchema,
    LLMService,
    ToolCall,
)
from .logging import get_logger
from .loop_guard import LoopGuard
from .prompt import build_system_prompt
from .session import SessionManager
from .tool_executor import ToolExecutor
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
        agent_name: str,
        service: LLMService,
        *,
        file_io: Any | None = None,
        mail_service: Any | None = None,
        config: AgentConfig | None = None,
        base_dir: str | Path,
        context: Any = None,
        admin: dict | None = None,
        streaming: bool = False,
        covenant: str = "",
        memory: str = "",
    ):
        import uuid as _uuid
        self.agent_name = agent_name
        self.agent_id = _uuid.uuid4().hex[:12]
        self.service = service
        self._config = config or AgentConfig()
        self._context = context
        self._admin = admin or {}
        self._cancel_event = threading.Event()
        self._started_at: str = ""
        self._uptime_anchor: float | None = None  # set in start(), None means not started

        # Base directory (shared root) and working directory (per-agent)
        self._base_dir = Path(base_dir)
        if not self._base_dir.is_dir():
            raise FileNotFoundError(f"base_dir does not exist: {self._base_dir}")
        self._workdir = WorkingDir(base_dir=base_dir, agent_name=agent_name)
        self._working_dir = self._workdir.path

        # LoggingService: always JSONL in working dir
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

        # Set by psyche capability to prevent stop() from overwriting memory.md
        self._eigen_owns_memory = False

        # Covenant and memory file paths
        system_dir = self._working_dir / "system"
        memory_file = system_dir / "memory.md"
        covenant_file = system_dir / "covenant.md"

        # Resume: restore covenant from file if not provided by constructor
        if not covenant and covenant_file.is_file():
            covenant = covenant_file.read_text()

        # If constructor memory is provided and memory file doesn't exist, write it
        if memory and not memory_file.is_file():
            system_dir.mkdir(exist_ok=True)
            memory_file.write_text(memory)

        # If constructor covenant is provided and covenant file doesn't exist, write it
        if covenant and not covenant_file.is_file():
            system_dir.mkdir(exist_ok=True)
            covenant_file.write_text(covenant)

        # Auto-load memory from file into prompt manager
        loaded_memory = ""
        if memory_file.is_file():
            loaded_memory = memory_file.read_text()

        # System prompt manager
        self._prompt_manager = SystemPromptManager()
        if covenant:
            self._prompt_manager.write_section("covenant", covenant, protected=True)
        if loaded_memory.strip():
            self._prompt_manager.write_section("memory", loaded_memory)

        # Write manifest — stable identity only (no covenant, no runtime state)
        from datetime import datetime, timezone
        self._started_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        manifest_data = {
            "agent_id": self.agent_id,
            "agent_name": self.agent_name,
            "started_at": self._started_at,
            "working_dir": str(self._working_dir),
            "admin": self._admin,
        }
        if self._mail_service is not None and self._mail_service.address:
            manifest_data["address"] = self._mail_service.address
        self._workdir.write_manifest(manifest_data)

        # Auto-inject identity into system prompt from manifest
        import json as _json
        self._prompt_manager.write_section(
            "identity", _json.dumps(manifest_data, indent=2), protected=True
        )

        # Post to billboard — ephemeral discovery index at ~/.stoai/billboard/
        self._billboard_path: Path | None = None
        try:
            billboard_dir = Path.home() / ".stoai" / "billboard"
            billboard_dir.mkdir(parents=True, exist_ok=True)
            self._billboard_path = billboard_dir / f"{self.agent_id}.json"
            import json as _json, os as _os
            tmp = self._billboard_path.with_suffix(".tmp")
            tmp.write_text(_json.dumps(manifest_data, indent=2))
            _os.replace(str(tmp), str(self._billboard_path))
        except OSError:
            self._billboard_path = None

        # Wire TCP discovery — banner on connect + info query handler
        if self._mail_service is not None:
            if hasattr(self._mail_service, '_banner_id'):
                self._mail_service._banner_id = self.agent_id
            if hasattr(self._mail_service, '_info_handler'):
                self._mail_service._info_handler = self._get_discovery_info

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

        # Session manager — LLM session, token tracking, compaction
        self._session = SessionManager(
            llm_service=service,
            config=self._config,
            agent_id=self.agent_id,
            agent_name=agent_name,
            streaming=streaming,
            build_system_prompt_fn=self._build_system_prompt,
            build_tool_schemas_fn=self._build_tool_schemas,
            logger_fn=self._log,
        )

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

    def _get_discovery_info(self) -> dict:
        """Return live agent info for TCP discovery queries."""
        info = {
            "_stoai": "agent",
            "agent_id": self.agent_id,
            "agent_name": self.agent_name,
            "started_at": self._started_at,
            "working_dir": str(self._working_dir),
            "admin": self._admin,
            "status": self._state.value,
        }
        if self._mail_service is not None and self._mail_service.address:
            info["address"] = self._mail_service.address
        # Capabilities (set by Agent subclass)
        if hasattr(self, "_capabilities"):
            info["capabilities"] = [name for name, _ in self._capabilities]
        return info

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

    @property
    def _chat(self) -> Any:
        """Proxy to SessionManager's chat session.

        Many parts of the codebase (intrinsics, capabilities, psyche)
        read ``self._chat`` directly — this property keeps them working.
        """
        return self._session.chat

    @_chat.setter
    def _chat(self, value: Any) -> None:
        self._session.chat = value

    @property
    def _streaming(self) -> bool:
        """Proxy to SessionManager's streaming flag."""
        return self._session.streaming

    @property
    def _token_decomp_dirty(self) -> bool:
        """Proxy to SessionManager's token decomp dirty flag."""
        return self._session.token_decomp_dirty

    @_token_decomp_dirty.setter
    def _token_decomp_dirty(self, value: bool) -> None:
        self._session.token_decomp_dirty = value

    @property
    def _interaction_id(self) -> str | None:
        """Proxy to SessionManager's interaction ID."""
        return self._session.interaction_id

    @_interaction_id.setter
    def _interaction_id(self, value: str | None) -> None:
        self._session.interaction_id = value

    @property
    def _intermediate_text_streamed(self) -> bool:
        """Proxy to SessionManager's intermediate text streamed flag."""
        return self._session.intermediate_text_streamed

    @_intermediate_text_streamed.setter
    def _intermediate_text_streamed(self, value: bool) -> None:
        self._session.intermediate_text_streamed = value

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

        # Export assembled system prompt to system/system.md
        system_dir = self._working_dir / "system"
        system_dir.mkdir(exist_ok=True)
        (system_dir / "system.md").write_text(self._build_system_prompt())

        # Restore chat session and token state from filesystem if available
        chat_history_file = self._working_dir / "history" / "chat_history.json"
        if chat_history_file.is_file():
            try:
                state = json.loads(chat_history_file.read_text())
                self.restore_chat(state)
                self._log("session_restored")
            except Exception as e:
                logger.warning(f"[{self.agent_name}] Failed to restore chat history: {e}")
        status_file = self._working_dir / "history" / "status.json"
        if status_file.is_file():
            try:
                status_state = json.loads(status_file.read_text())
                self.restore_token_state(status_state.get("tokens", {}))
            except Exception as e:
                logger.warning(f"[{self.agent_name}] Failed to restore token state: {e}")

        # Start MailService listener if configured
        if self._mail_service is not None:
            try:
                self._mail_service.listen(on_message=lambda payload: self._on_mail_received(payload))
            except RuntimeError:
                pass  # Already listening or no listen_port — that's fine

        self._thread = threading.Thread(
            target=self._run_loop,
            daemon=True,
            name=f"agent-{self.agent_name}",
        )
        self._thread.start()

    def stop(self, timeout: float = 5.0) -> None:
        """Signal shutdown and wait for the agent thread to exit."""
        self._log("agent_stop")
        self._shutdown.set()
        if self._thread:
            self._thread.join(timeout=timeout)
        self._session.close()

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

        # Close MCP clients
        for client in getattr(self, "_mcp_clients", []):
            try:
                client.close()
            except Exception:
                pass

        # Persist memory from prompt manager to file
        if not self._eigen_owns_memory:
            memory_content = self._prompt_manager.read_section("memory") or ""
            memory_file = self._working_dir / "system" / "memory.md"
            if memory_file.is_file() or memory_content:
                memory_file.parent.mkdir(exist_ok=True)
                memory_file.write_text(memory_content)

        # Remove billboard entry
        if self._billboard_path and self._billboard_path.is_file():
            try:
                self._billboard_path.unlink()
            except OSError:
                pass

        # Persist final state and release lock
        manifest_data = {
            "agent_id": self.agent_id,
            "agent_name": self.agent_name,
            "started_at": self._started_at,
            "working_dir": str(self._working_dir),
            "admin": self._admin,
        }
        if self._mail_service is not None and self._mail_service.address:
            manifest_data["address"] = self._mail_service.address
        self._workdir.write_manifest(manifest_data)
        self._workdir.release_lock()

    def _on_mail_received(self, payload: dict) -> None:
        """Callback for MailService — routes by mail type.

        silence-type emails set the cancel event (interrupt current work).
        kill-type emails signal shutdown (hard stop).
        Normal emails are delegated to ``_on_normal_mail`` (which capabilities
        like email can replace).

        This method is never replaced — it is the stable entry point for all
        incoming mail.
        """
        mail_type = payload.get("type", "normal")

        if mail_type == "silence":
            self._cancel_event.set()
            # Deactivate conscience if present (Agent layer has _capability_managers)
            cap_managers = getattr(self, "_capability_managers", {})
            conscience = cap_managers.get("conscience")
            if conscience is not None:
                conscience.stop()
            self._log(
                "silence_received",
                sender=payload.get("from", "unknown"),
            )
            return

        if mail_type == "kill":
            self._cancel_event.set()
            self._shutdown.set()
            self._log(
                "kill_received",
                sender=payload.get("from", "unknown"),
            )
            # Run stop() in a separate thread to avoid deadlocking
            # the mail listener thread (stop() joins the agent thread).
            threading.Thread(
                target=self.stop,
                daemon=True,
                name=f"kill-{self.agent_name}",
            ).start()
            return

        self._on_normal_mail(payload)

    def _on_normal_mail(self, payload: dict) -> None:
        """Handle a normal mail — notify agent via inbox.

        The message is already persisted to mailbox/inbox/ by MailService.
        This method only signals arrival and sends a notification.
        Capabilities (e.g. email) may replace this method.
        """
        from uuid import uuid4

        email_id = payload.get("_mailbox_id") or str(uuid4())
        sender = payload.get("from", "unknown")
        subject = payload.get("subject", "")
        message = payload.get("message", "")

        self._mail_arrived.set()

        preview = message[:200].replace("\n", " ")
        notification = (
            f'[Mail from {sender}]\n'
            f'  Subject: {subject}\n'
            f'  Preview: {preview}...\n'
            f'  ID: {email_id}\n'
            f'Use mail(action="read", id=["{email_id}"]) to read full message.'
        )

        self._log("mail_received", sender=sender, subject=subject, message=message)
        msg = _make_message(MSG_REQUEST, sender, notification)
        msg._mail_notification = {
            "email_id": email_id,
            "sender": sender,
            "subject": subject,
            "preview": preview,
        }
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
                "agent_name": self.agent_name,
                "ts": time.time(),
                **fields,
            })

    # ------------------------------------------------------------------
    # Main loop (final — do not override)
    # ------------------------------------------------------------------

    def _run_loop(self) -> None:
        """Wait for messages, process them. Agent persists between messages."""
        while True:
            while not self._shutdown.is_set():
                try:
                    msg = self.inbox.get(timeout=self._inbox_timeout)
                except queue.Empty:
                    continue
                msg = self._collapse_email_notifications(msg)
                self._set_state(AgentState.ACTIVE, reason=f"received {msg.type}")
                try:
                    self._handle_message(msg)
                except Exception as e:
                    err_desc = str(e) or repr(e)
                    logger.error(
                        f"[{self.agent_name}] Unhandled error in message handler: {err_desc}",
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
                    self._persist_chat_history()
                    self._set_state(AgentState.SLEEPING, reason="all done")

            # Check for nirvana (rebirth) before exiting
            if getattr(self, "_nirvana_requested", False):
                self._nirvana_requested = False
                self._perform_nirvana()
                self._shutdown.clear()
                continue  # re-enter the message loop
            break  # normal shutdown — exit

    def _perform_nirvana(self) -> None:
        """Rebirth: close old MCP clients, reload from working dir, reset session."""
        self._log("nirvana_start")

        # Close existing MCP clients
        for client in getattr(self, "_mcp_clients", []):
            try:
                client.close()
            except Exception:
                pass
        self._mcp_clients = []

        # Temporarily unseal to allow tool modifications
        self._sealed = False

        # Remove old MCP tool registrations (keep intrinsics and capability tools)
        cap_tool_names = {name for name, _ in getattr(self, "_capabilities", [])}
        mcp_names = list(self._mcp_handlers.keys())
        for name in mcp_names:
            if name not in self._intrinsics and name not in cap_tool_names:
                self._mcp_handlers.pop(name, None)
                self._mcp_schemas = [s for s in self._mcp_schemas if s.name != name]

        # Reload MCP servers from working dir
        if hasattr(self, "_load_mcp_from_workdir"):
            self._load_mcp_from_workdir()

        # Re-seal
        self._sealed = True

        # Reset session so next message creates fresh one with new tools
        self._session.chat = None

        self._log("nirvana_complete", tools=list(self._mcp_handlers.keys()))

    def _collapse_email_notifications(self, msg: Message) -> Message:
        """Collapse consecutive mail notification messages into one."""
        if msg._mail_notification is None:
            return msg

        notifications = [msg._mail_notification]
        requeue: list[Message] = []

        while True:
            try:
                queued = self.inbox.get_nowait()
            except queue.Empty:
                break
            if queued._mail_notification is not None:
                notifications.append(queued._mail_notification)
            else:
                requeue.append(queued)

        for m in requeue:
            self.inbox.put(m)

        if len(notifications) == 1:
            return msg

        lines = [f"[{len(notifications)} new messages arrived]", ""]
        for i, n in enumerate(notifications, 1):
            lines.append(
                f'{i}. From {n["sender"]} — Subject: {n["subject"]}\n'
                f'   Preview: {n["preview"]}...\n'
                f'   ID: {n["email_id"]}'
            )
        lines.append("")
        lines.append(
            'Use mail(action="check") to see your inbox, or '
            'mail(action="read", id=["..."]) to read a specific message.'
        )
        merged_content = "\n".join(lines)
        merged = _make_message(MSG_REQUEST, "system", merged_content)
        self._log("mail_notifications_collapsed", count=len(notifications))
        return merged

    # ------------------------------------------------------------------
    # Message handling
    # ------------------------------------------------------------------

    def _handle_message(self, msg: Message) -> None:
        """Route message by type. Subclasses may override for routing."""
        if msg.type in (MSG_REQUEST, MSG_USER_INPUT):
            self._handle_request(msg)
        else:
            logger.warning(f"[{self.agent_name}] Unknown message type: {msg.type}")

    def _handle_request(self, msg: Message) -> None:
        """Send request to LLM, process response with tool calls."""
        from datetime import datetime, timezone

        max_calls, dup_free, dup_hard = self._get_guard_limits()
        guard = LoopGuard(
            max_total_calls=max_calls,
            dup_free_passes=dup_free,
            dup_hard_block=dup_hard,
        )
        self._executor = ToolExecutor(
            dispatch_fn=self._dispatch_tool,
            make_tool_result_fn=lambda name, result, **kw: self.service.make_tool_result(
                name, result, provider=self._config.provider, **kw
            ),
            guard=guard,
            known_tools=set(self._intrinsics) | set(self._mcp_handlers),
            parallel_safe_tools=self._PARALLEL_SAFE_TOOLS,
            logger_fn=self._log,
        )
        content = self._pre_request(msg)
        current_time = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

        # Molt pressure — warn agent when context is getting full
        # Needs eigen intrinsic (always present) or psyche capability to self-molt
        cap_managers = getattr(self, "_capability_managers", {})
        has_molt = "eigen" in self._intrinsics or "psyche" in cap_managers
        pressure = self._session.get_context_pressure()
        if pressure >= 0.8 and has_molt:
            # Detect tool name: psyche if capability registered, else eigen
            tool_name = "psyche" if "psyche" in cap_managers else "eigen"
            self._session._compaction_warnings += 1
            warnings = self._session._compaction_warnings
            if warnings > 5:
                # Auto-forget — agent ignored 5 warnings
                self._log("auto_forget", reason="ignored 5 molt warnings", pressure=pressure)
                from .intrinsics import eigen as _eigen
                _eigen.context_forget(self)
                self._session._compaction_warnings = 0
                content = (
                    f"[system] Your conversation history was wiped because you ignored "
                    f"5 molt warnings. Check your email inbox and library for context. "
                    f"Start fresh.\n\n{content}"
                )
            elif warnings == 5:
                content = (
                    f"[system] FINAL — countdown 0. Context {pressure:.0%} full. "
                    f"Molt NOW or lose everything next turn. "
                    f"Write your briefing: what you're doing, what's done, what's pending, "
                    f"which library entries to load. "
                    f"{tool_name}(object=context, action=molt, summary=<briefing>).\n\n{content}"
                )
            elif warnings >= 3:
                remaining = 5 - warnings
                content = (
                    f"[system] Context pressure: {pressure:.0%} full — "
                    f"countdown {remaining} {'turn' if remaining == 1 else 'turns'} until auto-wipe. "
                    f"Deposit important data to library NOW ({tool_name} submit), then molt. "
                    f"Your molt summary is a briefing to your future self — "
                    f"the ONLY context you will have.\n\n{content}"
                )
            else:
                remaining = 5 - warnings
                content = (
                    f"[system] Context pressure: {pressure:.0%} full — "
                    f"countdown {remaining} turns until auto-wipe. "
                    f"Start tidying up: save important findings to library ({tool_name} submit). "
                    f"When ready, molt with a briefing to your future self: "
                    f"{tool_name}(object=context, action=molt, summary=<briefing>). "
                    f"Your summary is the ONLY thing you will see after molt — "
                    f"include what you're doing, what's done, what's pending, "
                    f"and which library entries to load.\n\n{content}"
                )

        content = f"[Current time: {current_time}]\n\n{content}"
        response = self._session.send(content)
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
        # Clear any stale cancel event from a previous silence.
        self._cancel_event.clear()
        guard = self._executor.guard
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
                self._cancel_event.clear()
                return {"text": "", "failed": False, "errors": []}

            stop_reason = guard.check_limit(len(response.tool_calls))
            if stop_reason:
                break

            invalid_reason = guard.check_invalid_tool_limit()
            if invalid_reason:
                break

            # Delegate to ToolExecutor
            tool_results, intercepted, intercept_text = self._executor.execute(
                response.tool_calls,
                on_result_hook=self._on_tool_result_hook,
                cancel_event=self._cancel_event,
                collected_errors=collected_errors,
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
                    self.agent_name,
                    collected_errors[-1],
                )
                break

            response = self._session.send(tool_results)

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

    # ------------------------------------------------------------------
    # LLM communication
    # ------------------------------------------------------------------

    def _build_system_prompt(self) -> str:
        """Build the system prompt from base + sections + tool inventory."""
        # Build tool inventory from full tool descriptions
        lines = []
        for name in self._intrinsics:
            info = ALL_INTRINSICS.get(name)
            if info:
                lines.append(f"### {name}\n{info['description']}")
        for s in self._mcp_schemas:
            if s.description:
                lines.append(f"### {s.name}\n{s.description}")
        if lines:
            self._prompt_manager.write_section(
                "tools", "\n\n".join(lines), protected=True
            )
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

    def get_token_usage(self) -> dict:
        """Return token usage summary (delegates to SessionManager)."""
        return self._session.get_token_usage()

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
        system_prompt: str = "",
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
                    system_prompt=system_prompt,
                )
            )
        # Update the live session's tools if one exists
        if self._chat is not None:
            self._chat.update_tools(self._build_tool_schemas())
        self._token_decomp_dirty = True

    def connect_mcp(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
    ) -> list[str]:
        """Connect to an MCP server and auto-register all its tools.

        Args:
            command: Executable to run (e.g., "uvx", "xhelio-spice-mcp").
            args: Arguments to the command.
            env: Environment variables for the subprocess.

        Returns:
            List of registered tool names.
        """
        from .services.mcp import MCPClient

        client = MCPClient(command=command, args=args, env=env)
        client.start()

        # Track for cleanup
        if not hasattr(self, "_mcp_clients"):
            self._mcp_clients: list = []
        self._mcp_clients.append(client)

        # List tools and register each one
        tools = client.list_tools()
        registered = []
        for tool in tools:
            name = tool["name"]

            def _make_handler(c: MCPClient, tool_name: str):
                def handler(tool_args: dict) -> dict:
                    return c.call_tool(tool_name, tool_args)
                return handler

            # Extract schema properties (MCP uses inputSchema with JSON Schema)
            schema = tool.get("schema", {})
            # Remove top-level keys that aren't valid for our FunctionSchema
            schema.pop("additionalProperties", None)

            self.add_tool(
                name,
                schema=schema,
                handler=_make_handler(client, name),
                description=tool.get("description", ""),
            )
            registered.append(name)

        return registered

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
        psyche → eigen). Must be called before start() (tool surface sealed).

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
        # Export updated system prompt to file and update live session
        prompt = self._build_system_prompt()
        system_md = self._working_dir / "system" / "system.md"
        system_md.parent.mkdir(exist_ok=True)
        system_md.write_text(prompt)
        if self._chat is not None:
            self._chat.update_system_prompt(prompt)

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
    # Session persistence (delegates to SessionManager)
    # ------------------------------------------------------------------

    def get_chat_state(self) -> dict:
        """Serialize current chat session for persistence."""
        return self._session.get_chat_state()

    def restore_chat(self, state: dict) -> None:
        """Restore or create a chat session from saved state."""
        self._session.restore_chat(state)

    def restore_token_state(self, state: dict) -> None:
        """Restore cumulative token counters from a saved session."""
        self._session.restore_token_state(state)

    def _persist_chat_history(self) -> None:
        """Save chat history and status to history/ and git-commit."""
        history_dir = self._working_dir / "history"
        history_dir.mkdir(exist_ok=True)
        try:
            # Chat history
            state = self.get_chat_state()
            if state:
                (history_dir / "chat_history.json").write_text(
                    json.dumps(state, ensure_ascii=False)
                )
            # Status (tokens, state, uptime)
            (history_dir / "status.json").write_text(
                json.dumps(self.status(), ensure_ascii=False, indent=2)
            )
            self._workdir.diff_and_commit("history/chat_history.json", "chat_history")
            self._workdir.diff_and_commit("history/status.json", "status")
        except Exception as e:
            logger.warning(f"[{self.agent_name}] Failed to persist session state: {e}")

    # ------------------------------------------------------------------
    # Status / introspection
    # ------------------------------------------------------------------

    def status(self) -> dict:
        """Return agent status for monitoring."""
        return {
            "agent_id": self.agent_id,
            "agent_name": self.agent_name,
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
