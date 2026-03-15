"""
Shared LLM utilities used by BaseAgent and its subclasses.

All functions are stateless (operate on passed-in state dicts,
on_event callbacks, etc.).
"""

import contextvars
import json
import threading
import time
from collections.abc import Callable
from pathlib import Path
from concurrent.futures import Future, ThreadPoolExecutor, as_completed

from .llm import LLMResponse
from .logging import get_logger

_logger = get_logger()

# Event type strings used with on_event callback
EVENT_DEBUG = "debug"
EVENT_LLM_CALL = "llm_call"
EVENT_LLM_RESPONSE = "llm_response"
EVENT_TOKEN_USAGE = "token_usage"
EVENT_THINKING = "thinking"

# Type alias for on_event callback
OnEventCallback = Callable[[str, dict], None] | None

# Directory to save chat history snapshots on reset
_RESET_SNAPSHOT_DIR = Path.home() / ".stoai" / "reset_snapshots"


def _save_reset_snapshot(chat, agent_name: str, error_context: str) -> None:
    """Save chat history to a timestamped JSON file when reset is triggered.

    This helps investigate the root cause of session resets.
    """
    try:
        _RESET_SNAPSHOT_DIR.mkdir(parents=True, exist_ok=True)

        import datetime

        timestamp = datetime.datetime.now().strftime("%Y%m%d_%H%M%S")
        filename = f"{agent_name}_{timestamp}_{error_context}.json"
        filepath = _RESET_SNAPSHOT_DIR / filename

        history = chat.get_history()
        with open(filepath, "w") as f:
            json.dump(history, f, indent=2, default=str)

        _logger.info(f"[{agent_name}] Saved reset snapshot to {filepath}")
    except Exception as e:
        _logger.warning(f"[{agent_name}] Failed to save reset snapshot: {e}", exc_info=True)

# ---------------------------------------------------------------------------
# Model context window limits (hardcoded fallback)
# ---------------------------------------------------------------------------

MODEL_CONTEXT_LIMITS: dict[str, int] = {
    "gemini-3-flash": 1_048_576,
    "gemini-2.5-flash": 1_048_576,
    "gemini-2.5-pro": 1_048_576,
    "gemini-2.0-flash": 1_048_576,
    "claude-opus-4": 200_000,
    "claude-sonnet-4": 200_000,
    "claude-haiku-4": 200_000,
    "claude-3-5-sonnet": 200_000,
    "claude-3-5-haiku": 200_000,
    "gpt-4o": 128_000,
    "gpt-4-turbo": 128_000,
    "gpt-4.1": 1_047_576,
    "o1": 200_000,
    "o3": 200_000,
    "MiniMax-M2.5": 1_000_000,
    "MiniMax-M2": 200_000,
}

# ---------------------------------------------------------------------------
# litellm registry — community-maintained model context windows
# ---------------------------------------------------------------------------

LITELLM_REGISTRY_URL = (
    "https://raw.githubusercontent.com/BerriAI/litellm/main/"
    "model_prices_and_context_window.json"
)
CACHE_MAX_AGE = 86400  # 24 hours

_litellm_cache: dict[str, int] | None = None
_litellm_lock = threading.Lock()


def _get_cache_path(data_dir: str | None = None) -> Path:
    """Return the cache path for the litellm registry.

    Args:
        data_dir: Optional directory for cache storage. If None, uses
            ``~/.stoai/``.
    """
    if data_dir:
        return Path(data_dir) / "model_context_windows.json"
    return Path.home() / ".stoai" / "model_context_windows.json"


