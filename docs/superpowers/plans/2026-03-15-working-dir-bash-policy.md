# Mandatory working_dir + Bash Policy Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `working_dir` a mandatory BaseAgent parameter (no implicit `Path.cwd()`) and replace the bash capability's flat `allowed_commands` with a file-based policy system (allow/deny lists with pipe awareness).

**Architecture:** Two independent changes. First, `working_dir` becomes a required keyword on `BaseAgent.__init__` — all callers must be explicit. Second, the bash capability replaces `allowed_commands` with `policy_file` (mandatory) or `yolo=True` (no restrictions). Policy files are JSON with `allow`/`deny` lists. The command checker parses pipes/chains to validate every command in the chain.

**Tech Stack:** Python 3.11+, JSON, shlex, subprocess, pytest

---

## File Structure

### Modified files
| File | What changes |
|------|-------------|
| `src/lingtai/agent.py:181,203` | `working_dir` becomes `str \| Path` (no `None` default), remove `Path.cwd()` fallback |
| `src/lingtai/capabilities/bash.py` | Replace `allowed_commands` with `policy_file`/`yolo`, add `BashPolicy` class, pipe-aware command checking |
| `tests/test_agent.py` | Add `working_dir="/tmp"` to all ~27 `BaseAgent(...)` calls, update `test_working_dir_default` |
| `tests/test_layers_bash.py` | Rewrite tests for new policy system |
| `tests/test_layers_email.py` | Add `working_dir="/tmp"` to all ~15 `BaseAgent(...)` calls |
| `tests/test_layers_delegate.py` | Add `working_dir="/tmp"` to all ~3 `BaseAgent(...)` calls |
| `tests/test_services_logging.py` | Add `working_dir="/tmp"` to `BaseAgent(...)` calls |
| `examples/two_agents.py` | Add explicit `working_dir=` |
| `examples/chat_agent.py` | Add explicit `working_dir=` |
| `examples/chat_web.py` | Add explicit `working_dir=` |
| `CLAUDE.md` | Update bash capability docs |

### New files
| File | Responsibility |
|------|---------------|
| `examples/bash_policy.json` | Example policy file with reasonable defaults |

---

## Chunk 1: Make working_dir mandatory

### Task 1: Make working_dir mandatory on BaseAgent

**Files:**
- Modify: `src/lingtai/agent.py:181,203`
- Modify: `tests/test_agent.py` (~27 call sites)
- Modify: `tests/test_layers_email.py` (~15 call sites)
- Modify: `tests/test_layers_delegate.py` (~3 call sites)
- Modify: `tests/test_layers_bash.py` (~1 call site)
- Modify: `tests/test_services_logging.py` (~3 call sites)
- Modify: `examples/two_agents.py`
- Modify: `examples/chat_agent.py`
- Modify: `examples/chat_web.py`

- [ ] **Step 1: Update BaseAgent.__init__ signature**

In `src/lingtai/agent.py`, change line 181:
```python
working_dir: str | Path | None = None,
```
to:
```python
working_dir: str | Path,
```

Change line 203:
```python
self._working_dir = Path(working_dir) if working_dir else Path.cwd()
```
to:
```python
self._working_dir = Path(working_dir)
```

- [ ] **Step 2: Update tests/test_agent.py**

Add `working_dir="/tmp"` to every `BaseAgent(...)` call that doesn't already have it. The one test that has `working_dir="/tmp/test"` already (`test_working_dir_resolved`) is fine.

Replace `test_working_dir_default` with a test that verifies `working_dir` is required:
```python
def test_working_dir_required():
    """working_dir must be explicitly provided."""
    with pytest.raises(TypeError):
        BaseAgent(agent_id="test", service=make_mock_service())
```

- [ ] **Step 3: Update tests/test_layers_email.py**

Add `working_dir="/tmp"` to every `BaseAgent(...)` call.

- [ ] **Step 4: Update tests/test_layers_delegate.py**

Add `working_dir="/tmp"` to every `BaseAgent(...)` call.

- [ ] **Step 5: Update tests/test_layers_bash.py**

Add `working_dir="/tmp"` to the `BaseAgent(...)` call in `TestAddCapability`.

- [ ] **Step 6: Update tests/test_services_logging.py**

Add `working_dir="/tmp"` to every `BaseAgent(...)` call.

- [ ] **Step 7: Update examples**

In `examples/two_agents.py`, add `working_dir="."` to both `BaseAgent(...)` calls.
In `examples/chat_agent.py`, add `working_dir="."` to the `BaseAgent(...)` call.
In `examples/chat_web.py`, add `working_dir="."` to the `BaseAgent(...)` call.

- [ ] **Step 8: Smoke-test and run tests**

Run: `source venv/bin/activate && python -c "import lingtai" && python -m pytest tests/ -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add src/lingtai/agent.py tests/ examples/
git commit -m "refactor: make working_dir a mandatory BaseAgent parameter"
```

---

## Chunk 2: Bash policy system

### Task 2: Create BashPolicy and rewrite bash capability

