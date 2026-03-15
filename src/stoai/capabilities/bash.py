"""Bash capability — shell command execution with file-based policy.

Adds the ability to run shell commands. This is a capability (not intrinsic)
because not every agent should have shell access — it's a powerful
capability that should be explicitly opted into.

Usage:
    agent.add_capability("bash", policy_file="path/to/policy.json")
    agent.add_capability("bash", yolo=True)  # no restrictions
"""
from __future__ import annotations

import json
import re
import subprocess
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "command": {
            "type": "string",
            "description": "The shell command to execute",
        },
        "timeout": {
            "type": "number",
            "description": "Timeout in seconds (default: 30)",
            "default": 30,
        },
        "working_dir": {
            "type": "string",
            "description": "Working directory for the command (optional)",
        },
    },
    "required": ["command"],
}

DESCRIPTION = "Execute a shell command and return its output."


class BashPolicy:
    """Command execution policy — allow/deny lists with pipe awareness.

    Policy resolution:
    - If both allow and deny are None → everything allowed
    - If only allow is set → only those commands permitted (allowlist mode)
    - If only deny is set → everything except those permitted (denylist mode)
    - If both set → must be in allow AND not in deny
    """

    def __init__(self, allow: list[str] | None = None, deny: list[str] | None = None):
        self._allow = set(allow) if allow else None
        self._deny = set(deny) if deny else None

    @classmethod
    def from_file(cls, path: str) -> "BashPolicy":
        """Load policy from a JSON file with allow/deny lists."""
        p = Path(path)
        if not p.is_file():
            raise FileNotFoundError(f"Policy file not found: {path}")
        data = json.loads(p.read_text())
        return cls(allow=data.get("allow"), deny=data.get("deny"))

    @classmethod
    def yolo(cls) -> "BashPolicy":
        """Create a policy that allows everything."""
        return cls()

    def is_allowed(self, command: str) -> bool:
        """Check if a command string is allowed by this policy.

        Parses pipes, chains, and subshells to check every command.
        """
        if self._allow is None and self._deny is None:
            return True
        commands = self._extract_commands(command)
        return all(self._check_single(cmd) for cmd in commands)

    def _check_single(self, cmd: str) -> bool:
        """Check a single command name against policy."""
        if self._deny is not None and cmd in self._deny:
            return False
        if self._allow is not None and cmd not in self._allow:
            return False
        return True

    @staticmethod
    def _extract_commands(command: str) -> list[str]:
        """Extract all command names from a potentially chained command string.

        Handles: |, &&, ||, ;, $(), backticks.
        Returns the first word of each sub-command.
        """
        flat = command
        # Expand $(...) subshells into the command chain
        flat = re.sub(r'\$\([^)]*\)', lambda m: '; ' + m.group()[2:-1] + ' ;', flat)
        # Expand backtick subshells
        flat = re.sub(r'`[^`]*`', lambda m: '; ' + m.group()[1:-1] + ' ;', flat)
        # Split on pipe/chain operators
        parts = re.split(r'\|{1,2}|&&|;', flat)
        commands = []
        for part in parts:
            tokens = part.strip().split()
            if tokens:
                commands.append(tokens[0])
        return commands


class BashManager:
    """Manages shell command execution for an agent."""

    def __init__(
        self,
        policy: BashPolicy,
        working_dir: str,
        max_output: int = 50_000,
    ):
        self._policy = policy
        self._working_dir = working_dir
        self._max_output = max_output

    def handle(self, args: dict) -> dict:
        command = args.get("command", "")
        if not command.strip():
            return {"error": "command is required"}

        # Check policy
        if not self._policy.is_allowed(command):
            denied = BashPolicy._extract_commands(command)
            return {
                "error": f"Command not allowed by policy. "
                f"Denied command(s): {', '.join(denied)}"
            }

        timeout = args.get("timeout", 30)
        cwd = args.get("working_dir", self._working_dir)

        try:
            result = subprocess.run(
                command,
                shell=True,
                capture_output=True,
                text=True,
                timeout=timeout,
                cwd=cwd,
            )
            stdout = result.stdout
            stderr = result.stderr
            if len(stdout) > self._max_output:
                stdout = stdout[: self._max_output] + f"\n... (truncated, {len(result.stdout)} chars total)"
            if len(stderr) > self._max_output:
                stderr = stderr[: self._max_output] + f"\n... (truncated, {len(result.stderr)} chars total)"

            return {
                "status": "ok",
                "exit_code": result.returncode,
                "stdout": stdout,
                "stderr": stderr,
            }
        except subprocess.TimeoutExpired:
            return {"error": f"Command timed out after {timeout}s"}
        except Exception as e:
            return {"error": f"Command failed: {e}"}


def setup(
    agent: "BaseAgent",
    policy_file: str | None = None,
    yolo: bool = False,
) -> BashManager:
    """Set up the bash capability on an agent.

    Args:
        agent: The agent to extend.
        policy_file: Path to JSON policy file (required unless yolo=True).
        yolo: If True, allow all commands (no policy file needed).

    Returns:
        The BashManager instance for programmatic access.
    """
    if yolo:
        policy = BashPolicy.yolo()
    elif policy_file is not None:
        policy = BashPolicy.from_file(policy_file)
    else:
        raise ValueError(
            "bash capability requires policy_file='path/to/policy.json' or yolo=True"
        )

    mgr = BashManager(
        policy=policy,
        working_dir=str(agent._working_dir),
    )
    agent.add_tool("bash", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)

    agent.update_system_prompt(
        "bash_instructions",
        "You can execute shell commands via the bash tool. "
        "Use it for system operations, running scripts, git commands, "
        "and other tasks that require shell access.",
    )
    return mgr
