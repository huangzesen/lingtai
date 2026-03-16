"""Tests for git-controlled agent working directory."""
from __future__ import annotations

import subprocess
from unittest.mock import MagicMock

import pytest

from stoai.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_start_creates_git_repo(tmp_path):
    """agent.start() should git init the working directory."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        git_dir = agent.working_dir / ".git"
        assert git_dir.is_dir(), "Working dir should have .git after start()"
    finally:
        agent.stop()


def test_start_creates_gitignore(tmp_path):
    """agent.start() should create opt-in .gitignore."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        gitignore = agent.working_dir / ".gitignore"
        assert gitignore.is_file()
        content = gitignore.read_text()
        assert "*" in content
        assert "!.gitignore" in content
        assert "!ltm/" in content
        assert "!ltm/**" in content
    finally:
        agent.stop()


def test_start_creates_ltm_dir(tmp_path):
    """agent.start() should create ltm/ directory and ltm.md."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_dir = agent.working_dir / "ltm"
        assert ltm_dir.is_dir()
        ltm_file = ltm_dir / "ltm.md"
        assert ltm_file.is_file()
    finally:
        agent.stop()


def test_start_makes_initial_commit(tmp_path):
    """agent.start() should make an initial git commit."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
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


def test_start_skips_git_init_on_resume(tmp_path):
    """If .git exists (resume), start() should not reinitialize."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    agent.stop()
    result = subprocess.run(
        ["git", "rev-list", "--count", "HEAD"],
        cwd=agent.working_dir,
        capture_output=True, text=True,
    )
    initial_commits = int(result.stdout.strip())

    agent2 = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent2.start()
    try:
        result2 = subprocess.run(
            ["git", "rev-list", "--count", "HEAD"],
            cwd=agent2.working_dir,
            capture_output=True, text=True,
        )
        resume_commits = int(result2.stdout.strip())
        assert resume_commits == initial_commits
    finally:
        agent2.stop()
