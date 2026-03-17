"""LoggingService — structured event logging backing agent observability.

First implementation: JSONLLoggingService (append JSON lines to a file).

Design principles:
- Single method: log(event) — thread-safe, fire-and-forget
- Persistent: events written to disk, not transient callbacks
- Swappable: Forum can subclass to build communication graphs in real-time
"""
from __future__ import annotations

import json
import threading
from abc import ABC, abstractmethod
from pathlib import Path


class LoggingService(ABC):
    """Abstract structured event logging service.

    Backs agent observability. Implementations provide the actual
    storage mechanism (JSONL file, database, network sink, etc.).
    """

    @abstractmethod
    def log(self, event: dict) -> None:
        """Log a structured event. Must be thread-safe."""

    def close(self) -> None:
        """Flush and release resources. Default no-op."""


class JSONLLoggingService(LoggingService):
    """Append structured events as JSON lines to a file.

    Thread-safe via lock. Flushes after every write for real-time tailing.
    """

    def __init__(self, path: Path | str) -> None:
        self._path = Path(path)
        self._path.parent.mkdir(parents=True, exist_ok=True)
        self._file = open(self._path, "a")
        self._lock = threading.Lock()
        self._closed = False

    def log(self, event: dict) -> None:
        if self._closed:
            return
        line = json.dumps(event, default=str)
        with self._lock:
            self._file.write(line + "\n")
            self._file.flush()

    def get_events(self) -> list[dict]:
        """Read all events from the JSONL file. Thread-safe."""
        with self._lock:
            # Re-read the file from start
            if not self._path.exists():
                return []
            events = []
            with open(self._path, "r") as f:
                for line in f:
                    line = line.strip()
                    if line:
                        try:
                            events.append(json.loads(line))
                        except json.JSONDecodeError:
                            continue
            return events

    def close(self) -> None:
        if not self._closed:
            self._closed = True
            self._file.close()
