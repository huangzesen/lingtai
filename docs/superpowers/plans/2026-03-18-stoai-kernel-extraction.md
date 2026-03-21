# lingtai-kernel Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the BaseAgent kernel into a standalone `lingtai-kernel` package in a separate repository, leaving `lingtai` as a batteries-included wrapper.

**Architecture:** Three prerequisite refactors decouple BaseAgent from non-kernel modules (FileIO, MCP, bash config). Then the kernel modules are copied to a new repo as `lingtai_kernel`, all imports updated, tests split, and `lingtai.__init__` re-exports the kernel's public API for backward compatibility.

**Tech Stack:** Python 3.11+, pytest, setuptools, pip editable installs

**Spec:** `docs/superpowers/specs/2026-03-18-lingtai-kernel-extraction-design.md`

---

## Current State

The LLMService refactor is complete:
- Adapter registry pattern replaces hardcoded imports
- Multimodal methods removed from LLMService and LLMAdapter
- Vision/web_search capabilities call adapters directly
- Agent provider routing removed

## File Structure

### What moves to `lingtai-kernel/src/lingtai_kernel/`

```
lingtai_kernel/
├── __init__.py           (new — public API exports)
├── base_agent.py         (from src/lingtai/base_agent.py, minus FileIO + MCP)
├── config.py             (from src/lingtai/config.py, minus bash_policy_file)
├── state.py              (unchanged)
├── types.py              (unchanged)
├── message.py            (unchanged)
├── workdir.py            (unchanged)
├── session.py            (unchanged)
├── tool_executor.py      (unchanged)
├── prompt.py             (unchanged)
├── logging.py            (unchanged)
├── token_counter.py      (unchanged)
├── llm_utils.py          (unchanged)
├── loop_guard.py         (unchanged)
├── tool_timing.py        (unchanged)
├── intrinsics/
│   ├── __init__.py       (unchanged)
│   ├── mail.py           (unchanged — zero relative imports)
│   ├── system.py         (unchanged — zero relative imports; combines clock+status)
│   └── eigen.py          (has deferred `from ..llm.interface import TextBlock` — resolves within kernel)
├── services/
│   ├── __init__.py       (new — empty)
│   ├── mail.py           (unchanged — ABC + TCPMailService)
│   └── logging.py        (unchanged — ABC + JSONLLoggingService)
└── llm/
    ├── __init__.py       (new — exports only, NO adapter registration)
    ├── base.py           (unchanged)
    ├── interface.py      (unchanged)
    ├── service.py        (unchanged — registry-based, no adapter imports)
    ├── api_gate.py       (unchanged)
    └── streaming.py      (unchanged)
```

### What stays in `lingtai/src/lingtai/` (modified)

```
lingtai/
├── __init__.py           (rewritten — re-exports from lingtai_kernel)
├── agent.py              (import BaseAgent from lingtai_kernel, + FileIO + MCP)
├── config.py             (DELETED — use lingtai_kernel.config)
├── base_agent.py         (DELETED — use lingtai_kernel.base_agent)
├── state.py, types.py, message.py, etc.  (DELETED — re-exported)
├── capabilities/         (imports updated to lingtai_kernel.*)
├── addons/               (imports updated to lingtai_kernel.*)
├── services/
│   ├── file_io.py        (stays — ABC + LocalFileIOService)
│   ├── vision.py         (stays)
│   ├── search.py         (stays)
│   └── mcp.py            (stays, import updated)
└── llm/
    ├── __init__.py        (updated — re-exports from lingtai_kernel.llm + registers adapters)
    ├── _register.py       (stays)
    ├── interface_converters.py  (import updated)
    ├── anthropic/         (imports updated)
    ├── openai/            (imports updated)
    ├── gemini/            (imports updated)
    ├── minimax/           (imports updated)
    └── custom/            (imports updated)
```

---

### Task 1: Move FileIO auto-creation from BaseAgent to Agent

BaseAgent currently imports `LocalFileIOService` (non-kernel) at line 122. This must move to Agent.

**Files:**
- Modify: `src/lingtai/base_agent.py:116-123`
- Modify: `src/lingtai/agent.py:44`
- Test: `tests/test_agent.py`

- [ ] **Step 1: Write test that BaseAgent works without FileIO**

Add to `tests/test_agent.py`:

```python
def test_base_agent_file_io_defaults_to_none(tmp_path):
    """BaseAgent should have _file_io=None when no file_io is passed."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent._file_io is None
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_agent.py::test_base_agent_file_io_defaults_to_none -v`
Expected: FAIL — `_file_io` is a `LocalFileIOService`, not None

- [ ] **Step 3: Update BaseAgent — remove FileIO auto-creation**

In `src/lingtai/base_agent.py`, replace lines 118-123:

```python
        # FileIOService: auto-create LocalFileIOService for backward compat
        if file_io is not None:
            self._file_io = file_io
        else:
            from .services.file_io import LocalFileIOService
            self._file_io = LocalFileIOService(root=self._working_dir)
```

With:

```python
        # FileIOService: optional, provided by Agent or host
        self._file_io = file_io
```

- [ ] **Step 4: Update Agent — add FileIO auto-creation**

