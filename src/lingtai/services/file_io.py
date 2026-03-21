"""FileIOService — abstract file access backing read/edit/write/glob/grep intrinsics.

First implementation: LocalFileIOService (local filesystem, text files only).
Future: RichFileIOService (PDF, images), RemoteFileIOService, SandboxedFileIOService.
"""
from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from pathlib import Path
from typing import Any


@dataclass
class GrepMatch:
    """A single grep match result."""
    path: str
    line_number: int
    line: str


class FileIOService(ABC):
    """Abstract file I/O service.

    Backs the read, edit, write, glob, and grep intrinsics.
    Implementations can provide local filesystem, remote, sandboxed, or
    format-aware (PDF, images) file access.
    """

    @abstractmethod
    def read(self, path: str) -> str:
        """Read file contents as text."""
        ...

    @abstractmethod
    def write(self, path: str, content: str) -> None:
        """Write content to a file (create or overwrite)."""
        ...

    @abstractmethod
    def edit(self, path: str, old_string: str, new_string: str) -> str:
        """Replace old_string with new_string in the file. Returns updated content."""
        ...

    @abstractmethod
    def glob(self, pattern: str, root: str | None = None) -> list[str]:
        """Find files matching a glob pattern."""
        ...

    @abstractmethod
    def grep(self, pattern: str, path: str | None = None, max_results: int = 50) -> list[GrepMatch]:
        """Search file contents by regex pattern."""
        ...


class LocalFileIOService(FileIOService):
    """Local filesystem implementation — text files only.

    This is the first and simplest implementation. It reads/writes files
    on the local filesystem using Path operations.
    """

    def __init__(self, root: Path | str | None = None):
        self._root = Path(root) if root else None

    def _resolve(self, path: str) -> Path:
        p = Path(path)
        if not p.is_absolute() and self._root:
            p = self._root / p
        return p

    def read(self, path: str) -> str:
        return self._resolve(path).read_text(encoding="utf-8")

    def write(self, path: str, content: str) -> None:
        p = self._resolve(path)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content, encoding="utf-8")

    def edit(self, path: str, old_string: str, new_string: str) -> str:
        p = self._resolve(path)
        content = p.read_text(encoding="utf-8")
        if old_string not in content:
            raise ValueError(f"old_string not found in {path}")
        count = content.count(old_string)
        if count > 1:
            raise ValueError(
                f"old_string appears {count} times in {path} — must be unique. "
                "Provide more context to make it unique."
            )
        content = content.replace(old_string, new_string, 1)
        p.write_text(content, encoding="utf-8")
        return content

    def glob(self, pattern: str, root: str | None = None) -> list[str]:
        import fnmatch
        import os

        search_root = Path(root) if root else (self._root or Path("."))
        results = []
        for dirpath, _dirnames, filenames in os.walk(search_root):
            for filename in filenames:
                full = os.path.join(dirpath, filename)
                # Match against pattern relative to search root
                rel = os.path.relpath(full, search_root)
                if fnmatch.fnmatch(rel, pattern):
                    results.append(full)
        return sorted(results)

    def grep(self, pattern: str, path: str | None = None, max_results: int = 50) -> list[GrepMatch]:
        import re

        regex = re.compile(pattern)
        search_path = Path(path) if path else (self._root or Path("."))
        results: list[GrepMatch] = []

        if search_path.is_file():
            files = [search_path]
        else:
            files = sorted(search_path.rglob("*"))

        for f in files:
            if not f.is_file():
                continue
            try:
                text = f.read_text(encoding="utf-8")
            except (UnicodeDecodeError, PermissionError):
                continue
            for i, line in enumerate(text.splitlines(), 1):
                if regex.search(line):
                    results.append(GrepMatch(path=str(f), line_number=i, line=line))
                    if len(results) >= max_results:
                        return results
        return results