**Files:**
- Modify: `src/lingtai/capabilities/bash.py`
- Modify: `tests/test_layers_bash.py`
- Create: `examples/bash_policy.json`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Write tests for BashPolicy**

Add to `tests/test_layers_bash.py`:

```python
import json

class TestBashPolicy:
    def test_load_from_file(self, tmp_path):
        """Policy should load allow/deny from JSON file."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["git", "ls"], "deny": ["rm"]}))
        from lingtai.capabilities.bash import BashPolicy
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("git status")
        assert policy.is_allowed("ls -la")
        assert not policy.is_allowed("rm -rf /")

    def test_allow_only(self, tmp_path):
        """With only allow list, unlisted commands are denied."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["git", "echo"]}))
        from lingtai.capabilities.bash import BashPolicy
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("git push")
        assert not policy.is_allowed("curl http://evil.com")

    def test_deny_only(self, tmp_path):
        """With only deny list, unlisted commands are allowed."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"deny": ["rm", "sudo"]}))
        from lingtai.capabilities.bash import BashPolicy
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("ls -la")
        assert not policy.is_allowed("rm file.txt")
        assert not policy.is_allowed("sudo apt install")

    def test_allow_and_deny(self, tmp_path):
        """Must be in allow AND not in deny."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["git", "rm"], "deny": ["rm"]}))
        from lingtai.capabilities.bash import BashPolicy
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("git status")
        assert not policy.is_allowed("rm file")  # in allow but also in deny

    def test_pipe_awareness(self, tmp_path):
        """Should check all commands in a pipe chain."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"deny": ["rm"]}))
        from lingtai.capabilities.bash import BashPolicy
        policy = BashPolicy.from_file(str(policy_file))
        assert not policy.is_allowed("ls | rm -rf /")
        assert not policy.is_allowed("echo hello && rm file")
        assert not policy.is_allowed("echo hello; rm file")
        assert policy.is_allowed("ls | grep foo | sort")

    def test_subshell_awareness(self, tmp_path):
        """Should check commands inside $() and backticks."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"deny": ["rm"]}))
        from lingtai.capabilities.bash import BashPolicy
        policy = BashPolicy.from_file(str(policy_file))
        assert not policy.is_allowed("echo $(rm file)")

    def test_yolo_allows_everything(self):
        """Yolo policy should allow all commands."""
        from lingtai.capabilities.bash import BashPolicy
        policy = BashPolicy.yolo()
        assert policy.is_allowed("rm -rf /")
        assert policy.is_allowed("sudo shutdown -h now")

    def test_missing_file_raises(self):
        """Loading from nonexistent file should raise."""
        from lingtai.capabilities.bash import BashPolicy
        import pytest
        with pytest.raises(FileNotFoundError):
            BashPolicy.from_file("/nonexistent/policy.json")

    def test_empty_policy_file(self, tmp_path):
        """Empty policy (no allow, no deny) should allow everything."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({}))
        from lingtai.capabilities.bash import BashPolicy
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("anything")
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `source venv/bin/activate && python -m pytest tests/test_layers_bash.py::TestBashPolicy -v`
Expected: FAIL (BashPolicy doesn't exist yet)

- [ ] **Step 3: Implement BashPolicy class**

In `src/lingtai/capabilities/bash.py`, add:

```python
import json
import re
from pathlib import Path

class BashPolicy:
    """Command execution policy — allow/deny lists with pipe awareness."""

    def __init__(self, allow: list[str] | None = None, deny: list[str] | None = None):
        self._allow = set(allow) if allow else None
        self._deny = set(deny) if deny else None

    @classmethod
    def from_file(cls, path: str) -> "BashPolicy":
        """Load policy from a JSON file."""
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
        """Check if a command string is allowed by this policy."""
        if self._allow is None and self._deny is None:
            return True
        commands = self._extract_commands(command)
        for cmd in commands:
            if not self._check_single(cmd):
                return False
        return True

    def _check_single(self, cmd: str) -> bool:
        """Check a single command (no pipes/chains) against policy."""
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
        # Replace subshell constructs with separators
        flat = command
        # Handle $(...) — replace with separator
        flat = re.sub(r'\$\([^)]*\)', lambda m: '; ' + m.group()[2:-1] + ' ;', flat)
        # Handle backticks — replace with separator
        flat = re.sub(r'`[^`]*`', lambda m: '; ' + m.group()[1:-1] + ' ;', flat)
        # Split on pipe/chain operators
        parts = re.split(r'\|{1,2}|&&|;', flat)
        commands = []
        for part in parts:
            tokens = part.strip().split()
            if tokens:
                commands.append(tokens[0])
        return commands