def _fetch_litellm_registry(data_dir: str | None = None) -> dict[str, int]:
    """Fetch max_input_tokens from litellm registry, cache locally.

    Returns a flat dict of {model_name: max_input_tokens}.
    Entries are stored in two forms:
    - Bare names (e.g., "gemini-3-flash-preview", "claude-sonnet-4-6")
    - Provider-stripped names from prefixed entries (e.g., "minimax/MiniMax-M2.5" -> "MiniMax-M2.5")
    This ensures prefix matching works for all models.
    """
    cache_path = _get_cache_path(data_dir)

    # Try reading from cache
    if cache_path.exists():
        try:
            age = time.time() - cache_path.stat().st_mtime
            if age < CACHE_MAX_AGE:
                cached = json.loads(cache_path.read_text(encoding="utf-8"))
                if isinstance(cached, dict) and cached:
                    return cached
        except Exception:
            pass

    # Fetch from GitHub
    try:
        import urllib.request
        req = urllib.request.Request(LITELLM_REGISTRY_URL, headers={
            "User-Agent": "stoai/1.0",
        })
        with urllib.request.urlopen(req, timeout=10) as resp:
            raw = json.loads(resp.read().decode("utf-8"))
    except Exception as e:
        _logger.debug("Failed to fetch litellm registry: %s", e)
        # Try stale cache
        if cache_path.exists():
            try:
                return json.loads(cache_path.read_text(encoding="utf-8"))
            except Exception:
                pass
        return {}

    # Extract max_input_tokens
    result: dict[str, int] = {}
    for model_key, info in raw.items():
        if not isinstance(info, dict):
            continue
        max_input = info.get("max_input_tokens")
        if not max_input or not isinstance(max_input, (int, float)):
            continue
        max_input = int(max_input)

        # Store under bare key
        result[model_key] = max_input

        # Also store under provider-stripped key (e.g., "minimax/MiniMax-M2.5" -> "MiniMax-M2.5")
        if "/" in model_key:
            bare = model_key.split("/", 1)[1]
            # Don't overwrite if bare name already has a value
            if bare not in result:
                result[bare] = max_input

    # Cache to disk
    try:
        cache_path.parent.mkdir(parents=True, exist_ok=True)
        cache_path.write_text(json.dumps(result), encoding="utf-8")
    except Exception:
        pass

    return result


def _get_litellm_registry() -> dict[str, int]:
    """Get litellm registry (lazy-loaded, thread-safe)."""
    global _litellm_cache
    if _litellm_cache is not None:
        return _litellm_cache
    with _litellm_lock:
        if _litellm_cache is not None:
            return _litellm_cache
        _litellm_cache = _fetch_litellm_registry()
        return _litellm_cache


def get_context_limit(model_name: str) -> int:
    """Return context window size for a model (longest prefix match), or 0 if unknown.

    Resolution order:
    1. litellm community registry (cached, refreshed daily)
    2. Hardcoded MODEL_CONTEXT_LIMITS dict
    """
    if not model_name:
        return 0

    # 1. Try litellm registry — exact match first, then longest prefix
    registry = _get_litellm_registry()
    if registry:
        # Exact match
        if model_name in registry:
            return registry[model_name]
        # Longest prefix match
        best, best_len = 0, 0
        for prefix, limit in registry.items():
            if model_name.startswith(prefix) and len(prefix) > best_len:
                best, best_len = limit, len(prefix)
        if best > 0:
            return best

    # 2. Hardcoded fallback — longest prefix match
    best, best_len = 0, 0
    for prefix, limit in MODEL_CONTEXT_LIMITS.items():
        if model_name.startswith(prefix) and len(prefix) > best_len:
            best, best_len = limit, len(prefix)
    return best


# LLM API call timeout thresholds (seconds)
_LLM_WARN_INTERVAL = 20  # log a warning every N seconds while waiting
_LLM_RETRY_TIMEOUT = 120  # abandon call and retry after this
_LLM_MAX_RETRIES = 4  # retry 4 times, then rollback takes over
_API_ERROR_RETRY_DELAYS = [10.0, 10.0]
_SESSION_RESET_THRESHOLD = 2  # rollback after 2 consecutive errors (~20s)


class _CancelledDuringLLM(Exception):
    """Raised when user cancellation is detected during an LLM API wait."""


def _is_stale_interaction_error(exc: Exception) -> bool:
    """Return True if the error indicates a stale/expired Interactions API session."""
    msg = str(exc).lower()
    return "interaction" in msg and (
        "not found" in msg or "invalid" in msg or "expired" in msg
    )


def _is_history_desync_error(exc: Exception) -> bool:
    """Return True if exc indicates the chat history is out of sync.

    These are 400-level errors caused by tool result messages that don't
    match the preceding tool calls (e.g., a response was lost due to
    timeout and we're sending stale tool results). Retrying with the
    same history will always fail — the only fix is to reset the session.

    Known patterns:
    - Anthropic: "tool call result does not follow tool call"
    - Gemini: "Please ensure that function response turn comes immediately
      after a model turn with function call"
    """
    msg = str(exc).lower()
    return (
        "tool call result does not follow tool call" in msg
        or "function response turn comes immediately" in msg
    )


