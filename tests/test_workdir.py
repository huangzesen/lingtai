"""Tests for WorkingDir — filesystem, locking, git, manifest."""
from __future__ import annotations

import json
import subprocess
from pathlib import Path

import pytest

from stoai.workdir import WorkingDir


def test_init_creates_agent_dir(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    assert wd.path == tmp_path / "alice"
    assert wd.path.is_dir()


def test_lock_prevents_second_instance(tmp_path):
    wd1 = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd1.acquire_lock()
    try:
        wd2 = WorkingDir(base_dir=tmp_path, agent_id="alice")
        with pytest.raises(RuntimeError, match="already in use"):
            wd2.acquire_lock()
    finally:
        wd1.release_lock()


def test_lock_release_allows_reuse(tmp_path):
    wd1 = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd1.acquire_lock()
    wd1.release_lock()
    wd2 = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd2.acquire_lock()  # should not raise
    wd2.release_lock()


def test_git_init_creates_repo(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    assert (wd.path / ".git").is_dir()
    assert (wd.path / ".gitignore").is_file()
    assert (wd.path / "system" / "covenant.md").is_file()
    assert (wd.path / "system" / "memory.md").is_file()


def test_git_init_skips_if_already_initialized(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    result1 = subprocess.run(
        ["git", "rev-list", "--count", "HEAD"],
        cwd=wd.path, capture_output=True, text=True,
    )
    wd.init_git()  # second call — should be no-op
    result2 = subprocess.run(
        ["git", "rev-list", "--count", "HEAD"],
        cwd=wd.path, capture_output=True, text=True,
    )
    assert result1.stdout.strip() == result2.stdout.strip()


def test_read_manifest_returns_empty_when_missing(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    assert wd.read_manifest() == ""


def test_write_and_read_manifest(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    manifest = {"agent_id": "alice", "covenant": "researcher", "started_at": "2026-01-01T00:00:00Z"}
    wd.write_manifest(manifest)
    covenant = wd.read_manifest()
    assert covenant == "researcher"


def test_diff_and_commit(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    # Write to tracked file
    memory_file = wd.path / "system" / "memory.md"
    memory_file.write_text("hello world")
    diff_text, commit_hash = wd.diff_and_commit("system/memory.md", "memory")
    assert commit_hash is not None
    assert diff_text  # should have some diff content


def test_diff_and_commit_no_changes(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    diff_text, commit_hash = wd.diff_and_commit("system/memory.md", "memory")
    assert diff_text is None
    assert commit_hash is None


def test_diff_read_only(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    memory_file = wd.path / "system" / "memory.md"
    memory_file.write_text("new content")
    result = wd.diff("system/memory.md")
    assert isinstance(result, str)
    # Should not commit — file should still show as changed
    status = subprocess.run(
        ["git", "status", "--porcelain", "system/memory.md"],
        cwd=wd.path, capture_output=True, text=True,
    )
    assert status.stdout.strip()  # still dirty


def test_invalid_agent_id_raises(tmp_path):
    with pytest.raises(ValueError, match="agent_id must match"):
        WorkingDir(base_dir=tmp_path, agent_id="bad agent!")
