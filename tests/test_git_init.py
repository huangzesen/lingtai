"""Tests for git-controlled agent working directory."""
from __future__ import annotations

import subprocess
from unittest.mock import MagicMock

import pytest

from lingtai_kernel.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_start_creates_git_repo(tmp_path):
    """agent.start() should git init the working directory."""
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    agent.start()
    try:
        git_dir = agent.working_dir / ".git"
        assert git_dir.is_dir(), "Working dir should have .git after start()"
    finally:
        agent.stop()


def test_start_creates_gitignore(tmp_path):
    """agent.start() should create opt-in .gitignore."""
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    agent.start()
    try:
        gitignore = agent.working_dir / ".gitignore"
        assert gitignore.is_file()
        content = gitignore.read_text()
        assert "*" in content
        assert "!.gitignore" in content
        assert "!system/" in content
        assert "!system/**" in content
    finally:
        agent.stop()


def test_start_creates_system_dir(tmp_path):
    """agent.start() should create system/ directory with covenant.md and memory.md."""
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    agent.start()
    try:
        system_dir = agent.working_dir / "system"
        assert system_dir.is_dir()
        assert (system_dir / "covenant.md").is_file()
        assert (system_dir / "memory.md").is_file()
    finally:
        agent.stop()


def test_start_makes_initial_commit(tmp_path):
    """agent.start() should make an initial git commit."""
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    agent.start()
    try:
        result = subprocess.run(
            ["git", "log", "--oneline", "-1"],
            cwd=agent.working_dir,
            capture_output=True, text=True,
        )
        assert result.returncode == 0
        assert "init" in result.stdout.lower()
    finally:
        agent.stop()


def test_start_skips_git_init_if_git_exists(tmp_path):
    """If .git already exists, start() should not reinitialize."""
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    agent.start()
    result = subprocess.run(
        ["git", "rev-list", "--count", "HEAD"],
        cwd=agent.working_dir,
        capture_output=True, text=True,
    )
    initial_commits = int(result.stdout.strip())

    # Stop and restart the same agent — git should not re-init
    agent.stop()
    agent.start()
    try:
        result2 = subprocess.run(
            ["git", "rev-list", "--count", "HEAD"],
            cwd=agent.working_dir,
            capture_output=True, text=True,
        )
        resume_commits = int(result2.stdout.strip())
        assert resume_commits == initial_commits
    finally:
        agent.stop()
