# Agent Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make each agent instance fully independent — path as identity, no `agent_id` in kernel, LLMService as ABC in kernel with concrete implementation in lingtai.

**Architecture:** Two changes across two repos (lingtai-kernel and lingtai). Change 1 replaces `base_dir + agent_id` with a single `working_dir` path throughout the kernel. Change 2 extracts the LLMService ABC from the concrete implementation, moving adapter machinery to lingtai.

**Tech Stack:** Python 3.11+, Go (orchestration TUI)

**Spec:** `docs/specs/2026-03-22-agent-isolation-design.md`

---

## File Structure

### lingtai-kernel changes

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai_kernel/workdir.py` | Modify | `__init__(working_dir)` — remove `base_dir`/`agent_id` params |
| `src/lingtai_kernel/base_agent.py` | Modify | `__init__(service, *, working_dir, ...)` — remove `agent_id`/`base_dir`. Update billboard, thread names, manifest, status, _log |
| `src/lingtai_kernel/session.py` | Modify | Remove `agent_id` param from SessionManager |
| `src/lingtai_kernel/intrinsics/mail.py` | Modify | Fix `_is_self_send()`, `"from"` fallback, remove `expected_agent_id` |
| `src/lingtai_kernel/intrinsics/system.py` | Modify | Remove `agent_id` from identity dict |
| `src/lingtai_kernel/services/mail.py` | Modify | Remove `expected_agent_id` from `MailService` ABC and `FilesystemMailService` |
| `src/lingtai_kernel/handshake.py` | Verify | No `agent_id` logic — no change expected |
| `src/lingtai_kernel/llm/service.py` | Rewrite | Replace concrete class with ABC only |
| `src/lingtai_kernel/llm/base.py` | Modify | Remove `LLMAdapter`, `APICallGate` import. Keep `ChatSession` + data types |
| `src/lingtai_kernel/llm/__init__.py` | Modify | Update exports — remove `LLMAdapter` |
| `tests/test_workdir.py` | Modify | Update all constructors, remove `agent_id` validation tests |
| `tests/test_base_agent.py` | Modify | Update all constructors |
| `tests/test_session.py` | Modify | Update SessionManager constructor |
| `tests/test_heartbeat.py` | Modify | Update all constructors |
| `tests/test_soul.py` | Modify | Update all constructors |
| `tests/test_filesystem_mail.py` | Modify | Update manifest dicts, remove `expected_agent_id` tests |
| `tests/test_mail_identity.py` | Modify | Update mock agent, manifest assertions |
| `tests/test_services_logging.py` | Modify | Update constructors |

### lingtai changes

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai/agent.py` | Modify | Own `base_dir/agent_id` convention, pass `working_dir=` to super |
| `src/lingtai/network.py` | Modify | Rekey from `agent_id` to `address` (path) |
| `src/lingtai/capabilities/avatar.py` | Modify | Construct avatar `working_dir`, update sender identity |
| `src/lingtai/capabilities/email.py` | Modify | Contacts schema: `agent_id` → `address`. Update i18n |
| `src/lingtai/llm/service.py` | Create | Concrete `LLMService` (moved from kernel) |
| `src/lingtai/llm/base.py` | Create | `LLMAdapter` ABC + `APICallGate` (moved from kernel) |
| `src/lingtai/llm/__init__.py` | Modify | Import from local `base.py` and `service.py` |
| `src/lingtai/llm/_register.py` | Modify | Import `LLMService` from `lingtai.llm.service` |
| `src/lingtai/llm/custom/adapter.py` | Modify | Import `LLMAdapter` from `lingtai.llm.base` |
| `src/lingtai/i18n/en.json` | Modify | Update email contact schema strings |
| `src/lingtai/i18n/zh.json` | Modify | Update email contact schema strings |
| `src/lingtai/i18n/wen.json` | Modify | Update email contact schema strings |
| `examples/chat_agent.py` | Modify | `working_dir=` instead of `agent_id=` + `base_dir=` |
| `examples/contemplate.py` | Modify | Same |
| `examples/chat_web.py` | Modify | Same |
| All test files (~30) | Modify | Update constructor calls |
| Go orchestration (8+ files) | Modify | Remove `AgentID` from Config, use path-based identity |