In `src/lingtai/agent.py`, add after `super().__init__(*args, **kwargs)` (line 44):

```python
        # Auto-create FileIOService if not provided by host
        if self._file_io is None:
            from .services.file_io import LocalFileIOService
            self._file_io = LocalFileIOService(root=self._working_dir)
```

- [ ] **Step 5: Run tests**

Run: `python -m pytest tests/test_agent.py tests/test_services_file_io.py tests/test_layers_file.py -v`
Expected: All PASS

- [ ] **Step 6: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All pass (511+)

- [ ] **Step 7: Smoke-test**

Run: `python -c "from lingtai import BaseAgent, Agent; print('OK')"`

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/base_agent.py src/lingtai/agent.py tests/test_agent.py
git commit -m "refactor: move FileIO auto-creation from BaseAgent to Agent"
```

---

### Task 2: Move connect_mcp() from BaseAgent to Agent

`BaseAgent.connect_mcp()` (line 950) imports `MCPClient` from `services.mcp` which stays in lingtai. Also move the MCP cleanup from BaseAgent's `stop()`.

**Files:**
- Modify: `src/lingtai/base_agent.py:950-1010`
- Modify: `src/lingtai/agent.py`
- Test: `tests/test_agent.py`

- [ ] **Step 1: Write test that connect_mcp is on Agent, not BaseAgent**

Add to `tests/test_agent.py`:

```python
def test_connect_mcp_is_on_agent_not_base(tmp_path):
    """connect_mcp should be defined on Agent, not BaseAgent."""
    assert hasattr(Agent, 'connect_mcp')
    # Verify it's not inherited from BaseAgent
    assert 'connect_mcp' not in BaseAgent.__dict__
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_agent.py::test_connect_mcp_is_on_agent_not_base -v`
Expected: FAIL — `connect_mcp` is in `BaseAgent.__dict__`

- [ ] **Step 3: Move connect_mcp from BaseAgent to Agent**

Cut the `connect_mcp()` method (lines 950-1000) from `src/lingtai/base_agent.py` and paste it into `src/lingtai/agent.py` as a method on `Agent`.

Also move the MCP client cleanup from BaseAgent's `stop()` method. In `base_agent.py`, find and remove the MCP cleanup code in `stop()`.

Note: `BaseAgent._perform_restart()` also references `_mcp_clients` via `getattr()` guards and `_load_mcp_from_workdir` via `hasattr()`. These are duck-typed accesses that harmlessly no-op on bare BaseAgent (no `_mcp_clients` attr). Leave them in place — they're safe and moving `_perform_restart()` would be a much larger change since it's tied to the main loop.

In `base_agent.py` `stop()`, find and remove:

```python
        # Clean up MCP clients
        for client in getattr(self, "_mcp_clients", []):
            try:
                client.stop()
            except Exception:
                pass
```

Add this cleanup to Agent's existing `stop()` method (before `super().stop()`):

```python
    def stop(self, timeout: float = 5.0) -> None:
        # Clean up MCP clients
        for client in getattr(self, "_mcp_clients", []):
            try:
                client.stop()
            except Exception:
                pass
        for name, mgr in self._addon_managers.items():
            ...  # existing addon cleanup
        super().stop(timeout=timeout)
```

- [ ] **Step 4: Run tests**

Run: `python -m pytest tests/test_agent.py -v`
Expected: All PASS

- [ ] **Step 5: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All pass

- [ ] **Step 6: Smoke-test**

Run: `python -c "from lingtai import BaseAgent, Agent; print('OK')"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/base_agent.py src/lingtai/agent.py tests/test_agent.py
git commit -m "refactor: move connect_mcp() from BaseAgent to Agent"
```

---

### Task 3: Remove bash_policy_file from AgentConfig

This field is only used by the bash capability. Capability-level config should be passed via `capabilities={"bash": {"policy_file": ...}}`.

**Files:**
- Modify: `src/lingtai/config.py:22`
- Modify: `src/lingtai/capabilities/bash.py:215`
- Test: `tests/test_agent.py`

- [ ] **Step 1: Write test**

Add to `tests/test_agent.py`:

```python
def test_agent_config_has_no_bash_policy_file():
    """AgentConfig should not have capability-specific fields."""
    from lingtai.config import AgentConfig
    assert not hasattr(AgentConfig, 'bash_policy_file') or 'bash_policy_file' not in AgentConfig.__dataclass_fields__
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_agent.py::test_agent_config_has_no_bash_policy_file -v`
Expected: FAIL

- [ ] **Step 3: Remove bash_policy_file from AgentConfig**

In `src/lingtai/config.py`, delete line 22:
```python
    bash_policy_file: str | None = None  # path to bash policy JSON
```

- [ ] **Step 4: Update bash capability fallback**

In `src/lingtai/capabilities/bash.py`, line 215, change:
```python
    resolved_policy_file = policy_file or getattr(agent._config, "bash_policy_file", None)
```
To:
```python
    resolved_policy_file = policy_file
```

- [ ] **Step 5: Run tests**

Run: `python -m pytest tests/test_agent.py tests/test_layers_bash.py -v`
Expected: All PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "from lingtai.config import AgentConfig; print('OK')"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/config.py src/lingtai/capabilities/bash.py tests/test_agent.py
git commit -m "refactor: remove bash_policy_file from AgentConfig"
```

