"""
Loop guard for agent tool-call loops.

Prevents runaway tool-call loops with:
  - A hard total-call limit (safety ceiling).
  - Per-(tool, args) duplicate tracking with escalating warnings
    and eventual hard-block, to detect and stop polling loops.
"""

from __future__ import annotations

import json
from dataclasses import dataclass


@dataclass(frozen=True)
class DupVerdict:
    """Result of duplicate-call tracking for a single tool invocation.

    Attributes:
        count: Total times this (name, args) has been seen (including this call).
        blocked: If True, the caller should skip execution entirely.
        warning: Warning text to inject into the result dict, or None.
    """
    count: int
    blocked: bool
    warning: str | None


# Keys stripped from tool args before computing the dedup key.
# These carry metadata, not semantic intent.
_STRIP_KEYS = frozenset({"commentary", "_sync"})


class LoopGuard:
    """Prevents runaway tool-call loops in agent conversations.

    Usage:
        guard = LoopGuard(max_total_calls=10)

        while True:
            # ... extract tool_calls from response ...

            reason = guard.check_limit(len(tool_calls))
            if reason:
                break

            for tc in tool_calls:
                verdict = guard.record_tool_call(tc.name, tc.args)
                if verdict.blocked:
                    # skip execution, return blocked result
                    ...
                else:
                    # execute tool
                    ...
                    if verdict.warning:
                        result["_duplicate_warning"] = verdict.warning

            guard.record_calls(len(tool_calls))

            # ... send results back ...
    """

    def __init__(
        self,
        max_total_calls: int = 10,
        dup_free_passes: int = 2,
        dup_hard_block: int = 8,
        invalid_tool_limit: int = 2,
        **_kwargs,
    ):
        self.max_total_calls = max_total_calls
        self.total_calls = 0
        self._dup_free_passes = dup_free_passes
        self._dup_hard_block = dup_hard_block
        self._dup_counts: dict[tuple[str, str], int] = {}
        # Track invalid/hallucinated tool names
        self._invalid_tool_limit = invalid_tool_limit
        self._invalid_tool_counts: dict[str, int] = {}
        self._total_invalid_tools = 0

    def check_limit(self, n_calls: int) -> str | None:
        """Check if executing n_calls would exceed the total limit.

        Returns a stop reason string or None to continue.
        """
        if n_calls <= 0:
            return None
        if self.total_calls + n_calls > self.max_total_calls:
            return (
                f"total call limit ({self.max_total_calls}) reached "
                f"after {self.total_calls} calls"
            )
        return None

    def record_invalid_tool(self, tool_name: str) -> None:
        """Record that a tool call was rejected as invalid/not-available."""
        self._invalid_tool_counts[tool_name] = (
            self._invalid_tool_counts.get(tool_name, 0) + 1
        )
        self._total_invalid_tools += 1

    def check_invalid_tool_limit(self) -> str | None:
        """Check if repeated invalid tool calls should stop the loop.

        Returns a stop reason string if any single tool name has been rejected
        more than ``invalid_tool_limit`` times, or if the total number of
        invalid tool calls exceeds ``invalid_tool_limit * 2``.
        Returns None to continue.
        """
        for name, count in self._invalid_tool_counts.items():
            if count > self._invalid_tool_limit:
                return (
                    f"tool '{name}' was rejected {count} times "
                    f"(limit: {self._invalid_tool_limit}) — "
                    f"the model is hallucinating tool names not in its schema"
                )
        if self._total_invalid_tools > self._invalid_tool_limit * 2:
            return (
                f"{self._total_invalid_tools} total invalid tool calls "
                f"(limit: {self._invalid_tool_limit * 2}) — "
                f"the model is hallucinating tool names"
            )
        return None

    def record_calls(self, n_calls: int) -> None:
        """Record that n_calls were executed."""
        self.total_calls += n_calls

    # ------------------------------------------------------------------
    # Duplicate call tracking
    # ------------------------------------------------------------------

    @staticmethod
    def _dedup_key(name: str, args: dict | None) -> tuple[str, str]:
        """Create a hashable key for duplicate detection.

        Strips metadata keys (commentary, _sync) before serializing,
        so that semantically identical calls with different metadata
        are treated as duplicates.
        """
        if not args:
            cleaned = {}
        else:
            cleaned = {k: v for k, v in args.items() if k not in _STRIP_KEYS}
        try:
            args_str = json.dumps(cleaned, sort_keys=True, default=str)
        except (TypeError, ValueError):
            args_str = str(sorted(cleaned.items()))
        return (name, args_str)

    def record_tool_call(self, name: str, args: dict | None) -> DupVerdict:
        """Record a tool call and return a verdict on whether to execute it.

        Call this *before* executing each tool. The verdict tells the caller:
          - Whether to skip execution (verdict.blocked)
          - Whether to inject a warning into the result (verdict.warning)
        """
        key = self._dedup_key(name, args)
        count = self._dup_counts.get(key, 0) + 1
        self._dup_counts[key] = count

        if count <= self._dup_free_passes:
            return DupVerdict(count=count, blocked=False, warning=None)

        if count >= self._dup_hard_block:
            return DupVerdict(
                count=count,
                blocked=True,
                warning=self._warning_for_count(name, count),
            )

        return DupVerdict(
            count=count,
            blocked=False,
            warning=self._warning_for_count(name, count),
        )

    def _warning_for_count(self, name: str, count: int) -> str:
        """Generate an escalating warning message for duplicate calls."""
        if count < self._dup_free_passes + 3:
            # Mild warning (passes N+1 to N+2)
            return (
                f"You have called '{name}' with identical arguments {count} times. "
                f"Consider whether this is necessary — repeated identical calls "
                f"waste tokens and rarely produce new information."
            )
        if count < self._dup_hard_block:
            # Strong warning (passes N+3+)
            return (
                f"STOP POLLING: '{name}' called {count} times with identical arguments. "
                f"This is a polling loop that wastes tokens. Do NOT call this tool again "
                f"with the same arguments. If you are waiting for a result, it will be "
                f"delivered to you automatically — do not poll for it."
            )
        # At or past hard block
        return (
            f"BLOCKED: '{name}' has been called {count} times with identical arguments "
            f"(hard limit: {self._dup_hard_block}). Execution was SKIPPED. "
            f"This call was a polling loop. Do NOT repeat it. "
            f"Proceed with a different approach or respond to the user."
        )

    @property
    def dup_counts(self) -> dict[tuple[str, str], int]:
        """Expose duplicate counts for testing/debugging."""
        return dict(self._dup_counts)