---

## Task Ordering

Tasks are grouped into two phases. Phase 1 (Change 1: path as identity) is done in lingtai-kernel first, then lingtai. Phase 2 (Change 2: LLMService ABC) follows. Each task is independently committable.

---

## Phase 1: Path as Identity

### Task 1: WorkingDir — accept `working_dir` path

**Repo:** lingtai-kernel
**Files:**
- Modify: `src/lingtai_kernel/workdir.py:36-44`
- Modify: `tests/test_workdir.py`

- [ ] **Step 1: Update WorkingDir tests**

Replace all `WorkingDir(base_dir=..., agent_id=...)` with `WorkingDir(working_dir=...)`. Remove `test_invalid_agent_id_raises` and `test_arbitrary_agent_id`. Add a new test for plain path:

```python
def test_workdir_accepts_path(tmp_path):
    wd = WorkingDir(working_dir=tmp_path / "myagent")
    assert wd.path == tmp_path / "myagent"
    assert wd.path.is_dir()

def test_workdir_creates_parents(tmp_path):
    wd = WorkingDir(working_dir=tmp_path / "deep" / "nested" / "agent")
    assert wd.path.is_dir()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_workdir.py -v`
Expected: FAIL — `__init__() got an unexpected keyword argument 'working_dir'`

- [ ] **Step 3: Implement WorkingDir changes**

```python
class WorkingDir:
    """Manages an agent's working directory — locking, git, manifest."""

    def __init__(self, working_dir: Path | str) -> None:
        self._path = Path(working_dir)
        self._path.mkdir(parents=True, exist_ok=True)
        self._lock_file: Any = None
```

Remove `_base_dir`, `_agent_id`, and the agent_id validation block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_workdir.py -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel"`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
cd ~/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/workdir.py tests/test_workdir.py
git commit -m "refactor: WorkingDir takes working_dir path instead of base_dir + agent_id"
```

---

### Task 2: BaseAgent — replace `agent_id` + `base_dir` with `working_dir`

**Repo:** lingtai-kernel
**Files:**
- Modify: `src/lingtai_kernel/base_agent.py:76-107, 182, 235, 388, 501, 556, 658, 1033-1051, 1194-1212`
- Modify: `tests/test_base_agent.py`

- [ ] **Step 1: Update test_base_agent.py**

Replace all `BaseAgent(service, agent_id="test", base_dir=tmp_path, ...)` with `BaseAgent(service, working_dir=tmp_path / "test", ...)`. Remove any `assert agent.agent_id == ...` assertions. Add `assert agent.working_dir == tmp_path / "test"`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_base_agent.py -v`
Expected: FAIL

- [ ] **Step 3: Implement BaseAgent constructor changes**

In `base_agent.py`:

Constructor signature:
```python
def __init__(
    self,
    service: LLMService,
    *,
    agent_name: str | None = None,
    working_dir: str | Path,
    file_io: Any | None = None,
    mail_service: Any | None = None,
    config: AgentConfig | None = None,
    context: Any = None,
    admin: dict | None = None,
    streaming: bool = False,
    covenant: str = "",
    memory: str = "",
):
```

Constructor body — replace lines 92-107:
```python
    self.agent_name = agent_name
    self.service = service
    self._config = config or AgentConfig()
    self._context = context
    self._admin = admin or {}
    self._cancel_event = threading.Event()
    self._started_at: str = ""
    self._uptime_anchor: float | None = None

    # Working directory (caller-owned path)
    self._workdir = WorkingDir(working_dir)
    self._working_dir = self._workdir.path
```

Add a property:
```python
@property
def working_dir(self) -> Path:
    return self._working_dir
```

Update billboard path (line 182) — use working_dir name as filename:
```python
self._billboard_path = billboard_dir / f"{self._working_dir.name}.json"
```

