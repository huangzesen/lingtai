# WorkingDir by agent_id — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decouple agent_name from the filesystem — working directories use agent_id, agent_name becomes a set-once true name (真名).

**Architecture:** WorkingDir takes agent_id instead of agent_name. BaseAgent generates agent_id before WorkingDir construction, makes agent_name optional (default None). Eigen intrinsic gets a `name` action for self-naming. Changes span lingtai-kernel (primary) and lingtai (tests + i18n).

**Tech Stack:** Python 3.11+, pytest, lingtai-kernel

**Note on eigen schema:** The spec describes the name action as `{"action": "name", "name": "<string>"}`. This plan uses `{"object": "name", "action": "set", "content": "<name>"}` instead, to stay consistent with eigen's existing `object`/`action`/`content` pattern. The spec should be updated to match.

---

**File structure:**

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `lingtai-kernel/src/lingtai_kernel/workdir.py` | Takes agent_id, validates hex, path = base_dir/agent_id |
| Modify | `lingtai-kernel/src/lingtai_kernel/base_agent.py` | agent_name optional, agent_id before WorkingDir, set_name(), all thread names use agent_id |
| Modify | `lingtai-kernel/src/lingtai_kernel/intrinsics/eigen.py` | New `name` action |
| Modify | `lingtai-kernel/src/lingtai_kernel/session.py` | Accept agent_name=None, fallback to agent_id for logging |
| Modify | `lingtai-kernel/src/lingtai_kernel/i18n/en.json` | Eigen name action i18n strings |
| Modify | `lingtai-kernel/src/lingtai_kernel/i18n/zh.json` | Eigen name action i18n strings |
| Modify | `lingtai/src/lingtai/i18n/wen.json` | Eigen name action i18n strings (文言) |
| Audit | `lingtai/src/lingtai/agent.py` | Verify `*args, **kwargs` passthrough still works after BaseAgent signature change |
| No change | `lingtai/src/lingtai/capabilities/avatar.py` | Already uses `agent_name=` as keyword — works as-is. Future: parent-given names |
| Modify | `lingtai-kernel/tests/test_workdir.py` | Update all tests to use agent_id |
| Modify | `lingtai/tests/test_workdir.py` | Update all tests to use agent_id (lingtai has its own copy) |
| Modify | `lingtai-kernel/tests/test_session.py` | Accept agent_name=None |
| Modify | `lingtai-kernel/tests/test_heartbeat.py` | Constructor signature change |
| Modify | `lingtai-kernel/tests/test_soul.py` | Constructor signature change |
| Modify | `lingtai-kernel/tests/test_services_logging.py` | Constructor signature change |
| Modify | `lingtai/tests/test_agent.py` | Constructor signature change + path assertions |
| Modify | `lingtai/tests/test_system.py` | Constructor signature + path assertions |
| Modify | `lingtai/tests/test_eigen.py` | Constructor signature + new name tests |
| Modify | `lingtai/tests/test_*.py` (all others) | Constructor signature change (agent_name→keyword) |

---

### Task 1: WorkingDir takes agent_id

**Files:**
- Modify: `lingtai-kernel/src/lingtai_kernel/workdir.py:30-46`
- Modify: `lingtai-kernel/tests/test_workdir.py`

- [ ] **Step 1: Update WorkingDir tests to use agent_id**

Change all `WorkingDir(base_dir=tmp_path, agent_name="alice")` to `WorkingDir(base_dir=tmp_path, agent_id="a1b2c3d4e5f6")`. Update `test_invalid_agent_name_raises` to `test_invalid_agent_id_raises` — test non-hex and wrong-length strings. Update `test_init_creates_agent_dir` assertion from `tmp_path / "alice"` to `tmp_path / "a1b2c3d4e5f6"`.