---

### Task 4: Verify all prerequisite refactors — BaseAgent is kernel-clean

Before extracting, confirm BaseAgent has zero non-kernel imports.

**Files:**
- Test: `tests/test_agent.py`

- [ ] **Step 1: Write verification test**

Add to `tests/test_agent.py`:

```python
def test_base_agent_has_no_non_kernel_imports():
    """BaseAgent module should not import from non-kernel modules."""
    import ast
    from pathlib import Path
    source = Path("src/lingtai/base_agent.py").read_text()
    tree = ast.parse(source)

    non_kernel = {"services.file_io", "services.mcp", "services.vision", "services.search",
                  "capabilities", "addons", "agent"}

    for node in ast.walk(tree):
        if isinstance(node, (ast.Import, ast.ImportFrom)):
            if isinstance(node, ast.ImportFrom) and node.module:
                for nk in non_kernel:
                    assert nk not in node.module, f"base_agent.py imports from non-kernel: {node.module}"
```

- [ ] **Step 2: Run test**

Run: `python -m pytest tests/test_agent.py::test_base_agent_has_no_non_kernel_imports -v`
Expected: PASS — BaseAgent is clean

- [ ] **Step 3: Run full test suite one final time**

Run: `python -m pytest tests/ -v`
Expected: All pass. This is the last check before we start extracting.

- [ ] **Step 4: Commit**

```bash
git add tests/test_agent.py
git commit -m "test: verify BaseAgent has no non-kernel imports"
```

---

### Task 5: Create lingtai-kernel repository and copy kernel modules

Create the new repo at `../lingtai-kernel/` (sibling directory) with all kernel modules.

**Files:**
- Create: `../lingtai-kernel/` (entire repo structure)

- [ ] **Step 1: Create repository structure**

```bash
mkdir -p ../lingtai-kernel/src/lingtai_kernel/intrinsics
mkdir -p ../lingtai-kernel/src/lingtai_kernel/services
mkdir -p ../lingtai-kernel/src/lingtai_kernel/llm
mkdir -p ../lingtai-kernel/tests
```

- [ ] **Step 2: Create pyproject.toml**

Write `../lingtai-kernel/pyproject.toml`:

```toml
[build-system]
requires = ["setuptools>=68.0"]
build-backend = "setuptools.build_meta"

[project]
name = "lingtai-kernel"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = []
description = "Minimal agent kernel — think, communicate, remember, host tools"
license = "MIT"

[tool.setuptools.packages.find]
where = ["src"]

[tool.pytest.ini_options]
testpaths = ["tests"]
```

- [ ] **Step 3: Copy kernel modules (no modifications yet)**

```bash
# Core modules
for f in base_agent.py config.py state.py types.py message.py workdir.py \
         session.py tool_executor.py prompt.py logging.py token_counter.py \
         llm_utils.py loop_guard.py tool_timing.py; do
    cp src/lingtai/$f ../lingtai-kernel/src/lingtai_kernel/$f
done

# Intrinsics (3 intrinsics: mail, system, eigen)
cp src/lingtai/intrinsics/__init__.py ../lingtai-kernel/src/lingtai_kernel/intrinsics/
cp src/lingtai/intrinsics/mail.py ../lingtai-kernel/src/lingtai_kernel/intrinsics/
cp src/lingtai/intrinsics/system.py ../lingtai-kernel/src/lingtai_kernel/intrinsics/
cp src/lingtai/intrinsics/eigen.py ../lingtai-kernel/src/lingtai_kernel/intrinsics/
# Note: eigen.py has a deferred `from ..llm.interface import TextBlock` which resolves
# correctly within lingtai_kernel since both modules move together.

# Services (ABCs + kernel impls)
touch ../lingtai-kernel/src/lingtai_kernel/services/__init__.py
cp src/lingtai/services/mail.py ../lingtai-kernel/src/lingtai_kernel/services/
cp src/lingtai/services/logging.py ../lingtai-kernel/src/lingtai_kernel/services/

# LLM protocol (no adapters)
cp src/lingtai/llm/base.py ../lingtai-kernel/src/lingtai_kernel/llm/
cp src/lingtai/llm/interface.py ../lingtai-kernel/src/lingtai_kernel/llm/
cp src/lingtai/llm/service.py ../lingtai-kernel/src/lingtai_kernel/llm/
cp src/lingtai/llm/api_gate.py ../lingtai-kernel/src/lingtai_kernel/llm/
cp src/lingtai/llm/streaming.py ../lingtai-kernel/src/lingtai_kernel/llm/
```

- [ ] **Step 4: Verify file count**

```bash
find ../lingtai-kernel/src/lingtai_kernel -name "*.py" | wc -l
```
Expected: ~25 files

- [ ] **Step 5: Commit the copy (in lingtai-kernel repo)**

```bash
cd ../lingtai-kernel && git init && git add -A && git commit -m "init: copy kernel modules from lingtai"
cd ../lingtai
```

---

### Task 6: Create lingtai_kernel `__init__.py` and `llm/__init__.py`

Define the public API exports for the kernel package.