Update thread names (lines 388, 501, 556) — use `agent_name` or working_dir name:
```python
display = self.agent_name or self._working_dir.name
# line 388:
name=f"agent-{display}",
# line 501:
self._soul_timer.name = f"soul-{display}"
# line 556:
name=f"heartbeat-{display}",
```

Update `_log()` (line 658):
```python
"address": str(self._working_dir),
"agent_name": self.agent_name,
```

Update `_build_manifest()` (lines 1039-1051):
```python
data = {
    "agent_name": self.agent_name,
    "address": str(self._working_dir),
    "started_at": self._started_at,
    "admin": self._admin,
    "language": self._config.language,
    "vigil": self._config.vigil,
    "soul_delay": self._soul_delay,
}
if self._mail_service is not None and self._mail_service.address:
    data["address"] = self._mail_service.address
return data
```

Update `status()` (lines 1201-1212):
```python
return {
    "address": str(self._working_dir),
    "agent_name": self.agent_name,
    "agent_type": self.agent_type,
    "state": self._state.value,
    ...
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_base_agent.py -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel"`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
cd ~/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/base_agent.py tests/test_base_agent.py
git commit -m "refactor: BaseAgent takes working_dir instead of agent_id + base_dir"
```

---

### Task 3: SessionManager — remove `agent_id` parameter

**Repo:** lingtai-kernel
**Files:**
- Modify: `src/lingtai_kernel/session.py:37-53`
- Modify: `tests/test_session.py`

- [ ] **Step 1: Update test_session.py**

Remove `agent_id=` from all `SessionManager(...)` constructor calls. Verify `agent_name` is passed for display name.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_session.py -v`
Expected: FAIL

- [ ] **Step 3: Implement SessionManager changes**

Remove `agent_id: str` from `__init__` parameters. Remove `self._agent_id`. Change display name fallback:
```python
self._display_name = agent_name or "agent"
```

Update `base_agent.py` line 235 — remove `agent_id=self.agent_id` from SessionManager construction.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_session.py -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd ~/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/session.py src/lingtai_kernel/base_agent.py tests/test_session.py
git commit -m "refactor: SessionManager removes agent_id parameter"
```

---

### Task 4: Mail intrinsic + MailService — remove `agent_id` from handshake

**Repo:** lingtai-kernel
**Files:**
- Modify: `src/lingtai_kernel/services/mail.py:39, 126, 144-149`
- Modify: `src/lingtai_kernel/intrinsics/mail.py:203-205, 294-315, 366`
- Modify: `tests/test_filesystem_mail.py`
- Modify: `tests/test_mail_identity.py`

- [ ] **Step 1: Update mail tests**

In `test_filesystem_mail.py`:
- Remove `"agent_id"` from `_make_agent_dir()` helper manifests. Keep `"address"` field.
- Remove `expected_agent_id=` from send calls.
- Remove handshake-mismatch-by-agent_id test (if any). Replace with address-based check if needed.

In `test_mail_identity.py`:
- Remove `agent.agent_id` setup on mock agents. Use `agent._working_dir` for identity.
- Remove `"agent_id"` from manifest assertions. Use `"address"` instead.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_filesystem_mail.py tests/test_mail_identity.py -v`
Expected: FAIL

- [ ] **Step 3: Implement mail changes**

In `services/mail.py` — `MailService` ABC:
- Remove `expected_agent_id` parameter from `send()`.
- Remove agent_id handshake validation from `FilesystemMailService.send()` (lines 144-149). Keep heartbeat and `.agent.json` existence checks.

In `intrinsics/mail.py`:
- `_is_self_send()` (line 205): change `if address == agent.agent_id` to `if address == str(agent._working_dir)`.
- Remove `expected_agent_id` from contact-based send (lines 294-315). Remove `expected_id = c.get("agent_id")` and `expected_agent_id=expected_id`.
- `"from"` fallback (line 366): change `agent.agent_id` to `str(agent._working_dir)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_filesystem_mail.py tests/test_mail_identity.py -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd ~/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/services/mail.py src/lingtai_kernel/intrinsics/mail.py tests/test_filesystem_mail.py tests/test_mail_identity.py
git commit -m "refactor: remove agent_id from mail handshake, use address (path) only"
```

