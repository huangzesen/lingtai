# Deep Refresh — Full Agent Reconstruct on `refresh`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `refresh` fully reconstruct the agent from `init.json` + `mcp/servers.json`, preserving only conversation history and working directory files.

**Architecture:** `Agent._perform_refresh()` overrides the kernel's MCP-only version. It reads `init.json` (the operator's declaration of intent) and `mcp/servers.json` (MCP tool registry), tears down all runtime state (capabilities, addons, MCP clients, tool registrations), then re-runs the full construction sequence with preserved `ChatInterface` history. The env-resolution helpers move from `cli.py` to `agent.py` so both boot and refresh share them.

**Tech Stack:** Python 3.11+, existing lingtai/lingtai-kernel infrastructure. No new dependencies.

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai/agent.py` | Modify | Add `_perform_refresh()` override, add `_read_init()` helper, import env-resolution helpers |
| `src/lingtai/cli.py` | Modify | Import helpers from `agent.py` instead of defining locally |
| `src/lingtai/config_resolve.py` | Create | Shared env-resolution helpers (`resolve_env`, `_resolve_env_fields`, `_resolve_capabilities`, `_resolve_addons`, `load_env_file`) |
| `tests/test_deep_refresh.py` | Create | Tests for the deep refresh behavior |

---

### Task 1: Extract env-resolution helpers into `config_resolve.py`

**Files:**
- Create: `src/lingtai/config_resolve.py`
- Modify: `src/lingtai/cli.py`
- Test: `tests/test_deep_refresh.py`

These functions currently live in `cli.py` but are needed by `Agent._perform_refresh()`. Move them to a shared module.

- [ ] **Step 1: Write the failing test**

```python
# tests/test_deep_refresh.py
"""Tests for deep refresh (full agent reconstruct from init.json)."""
from __future__ import annotations

import os


def test_resolve_env_fields_resolves_env_var(monkeypatch):
    """_resolve_env_fields replaces *_env keys with env var values."""
    from lingtai.config_resolve import _resolve_env_fields

    monkeypatch.setenv("TEST_SECRET", "hunter2")
    result = _resolve_env_fields({"api_key": None, "api_key_env": "TEST_SECRET"})
    assert result == {"api_key": "hunter2"}
    assert "api_key_env" not in result


def test_resolve_capabilities_resolves_env():
    """_resolve_capabilities applies _resolve_env_fields to each capability."""
    from lingtai.config_resolve import _resolve_capabilities

    caps = {"bash": {"policy_file": "p.json"}, "vision": {}}
    result = _resolve_capabilities(caps)
    assert result == {"bash": {"policy_file": "p.json"}, "vision": {}}


def test_resolve_addons_none():
    """_resolve_addons returns None for None/empty input."""
    from lingtai.config_resolve import _resolve_addons

    assert _resolve_addons(None) is None
    assert _resolve_addons({}) is None
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_deep_refresh.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'lingtai.config_resolve'`

- [ ] **Step 3: Create `config_resolve.py` with the helpers**

Move these functions from `cli.py`:
- `resolve_env(value, env_name)`
- `load_env_file(path)`
- `_resolve_env_fields(d)`
- `_resolve_capabilities(capabilities)`
- `_resolve_addons(addons)`

```python
# src/lingtai/config_resolve.py
"""Shared config resolution helpers — env vars, capabilities, addons."""
from __future__ import annotations

import os
from pathlib import Path


def resolve_env(value: str | None, env_name: str | None) -> str | None:
    """Resolve a value from env var name, falling back to raw value."""
    if env_name:
        env_val = os.environ.get(env_name)
        if env_val:
            return env_val
    return value


def load_env_file(path: str | Path) -> None:
    """Load a .env file into os.environ. Existing vars are not overwritten."""
    env_path = Path(path).expanduser()
    if not env_path.is_file():
        return
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, _, val = line.partition("=")
        if not _:
            continue
        key = key.strip()
        val = val.strip().strip("'\"")
        if key not in os.environ:
            os.environ[key] = val


def _resolve_env_fields(d: dict) -> dict:
    """Resolve ``*_env`` keys in a dict using ``resolve_env``."""
    result = dict(d)
    env_keys = [k for k in result if k.endswith("_env")]
    for env_key in env_keys:
        base_key = env_key[: -len("_env")]
        result[base_key] = resolve_env(result.get(base_key), result.pop(env_key))
    return result


def _resolve_capabilities(capabilities: dict) -> dict:
    """Resolve ``*_env`` fields in each capability's kwargs."""
    resolved = {}
    for name, kwargs in capabilities.items():
        if isinstance(kwargs, dict) and kwargs:
            resolved[name] = _resolve_env_fields(kwargs)
        else:
            resolved[name] = kwargs
    return resolved