def _is_precondition_error(exc: Exception) -> bool:
    """Return True if exc indicates a corrupted Interactions API session.

    The Gemini Interactions API returns a ClientError (400) with status
    FAILED_PRECONDITION when the server-side conversation state is
    inconsistent — e.g., after a truncated model response.  Retrying
    with the same session will always fail — the only fix is to reset.
    """
    try:
        from google.genai import errors as genai_errors
    except ImportError:
        return False
    if not isinstance(exc, genai_errors.ClientError):
        return False
    status = getattr(exc, "status", "") or ""
    msg = getattr(exc, "message", "") or ""
    return "FAILED_PRECONDITION" in status or "precondition check failed" in msg.lower()


def _is_bad_request_error(exc: Exception) -> bool:
    """Return True if exc is a 400 Bad Request from any provider.

    This is a broad catch-all for malformed requests (corrupted history,
    protocol violations, etc.) that the specific detectors above didn't
    match.  The only recovery is to reset the session.
    """
    # Anthropic/MiniMax: anthropic.BadRequestError (400)
    try:
        import anthropic

        if isinstance(exc, anthropic.BadRequestError):
            return True
    except ImportError:
        pass
    # OpenAI: openai.BadRequestError (400)
    try:
        import openai

        if isinstance(exc, openai.BadRequestError):
            return True
    except ImportError:
        pass
    # Gemini: google.genai.errors.ClientError (400)
    try:
        from google.genai import errors as genai_errors

        if isinstance(exc, genai_errors.ClientError):
            return True
    except ImportError:
        pass
    return False


def _is_retryable_api_error(exc: Exception) -> bool:
    """Return True if exc is a transient API server error worth retrying.

    Uses lazy imports so only the active provider's SDK is checked.
    This runs *after* the SDK's own built-in retries (typically 2-3 attempts
    with sub-second backoff) have already been exhausted.
    """
    # Anthropic/MiniMax: anthropic.InternalServerError (500+)
    try:
        import anthropic

        if isinstance(exc, anthropic.InternalServerError):
            return True
    except ImportError:
        pass
    # OpenAI: openai.InternalServerError (500+)
    try:
        import openai

        if isinstance(exc, openai.InternalServerError):
            return True
    except ImportError:
        pass
    # Gemini: google.genai.errors.ServerError
    try:
        from google.genai import errors as genai_errors

        if isinstance(exc, genai_errors.ServerError):
            return True
    except ImportError:
        pass
    return False