---

### Task 5: System intrinsic — remove `agent_id` from identity

**Repo:** lingtai-kernel
**Files:**
- Modify: `src/lingtai_kernel/intrinsics/system.py:100`

- [ ] **Step 1: Update system intrinsic**

Line 100: change `"agent_id": agent.agent_id` to `"address": str(agent._working_dir)`.

- [ ] **Step 2: Run all kernel tests**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: PASS (all remaining `agent_id` references in test files should have been caught by Tasks 1-4; if not, fix them here)

- [ ] **Step 3: Commit**

```bash
cd ~/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/intrinsics/system.py
git commit -m "refactor: system intrinsic uses address instead of agent_id"
```

---

### Task 6: Remaining kernel test files — bulk update constructors

**Repo:** lingtai-kernel
**Files:**
- Modify: `tests/test_heartbeat.py`
- Modify: `tests/test_soul.py`
- Modify: `tests/test_services_logging.py`
- Modify: Any other test file still using `agent_id=` or `base_dir=`

- [ ] **Step 1: Grep and fix all remaining `agent_id` references**

Run: `grep -rn "agent_id" ~/Documents/GitHub/lingtai-kernel/tests/`

For each file: replace `agent_id="test", base_dir=tmp_path` with `working_dir=tmp_path / "test"`.

- [ ] **Step 2: Grep and fix all remaining `base_dir` references**

Run: `grep -rn "base_dir" ~/Documents/GitHub/lingtai-kernel/tests/`

- [ ] **Step 3: Run full kernel test suite**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 4: Smoke-test import**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel"`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
cd ~/Documents/GitHub/lingtai-kernel
git add tests/
git commit -m "test: update all kernel tests for working_dir constructor"
```

---

### Task 7: lingtai Agent — own the `base_dir / agent_id` convention

**Repo:** lingtai
**Files:**
- Modify: `src/lingtai/agent.py`
- Modify: `tests/test_agent.py` (and others that construct Agent directly)

- [ ] **Step 1: Update Agent constructor**

In `agent.py`, the `Agent.__init__` should accept either:
- `working_dir=` directly (new way), OR
- `base_dir=` + some way to generate a directory name (lingtai's convenience)

The simplest approach: Agent takes `working_dir=` and passes it to `super().__init__()`. The `base_dir` + `agent_id` convenience stays as a lingtai-level factory or the caller's responsibility.

```python
class Agent(BaseAgent):
    def __init__(
        self,
        *args,
        combo_name: str | None = None,
        capabilities: list | dict | None = None,
        **kwargs,
    ):
        self._combo_name = combo_name
        # capabilities setup...
        super().__init__(*args, **kwargs)  # working_dir= flows through kwargs
```

Update `revive()` / `from_working_dir()` — stop reading `agent_meta.get("agent_id")`, just pass the directory path as `working_dir=`.

- [ ] **Step 2: Update lingtai tests**

Grep all test files for `agent_id=` and `base_dir=`. Replace with `working_dir=tmp_path / "test"`. This is ~30 files — mechanical replacement.

- [ ] **Step 3: Run lingtai test suite**

Run: `cd ~/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 4: Smoke-test import**