def _resolve_addons(addons: dict | None) -> dict | None:
    """Resolve *_env fields in addon configs to actual values."""
    if not addons:
        return addons
    resolved = {}
    for name, cfg in addons.items():
        if isinstance(cfg, dict):
            resolved[name] = _resolve_env_fields(cfg)
    return resolved or None
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_deep_refresh.py -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Update `cli.py` to import from `config_resolve`**

Replace the local function definitions with imports:

```python
# In cli.py, replace the local definitions with:
from lingtai.config_resolve import (
    resolve_env,
    load_env_file,
    _resolve_env_fields,
    _resolve_capabilities,
    _resolve_addons,
)
```

Remove the 5 function bodies from `cli.py` (`resolve_env`, `load_env_file`, `_resolve_env_fields`, `_resolve_capabilities`, `_resolve_addons`).

- [ ] **Step 6: Smoke-test both modules**

Run: `python -c "import lingtai.config_resolve; import lingtai.cli; print('ok')"`
Expected: `ok`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/config_resolve.py src/lingtai/cli.py tests/test_deep_refresh.py
git commit -m "refactor: extract env-resolution helpers into config_resolve.py"
```

---

### Task 2: Implement `Agent._perform_refresh()` — full reconstruct

**Files:**
- Modify: `src/lingtai/agent.py`
- Test: `tests/test_deep_refresh.py`

The core change. `Agent._perform_refresh()` overrides the kernel version to do a full reconstruct from `init.json`.

- [ ] **Step 1: Write the failing test**

Append to `tests/test_deep_refresh.py`:

```python
import json
from pathlib import Path
from unittest.mock import MagicMock, patch


def _make_init(
    capabilities: dict | None = None,
    addons: dict | None = None,
    provider: str = "openai",
    model: str = "gpt-4o",
) -> dict:
    """Build a minimal valid init.json dict."""
    data = {
        "manifest": {
            "agent_name": "test-agent",
            "language": "en",
            "llm": {
                "provider": provider,
                "model": model,
                "api_key": "test-key",
                "base_url": None,
            },
            "capabilities": capabilities or {},
            "soul": {"delay": 60},
            "stamina": 3600,
            "context_limit": None,
            "molt_pressure": 0.8,
            "molt_prompt": "",
            "max_turns": 100,
            "admin": {"karma": True},
            "streaming": False,
        },
        "principle": "",
        "covenant": "",
        "memory": "",
        "prompt": "",
    }
    if addons:
        data["addons"] = addons
    return data


def _make_agent(tmp_path: Path, init_data: dict | None = None):
    """Create an Agent with a mock LLM service in a temp working dir."""
    from lingtai.agent import Agent
    from lingtai_kernel.config import AgentConfig

    # Write init.json
    init = init_data or _make_init()
    (tmp_path / "init.json").write_text(json.dumps(init))

    service = MagicMock()
    service.provider = "openai"
    service.model = "gpt-4o"
    service._base_url = None

    agent = Agent(
        service,
        agent_name="test-agent",
        working_dir=tmp_path,
        config=AgentConfig(),
        capabilities=init["manifest"]["capabilities"] or None,
    )
    return agent


def test_deep_refresh_loads_new_capability(tmp_path):
    """After editing init.json to add a capability, refresh picks it up."""
    agent = _make_agent(tmp_path, _make_init(capabilities={}))

    # Simulate start (seals tools)
    agent._sealed = True

    # Fake a ChatInterface for history preservation
    mock_interface = MagicMock()
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = mock_interface
    agent._session = mock_session

    # Update init.json to add "read" capability
    new_init = _make_init(capabilities={"read": {}})
    (tmp_path / "init.json").write_text(json.dumps(new_init))

    # Perform refresh
    agent._perform_refresh()

    # "read" capability should now be registered
    cap_names = [name for name, _ in agent._capabilities]
    assert "read" in cap_names
    assert agent._sealed is True  # re-sealed after refresh


def test_deep_refresh_no_init_json_is_noop(tmp_path):
    """If init.json is missing, refresh is a no-op (no crash)."""
    agent = _make_agent(tmp_path)
    (tmp_path / "init.json").unlink()

    agent._sealed = True
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = MagicMock()
    agent._session = mock_session

    old_caps = list(agent._capabilities)
    agent._perform_refresh()
    # Nothing changed
    assert agent._capabilities == old_caps
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_deep_refresh.py::test_deep_refresh_loads_new_capability tests/test_deep_refresh.py::test_deep_refresh_no_init_json_is_noop -v`
Expected: FAIL — `_perform_refresh` is the kernel's MCP-only version, doesn't load capabilities.

- [ ] **Step 3: Implement `_read_init()` helper on Agent**

Add to `agent.py`:

```python
def _read_init(self) -> dict | None:
    """Read and validate init.json from working directory.

    Returns the parsed dict, or None if missing/invalid.
    """
    import json
    from .init_schema import validate_init

    init_path = self._working_dir / "init.json"
    if not init_path.is_file():
        return None

    try:
        data = json.loads(init_path.read_text())
    except (json.JSONDecodeError, OSError):
        self._log("refresh_init_error", error="failed to read init.json")
        return None

    try:
        validate_init(data)
    except ValueError as e:
        self._log("refresh_init_error", error=str(e))
        return None

    return data