def _send_with_retry(
    submit_fn,
    timeout_pool: ThreadPoolExecutor,
    cancel_event: threading.Event | None,
    retry_timeout: float,
    agent_name: str,
    on_reset=None,
    max_retries: int | None = None,
    reset_threshold: int | None = None,
    on_event: OnEventCallback = None,
) -> LLMResponse:
    """Core retry loop for LLM API calls — used by both send and stream paths.

    Two recovery mechanisms:
    1. Retry up to *max_retries* attempts (default ``_LLM_MAX_RETRIES``).
    2. After *reset_threshold* consecutive failures (default
       ``_SESSION_RESET_THRESHOLD``), call ``on_reset(chat, message)`` which
       creates a new chat with the last assistant turn dropped, then continue
       retrying with the new session.

    Args:
        submit_fn: Callable that takes (chat, message) and returns the Future
            result by calling ``timeout_pool.submit(...)`` internally. Must also
            accept updated (chat, message) after a reset. Returns a callable
            ``() -> Future``, plus mutable ``chat`` and ``message`` via closure.
        on_event: Optional callback ``(event_type, payload_dict)`` for emitting
            events. If None, events are silently dropped.

    ``on_reset(chat, message)`` must return ``(new_chat, new_message)``.
    """
    if max_retries is None:
        max_retries = _LLM_MAX_RETRIES
    if reset_threshold is None:
        reset_threshold = _SESSION_RESET_THRESHOLD

    def _emit(event_type: str, **payload):
        if on_event:
            on_event(event_type, payload)

    # submit_fn is a mutable-state closure: submit_fn() -> Future
    # It also exposes .chat and .message for reset, and .update(chat, msg).
    last_exc = None
    consecutive_errors = 0
    _bad_request_reset_done = False
    _desync_reset_done = False
    for attempt in range(1 + max_retries):
        future: Future = submit_fn()
        t0 = time.monotonic()
        try:
            while True:
                if cancel_event and cancel_event.is_set():
                    future.cancel()
                    _emit(
                        EVENT_DEBUG,
                        agent=agent_name,
                        level="info",
                        msg=f"[{agent_name}] LLM call cancelled by user",
                    )
                    raise _CancelledDuringLLM()

                elapsed = time.monotonic() - t0
                remaining = retry_timeout - elapsed
                if remaining <= 0:
                    break
                wait = min(_LLM_WARN_INTERVAL, remaining)
                try:
                    result = future.result(timeout=wait)
                    consecutive_errors = 0
                    return result
                except TimeoutError:
                    elapsed = time.monotonic() - t0
                    if elapsed >= retry_timeout:
                        break
                    _emit(
                        EVENT_LLM_CALL,
                        agent=agent_name,
                        level="warning",
                        msg=f"[{agent_name}] LLM API not responding after {elapsed:.0f}s (attempt {attempt + 1})...",
                    )

            elapsed = time.monotonic() - t0
            future.cancel()
            last_exc = TimeoutError(f"LLM API call timed out after {elapsed:.0f}s")
            consecutive_errors += 1
            if attempt < max_retries:
                if consecutive_errors >= reset_threshold and on_reset:
                    try:
                        new_chat, new_msg = on_reset(submit_fn.chat, submit_fn.message)
                        submit_fn.update(new_chat, new_msg)
                    except Exception as reset_err:
                        _logger.warning("[%s] Session reset failed after timeout: %s", agent_name, reset_err)
                    consecutive_errors = 0
                _emit(
                    EVENT_LLM_CALL,
                    agent=agent_name,
                    level="warning",
                    msg=f"[{agent_name}] LLM API timed out after {elapsed:.0f}s, retrying ({attempt + 1}/{max_retries})...",
                )
            else:
                _emit(
                    EVENT_LLM_CALL,
                    agent=agent_name,
                    level="error",
                    msg=f"[{agent_name}] LLM API timed out after {elapsed:.0f}s, no retries left",
                )
        except _CancelledDuringLLM:
            raise
        except Exception as exc:
            if _is_history_desync_error(exc) and on_reset and not _desync_reset_done:
                _desync_reset_done = True
                _emit(
                    EVENT_LLM_CALL,
                    agent=agent_name,
                    level="warning",
                    msg=f"[{agent_name}] History desync detected (tool results don't match tool calls), resetting session: {exc}",
                )
                _save_reset_snapshot(submit_fn.chat, agent_name, "desync")
                try:
                    new_chat, new_msg = on_reset(submit_fn.chat, submit_fn.message)
                    submit_fn.update(new_chat, new_msg)
                except Exception as reset_err:
                    _logger.warning("[%s] Session reset failed after desync: %s", agent_name, reset_err, exc_info=True)
                    raise exc from reset_err
                consecutive_errors = 0
                last_exc = exc
                continue
            if _is_precondition_error(exc) and on_reset:
                _emit(
                    EVENT_LLM_CALL,
                    agent=agent_name,
                    level="warning",
                    msg=f"[{agent_name}] Precondition check failed (corrupted session state), resetting: {exc}",
                )
                try:
                    new_chat, new_msg = on_reset(submit_fn.chat, submit_fn.message)
                    submit_fn.update(new_chat, new_msg)
                except Exception as reset_err:
                    _logger.warning("[%s] Session reset failed after precondition error: %s", agent_name, reset_err, exc_info=True)
                    raise exc from reset_err
                consecutive_errors = 0
                last_exc = exc
                continue
            if _is_bad_request_error(exc) and on_reset and not _bad_request_reset_done:
                _bad_request_reset_done = True
                _emit(
                    EVENT_LLM_CALL,
                    agent=agent_name,
                    level="warning",
                    msg=f"[{agent_name}] Bad request (likely corrupted history), resetting session: {exc}",
                )
                _save_reset_snapshot(submit_fn.chat, agent_name, "bad_request")
                try:
                    new_chat, new_msg = on_reset(submit_fn.chat, submit_fn.message)
                    submit_fn.update(new_chat, new_msg)
                except Exception as reset_err:
                    _logger.warning("[%s] Session reset failed after bad request: %s", agent_name, reset_err, exc_info=True)
                    raise exc from reset_err
                consecutive_errors = 0
                last_exc = exc
                continue
            if _is_retryable_api_error(exc) and attempt < max_retries:
                consecutive_errors += 1
                delay = _API_ERROR_RETRY_DELAYS[min(attempt, len(_API_ERROR_RETRY_DELAYS) - 1)]
                if consecutive_errors >= reset_threshold and on_reset:
                    try:
                        new_chat, new_msg = on_reset(submit_fn.chat, submit_fn.message)
                        submit_fn.update(new_chat, new_msg)
                    except Exception as reset_err:
                        _logger.warning("[%s] Session reset failed after API error: %s", agent_name, reset_err)
                    consecutive_errors = 0
                _emit(
                    EVENT_LLM_CALL,
                    agent=agent_name,
                    level="warning",
                    msg=f"[{agent_name}] API server error, retrying in {delay}s ({attempt + 1}/{max_retries})...",
                )
                last_exc = exc
                time.sleep(delay)
                continue
            raise

    raise last_exc