Run: `cd ~/Documents/GitHub/lingtai && python -c "import lingtai"`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
cd ~/Documents/GitHub/lingtai
git add src/lingtai/agent.py tests/
git commit -m "refactor: Agent takes working_dir, owns base_dir/id convention"
```

---

### Task 8: Avatar capability — update spawning

**Repo:** lingtai
**Files:**
- Modify: `src/lingtai/capabilities/avatar.py:142, 154, 219, 243, 251, 264`

- [ ] **Step 1: Update avatar.py**

- Line 142: `"agent_id": existing.agent_id` → `"address": str(existing.working_dir)`
- Line 154: `parent._base_dir` → derive from `parent.working_dir.parent` or a convention
- Line 219: `agent_id=avatar_id` → `working_dir=parent.working_dir.parent / avatar_id`
- Line 243: `sender=parent.agent_id` → `sender=str(parent.working_dir)`
- Line 251: `agent_id=avatar.agent_id` → `address=str(avatar.working_dir)`
- Line 264: `"agent_id": avatar.agent_id` → `"address": str(avatar.working_dir)`

- [ ] **Step 2: Run avatar tests**

Run: `cd ~/Documents/GitHub/lingtai && python -m pytest tests/test_layers_avatar.py -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
cd ~/Documents/GitHub/lingtai
git add src/lingtai/capabilities/avatar.py
git commit -m "refactor: avatar capability uses working_dir path, not agent_id"
```

---

### Task 9: Email capability + i18n — contacts use `address` not `agent_id`

**Repo:** lingtai
**Files:**
- Modify: `src/lingtai/capabilities/email.py:113-116, 609, 801, 982-994`
- Modify: `src/lingtai/i18n/en.json`
- Modify: `src/lingtai/i18n/zh.json`
- Modify: `src/lingtai/i18n/wen.json`

- [ ] **Step 1: Update email.py**

- Contact schema (line 113-116): rename `agent_id` field to `address`.
- Lines 609, 801: `self._agent.agent_id` → `str(self._agent.working_dir)`.
- Lines 982-994: `agent_id` parameter in contact operations → `address`.

- [ ] **Step 2: Update i18n files**

In all three locale files, find the email contact schema strings referencing `agent_id` and rename to `address`. Ensure all three files (en.json, zh.json, wen.json) are updated.

- [ ] **Step 3: Run email tests**

Run: `cd ~/Documents/GitHub/lingtai && python -m pytest tests/test_layers_email.py -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd ~/Documents/GitHub/lingtai
git add src/lingtai/capabilities/email.py src/lingtai/i18n/
git commit -m "refactor: email contacts use address (path) instead of agent_id"
```

---

### Task 10: Network module — rekey by address

**Repo:** lingtai
**Files:**
- Modify: `src/lingtai/network.py:27, 58, 69, 87-105, 162-237`
- Modify: `tests/test_network.py`

- [ ] **Step 1: Update network.py**

- `AgentNode.agent_id` → `AgentNode.address` (the working dir path string).
- All query methods (`children_of`, `contacts_of`, `mail_of`): accept `address` instead of `agent_id`.
- `_discover_agents()`: read `address` from manifest (already there as `"address"` or derive from directory path). Key nodes dict by `address`.
- Avatar ledger reading: `child_id = record.get("address", "")` instead of `record.get("agent_id", "")`.

- [ ] **Step 2: Update network tests**

Update all test assertions and data to use `address` instead of `agent_id`.

- [ ] **Step 3: Run network tests**

Run: `cd ~/Documents/GitHub/lingtai && python -m pytest tests/test_network.py -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd ~/Documents/GitHub/lingtai
git add src/lingtai/network.py tests/test_network.py
git commit -m "refactor: network module keyed by address (path) instead of agent_id"
```

---

### Task 11: Examples — update constructor calls

**Repo:** lingtai
**Files:**
- Modify: `examples/chat_agent.py`
- Modify: `examples/contemplate.py`
- Modify: `examples/chat_web.py`

- [ ] **Step 1: Update all examples**

Replace pattern:
```python
agent_id = secrets.token_hex(3)
mail_svc = FilesystemMailService(working_dir=base_dir / agent_id)
agent = Agent(service=svc, agent_id=agent_id, base_dir=base_dir, ...)
```

With:
```python
agent_dir = base_dir / secrets.token_hex(3)
mail_svc = FilesystemMailService(working_dir=agent_dir)
agent = Agent(service=svc, working_dir=agent_dir, ...)
```

- [ ] **Step 2: Commit**

```bash
cd ~/Documents/GitHub/lingtai
git add examples/
git commit -m "refactor: examples use working_dir instead of agent_id + base_dir"
```

---

### Task 12: Go orchestration — remove AgentID from config

**Repo:** lingtai
**Files:**
- Modify: `app/orchestration/internal/config/loader.go`
- Modify: `app/orchestration/internal/config/loader_test.go`
- Modify: `app/orchestration/internal/setup/wizard.go`
- Modify: `app/orchestration/internal/manage/list.go`
- Modify: `app/orchestration/internal/tui/app.go`
- Modify: `app/orchestration/internal/tui/status.go`
- Modify: `app/orchestration/internal/agent/mail.go`
- Modify: `app/orchestration/internal/agent/mail_test.go`
- Modify: `app/orchestration/internal/agent/process_test.go`
- Modify: `app/orchestration/internal/setup/tests_test.go`

- [ ] **Step 1: Config — replace AgentID with WorkingDir**

In `config/loader.go`:
- Remove `AgentID` field from `Config` struct. Add `WorkingDir string` if not already present as a direct field.
- `WorkingDir()` method becomes a simple field accessor instead of computing `filepath.Join(c.ProjectDir, c.AgentID)`.
- `DisplayName()` fallback: use `filepath.Base(c.WorkingDir)` instead of `c.AgentID`.
- Remove agent_id validation.

The wizard still generates a hex directory name — that's the Go layer's convention. It writes `"working_dir"` to config.json instead of `"agent_id"`.

- [ ] **Step 2: Wizard — write working_dir instead of agent_id**

In `setup/wizard.go`:
- Keep `generateAgentID()` as `generateDirName()` — it generates the directory name.
- Line 1769: write `"working_dir": filepath.Join(m.outputDir, m.agentID)` instead of `"agent_id": m.agentID`.
- Line 1866: derive from config's WorkingDir.

- [ ] **Step 3: List/Status — use directory name or address**

In `manage/list.go`:
- `Spirit.AgentID` → `Spirit.DirName` (directory basename) or `Spirit.Address` (full path).
- Discovery via `entry.Name()` stays the same — it reads directory names.

In `tui/app.go`:
- `activeID` → track by directory path or basename.
- Matching: by name or path.

In `tui/status.go`:
- Construct paths using directory entry name, not `AgentID`.

- [ ] **Step 4: Mail — remove agent_id from manifest**

In `agent/mail.go` line 179: remove `"agent_id": humanID`. Write `"address"` instead.

- [ ] **Step 5: Update Go tests**

Update all Go test files to remove `"agent_id"` from manifest dicts and config. Use `"working_dir"` in config.

- [ ] **Step 6: Run Go tests**

Run: `cd ~/Documents/GitHub/lingtai/app/orchestration && go test ./...`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
cd ~/Documents/GitHub/lingtai
git add app/orchestration/
git commit -m "refactor: Go orchestration uses working_dir path, removes AgentID"
```

