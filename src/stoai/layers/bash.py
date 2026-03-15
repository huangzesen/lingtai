"""Bash layer — shell command execution.

Adds the ability to run shell commands. This is a layer (not intrinsic)
because not every agent should have shell access — it's a powerful
capability that should be explicitly opted into.

Usage:
    from stoai.layers.bash import add_bash_layer
    add_bash_layer(agent, allowed_commands=None)  # unrestricted
    add_bash_layer(agent, allowed_commands=["git", "npm", "python"])  # restricted
"""
from __future__ import annotations

import subprocess
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


class BashManager:
    """Manages shell command execution for an agent."""

    def __init__(
        self,
        working_dir: str | None = None,
        allowed_commands: list[str] | None = None,
        max_output: int = 50_000,
    ):
        """
        Args:
            working_dir: Default working directory for commands.
            allowed_commands: If set, only these command prefixes are allowed.
                None means unrestricted.
            max_output: Maximum output length in characters (default 50k).
        """
        self._working_dir = working_dir
        self._allowed_commands = allowed_commands
        self._max_output = max_output

    def handle(self, args: dict) -> dict:
        command = args.get("command", "")
        if not command.strip():
            return {"error": "command is required"}

        # Check allowlist
        if self._allowed_commands is not None:
            cmd_prefix = command.strip().split()[0]
            if cmd_prefix not in self._allowed_commands:
                return {
                    "error": f"Command '{cmd_prefix}' not allowed. "
                    f"Allowed: {', '.join(self._allowed_commands)}"
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


def add_bash_layer(
    agent: "BaseAgent",
    working_dir: str | None = None,
    allowed_commands: list[str] | None = None,
) -> BashManager:
    """Add shell execution capability to an agent.

    Args:
        agent: The agent to extend.
        working_dir: Default working directory for commands.
        allowed_commands: If set, restrict to these command prefixes.
            None means unrestricted access.

    Returns:
        The BashManager instance for programmatic access.
    """
    mgr = BashManager(working_dir=working_dir, allowed_commands=allowed_commands)
    agent.add_tool("bash", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)

    restrictions = ""
    if allowed_commands:
        restrictions = f" You can only run: {', '.join(allowed_commands)}."

    agent.update_system_prompt(
        "bash_instructions",
        "You can execute shell commands via the bash tool. "
        "Use it for system operations, running scripts, git commands, "
        f"and other tasks that require shell access.{restrictions}",
    )
    return mgr