```python
# test_workdir.py — replace all WorkingDir calls:
_TEST_ID = "a1b2c3d4e5f6"

def test_init_creates_agent_dir(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id=_TEST_ID)
    assert wd.path == tmp_path / _TEST_ID
    assert wd.path.is_dir()

def test_lock_prevents_second_instance(tmp_path):
    wd1 = WorkingDir(base_dir=tmp_path, agent_id=_TEST_ID)
    wd1.acquire_lock()
    try:
        wd2 = WorkingDir(base_dir=tmp_path, agent_id=_TEST_ID)
        with pytest.raises(RuntimeError, match="already in use"):
            wd2.acquire_lock()
    finally:
        wd1.release_lock()

def test_lock_release_allows_reuse(tmp_path):
    wd1 = WorkingDir(base_dir=tmp_path, agent_id=_TEST_ID)
    wd1.acquire_lock()
    wd1.release_lock()
    wd2 = WorkingDir(base_dir=tmp_path, agent_id=_TEST_ID)
    wd2.acquire_lock()
    wd2.release_lock()

def test_invalid_agent_id_raises(tmp_path):
    with pytest.raises(ValueError, match="agent_id must match"):
        WorkingDir(base_dir=tmp_path, agent_id="not-hex!")
    with pytest.raises(ValueError, match="agent_id must match"):
        WorkingDir(base_dir=tmp_path, agent_id="abc")  # too short
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_workdir.py -v`
Expected: FAIL — WorkingDir doesn't accept agent_id param yet

- [ ] **Step 3: Update WorkingDir implementation**

```python
# workdir.py — replace lines 30-46:
_AGENT_ID_RE = re.compile(r"^[0-9a-f]{12}$")
_LOCK_FILE = ".agent.lock"
_MANIFEST_FILE = ".agent.json"


class WorkingDir:
    """Manages an agent's working directory — locking, git, manifest."""

    def __init__(self, base_dir: Path | str, agent_id: str) -> None:
        if not _AGENT_ID_RE.match(agent_id):
            raise ValueError(
                f"agent_id must match [0-9a-f]{{12}}, got: {agent_id!r}"
            )
        self._base_dir = Path(base_dir)
        self._agent_id = agent_id
        self._path = self._base_dir / agent_id
        self._path.mkdir(exist_ok=True)
        self._lock_file: Any = None
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_workdir.py -v`
Expected: PASS

- [ ] **Step 5: Smoke test import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.workdir import WorkingDir; print('ok')"`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/workdir.py tests/test_workdir.py
git commit -m "refactor: WorkingDir takes agent_id instead of agent_name

Working directory path changes from {base_dir}/{agent_name}/ to
{base_dir}/{agent_id}/. Validates 12-char lowercase hex."
```

---

### Task 2: BaseAgent — agent_name optional, agent_id before WorkingDir

**Files:**
- Modify: `lingtai-kernel/src/lingtai_kernel/base_agent.py:76-107,234-244,260-270,387,468,527,582,636-645,1016-1033`
- Modify: `lingtai-kernel/src/lingtai_kernel/session.py:37-52`

- [ ] **Step 1: Write tests for agent_name=None and set_name()**

Add to a kernel test file. Since the kernel has no `test_agent.py` yet (those tests are in the lingtai repo), create these in the appropriate kernel test file or add a new `test_base_agent.py`:

```python
# lingtai-kernel/tests/test_base_agent.py (NEW FILE)
from __future__ import annotations
from unittest.mock import MagicMock
import pytest
from lingtai_kernel.base_agent import BaseAgent

def make_mock_service():
    svc = MagicMock()
    svc.model = "test-model"
    svc.make_tool_result.return_value = {"role": "tool", "content": "ok"}
    return svc

def test_agent_no_name(tmp_path):
    """Agent can be created without agent_name."""
    agent = BaseAgent(service=make_mock_service(), base_dir=tmp_path)
    assert agent.agent_name is None
    assert agent.agent_id  # has an id
    assert agent.working_dir == tmp_path / agent.agent_id

