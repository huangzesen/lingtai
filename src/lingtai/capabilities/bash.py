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

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

PROVIDERS = {"providers": [], "default": "builtin"}

_DEFAULT_POLICY_FILE = Path(__file__).parent / "bash_policy.json"

def get_description(lang: str = "en") -> str:
    return t(lang, "bash.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "command": {
                "type": "string",
                "description": t(lang, "bash.command"),
            },
            "timeout": {
                "type": "number",
                "description": t(lang, "bash.timeout"),
                "default": 30,
            },
            "working_dir": {
                "type": "string",
                "description": t(lang, "bash.working_dir"),
            },
        },
        "required": ["command"],
    }



class BashPolicy:
    """Command execution policy — allow/deny lists with pipe awareness.

    Two modes, determined by the policy file content:
    - **Denylist mode** (only ``deny`` key): everything allowed except denied commands.
    - **Allowlist mode** (``allow`` key present): only listed commands allowed,
      everything else blocked. ``deny`` key is ignored in this mode.

    The mode is implicit — if ``allow`` is present, it's allowlist mode.
    """

    def __init__(self, allow: list[str] | None = None, deny: list[str] | None = None):
        self._allow = set(allow) if allow else None
        # deny is only used in denylist mode (when allow is absent)
        self._deny = set(deny) if deny and not allow else None

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

    def describe(self) -> str:
        """Return a human-readable summary of the policy rules."""
        if self._allow is None and self._deny is None:
            return ""
        if self._allow is not None:
            return (
                f"ALLOWLIST MODE: Only these commands are permitted (all others blocked): "
                f"{', '.join(sorted(self._allow))}"
            )
        return (
            f"DENYLIST MODE: All commands are allowed except: "
            f"{', '.join(sorted(self._deny))}"
        )

    def is_allowed(self, command: str) -> bool:
        """Check if a command string is allowed by this policy.

        Parses pipes, chains, and subshells to check every command.
        """
        if self._allow is None and self._deny is None:
            return True
        commands = self._extract_commands(command)
        return all(self._check_single(cmd) for cmd in commands)

    def _check_single(self, cmd: str) -> bool:
        """Check a single command name against policy.

        Allowlist mode: command must be in allow set.
        Denylist mode: command must not be in deny set.
        """
        if self._allow is not None:
            return cmd in self._allow
        if self._deny is not None:
            return cmd not in self._deny
        return True

    @staticmethod
    def _extract_commands(command: str) -> list[str]:
        """Extract all command names from a potentially chained command string.

        Handles: |, &&, ||, ;, newlines, $(), backticks, env-var prefixes.
        Returns the first actual command word of each sub-command.
        """
        flat = command
        # Expand $(...) subshells into the command chain
        flat = re.sub(r'\$\([^)]*\)', lambda m: '; ' + m.group()[2:-1] + ' ;', flat)
        # Expand backtick subshells
        flat = re.sub(r'`[^`]*`', lambda m: '; ' + m.group()[1:-1] + ' ;', flat)
        # Split on pipe/chain operators AND newlines
        parts = re.split(r'\|{1,2}|&&|;|\n', flat)
        commands = []
        for part in parts:
            tokens = part.strip().split()
            # Skip env-var assignments (FOO=bar cmd ...) to find the real command
            while tokens and re.fullmatch(r'[A-Za-z_]\w*=\S*', tokens[0]):
                tokens = tokens[1:]
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
            return {"status": "error", "message": "command is required"}

        # Check policy
        if not self._policy.is_allowed(command):
            denied = BashPolicy._extract_commands(command)
            return {
                "status": "error",
                "message": f"Command not allowed by policy. "
                f"Denied command(s): {', '.join(denied)}",
            }

        timeout = args.get("timeout", 30)
        cwd = args.get("working_dir", self._working_dir)

        # Validate working_dir is under the agent's working directory
        try:
            resolved = str(Path(cwd).resolve())
            sandbox = str(Path(self._working_dir).resolve())
            if not (resolved == sandbox or resolved.startswith(sandbox + "/")):
                return {
                    "status": "error",
                    "message": f"working_dir must be under agent working directory: "
                    f"{self._working_dir}",
                }
        except (ValueError, OSError):
            return {"status": "error", "message": "Invalid working_dir path"}

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
            return {"status": "error", "message": f"Command timed out after {timeout}s"}
        except Exception as e:
            return {"status": "error", "message": f"Command failed: {e}"}


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
    # Resolve policy: explicit arg or default
    resolved_policy_file = policy_file

    if yolo:
        policy = BashPolicy.yolo()
    elif resolved_policy_file is not None:
        policy = BashPolicy.from_file(resolved_policy_file)
    else:
        policy = BashPolicy.from_file(str(_DEFAULT_POLICY_FILE))

    lang = agent._config.language

    mgr = BashManager(
        policy=policy,
        working_dir=str(agent._working_dir),
    )
    # Build description with policy rules
    desc = get_description(lang)
    policy_summary = policy.describe()
    if policy_summary:
        desc = f"{desc}\n\n{policy_summary}"

    agent.add_tool("bash", schema=get_schema(lang), handler=mgr.handle, description=desc)
    return mgr