---

### Task 13: Final Phase 1 verification

**Repo:** both

- [ ] **Step 1: Verify no `agent_id` remains in kernel source**

Run: `grep -rn "agent_id" ~/Documents/GitHub/lingtai-kernel/src/`
Expected: NO matches

- [ ] **Step 2: Verify no `base_dir` remains in kernel source (except unrelated uses)**

Run: `grep -rn "base_dir" ~/Documents/GitHub/lingtai-kernel/src/`
Expected: NO matches in agent/workdir code

- [ ] **Step 3: Run full kernel test suite**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 4: Run full lingtai test suite**

Run: `cd ~/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 5: Run Go tests**

Run: `cd ~/Documents/GitHub/lingtai/app/orchestration && go test ./...`
Expected: ALL PASS

---

## Phase 2: LLMService ABC in Kernel

### Task 14: Extract LLMService ABC in kernel

**Repo:** lingtai-kernel
**Files:**
- Rewrite: `src/lingtai_kernel/llm/service.py`
- Modify: `src/lingtai_kernel/llm/__init__.py`

- [ ] **Step 1: Rewrite kernel's service.py as ABC**

Replace the entire contents of `src/lingtai_kernel/llm/service.py` with:

```python
"""LLMService ABC — protocol for LLM access.

The kernel depends only on this interface. Concrete implementations
(adapter-based, local model, mock) live outside the kernel.
"""
from __future__ import annotations

from abc import ABC, abstractmethod
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .base import ChatSession, FunctionSchema, LLMResponse
    from .interface import ChatInterface, ToolResultBlock


