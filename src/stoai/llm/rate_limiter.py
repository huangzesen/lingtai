"""Shared rate limiter for all LLM adapters."""
import threading
import time


class RateLimiter:
    """Thread-safe rate limiter — enforces minimum interval between API calls."""

    def __init__(self, min_interval: float):
        """Initialize with minimum interval in seconds.

        Args:
            min_interval: Minimum seconds between API calls. 0 disables rate limiting.
        """
        self._min_interval = min_interval
        self._lock = threading.Lock()
        self._last_call = 0.0

    def wait(self) -> None:
        """Block until min_interval has elapsed since the last call."""
        if self._min_interval <= 0:
            return
        with self._lock:
            now = time.monotonic()
            elapsed = now - self._last_call
            if elapsed < self._min_interval:
                time.sleep(self._min_interval - elapsed)
            self._last_call = time.monotonic()