def test_set_name_once(tmp_path):
    """set_name works when agent_name is None."""
    agent = BaseAgent(service=make_mock_service(), base_dir=tmp_path)
    agent.set_name("悟空")
    assert agent.agent_name == "悟空"

def test_set_name_twice_fails(tmp_path):
    """set_name raises if name already set."""
    agent = BaseAgent(service=make_mock_service(), base_dir=tmp_path)
    agent.set_name("悟空")
    with pytest.raises(RuntimeError, match="already named"):
        agent.set_name("八戒")

def test_set_name_empty_fails(tmp_path):
    """set_name rejects empty string."""
    agent = BaseAgent(service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(ValueError, match="cannot be empty"):
        agent.set_name("")

def test_agent_with_name_at_construction(tmp_path):
    """agent_name can still be provided at construction."""
    agent = BaseAgent(service=make_mock_service(), base_dir=tmp_path, agent_name="alice")
    assert agent.agent_name == "alice"
    with pytest.raises(RuntimeError, match="already named"):
        agent.set_name("bob")

def test_set_name_updates_manifest(tmp_path):
    """set_name persists to .agent.json."""
    import json
    agent = BaseAgent(service=make_mock_service(), base_dir=tmp_path)
    agent.set_name("悟空")
    manifest = json.loads((agent.working_dir / ".agent.json").read_text())
    assert manifest["agent_name"] == "悟空"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_base_agent.py -v`
Expected: FAIL

- [ ] **Step 3: Update BaseAgent constructor and add set_name()**

In `base_agent.py`:

Constructor signature — `agent_name` moves from positional to keyword-only, defaults to `None`:

```python
def __init__(
    self,
    service: LLMService,
    *,
    agent_name: str | None = None,
    file_io: Any | None = None,
    mail_service: Any | None = None,
    config: AgentConfig | None = None,
    base_dir: str | Path,
    context: Any = None,
    admin: dict | None = None,
    streaming: bool = False,
    covenant: str = "",
    memory: str = "",
):
    import uuid as _uuid
    self.agent_name = agent_name
    self.agent_id = _uuid.uuid4().hex[:12]
    ...
    self._workdir = WorkingDir(base_dir=base_dir, agent_id=self.agent_id)
    ...
```

Add `set_name()` method (near the properties section, after line ~298):

```python
def set_name(self, name: str) -> None:
    """Set the agent's true name (真名). Can only be done once."""
    if not name:
        raise ValueError("Agent name cannot be empty.")
    if self.agent_name is not None:
        raise RuntimeError(
            f"Agent is already named '{self.agent_name}'. "
            "The true name can only be set once."
        )
    self.agent_name = name
    # Update manifest on disk
    self._workdir.write_manifest(self._build_manifest())
    # Update identity in system prompt
    import json as _json
    self._prompt_manager.write_section(
        "identity", _json.dumps(self._build_manifest(), indent=2), protected=True
    )
```

Update ALL thread names to use agent_id (not just the main one):
- Line ~387: `name=f"agent-{self.agent_id}"` (main loop)
- Line ~468: `name=f"kill-{self.agent_id}"` (kill handler)
- Line ~527: `name=f"soul-{self.agent_id}"` (soul/inner voice)
- Line ~582: `name=f"heartbeat-{self.agent_id}"` (heartbeat)

Update SessionManager construction — do the fallback at the call site:
```python
agent_name=agent_name,  # pass as-is; SessionManager handles None
```

Update `_log` — `agent_name` can be None in JSONL output. This is acceptable; consumers should handle null.

- [ ] **Step 4: Update SessionManager to handle None agent_name**

In `session.py` line 43, change type hint:
```python
agent_name: str | None = None,
```

And line 52 — fallback to agent_id for logging strings:
```python
self._agent_name = agent_name
self._display_name = agent_name or agent_id  # for log messages only
```

Then replace all `self._agent_name` references in log/debug strings with `self._display_name`. Keep `self._agent_name` as-is (may be None) for anything that reads the actual name.

- [ ] **Step 5: Update ALL existing kernel tests — constructor signature**

Every `BaseAgent(agent_name="test", service=..., base_dir=...)` becomes `BaseAgent(service=..., base_dir=..., agent_name="test")`.

Files to update (kernel repo):
- `tests/test_heartbeat.py` — ~11 instances
- `tests/test_soul.py` — ~15 instances
- `tests/test_services_logging.py` — ~3 instances
- `tests/test_session.py` — 1 instance

All existing tests already use keyword form `agent_name="test"`, so the change is just reordering: move `service=` before the `*` barrier.

- [ ] **Step 6: Run all kernel tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: PASS

- [ ] **Step 7: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.base_agent import BaseAgent; print('ok')"`
Expected: `ok`

- [ ] **Step 8: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/base_agent.py src/lingtai_kernel/session.py tests/
git commit -m "refactor: agent_name optional, WorkingDir uses agent_id

agent_name moves from positional to keyword-only, defaults to None.
agent_id generated before WorkingDir construction. New set_name()
method for setting the true name (真名) — set once, never changed.
All thread names use agent_id. i18n-friendly names (no ASCII restriction)."
```

---

### Task 3: Eigen `name` action

**Files:**
- Modify: `lingtai-kernel/src/lingtai_kernel/intrinsics/eigen.py`
- Modify: `lingtai-kernel/src/lingtai_kernel/i18n/en.json`
- Modify: `lingtai-kernel/src/lingtai_kernel/i18n/zh.json`
- Modify: `lingtai/src/lingtai/i18n/wen.json` (文言 locale — must be kept in sync)

- [ ] **Step 1: Write tests for eigen name action**

Add to `lingtai/tests/test_eigen.py`:

```python
def test_eigen_name_sets_agent_name(tmp_path):
    """eigen name action sets agent true name."""
    agent = BaseAgent(service=make_mock_service(), base_dir=tmp_path)
    assert agent.agent_name is None
    result = agent._intrinsics["eigen"]({"object": "name", "action": "set", "content": "悟空"})
    assert result["status"] == "ok"
    assert agent.agent_name == "悟空"

def test_eigen_name_rejects_second_set(tmp_path):
    """eigen name action fails if already named."""
    agent = BaseAgent(service=make_mock_service(), base_dir=tmp_path, agent_name="alice")
    result = agent._intrinsics["eigen"]({"object": "name", "action": "set", "content": "bob"})
    assert "error" in result

def test_eigen_name_rejects_empty(tmp_path):
    """eigen name action fails with empty name."""
    agent = BaseAgent(service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["eigen"]({"object": "name", "action": "set", "content": ""})
    assert "error" in result
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_eigen.py -v -k "eigen_name"`
Expected: FAIL

- [ ] **Step 3: Add i18n strings to all three locales**

In kernel `en.json`, add:
```json
"eigen.name_description": "Your true name — set once, never changed. Use content to provide your chosen name."
```

In kernel `zh.json`, add:
```json
"eigen.name_description": "你的真名——只能设置一次，永不更改。用content提供你选择的名字。"
```

In lingtai `wen.json`, add:
```json
"eigen.name_description": "汝之真名——一設不易。以content書汝所擇之名。"
```

- [ ] **Step 4: Update eigen schema to include name object**

In `eigen.py`, update `get_schema()` — add `"name"` to object enum, add `"set"` to action enum:

```python
"object": {
    "type": "string",
    "enum": ["memory", "context", "name"],
    ...
},
"action": {
    "type": "string",
    "enum": ["edit", "load", "molt", "set"],
    ...
},
```

- [ ] **Step 5: Add name handler to eigen.py**

In `handle()`, add branch for `obj == "name"`:

```python
elif obj == "name":
    if action == "set":
        return _name_set(agent, args)
    else:
        return {"error": f"Unknown name action: {action}. Use set."}
```

Update the fallback error message: `"Unknown object: {obj}. Use memory, context, or name."`

Add handler:

```python
def _name_set(agent, args: dict) -> dict:
    """Set the agent's true name."""
    name = args.get("content", "").strip()
    if not name:
        return {"error": "Name cannot be empty. Provide your chosen name in 'content'."}
    try:
        agent.set_name(name)
    except RuntimeError as e:
        return {"error": str(e)}
    return {"status": "ok", "name": name}
```

- [ ] **Step 6: Update i18n description strings in all three locales**

Update `eigen.object_description` — add "name" mention:

en.json:
```json
"eigen.object_description": "memory: your working notes (system/memory.md). context: manage conversation context. name: your true name (set once)."
```

zh.json:
```json
"eigen.object_description": "memory：你的工作笔记（system/memory.md）。context：管理对话上下文。name：你的真名（只能设置一次）。"
```

wen.json:
```json
"eigen.object_description": "memory：工作札記（system/memory.md）。context：管理對話上下文。name：汝之真名（一設不易）。"
```

Update `eigen.action_description` — add "name: set":

en.json:
```json
"eigen.action_description": "memory: edit | load.\ncontext: molt.\nname: set."
```

zh.json:
```json
"eigen.action_description": "memory：edit | load。\ncontext：molt。\nname：set。"
```

wen.json:
```json
"eigen.action_description": "memory：edit | load。\ncontext：molt。\nname：set。"
```

- [ ] **Step 7: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_eigen.py -v`
Expected: PASS (all eigen tests including new name tests)

- [ ] **Step 8: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.intrinsics.eigen import get_schema; s = get_schema(); assert 'name' in s['properties']['object']['enum']; print('ok')"`
Expected: `ok`

- [ ] **Step 9: Commit (kernel)**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/intrinsics/eigen.py src/lingtai_kernel/i18n/en.json src/lingtai_kernel/i18n/zh.json
git commit -m "feat: eigen 'name' action — set true name (真名) once

Agents born without a name can set their true name via
eigen(object=name, action=set, content=<name>). i18n-friendly,
set-once semantics. Used by root agents to self-name."
```

- [ ] **Step 10: Commit (lingtai — wen.json + tests)**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
git add src/lingtai/i18n/wen.json tests/test_eigen.py
git commit -m "feat: eigen name action — wen.json i18n + tests"
```

---

### Task 4: Update lingtai tests and audit Agent passthrough

**Files:**
- Audit: `lingtai/src/lingtai/agent.py` — verify `*args, **kwargs` passthrough
- Modify: `lingtai/tests/test_workdir.py` — update WorkingDir calls to use agent_id
- Modify: `lingtai/tests/test_agent.py` — ~30 instances
- Modify: `lingtai/tests/test_system.py` — ~15 instances, path assertions
- Modify: `lingtai/tests/test_eigen.py` — ~10 instances
- Modify: `lingtai/tests/test_intrinsics_comm.py`
- Modify: `lingtai/tests/test_memory.py`
- Modify: `lingtai/tests/test_mail_intrinsic.py`
- Modify: `lingtai/tests/test_services_logging.py`
- Modify: `lingtai/tests/test_compaction.py`
- Modify: `lingtai/tests/test_override_intrinsic.py`
- Modify: `lingtai/tests/test_silence_kill.py`
- Modify: `lingtai/tests/test_streaming.py`
- Modify: `lingtai/tests/test_git_init.py`
- Modify: `lingtai/tests/test_layers_bash.py`
- Modify: `lingtai/tests/test_layers_email.py` — ~60 instances
- Modify: `lingtai/tests/test_layers_compose.py`
- Modify: `lingtai/tests/test_layers_draw.py`
- Modify: `lingtai/tests/test_layers_talk.py`
- Modify: `lingtai/tests/test_layers_listen.py`
- Modify: `lingtai/tests/test_layers_avatar.py`
- Modify: `lingtai/tests/test_layers_file.py`
- Modify: `lingtai/tests/test_agent_capabilities.py`
- Modify: `lingtai/tests/test_vision_capability.py`
- Modify: `lingtai/tests/test_web_search_capability.py`
- Modify: `lingtai/tests/test_psyche.py`
- Modify: `lingtai/tests/test_library.py`
- Modify: `lingtai/tests/test_three_agent_email.py`
- Modify: `lingtai/tests/test_app_launch.py`

- [ ] **Step 1: Audit Agent `*args` passthrough**

Read `lingtai/src/lingtai/agent.py` constructor. It uses `*args, **kwargs`. After this change, `BaseAgent.__init__` has `service` as the only positional arg. Verify that all callers pass `service` as the first positional arg or as a keyword. If any caller does `Agent("myname", svc, ...)`, the `*args` would swallow `"myname"` and pass it as `service` — which would be wrong.

Search all test files for `Agent(` calls that pass a string as the first positional arg. If found, fix them. If all callers already use keyword form, no change needed to `agent.py`.

- [ ] **Step 2: Update lingtai/tests/test_workdir.py**

Same pattern as Task 1: `WorkingDir(base_dir=tmp_path, agent_name="alice")` → `WorkingDir(base_dir=tmp_path, agent_id="a1b2c3d4e5f6")`. Update path assertions.

- [ ] **Step 3: Update test_system.py path assertion**

In `test_system_show_returns_identity`:
```python
assert agent.agent_id in identity["working_dir"]  # was: "alice" in working_dir
```

- [ ] **Step 4: Bulk update all BaseAgent/Agent constructor calls**

Pattern: `BaseAgent(agent_name="x", service=svc, base_dir=p)` → `BaseAgent(service=svc, agent_name="x", base_dir=p)`.

For Agent: `Agent(agent_name="x", service=svc, base_dir=p, capabilities=...)` → `Agent(service=svc, agent_name="x", base_dir=p, capabilities=...)`.

Use grep to find ALL instances across ALL test files listed above. This is the largest mechanical change.

- [ ] **Step 5: Run all lingtai tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: PASS

- [ ] **Step 6: Smoke test full import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai; print('ok')"`
Expected: `ok`

- [ ] **Step 7: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
git add tests/
git commit -m "test: update all tests for agent_name keyword-only constructor

BaseAgent(agent_name=x, service=svc) → BaseAgent(service=svc, agent_name=x).
Path assertions use agent_id instead of agent_name.
WorkingDir tests updated to use agent_id."
```

---

### Task 5: Final verification + CLAUDE.md

- [ ] **Step 1: Run kernel tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 2: Run lingtai tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 3: Smoke test both packages**

Run:
```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.base_agent import BaseAgent; print('kernel ok')"
cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai; print('lingtai ok')"
```

- [ ] **Step 4: Update CLAUDE.md constructor signature**

Update the BaseAgent constructor signature in CLAUDE.md from:
```python
def __init__(
    self,
    agent_name: str,
    service: LLMService,
    *,
    ...
```

To:
```python
def __init__(
    self,
    service: LLMService,
    *,
    agent_name: str | None = None,
    ...
```

Also update the Extension Pattern examples:
```python
agent = Agent(
    service=svc, agent_name="alice", base_dir="/agents",
    capabilities=["file", "vision", "web_search", "bash"],
)
```

And document `set_name()` in the BaseAgent section. Update the agent_id description to mention it's used for filesystem paths.

- [ ] **Step 5: Update design spec**

Update `docs/superpowers/specs/2026-03-20-workdir-by-agent-id-design.md` Section 3 to match the plan's eigen schema (`object=name, action=set, content=<name>` instead of `action=name, name=<string>`).

- [ ] **Step 6: Commit CLAUDE.md + spec**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
git add CLAUDE.md docs/superpowers/specs/2026-03-20-workdir-by-agent-id-design.md
git commit -m "docs: update CLAUDE.md and spec for agent_id working directories

Constructor signature, extension patterns, and set_name() documented.
Spec updated to match implementation's eigen schema pattern."
```