**Files:**
- Create: `../lingtai-kernel/src/lingtai_kernel/__init__.py`
- Create: `../lingtai-kernel/src/lingtai_kernel/llm/__init__.py`

- [ ] **Step 1: Create kernel `__init__.py`**

Write `../lingtai-kernel/src/lingtai_kernel/__init__.py`:

```python
"""lingtai-kernel — minimal agent kernel: think, communicate, remember, host tools."""
from .types import UnknownToolError
from .config import AgentConfig
from .base_agent import BaseAgent
from .state import AgentState
from .message import Message, MSG_REQUEST, MSG_USER_INPUT

__all__ = [
    "BaseAgent",
    "AgentConfig",
    "AgentState",
    "Message",
    "MSG_REQUEST",
    "MSG_USER_INPUT",
    "UnknownToolError",
]
```

- [ ] **Step 2: Create kernel `llm/__init__.py`**

Write `../lingtai-kernel/src/lingtai_kernel/llm/__init__.py`:

```python
"""LLM protocol layer — adapter ABCs, session management, provider-agnostic types."""
from .base import LLMAdapter, ChatSession, LLMResponse, ToolCall, FunctionSchema
from .service import LLMService

__all__ = [
    "LLMAdapter",
    "ChatSession",
    "LLMResponse",
    "ToolCall",
    "FunctionSchema",
    "LLMService",
]
```

Note: NO adapter registration here. The kernel defines the protocol; `lingtai` registers adapters.

- [ ] **Step 3: Commit**

```bash
cd ../lingtai-kernel && git add -A && git commit -m "feat: add public API exports"
cd ../lingtai
```

---

### Task 7: Update all relative imports in lingtai-kernel

All kernel modules use `from .xxx import` (relative to `lingtai`). These must stay as relative imports but now relative to `lingtai_kernel`. Since the internal structure is identical, **most imports need zero changes** — they're already relative within the same package.

The only imports that need updating are those referencing non-kernel modules that were removed.

**Files:**
- Modify: `../lingtai-kernel/src/lingtai_kernel/base_agent.py` (verify no non-kernel imports remain)
- Modify: `../lingtai-kernel/src/lingtai_kernel/llm/service.py` (verify no adapter imports)

- [ ] **Step 1: Verify all imports are self-contained**

```bash
cd ../lingtai-kernel
grep -rn "from \." src/lingtai_kernel/ | grep -v __pycache__ | sort
```

Verify every relative import resolves within `lingtai_kernel/`. No references to `file_io`, `mcp`, `vision`, `search`, `capabilities`, `addons`, `_register`, or any adapter directory.

- [ ] **Step 2: Remove the `_register` import from llm/__init__.py if accidentally copied**

