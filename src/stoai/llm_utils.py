"""
Shared LLM utilities used by BaseAgent and its subclasses.

All functions are stateless (operate on passed-in state dicts).
"""

import contextvars
import json
import time
from pathlib import Path
from concurrent.futures import Future, ThreadPoolExecutor, as_completed

from .llm import LLMResponse
from .logging import get_logger

_logger = get_logger()

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


# LLM API call timeout thresholds (seconds)
_LLM_WARN_INTERVAL = 20  # log a warning every N seconds while waiting
_LLM_RETRY_TIMEOUT = 120  # abandon call and retry after this
_LLM_MAX_RETRIES = 4  # retry 4 times, then rollback takes over
_API_ERROR_RETRY_DELAYS = [10.0, 10.0]
_SESSION_RESET_THRESHOLD = 2  # rollback after 2 consecutive errors (~20s)


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
    retry_timeout: float,
    agent_name: str,
    on_reset=None,
    max_retries: int | None = None,
    reset_threshold: int | None = None,
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

    ``on_reset(chat, message)`` must return ``(new_chat, new_message)``.
    """
    if max_retries is None:
        max_retries = _LLM_MAX_RETRIES
    if reset_threshold is None:
        reset_threshold = _SESSION_RESET_THRESHOLD

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
                    _logger.warning(
                        "[%s] LLM API not responding after %.0fs (attempt %d)...",
                        agent_name, elapsed, attempt + 1,
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
                _logger.warning(
                    "[%s] LLM API timed out after %.0fs, retrying (%d/%d)...",
                    agent_name, elapsed, attempt + 1, max_retries,
                )
            else:
                _logger.error(
                    "[%s] LLM API timed out after %.0fs, no retries left",
                    agent_name, elapsed,
                )
        except Exception as exc:
            if _is_history_desync_error(exc) and on_reset and not _desync_reset_done:
                _desync_reset_done = True
                _logger.warning(
                    "[%s] History desync detected, resetting session: %s",
                    agent_name, exc,
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
                _logger.warning(
                    "[%s] Precondition check failed, resetting session: %s",
                    agent_name, exc,
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
                _logger.warning(
                    "[%s] Bad request (likely corrupted history), resetting session: %s",
                    agent_name, exc,
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
                _logger.warning(
                    "[%s] API server error, retrying in %.0fs (%d/%d)...",
                    agent_name, delay, attempt + 1, max_retries,
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
    retry_timeout: float,
    agent_name: str,
    logger,
    on_reset=None,
    max_retries: int | None = None,
    reset_threshold: int | None = None,
) -> LLMResponse:
    """Send a message to the LLM with periodic warnings and retry on timeout."""
    submit_fn = _SubmitFn(timeout_pool, chat, message, "send")
    return _send_with_retry(
        submit_fn, timeout_pool, retry_timeout,
        agent_name, on_reset, max_retries, reset_threshold,
    )


def send_with_timeout_stream(
    chat,
    message,
    timeout_pool: ThreadPoolExecutor,
    retry_timeout: float,
    agent_name: str,
    logger,
    on_chunk=None,
    on_reset=None,
    max_retries: int | None = None,
    reset_threshold: int | None = None,
) -> LLMResponse:
    """Like ``send_with_timeout`` but uses ``chat.send_stream()`` for incremental text.

    ``on_chunk`` is called from the thread-pool thread as text deltas arrive.
    """
    extra_args = (on_chunk,) if on_chunk is not None else ()
    submit_fn = _SubmitFn(timeout_pool, chat, message, "send_stream", extra_args)
    return _send_with_retry(
        submit_fn, timeout_pool, retry_timeout,
        agent_name, on_reset, max_retries, reset_threshold,
    )


def track_llm_usage(
    response: LLMResponse,
    token_state: dict,
    agent_name: str,
    last_tool_context: str,
    *,
    system_tokens: int = 0,
    tools_tokens: int = 0,
):
    """Accumulate token usage from an LLMResponse.

    Shared implementation used by BaseAgent and its subclasses.

    Args:
        response: The LLMResponse to extract usage from.
        token_state: Mutable dict with keys 'input', 'output', 'thinking',
            'cached', 'api_calls'. Updated in-place.
        agent_name: Label for log messages.
        last_tool_context: Tool context string for the token log.
        system_tokens: Approximate token count of the system prompt (0 = unknown).
        tools_tokens: Approximate token count of tool declarations (0 = unknown).
    """
    usage = response.usage
    token_state["input"] += usage.input_tokens
    token_state["output"] += usage.output_tokens
    token_state["thinking"] += usage.thinking_tokens
    token_state["cached"] += usage.cached_tokens
    token_state["api_calls"] += 1


def execute_tools_batch(
    function_calls: list,
    tool_executor,
    parallel_safe_tools: set[str],
    parallel_enabled: bool,
    max_workers: int,
    agent_name: str,
    logger,
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