```

- [ ] **Step 4: Implement `_perform_refresh()` override on Agent**

Add to `agent.py`, overriding the kernel version:

```python
def _perform_refresh(self) -> None:
    """Full reconstruct from init.json, preserving conversation history."""
    self._log("refresh_start")

    # --- Read config ---
    data = self._read_init()
    if data is None:
        self._log("refresh_skipped", reason="no valid init.json")
        return

    from .config_resolve import (
        load_env_file,
        resolve_env,
        _resolve_capabilities,
        _resolve_addons,
    )
    from lingtai_kernel.config import AgentConfig

    # Load env file if specified
    env_file = data.get("env_file")
    if env_file:
        load_env_file(env_file)

    m = data["manifest"]

    # --- Save conversation history ---
    saved_interface = None
    if self._session.chat is not None:
        saved_interface = self._session.chat.interface

    # --- Tear down ---
    # Stop addon managers
    for name, mgr in self._addon_managers.items():
        if hasattr(mgr, "stop"):
            try:
                mgr.stop()
            except Exception:
                pass

    # Close MCP clients
    for client in getattr(self, "_mcp_clients", []):
        try:
            client.close()
        except Exception:
            pass
    self._mcp_clients = []

    # Unseal
    self._sealed = False

    # Clear all non-intrinsic tool registrations
    self._mcp_handlers.clear()
    self._mcp_schemas.clear()

    # Clear capability and addon tracking
    self._capabilities.clear()
    self._capability_managers.clear()
    self._addon_managers.clear()

    # Re-wire intrinsics (reset any overrides from capabilities like email/psyche)
    self._intrinsics.clear()
    self._wire_intrinsics()

    # --- Reconstruct LLM service if changed ---
    llm = m["llm"]
    api_key = resolve_env(llm["api_key"], llm.get("api_key_env"))
    new_provider = llm["provider"]
    new_model = llm["model"]
    new_base_url = llm["base_url"]

    if (
        new_provider != self.service.provider
        or new_model != self.service.model
        or new_base_url != getattr(self.service, "_base_url", None)
    ):
        from .llm.service import LLMService

        self.service = LLMService(
            provider=new_provider,
            model=new_model,
            api_key=api_key,
            base_url=new_base_url,
        )
        self._session._llm_service = self.service

    # --- Reload config ---
    soul = m["soul"]
    self._config = AgentConfig(
        stamina=m["stamina"],
        soul_delay=soul["delay"],
        max_turns=m["max_turns"],
        language=m["language"],
        context_limit=m["context_limit"],
        molt_pressure=m["molt_pressure"],
        molt_prompt=m["molt_prompt"],
    )
    self._soul_delay = max(1.0, self._config.soul_delay)

    # --- Reload covenant and memory into prompt manager ---
    covenant = data.get("covenant", "")
    system_dir = self._working_dir / "system"
    covenant_file = system_dir / "covenant.md"
    memory_file = system_dir / "memory.md"

    if not covenant and covenant_file.is_file():
        covenant = covenant_file.read_text()
    if covenant:
        self._prompt_manager.write_section("covenant", covenant, protected=True)

    loaded_memory = ""
    if memory_file.is_file():
        loaded_memory = memory_file.read_text()
    if loaded_memory.strip():
        self._prompt_manager.write_section("memory", loaded_memory)

    # --- Reload principle ---
    principle = data.get("principle", "")
    if principle:
        self._prompt_manager.write_section("principle", principle, protected=True)

    # --- Re-run capability setup ---
    capabilities = _resolve_capabilities(m["capabilities"])
    if capabilities:
        from .capabilities import expand_groups, _GROUPS

        # Expand groups
        expanded: dict[str, dict] = {}
        for name, cap_kwargs in capabilities.items():
            if name in _GROUPS:
                for sub in _GROUPS[name]:
                    expanded[sub] = {}
            else:
                expanded[name] = cap_kwargs
        capabilities = expanded

        for name, cap_kwargs in capabilities.items():
            self._setup_capability(name, **cap_kwargs)

    # --- Re-run addon setup ---
    addons = _resolve_addons(data.get("addons"))
    if addons:
        from .addons import setup_addon

        for addon_name, addon_kwargs in addons.items():
            mgr = setup_addon(self, addon_name, **(addon_kwargs or {}))
            self._addon_managers[addon_name] = mgr

    # --- Reload MCP from mcp/servers.json ---
    self._load_mcp_from_workdir()

    # --- Persist LLM config ---
    try:
        import json as _json

        llm_config: dict = {
            "provider": self.service.provider,
            "model": self.service.model,
        }
        _base_url = getattr(self.service, "_base_url", None)
        if isinstance(_base_url, str) and _base_url:
            llm_config["base_url"] = _base_url
        llm_dir = self._working_dir / "system"
        llm_dir.mkdir(exist_ok=True)
        (llm_dir / "llm.json").write_text(
            _json.dumps(llm_config, ensure_ascii=False)
        )
    except (TypeError, AttributeError, OSError):
        pass

    # --- Re-write manifest ---
    self._workdir.write_manifest(self._build_manifest())

    # --- Re-seal ---
    self._sealed = True

    # --- Rebuild session with preserved history ---
    if saved_interface is not None:
        self._session._rebuild_session(saved_interface)
    # If no session existed, ensure_session() will create one on next message

    # --- Start addon managers ---
    for name, mgr in self._addon_managers.items():
        if hasattr(mgr, "start"):
            mgr.start()

    self._log(
        "refresh_complete",
        capabilities=[name for name, _ in self._capabilities],
        addons=list(self._addon_managers.keys()),
        tools=list(self._mcp_handlers.keys()),
    )
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `python -m pytest tests/test_deep_refresh.py -v`
Expected: PASS (all 5 tests)