```

- [ ] **Step 4: Run BashPolicy tests**

Run: `source venv/bin/activate && python -m pytest tests/test_layers_bash.py::TestBashPolicy -v`
Expected: All PASS

- [ ] **Step 5: Rewrite BashManager and setup() to use BashPolicy**

Replace `BashManager.__init__` and `handle()` allowlist check:

```python
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
```

Replace `setup()`:

```python
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
```

- [ ] **Step 6: Rewrite existing BashManager tests**

Replace the existing `TestBashManager` and setup tests in `tests/test_layers_bash.py`:

```python
class TestBashManager:
    def test_echo(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp")
        result = mgr.handle({"command": "echo hello"})
        assert result["status"] == "ok"
        assert result["exit_code"] == 0
        assert "hello" in result["stdout"]

    def test_nonexistent_command(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp")
        result = mgr.handle({"command": "definitely_not_a_real_command_xyz"})
        assert result["status"] == "ok"
        assert result["exit_code"] != 0

    def test_empty_command(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp")
        result = mgr.handle({"command": ""})
        assert "error" in result

    def test_timeout(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp")
        result = mgr.handle({"command": "sleep 10", "timeout": 0.5})
        assert "error" in result
        assert "timed out" in result["error"]

    def test_policy_denies(self, tmp_path):
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"deny": ["rm"]}))
        policy = BashPolicy.from_file(str(policy_file))
        mgr = BashManager(policy=policy, working_dir="/tmp")
        result = mgr.handle({"command": "rm -rf /"})
        assert "error" in result
        assert "not allowed" in result["error"]

    def test_policy_allows(self, tmp_path):
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["echo", "ls"]}))
        policy = BashPolicy.from_file(str(policy_file))
        mgr = BashManager(policy=policy, working_dir="/tmp")
        result = mgr.handle({"command": "echo ok"})
        assert result["status"] == "ok"

    def test_working_dir(self, tmp_path):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir=str(tmp_path))
        result = mgr.handle({"command": "pwd"})
        assert result["status"] == "ok"
        assert str(tmp_path) in result["stdout"]

    def test_output_truncation(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp", max_output=20)
        result = mgr.handle({"command": "echo 'a very long output string that exceeds the limit'"})
        assert "truncated" in result["stdout"]


class TestSetupBash:
    def test_setup_with_policy_file(self, tmp_path):
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["echo"]}))
        agent = MagicMock()
        agent._working_dir = Path("/tmp")
        mgr = setup_bash(agent, policy_file=str(policy_file))
        assert isinstance(mgr, BashManager)
        agent.add_tool.assert_called_once()

    def test_setup_yolo(self):
        agent = MagicMock()
        agent._working_dir = Path("/tmp")
        mgr = setup_bash(agent, yolo=True)
        assert isinstance(mgr, BashManager)
        agent.add_tool.assert_called_once()

    def test_setup_requires_policy_or_yolo(self):
        agent = MagicMock()
        agent._working_dir = Path("/tmp")
        with pytest.raises(ValueError, match="policy_file"):
            setup_bash(agent)


class TestAddCapability:
    def test_add_capability_bash_yolo(self):
        from lingtai.agent import BaseAgent
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = BaseAgent(agent_id="test", service=svc, working_dir="/tmp")
        mgr = agent.add_capability("bash", yolo=True)
        assert isinstance(mgr, BashManager)
        assert "bash" in agent._mcp_handlers

    def test_add_capability_bash_with_policy(self, tmp_path):
        from lingtai.agent import BaseAgent
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["echo"]}))
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = BaseAgent(agent_id="test", service=svc, working_dir="/tmp")
        mgr = agent.add_capability("bash", policy_file=str(policy_file))
        assert isinstance(mgr, BashManager)
```

- [ ] **Step 7: Run all bash tests**

Run: `source venv/bin/activate && python -m pytest tests/test_layers_bash.py -v`
Expected: All PASS

- [ ] **Step 8: Create example policy file**

Create `examples/bash_policy.json`:
```json
{
    "allow": [
        "git", "python", "python3", "node", "npm", "npx",
        "ls", "cat", "head", "tail", "less", "more",
        "find", "grep", "rg", "ag", "awk", "sed",
        "wc", "sort", "uniq", "diff", "tr", "cut",
        "echo", "printf", "pwd", "cd", "pushd", "popd",
        "mkdir", "cp", "mv", "ln", "touch",
        "date", "env", "which", "whoami", "uname", "hostname",
        "tar", "gzip", "gunzip", "zip", "unzip",
        "tee", "xargs", "true", "false", "test"
    ],
    "deny": [
        "rm", "rmdir", "shred", "dd",
        "sudo", "su", "doas",
        "chmod", "chown", "chgrp",
        "mount", "umount", "mkfs", "fdisk",
        "apt", "apt-get", "yum", "dnf", "brew", "pip", "pip3",
        "kill", "killall", "pkill", "shutdown", "reboot", "systemctl",
        "curl", "wget", "nc", "ncat",
        "eval", "exec"
    ]
}
```

- [ ] **Step 9: Update CLAUDE.md**

Update the capabilities description to mention the new bash policy system:
- Capabilities section: update bash description
- Extension pattern: update `add_capability("bash")` example

- [ ] **Step 10: Run full test suite**

Run: `source venv/bin/activate && python -c "import lingtai" && python -m pytest tests/ -v`
Expected: All PASS

- [ ] **Step 11: Commit**

```bash
git add src/lingtai/capabilities/bash.py tests/test_layers_bash.py examples/bash_policy.json CLAUDE.md
git commit -m "feat: bash policy system — file-based allow/deny with pipe awareness"
```