class LLMService(ABC):
    """Protocol for LLM access. Kernel depends only on this."""

    @property
    @abstractmethod
    def model(self) -> str:
        """Default model identifier."""

    @property
    @abstractmethod
    def provider(self) -> str:
        """Provider name."""

    @abstractmethod
    def create_session(
        self,
        system_prompt: str,
        tools: "list[FunctionSchema] | None" = None,
        *,
        model: str | None = None,
        thinking: str = "default",
        agent_type: str = "",
        tracked: bool = True,
        interaction_id: str | None = None,
        json_schema: dict | None = None,
        force_tool_call: bool = False,
        provider: str | None = None,
        interface: "ChatInterface | None" = None,
    ) -> "ChatSession":
        """Start a new multi-turn conversation."""

    @abstractmethod
    def resume_session(
        self, saved_state: dict, *, thinking: str = "high"
    ) -> "ChatSession":
        """Restore a session from saved state."""

    @abstractmethod
    def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        system_prompt: str | None = None,
        temperature: float | None = None,
        json_schema: dict | None = None,
        max_output_tokens: int | None = None,
        provider: str | None = None,
    ) -> "LLMResponse":
        """Single-turn generation."""

    @abstractmethod
    def make_tool_result(
        self,
        tool_name: str,
        result: dict,
        *,
        tool_call_id: str | None = None,
        provider: str | None = None,
    ) -> "ToolResultBlock":
        """Build a canonical ToolResultBlock."""
```

- [ ] **Step 2: Smoke-test kernel import**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.llm import LLMService; print(LLMService)"`
Expected: Shows the ABC class

- [ ] **Step 3: Commit**

```bash
cd ~/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/llm/service.py
git commit -m "refactor: LLMService becomes ABC — protocol only in kernel"
```

---

### Task 15: Move LLMAdapter and APICallGate to lingtai

**Repo:** lingtai-kernel (remove) + lingtai (add)
**Files:**
- Modify: `lingtai-kernel/src/lingtai_kernel/llm/base.py` — remove `LLMAdapter`, keep `ChatSession` + data types
- Remove: `lingtai-kernel/src/lingtai_kernel/llm/api_gate.py`
- Modify: `lingtai-kernel/src/lingtai_kernel/llm/__init__.py` — remove `LLMAdapter` from exports
- Create: `lingtai/src/lingtai/llm/base.py` — `LLMAdapter` ABC + `APICallGate`
- Modify: `lingtai/src/lingtai/llm/__init__.py`
- Modify: `lingtai/src/lingtai/llm/custom/adapter.py` — import from `lingtai.llm.base`

- [ ] **Step 1: Create lingtai/llm/base.py with LLMAdapter + APICallGate**

Copy `LLMAdapter` class and `APICallGate` class to `lingtai/src/lingtai/llm/base.py`. Update imports:
- `LLMAdapter` imports `ChatSession`, `FunctionSchema`, `LLMResponse` from `lingtai_kernel.llm.base`
- `APICallGate` is self-contained (just threading + time)

- [ ] **Step 2: Remove LLMAdapter from kernel's base.py**

In `lingtai_kernel/llm/base.py`:
- Remove `from .api_gate import APICallGate`
- Remove the entire `LLMAdapter` class
- Keep: `ChatSession`, `LLMResponse`, `ToolCall`, `UsageMetadata`, `FunctionSchema`

- [ ] **Step 3: Delete kernel's api_gate.py**

Remove `lingtai_kernel/llm/api_gate.py`.

- [ ] **Step 4: Update kernel's llm/__init__.py**

```python
"""LLM protocol layer — session ABC, service ABC, provider-agnostic types."""
from .base import ChatSession, LLMResponse, ToolCall, FunctionSchema
from .service import LLMService

__all__ = [
    "ChatSession",
    "LLMResponse",
    "ToolCall",
    "FunctionSchema",
    "LLMService",
]
```

- [ ] **Step 5: Update lingtai's llm/__init__.py**