Verify `../lingtai-kernel/src/lingtai_kernel/llm/__init__.py` does NOT contain:
```python
from ._register import register_all_adapters
```
(It shouldn't — we wrote it fresh in Task 6.)

- [ ] **Step 3: Install and smoke-test**

```bash
cd ../lingtai-kernel
pip install -e .
python -c "from lingtai_kernel import BaseAgent; print('BaseAgent OK')"
python -c "from lingtai_kernel.llm import LLMService; print('LLMService OK')"
python -c "from lingtai_kernel.services.mail import MailService, TCPMailService; print('Mail OK')"
python -c "from lingtai_kernel.services.logging import LoggingService, JSONLLoggingService; print('Logging OK')"
python -c "from lingtai_kernel.intrinsics import ALL_INTRINSICS; print(f'Intrinsics: {len(ALL_INTRINSICS)} OK')"  # expect 3
```

Expected: All print OK. 4 intrinsics.

- [ ] **Step 4: Commit**

```bash
cd ../lingtai-kernel && git add -A && git commit -m "fix: verify all imports are self-contained"
cd ../lingtai
```

---

### Task 8: Copy and adapt kernel tests

Move pure kernel tests to `lingtai-kernel/tests/`. Tests that exercise Agent or capabilities stay in `lingtai/tests/`.

**Files:**
- Copy: kernel-only tests to `../lingtai-kernel/tests/`
- Modify: update `import lingtai.` → `import lingtai_kernel.` in copied tests

**Kernel-only tests** (test modules that ONLY import from kernel):
- `test_state.py` — AgentState enum
- `test_types.py` — UnknownToolError
- `test_message.py` — Message dataclass
- `test_workdir.py` — WorkingDir
- `test_tool_executor.py` — ToolExecutor
- `test_prompt.py` — SystemPromptManager
- `test_loop_guard.py` — LoopGuard
- `test_token_counter.py` — count_tokens
- `test_llm_service.py` — LLMService (context limits, registry — but registry tests use `_register.py` which stays in lingtai, so only the context limit tests move)
- `test_llm_utils.py` — send_with_timeout
- `test_streaming.py` — StreamingAccumulator
- `test_api_gate.py` — APICallGate
- `test_services_mail.py` — MailService/TCPMailService
- `test_services_logging.py` — LoggingService/JSONLLoggingService
- `test_session.py` — SessionManager

**Integration tests that stay in lingtai** (import Agent, capabilities, or adapters):
- `test_compaction.py` — imports `Agent`, not kernel-only
- `test_agent.py`, `test_agent_capabilities.py`
- `test_adapter_registry.py`
- `test_mail_intrinsic.py`, `test_eigen.py`, `test_system.py`, `test_intrinsics_comm.py`
- `test_override_intrinsic.py`, `test_memory.py`
- `test_conscience.py`, `test_psyche.py`
- `test_vision_capability.py`, `test_web_search_capability.py`
- `test_layers_*.py`, `test_addons.py`, `test_addon_gmail_*.py`
- `test_three_agent_email.py`, `test_silence_kill.py`
- `test_git_init.py`

- [ ] **Step 1: Copy kernel-only tests**

```bash
for f in test_state.py test_types.py test_message.py test_workdir.py \
         test_tool_executor.py test_prompt.py test_loop_guard.py \
         test_token_counter.py test_llm_utils.py test_streaming.py \
         test_api_gate.py test_services_mail.py test_services_logging.py \
         test_session.py; do
    cp tests/$f ../lingtai-kernel/tests/$f
done
touch ../lingtai-kernel/tests/__init__.py
```

- [ ] **Step 2: Update imports in copied tests**

In all copied test files, replace `lingtai.` with `lingtai_kernel.` and `from lingtai ` with `from lingtai_kernel `:

```bash
cd ../lingtai-kernel
# Use sed for bulk replacement
find tests -name "*.py" -exec sed -i '' \
    -e 's/from lingtai\./from lingtai_kernel./g' \
    -e 's/from lingtai /from lingtai_kernel /g' \
    -e 's/import lingtai\./import lingtai_kernel./g' \
    {} +
```

Manually verify a few files to ensure correctness.

- [ ] **Step 3: Create a subset of test_llm_service.py for kernel**

The kernel version should only include the context-limit tests, not the adapter registry or multimodal tests (those depend on `_register.py` in lingtai):

Write `../lingtai-kernel/tests/test_llm_service.py`:

```python
"""Tests for lingtai_kernel.llm.service — model registry and context limits."""
from lingtai_kernel.llm.service import get_context_limit, DEFAULT_CONTEXT_WINDOW


def test_get_context_limit_unknown():
    """Unknown models should return default 256k."""
    limit = get_context_limit("totally-unknown-model-xyz")
    assert limit == DEFAULT_CONTEXT_WINDOW


def test_get_context_limit_empty():
    """Empty model name returns default 256k."""
    assert get_context_limit("") == DEFAULT_CONTEXT_WINDOW
```

- [ ] **Step 4: Run kernel tests**

```bash
cd ../lingtai-kernel
python -m pytest tests/ -v
```

Expected: All PASS

- [ ] **Step 5: Commit**

```bash
cd ../lingtai-kernel && git add -A && git commit -m "test: add kernel-only tests"
cd ../lingtai
```

---

### Task 9: Update all imports in lingtai to use lingtai_kernel (BEFORE deletion)

Update imports first while old files still exist. Both import paths work during this step (old relative imports still resolve, new `lingtai_kernel` imports also work since the kernel is installed). This is safer than deleting first.

Do all the import updates described in the original Task 10 below. Once all imports are updated and tests pass, proceed to deletion.

- [ ] **Step 1: Install lingtai-kernel as editable dependency**

```bash
pip install -e ../lingtai-kernel
```

Then perform all import updates (Steps 1-9 from the original Task 10 section below).

- [ ] **Step 2: Run full test suite to verify imports work**

```bash
python -m pytest tests/ -v
```

Expected: All pass — both old files and new `lingtai_kernel` imports work simultaneously.

- [ ] **Step 3: Commit import updates**

```bash
git add src/lingtai/
git commit -m "refactor: update all imports to use lingtai_kernel"
```

---

### Task 10: Remove kernel modules from lingtai

Now that imports point to `lingtai_kernel`, delete the kernel source files from `src/lingtai/`.

**Files:**
- Delete: core modules, intrinsics/, services/mail.py, services/logging.py, llm protocol files
- Keep: agent.py, capabilities/, addons/, services/{file_io,vision,search,mcp}.py, llm/{adapters,_register,interface_converters}

- [ ] **Step 1: Delete kernel modules from lingtai**

```bash
# Core kernel modules
rm src/lingtai/base_agent.py
rm src/lingtai/config.py
rm src/lingtai/state.py
rm src/lingtai/types.py
rm src/lingtai/message.py
rm src/lingtai/workdir.py
rm src/lingtai/session.py
rm src/lingtai/tool_executor.py
rm src/lingtai/prompt.py
rm src/lingtai/logging.py
rm src/lingtai/token_counter.py
rm src/lingtai/llm_utils.py
rm src/lingtai/loop_guard.py
rm src/lingtai/tool_timing.py

# Intrinsics
rm -r src/lingtai/intrinsics/

# Kernel services (keep file_io, vision, search, mcp)
rm src/lingtai/services/mail.py
rm src/lingtai/services/logging.py

# LLM protocol (keep adapters, _register, interface_converters)
rm src/lingtai/llm/base.py
rm src/lingtai/llm/interface.py
rm src/lingtai/llm/service.py
rm src/lingtai/llm/api_gate.py
rm src/lingtai/llm/streaming.py
```

- [ ] **Step 2: Verify what remains**

```bash
find src/lingtai -name "*.py" -not -path "*__pycache__*" | sort
```

Expected remaining files:
```
src/lingtai/__init__.py
src/lingtai/agent.py
src/lingtai/capabilities/__init__.py
src/lingtai/capabilities/bash.py
src/lingtai/capabilities/compose.py
src/lingtai/capabilities/conscience.py
src/lingtai/capabilities/delegate.py
src/lingtai/capabilities/draw.py
src/lingtai/capabilities/edit.py
src/lingtai/capabilities/email.py
src/lingtai/capabilities/glob.py
src/lingtai/capabilities/grep.py
src/lingtai/capabilities/listen.py
src/lingtai/capabilities/psyche.py
src/lingtai/capabilities/read.py
src/lingtai/capabilities/talk.py
src/lingtai/capabilities/vision.py
src/lingtai/capabilities/web_search.py
src/lingtai/capabilities/write.py
src/lingtai/addons/__init__.py
src/lingtai/addons/gmail/__init__.py
src/lingtai/addons/gmail/manager.py
src/lingtai/addons/gmail/service.py
src/lingtai/services/__init__.py
src/lingtai/services/file_io.py
src/lingtai/services/mcp.py
src/lingtai/services/search.py
src/lingtai/services/vision.py
src/lingtai/llm/__init__.py
src/lingtai/llm/_register.py
src/lingtai/llm/interface_converters.py
src/lingtai/llm/anthropic/...
src/lingtai/llm/openai/...
src/lingtai/llm/gemini/...
src/lingtai/llm/minimax/...
src/lingtai/llm/custom/...
```

- [ ] **Step 3: Commit deletion**

```bash
git add -A
git commit -m "refactor: remove kernel modules from lingtai (now in lingtai-kernel)"
```

---

### Task 9 Import Update Details

Reference for Task 9 — every file in `src/lingtai/` that imports from kernel modules must be updated to import from `lingtai_kernel`.

**Files:**
- Modify: `src/lingtai/agent.py`
- Modify: `src/lingtai/capabilities/*.py` (all 16)
- Modify: `src/lingtai/capabilities/__init__.py`
- Modify: `src/lingtai/services/mcp.py`
- Modify: `src/lingtai/services/file_io.py` (if it imports kernel types)
- Modify: `src/lingtai/addons/gmail/__init__.py`, `service.py`
- Modify: `src/lingtai/llm/__init__.py`
- Modify: `src/lingtai/llm/_register.py`
- Modify: `src/lingtai/llm/interface_converters.py`
- Modify: `src/lingtai/llm/{anthropic,openai,gemini,minimax,custom}/adapter.py`
- Modify: `src/lingtai/llm/minimax/mcp_client.py`, `mcp_media_client.py`

- [ ] **Step 1: Update agent.py**

```python
# Old:
from .base_agent import BaseAgent
# New:
from lingtai_kernel.base_agent import BaseAgent
```

- [ ] **Step 2: Update capabilities/__init__.py**

Replace `TYPE_CHECKING` import:
```python
# Old:
from ..base_agent import BaseAgent
# New:
from lingtai_kernel.base_agent import BaseAgent
```

- [ ] **Step 3: Update capability modules**

For each capability that has `TYPE_CHECKING: from ..base_agent import BaseAgent`:
- `bash.py`, `edit.py`, `glob.py`, `grep.py`, `read.py`, `write.py`, `vision.py`, `web_search.py`, `psyche.py`, `conscience.py`

Change to:
```python
if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent
```

For capabilities that import `from ..logging import get_logger`:
- `compose.py`, `draw.py`, `talk.py`, `listen.py`

Change to:
```python
from lingtai_kernel.logging import get_logger
```

For `email.py` that imports from intrinsics:
```python
# Old:
from ..intrinsics.mail import (...)
# New:
from lingtai_kernel.intrinsics.mail import (...)
```

For `delegate.py` that imports kernel types:
```python
# Old:
from ..services.mail import TCPMailService
from ..config import AgentConfig
# New:
from lingtai_kernel.services.mail import TCPMailService
from lingtai_kernel.config import AgentConfig
# Note: `from ..agent import Agent` stays as-is (relative within lingtai)
```

- [ ] **Step 4: Update services/**

In `services/mcp.py`:
```python
# Old:
from ..logging import get_logger
# New:
from lingtai_kernel.logging import get_logger
```

In `services/vision.py` and `services/search.py` (TYPE_CHECKING imports):
```python
# Old:
from ..llm.service import LLMService
# New:
from lingtai_kernel.llm.service import LLMService
```

- [ ] **Step 5: Update addons/**

In `addons/__init__.py` (TYPE_CHECKING):
```python
# Old:
from ..base_agent import BaseAgent
# New:
from lingtai_kernel.base_agent import BaseAgent
```

In `addons/gmail/__init__.py`:
```python
# Old:
from ...services.mail import MailService, TCPMailService
# New:
from lingtai_kernel.services.mail import MailService, TCPMailService
```

In `addons/gmail/service.py`:
```python
# Old:
from ...services.mail import MailService
# New:
from lingtai_kernel.services.mail import MailService
```

In `addons/gmail/manager.py` (TYPE_CHECKING + deferred):
```python
# Old:
from ...base_agent import BaseAgent  # TYPE_CHECKING
from ...message import _make_message, MSG_REQUEST  # deferred
# New:
from lingtai_kernel.base_agent import BaseAgent
from lingtai_kernel.message import _make_message, MSG_REQUEST
```

- [ ] **Step 6: Update llm adapter modules**

For each adapter that imports `from ...logging`:
- `llm/anthropic/adapter.py`
- `llm/openai/adapter.py`
- `llm/gemini/adapter.py`
- `llm/minimax/adapter.py`
- `llm/minimax/mcp_client.py`
- `llm/minimax/mcp_media_client.py`

Change:
```python
# Old:
from ...logging import get_logger
# New:
from lingtai_kernel.logging import get_logger
```

For adapters that import from `..base`, `..interface`, `..streaming`, `..interface_converters`:
```python
# Old:
from ..base import LLMAdapter, ChatSession, ...
from ..interface import ChatInterface, ...
from ..streaming import StreamingAccumulator
# New:
from lingtai_kernel.llm.base import LLMAdapter, ChatSession, ...
from lingtai_kernel.llm.interface import ChatInterface, ...
from lingtai_kernel.llm.streaming import StreamingAccumulator
```

Note: `..interface_converters` stays as relative import since `interface_converters.py` is still in `lingtai.llm/`:
```python
from ..interface_converters import ...  # stays relative — both in lingtai.llm
```

But `interface_converters.py` itself imports from kernel:
```python
# Old:
from .interface import ChatInterface, TextBlock, ...
# New:
from lingtai_kernel.llm.interface import ChatInterface, TextBlock, ...
```

- [ ] **Step 7: Update llm/__init__.py**

```python
"""LLM adapter layer — multi-provider support with kernel protocol re-exports."""
from lingtai_kernel.llm.base import LLMAdapter, ChatSession, LLMResponse, ToolCall, FunctionSchema
from lingtai_kernel.llm.service import LLMService

__all__ = [
    "LLMAdapter",
    "ChatSession",
    "LLMResponse",
    "ToolCall",
    "FunctionSchema",
    "LLMService",
]

# Register built-in adapters on import
from ._register import register_all_adapters as _register_all_adapters
_register_all_adapters()
```

- [ ] **Step 8: Update llm/_register.py**

```python
# Old:
from .service import LLMService
# New:
from lingtai_kernel.llm.service import LLMService
```

- [ ] **Step 9: Smoke-test imports**

```bash
python -c "from lingtai.agent import Agent; print('Agent OK')"
python -c "from lingtai.capabilities.vision import VisionManager; print('vision OK')"
python -c "from lingtai.llm import LLMService; print('LLM OK')"
python -c "import lingtai; print('lingtai OK')"
```

- [ ] **Step 10: These import updates are performed as part of Task 9.**

---

### Task 11: Rewrite lingtai `__init__.py` and `llm/__init__.py` with re-exports

The main `__init__.py` must re-export kernel types so `from lingtai import BaseAgent` still works.

**Files:**
- Modify: `src/lingtai/__init__.py`

- [ ] **Step 1: Rewrite `__init__.py`**

```python
"""lingtai — generic AI agent framework with intrinsic tools, composable capabilities, and pluggable services."""

# Re-export kernel public API (backward compatibility)
from lingtai_kernel import (
    BaseAgent,
    AgentConfig,
    AgentState,
    Message,
    MSG_REQUEST,
    MSG_USER_INPUT,
    UnknownToolError,
)

# Own exports
from .agent import Agent

# Capabilities
from .capabilities import setup_capability
from .capabilities.bash import BashManager
from .capabilities.delegate import DelegateManager
from .capabilities.email import EmailManager

# Services — kernel
from lingtai_kernel.services.mail import MailService, TCPMailService
from lingtai_kernel.services.logging import LoggingService, JSONLLoggingService

# Services — lingtai
from .services.file_io import FileIOService, LocalFileIOService, GrepMatch
from .services.vision import VisionService, LLMVisionService
from .services.search import SearchService, LLMSearchService, SearchResult

__all__ = [
    # Core (re-exported from kernel)
    "BaseAgent",
    "Agent",
    "Message",
    "AgentState",
    "MSG_REQUEST",
    "MSG_USER_INPUT",
    "AgentConfig",
    "UnknownToolError",
    # Capabilities
    "setup_capability",
    "BashManager",
    "DelegateManager",
    "EmailManager",
    # Services
    "FileIOService",
    "LocalFileIOService",
    "GrepMatch",
    "MailService",
    "TCPMailService",
    "VisionService",
    "LLMVisionService",
    "SearchService",
    "LLMSearchService",
    "SearchResult",
    "LoggingService",
    "JSONLLoggingService",
]
```

- [ ] **Step 2: Smoke-test re-exports**

```bash
python -c "from lingtai import BaseAgent, Agent, AgentConfig, AgentState, Message; print('re-exports OK')"
python -c "from lingtai import MailService, TCPMailService, LoggingService; print('kernel services OK')"
python -c "from lingtai import FileIOService, LocalFileIOService; print('lingtai services OK')"
```

- [ ] **Step 3: Do NOT commit yet** — continue to Task 12.

---

### Task 12: Update lingtai pyproject.toml

Add `lingtai-kernel` as a hard dependency.

**Files:**
- Modify: `pyproject.toml`

- [ ] **Step 1: Add lingtai-kernel dependency**

In `pyproject.toml`, add to `[project]`:
```toml
dependencies = ["lingtai-kernel"]
```

Or during dev, keep `dependencies = []` and rely on `pip install -e ../lingtai-kernel`.

- [ ] **Step 2: Install both packages in dev mode**

```bash
pip install -e ../lingtai-kernel
pip install -e .
```

- [ ] **Step 3: Smoke-test the full stack**

```bash
python -c "import lingtai_kernel; print('kernel OK')"
python -c "import lingtai; print('lingtai OK')"
python -c "from lingtai import BaseAgent, Agent; print('imports OK')"
python -c "from lingtai.llm import LLMService; print(sorted(LLMService._adapter_registry.keys()))"
```

- [ ] **Step 4: Commit everything from Tasks 9-12**

```bash
git add -A
git commit -m "refactor: remove kernel modules, import from lingtai_kernel

Kernel modules now live in lingtai-kernel (separate repo).
All imports updated. Re-exports preserve backward compatibility."
```

---

### Task 13: Update lingtai tests to import from lingtai_kernel where needed

Tests in `lingtai/tests/` that directly imported kernel internals need updating.

**Files:**
- Modify: `tests/test_agent.py` and others that import kernel types directly

- [ ] **Step 1: Find broken test imports**

```bash
python -m pytest tests/ --collect-only 2>&1 | grep "ImportError\|ModuleNotFoundError" | head -20
```

- [ ] **Step 2: Fix imports in test files**

For each test file that fails to import:
- Replace `from lingtai.base_agent import BaseAgent` → `from lingtai import BaseAgent` (uses re-export)
- Replace `from lingtai.config import AgentConfig` → `from lingtai import AgentConfig`
- Replace `from lingtai.state import AgentState` → `from lingtai import AgentState`
- Replace `from lingtai.llm.service import LLMService` → `from lingtai.llm import LLMService` (re-exported via lingtai.llm)
- Replace `from lingtai.llm.base import ...` → `from lingtai.llm import ...`
- Replace `from lingtai.services.mail import ...` → `from lingtai import MailService, TCPMailService`
- Replace `from lingtai.services.logging import ...` → `from lingtai import LoggingService, JSONLLoggingService`
- Replace `from lingtai.intrinsics import ...` → `from lingtai_kernel.intrinsics import ...`

The strategy: use `lingtai` re-exports for public API, use `lingtai_kernel` directly only for kernel internals (like intrinsics, session, etc.).

- [ ] **Step 3: Run full test suite**

```bash
python -m pytest tests/ -v
```

Expected: All pass (511+)

- [ ] **Step 4: Commit**

```bash
git add tests/
git commit -m "test: update imports after kernel extraction"
```

---

### Task 14: Final verification

- [ ] **Step 1: Run lingtai-kernel tests**

```bash
cd ../lingtai-kernel && python -m pytest tests/ -v
```

Expected: All PASS

- [ ] **Step 2: Run lingtai tests**

```bash
cd ../lingtai && python -m pytest tests/ -v
```

Expected: All PASS (511+)

- [ ] **Step 3: Smoke-test the full stack**

```bash
# Kernel standalone
python -c "
from lingtai_kernel import BaseAgent
from lingtai_kernel.llm import LLMService, LLMAdapter
from lingtai_kernel.services.mail import MailService, TCPMailService
from lingtai_kernel.intrinsics import ALL_INTRINSICS
print(f'Kernel OK: {len(ALL_INTRINSICS)} intrinsics')
"

# lingtai with re-exports
python -c "
from lingtai import BaseAgent, Agent, AgentConfig
from lingtai import MailService, FileIOService
from lingtai.llm import LLMService
print(f'lingtai OK: {len(LLMService._adapter_registry)} adapters registered')
"

# Third-party extension simulation
python -c "
from lingtai_kernel import BaseAgent
from lingtai_kernel.llm import LLMAdapter, LLMService
print('Third-party kernel import OK')
"
```

- [ ] **Step 4: Verify lingtai-kernel has zero non-stdlib dependencies**

```bash
cd ../lingtai-kernel
python -c "
import importlib, ast
from pathlib import Path

stdlib = set(importlib.util.find_spec(m).origin for m in ['os','sys','json','threading'] if importlib.util.find_spec(m))
for f in Path('src/lingtai_kernel').rglob('*.py'):
    tree = ast.parse(f.read_text())
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                top = alias.name.split('.')[0]
                if top not in ('lingtai_kernel',) and importlib.util.find_spec(top) is None:
                    print(f'WARNING: {f} imports non-stdlib: {alias.name}')
print('Dependency check complete')
"
```

- [ ] **Step 5: Final commit and tag**

```bash
cd ../lingtai-kernel && git add -A && git commit -m "v0.1.0: lingtai-kernel initial release" && git tag v0.1.0
cd ../lingtai && git add -A && git commit -m "v0.1.0: lingtai depends on lingtai-kernel"
```