class _SubmitFn:
    """Mutable callable that wraps chat.send or chat.send_stream for _send_with_retry."""

    __slots__ = ("chat", "message", "_pool", "_method", "_extra_args")

    def __init__(self, pool, chat, message, method: str, extra_args: tuple = ()):
        self._pool = pool
        self.chat = chat
        self.message = message
        self._method = method
        self._extra_args = extra_args

    def __call__(self) -> Future:
        fn = getattr(self.chat, self._method)
        return self._pool.submit(fn, self.message, *self._extra_args)

    def update(self, chat, message):
        self.chat = chat
        self.message = message


def send_with_timeout(
    chat,
    message,
    timeout_pool: ThreadPoolExecutor,
    cancel_event: threading.Event | None,
    retry_timeout: float,
    agent_name: str,
    logger,
    on_reset=None,
    max_retries: int | None = None,
    reset_threshold: int | None = None,
    on_event: OnEventCallback = None,
) -> LLMResponse:
    """Send a message to the LLM with periodic warnings and retry on timeout."""
    submit_fn = _SubmitFn(timeout_pool, chat, message, "send")
    return _send_with_retry(
        submit_fn, timeout_pool, cancel_event, retry_timeout,
        agent_name, on_reset, max_retries, reset_threshold,
        on_event=on_event,
    )


def send_with_timeout_stream(
    chat,
    message,
    timeout_pool: ThreadPoolExecutor,
    cancel_event: threading.Event | None,
    retry_timeout: float,
    agent_name: str,
    logger,
    on_chunk=None,
    on_reset=None,
    max_retries: int | None = None,
    reset_threshold: int | None = None,
    on_event: OnEventCallback = None,
) -> LLMResponse:
    """Like ``send_with_timeout`` but uses ``chat.send_stream()`` for incremental text.

    ``on_chunk`` is called from the thread-pool thread as text deltas arrive.
    """
    extra_args = (on_chunk,) if on_chunk is not None else ()
    submit_fn = _SubmitFn(timeout_pool, chat, message, "send_stream", extra_args)
    return _send_with_retry(
        submit_fn, timeout_pool, cancel_event, retry_timeout,
        agent_name, on_reset, max_retries, reset_threshold,
        on_event=on_event,
    )


def track_llm_usage(
    response: LLMResponse,
    token_state: dict,
    agent_name: str,
    last_tool_context: str,
    *,
    system_tokens: int = 0,
    tools_tokens: int = 0,
    on_event: OnEventCallback = None,
):
    """Accumulate token usage from an LLMResponse and emit via on_event callback.

    Shared implementation used by BaseAgent and its subclasses.

    Args:
        response: The LLMResponse to extract usage from.
        token_state: Mutable dict with keys 'input', 'output', 'thinking',
            'cached', 'api_calls'. Updated in-place.
        agent_name: Label for log messages.
        last_tool_context: Tool context string for the token log.
        system_tokens: Approximate token count of the system prompt (0 = unknown).
        tools_tokens: Approximate token count of tool declarations (0 = unknown).
        on_event: Optional callback ``(event_type, payload_dict)`` for emitting events.
    """
    usage = response.usage
    call_input = usage.input_tokens
    call_output = usage.output_tokens
    call_thinking = usage.thinking_tokens
    call_cached = usage.cached_tokens
    token_state["input"] += call_input
    token_state["output"] += call_output
    token_state["thinking"] += call_thinking
    token_state["cached"] += call_cached
    token_state["api_calls"] += 1

    # Compute history tokens when decomposition is available
    has_decomp = system_tokens > 0 or tools_tokens > 0
    history_tokens = (
        max(0, call_input - system_tokens - tools_tokens) if has_decomp else 0
    )

    # Build log message
    if has_decomp:
        decomp_str = (
            f" (sys:{system_tokens} tools:{tools_tokens} hist:{history_tokens})"
        )
    else:
        decomp_str = ""

    def _emit(event_type: str, **payload):
        if on_event:
            on_event(event_type, payload)

    _emit(
        EVENT_LLM_RESPONSE,
        agent=agent_name,
        level="debug",
        msg=f"[LLM] {agent_name} responded — in:{call_input} out:{call_output} think:{call_thinking}",
        data={
            "agent_name": agent_name,
            "input_tokens": call_input,
            "output_tokens": call_output,
            "thinking_tokens": call_thinking,
        },
    )

    # Truncate tool context for logging (not data truncation)
    _tool_ctx = last_tool_context
    if _tool_ctx and len(_tool_ctx) > 500:
        _tool_ctx = _tool_ctx[:500] + "..."

    _emit(
        EVENT_TOKEN_USAGE,
        agent=agent_name,
        level="debug",
        msg=(
            f"[Tokens] {agent_name} in:{call_input}{decomp_str} out:{call_output} "
            f"think:{call_thinking} | cum_in:{token_state['input']} "
            f"cum_out:{token_state['output']} calls:{token_state['api_calls']}"
        ),
        data={
            "agent_name": agent_name,
            "tool_context": _tool_ctx if _tool_ctx else "unknown",
            "input_tokens": call_input,
            "output_tokens": call_output,
            "thinking_tokens": call_thinking,
            "cached_tokens": call_cached,
            "system_tokens": system_tokens,
            "tools_tokens": tools_tokens,
            "history_tokens": history_tokens,
            "cumulative_input": token_state["input"],
            "cumulative_output": token_state["output"],
            "cumulative_thinking": token_state["thinking"],
            "cumulative_cached": token_state["cached"],
            "api_calls": token_state["api_calls"],
        },
    )
    # Always emit thinking events so consumers can forward to frontend
    if response.thoughts:
        combined = "\n\n".join(response.thoughts)
        _emit(
            EVENT_THINKING,
            agent=agent_name,
            level="debug",
            msg=f"[Thinking] {combined}",
            data={"text": combined},
        )