- [ ] **Step 6: Smoke-test the module**

Run: `python -c "import lingtai.agent; print('ok')"`
Expected: `ok`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/agent.py tests/test_deep_refresh.py
git commit -m "feat: deep refresh — full agent reconstruct from init.json"
```

---

### Task 3: Additional test coverage

**Files:**
- Test: `tests/test_deep_refresh.py`

Cover edge cases: invalid init.json, LLM provider change, capability removal, addon stop/start cycle.

- [ ] **Step 1: Write test for invalid init.json (no crash, keeps old config)**

```python
def test_deep_refresh_invalid_init_keeps_old_config(tmp_path):
    """If init.json is invalid, refresh logs error and keeps old state."""
    agent = _make_agent(tmp_path, _make_init(capabilities={"read": {}}))
    agent._sealed = True
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = MagicMock()
    agent._session = mock_session

    # Write invalid init.json
    (tmp_path / "init.json").write_text("not json")

    old_caps = list(agent._capabilities)
    agent._perform_refresh()

    # Old capabilities preserved (refresh was a no-op)
    assert agent._capabilities == old_caps
```

- [ ] **Step 2: Write test for capability removal on refresh**

```python
def test_deep_refresh_removes_old_capabilities(tmp_path):
    """Capabilities removed from init.json are gone after refresh."""
    agent = _make_agent(tmp_path, _make_init(capabilities={"read": {}, "write": {}}))
    agent._sealed = True
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = MagicMock()
    agent._session = mock_session

    assert len(agent._capabilities) == 2

    # Remove "write" from init.json
    new_init = _make_init(capabilities={"read": {}})
    (tmp_path / "init.json").write_text(json.dumps(new_init))

    agent._perform_refresh()

    cap_names = [name for name, _ in agent._capabilities]
    assert "read" in cap_names
    assert "write" not in cap_names
```

- [ ] **Step 3: Write test for conversation history preservation**

```python
def test_deep_refresh_preserves_chat_history(tmp_path):
    """ChatInterface is passed through to _rebuild_session after refresh."""
    agent = _make_agent(tmp_path, _make_init())
    agent._sealed = True

    mock_interface = MagicMock()
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = mock_interface
    agent._session = mock_session

    agent._perform_refresh()

    mock_session._rebuild_session.assert_called_once_with(mock_interface)
```

- [ ] **Step 4: Write test for re-seal after refresh**

```python
def test_deep_refresh_reseals(tmp_path):
    """Tool surface is re-sealed after refresh completes."""
    agent = _make_agent(tmp_path, _make_init())
    agent._sealed = True
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = MagicMock()
    agent._session = mock_session

    agent._perform_refresh()

    assert agent._sealed is True
```

- [ ] **Step 5: Run all tests**

Run: `python -m pytest tests/test_deep_refresh.py -v`
Expected: PASS (all tests)

- [ ] **Step 6: Commit**

```bash
git add tests/test_deep_refresh.py
git commit -m "test: edge cases for deep refresh — invalid json, removal, history, re-seal"
```

---

### Task 4: Final verification

- [ ] **Step 1: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All tests pass, including existing tests (no regressions from cli.py refactor).

- [ ] **Step 2: Smoke-test both entry points**

Run: `python -c "import lingtai; import lingtai.cli; import lingtai.config_resolve; print('ok')"`
Expected: `ok`

- [ ] **Step 3: Commit if any fixups needed**

Only if previous steps revealed issues.
