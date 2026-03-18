"""WorkingDir — agent working directory: lock, git, manifest."""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

if sys.platform == "win32":
    import msvcrt as _msvcrt

    def _lock_fd(fd):
        _msvcrt.locking(fd.fileno(), _msvcrt.LK_NBLCK, 1)

    def _unlock_fd(fd):
        _msvcrt.locking(fd.fileno(), _msvcrt.LK_UNLCK, 1)
else:
    import fcntl as _fcntl

    def _lock_fd(fd):
        _fcntl.flock(fd, _fcntl.LOCK_EX | _fcntl.LOCK_NB)

    def _unlock_fd(fd):
        _fcntl.flock(fd, _fcntl.LOCK_UN)


_AGENT_NAME_RE = re.compile(r"^[a-zA-Z0-9_-]+$")
_LOCK_FILE = ".agent.lock"
_MANIFEST_FILE = ".agent.json"


class WorkingDir:
    """Manages an agent's working directory — locking, git, manifest."""

    def __init__(self, base_dir: Path | str, agent_name: str) -> None:
        if not _AGENT_NAME_RE.match(agent_name):
            raise ValueError(
                f"agent_name must match [a-zA-Z0-9_-]+, got: {agent_name!r}"
            )
        self._base_dir = Path(base_dir)
        self._agent_name = agent_name
        self._path = self._base_dir / agent_name
        self._path.mkdir(exist_ok=True)
        self._lock_file: Any = None

    @property
    def path(self) -> Path:
        return self._path

    # --- Lock lifecycle ---

    def acquire_lock(self) -> None:
        lock_path = self._path / _LOCK_FILE
        self._lock_file = open(lock_path, "w")
        try:
            _lock_fd(self._lock_file)
        except OSError:
            self._lock_file.close()
            self._lock_file = None
            raise RuntimeError(
                f"Working directory '{self._path}' is already in use "
                f"by another agent. Each agent needs its own directory."
            )

    def release_lock(self) -> None:
        if self._lock_file is not None:
            try:
                _unlock_fd(self._lock_file)
                self._lock_file.close()
            except OSError:
                pass
            self._lock_file = None

    # --- Git operations ---

    def init_git(self) -> None:
        git_dir = self._path / ".git"
        if git_dir.is_dir():
            return

        try:
            subprocess.run(
                ["git", "init"], cwd=self._path,
                capture_output=True, check=True,
            )
            subprocess.run(
                ["git", "config", "user.email", "agent@stoai"],
                cwd=self._path, capture_output=True, check=True,
            )
            subprocess.run(
                ["git", "config", "user.name", "StoAI Agent"],
                cwd=self._path, capture_output=True, check=True,
            )

            gitignore = self._path / ".gitignore"
            gitignore.write_text(
                "# Track nothing by default\n"
                "*\n"
                "# Except these\n"
                "!.gitignore\n"
                "!system/\n"
                "!system/**\n"
                "!history/\n"
                "!history/**\n"
                "!logs/\n"
                "!logs/**\n"
                "!conscience/\n"
                "!conscience/**\n"
                "!mailbox/\n"
                "!mailbox/**\n"
                "!mcp/\n"
                "!mcp/**\n"
            )

            system_dir = self._path / "system"
            system_dir.mkdir(exist_ok=True)
            covenant_file = system_dir / "covenant.md"
            if not covenant_file.is_file():
                covenant_file.write_text("")
            memory_file = system_dir / "memory.md"
            if not memory_file.is_file():
                memory_file.write_text("")

            subprocess.run(
                ["git", "add", ".gitignore", "system/"],
                cwd=self._path, capture_output=True, check=True,
            )
            subprocess.run(
                ["git", "commit", "-m", "init: agent working directory"],
                cwd=self._path, capture_output=True, check=True,
            )
        except (FileNotFoundError, subprocess.CalledProcessError):
            system_dir = self._path / "system"
            system_dir.mkdir(exist_ok=True)
            covenant_file = system_dir / "covenant.md"
            if not covenant_file.is_file():
                covenant_file.write_text("")
            memory_file = system_dir / "memory.md"
            if not memory_file.is_file():
                memory_file.write_text("")

    def diff(self, rel_path: str) -> str:
        try:
            result = subprocess.run(
                ["git", "diff", rel_path],
                cwd=self._path, capture_output=True, text=True,
            )
            diff_text = result.stdout.strip()
            if not diff_text:
                status_result = subprocess.run(
                    ["git", "status", "--porcelain", rel_path],
                    cwd=self._path, capture_output=True, text=True,
                )
                if status_result.stdout.strip():
                    file_path = self._path / rel_path
                    diff_text = f"(new/untracked file)\n{file_path.read_text()}"
        except (FileNotFoundError, subprocess.CalledProcessError):
            diff_text = ""
        return diff_text

    def diff_and_commit(self, rel_path: str, label: str) -> tuple[str | None, str | None]:
        try:
            diff_result = subprocess.run(
                ["git", "diff", rel_path],
                cwd=self._path, capture_output=True, text=True,
            )
            diff_cached = subprocess.run(
                ["git", "diff", "--cached", rel_path],
                cwd=self._path, capture_output=True, text=True,
            )
            status_result = subprocess.run(
                ["git", "status", "--porcelain", rel_path],
                cwd=self._path, capture_output=True, text=True,
            )

            has_changes = bool(
                diff_result.stdout.strip()
                or diff_cached.stdout.strip()
                or status_result.stdout.strip()
            )

            if not has_changes:
                return None, None

            diff_text = diff_result.stdout or status_result.stdout

            subprocess.run(
                ["git", "add", rel_path],
                cwd=self._path, capture_output=True, check=True,
            )

            if not diff_text.strip():
                staged = subprocess.run(
                    ["git", "diff", "--cached", rel_path],
                    cwd=self._path, capture_output=True, text=True,
                )
                diff_text = staged.stdout

            subprocess.run(
                ["git", "commit", "-m", f"system: update {label}"],
                cwd=self._path, capture_output=True, check=True,
            )

            hash_result = subprocess.run(
                ["git", "rev-parse", "--short", "HEAD"],
                cwd=self._path, capture_output=True, text=True,
            )
            commit_hash = hash_result.stdout.strip()

            return diff_text, commit_hash

        except (FileNotFoundError, subprocess.CalledProcessError):
            return None, None

    # --- Manifest ---

    def read_manifest(self) -> str:
        """Read the covenant from the manifest file. Returns empty string if missing."""
        path = self._path / _MANIFEST_FILE
        if not path.is_file():
            return ""
        try:
            data = json.loads(path.read_text())
            return data.get("covenant", "")
        except (json.JSONDecodeError, OSError):
            corrupt = self._path / ".agent.json.corrupt"
            try:
                path.rename(corrupt)
            except OSError:
                pass
            return ""

    def write_manifest(self, manifest: dict) -> None:
        target = self._path / _MANIFEST_FILE
        tmp = self._path / ".agent.json.tmp"
        tmp.write_text(json.dumps(manifest, indent=2))
        os.replace(str(tmp), str(target))