def execute_tools_batch(
    function_calls: list,
    tool_executor,
    parallel_safe_tools: set[str],
    parallel_enabled: bool,
    max_workers: int,
    agent_name: str,
    logger,
    on_event: OnEventCallback = None,
) -> list[tuple[str | None, str, dict, dict]]:
    """Execute tool calls, parallelizing when all are in the safe set.

    Shared implementation used by OrchestratorAgent.

    Returns list of (tool_call_id, tool_name, tool_args, result) in original order.
    """
    parsed = [
        (
            getattr(fc, "id", None),
            fc.name,
            fc.args
            if isinstance(fc.args, dict)
            else (dict(fc.args) if fc.args else {}),
        )
        for fc in function_calls
    ]

    all_safe = (
        parallel_enabled
        and len(parsed) > 1
        and all(name in parallel_safe_tools for _, name, _ in parsed)
    )

    if not all_safe:
        return [
            (tc_id, name, args, tool_executor(name, args, tc_id))
            for tc_id, name, args in parsed
        ]

    if on_event:
        on_event(EVENT_DEBUG, {
            "agent": agent_name,
            "level": "debug",
            "msg": f"[{agent_name}] Parallel: {len(parsed)} tools concurrently",
        })
    workers = min(len(parsed), max_workers)
    results_by_idx: dict[int, dict] = {}

    with ThreadPoolExecutor(max_workers=workers) as pool:
        futures = {
            pool.submit(
                contextvars.copy_context().run, tool_executor, name, args, tc_id
            ): idx
            for idx, (tc_id, name, args) in enumerate(parsed)
        }
        for future in as_completed(futures):
            idx = futures[future]
            try:
                results_by_idx[idx] = future.result()
            except Exception as e:
                results_by_idx[idx] = {
                    "status": "error",
                    "message": f"Parallel execution error: {e}",
                }

    return [
        (parsed[i][0], parsed[i][1], parsed[i][2], results_by_idx[i])
        for i in range(len(parsed))
    ]


def build_outcome_summary(tool_outcomes: list[dict]) -> str:
    """Build a concise text summary from structured tool outcomes.

    Used by the orchestrator's _execute_task fallback when the LLM
    produces no text.
    """
    parts = []
    for o in tool_outcomes:
        s = f"{o['tool']}={o['status']}"
        if o.get("label"):
            s += f"(label={o['label']}"
            if o.get("num_points"):
                s += f", {o['num_points']} pts"
            if o.get("units"):
                s += f", {o['units']}"
            s += ")"
        elif o.get("message") and o["status"] == "error":
            msg = str(o["message"])
            if len(msg) > 500:
                msg = msg[:500] + "..."
            s += f"({msg})"
        parts.append(s)
    return "Outcomes: " + "; ".join(parts)