```python
"""LLM adapter layer — multi-provider support with kernel protocol re-exports."""
from lingtai_kernel.llm.base import ChatSession, LLMResponse, ToolCall, FunctionSchema
from lingtai_kernel.llm.interface import ChatInterface
from lingtai_kernel.llm.service import LLMService
from .base import LLMAdapter

__all__ = [
    "LLMAdapter",
    "ChatSession",
    "LLMResponse",
    "ToolCall",
    "FunctionSchema",
    "ChatInterface",
    "LLMService",
]

from ._register import register_all_adapters as _register_all_adapters
_register_all_adapters()
```

- [ ] **Step 6: Update custom adapter import**

In `lingtai/llm/custom/adapter.py`: change `from lingtai_kernel.llm.base import LLMAdapter` to `from lingtai.llm.base import LLMAdapter`.

- [ ] **Step 7: Smoke-test both packages**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel"`
Run: `cd ~/Documents/GitHub/lingtai && python -c "import lingtai"`
Expected: No errors for both

- [ ] **Step 8: Commit (kernel)**

```bash
cd ~/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/llm/
git commit -m "refactor: remove LLMAdapter and APICallGate from kernel"
```

- [ ] **Step 9: Commit (lingtai)**

```bash
cd ~/Documents/GitHub/lingtai
git add src/lingtai/llm/
git commit -m "refactor: LLMAdapter and APICallGate now live in lingtai"
```

---

### Task 16: Move concrete LLMService to lingtai

**Repo:** lingtai
**Files:**
- Create: `src/lingtai/llm/service.py` — concrete `LLMService` (adapter-based)
- Modify: `src/lingtai/llm/__init__.py` — import concrete service
- Modify: `src/lingtai/llm/_register.py` — import from local service

- [ ] **Step 1: Create lingtai/llm/service.py**

Copy the old concrete `LLMService` class from kernel (before Task 14 replaced it). Place it in `lingtai/src/lingtai/llm/service.py`. Make it subclass the kernel ABC:

```python
from lingtai_kernel.llm.service import LLMService as LLMServiceABC

class LLMService(LLMServiceABC):
    """Adapter-based LLM service — concrete implementation."""
    # ... (all the existing implementation: adapter registry, cache, etc.)
```

Also move `get_context_limit()`, `_fetch_litellm_registry()`, `_generate_session_id()` into this file.

- [ ] **Step 2: Update lingtai's __init__.py**

Change `LLMService` import to come from local module:
```python
from .service import LLMService  # concrete, not the kernel ABC
```

- [ ] **Step 3: Update _register.py**

Change import from `lingtai_kernel.llm.service` to `lingtai.llm.service`.

- [ ] **Step 4: Smoke-test**

Run: `cd ~/Documents/GitHub/lingtai && python -c "from lingtai.llm import LLMService; print(LLMService.__mro__)"`
Expected: Shows `LLMService -> LLMServiceABC -> ABC -> object`

- [ ] **Step 5: Run full lingtai test suite**

Run: `cd ~/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
cd ~/Documents/GitHub/lingtai
git add src/lingtai/llm/
git commit -m "refactor: concrete LLMService moved from kernel to lingtai"
```

---

### Task 17: Final verification — both repos clean

**Repo:** both

- [ ] **Step 1: Verify kernel llm/ is protocol-only**

Run: `ls ~/Documents/GitHub/lingtai-kernel/src/lingtai_kernel/llm/`
Expected: `__init__.py`, `service.py` (ABC), `base.py` (ChatSession + types), `interface.py`, `streaming.py`
NO `api_gate.py`. No adapter references.

- [ ] **Step 2: Verify no `agent_id` in kernel source**

Run: `grep -rn "agent_id" ~/Documents/GitHub/lingtai-kernel/src/`
Expected: NO matches

- [ ] **Step 3: Run full kernel test suite**

Run: `cd ~/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 4: Run full lingtai test suite**

Run: `cd ~/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 5: Run Go tests**

Run: `cd ~/Documents/GitHub/lingtai/app/orchestration && go test ./...`
Expected: ALL PASS

- [ ] **Step 6: Smoke-test both packages**

Run: `python -c "import lingtai_kernel; import lingtai; print('OK')"`
Expected: OK
