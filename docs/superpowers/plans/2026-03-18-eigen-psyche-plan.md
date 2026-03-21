# Eigen/Psyche Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the current `anima` capability into a two-layer architecture: `eigen` (intrinsic — bare essentials) and `psyche` (capability — full inner architecture). Rename `anima` → `psyche`, extract `molt` + `memory` into a new `eigen` intrinsic that replaces the current `memory` intrinsic. Remove `forget` from agent-facing API (keep as internal method for auto-wipe).

**Architecture:**
- `eigen` replaces the current `memory` intrinsic. It provides: `memory` (edit/load `system/memory.md`) and `molt` (molt with briefing). `forget` is an internal method only.
- `psyche` replaces `anima` as a capability. It provides: `character` (identity), `library` (external brain), and upgrades eigen's `memory.edit` → `construct(ids, notes)` which builds memory from library entries + free text. `molt` is inherited from eigen.
- Psyche upgrades eigen the same way email upgrades the mail intrinsic — via `override_intrinsic`.

**Tech Stack:** Python, existing lingtai intrinsic/capability patterns

---

### File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai/intrinsics/eigen.py` | **Create** | New intrinsic: memory edit/load + molt. Replaces `memory` intrinsic. |
| `src/lingtai/capabilities/psyche.py` | **Create** | New capability: character + library + memory construct/load + molt (inherited). Replaces `anima`. |
| `src/lingtai/intrinsics/memory.py` | **Delete** | Replaced by eigen |
| `src/lingtai/capabilities/anima.py` | **Delete** | Replaced by psyche |
| `src/lingtai/base_agent.py` | **Modify** | Wire eigen as intrinsic (replacing memory), update molt pressure to reference eigen/psyche |
| `src/lingtai/agent.py` | **Modify** | Map `"psyche"` capability name (replace `"anima"`), keep `"anima"` as alias for backward compat during migration |
| `tests/test_eigen.py` | **Create** | Tests for eigen intrinsic |
| `tests/test_psyche.py` | **Create** | Tests for psyche capability |
| `tests/test_anima.py` | **Delete or update** | Migrate to test_psyche.py |

---

### Task 1: Create `eigen` intrinsic

**Files:**
- Create: `src/lingtai/intrinsics/eigen.py`
- Modify: `src/lingtai/base_agent.py` (wire eigen instead of memory)
- Create: `tests/test_eigen.py`

- [ ] **Step 1: Write eigen intrinsic**

`src/lingtai/intrinsics/eigen.py`:

```python
"""Eigen intrinsic — bare essentials of agent self.

Objects:
    memory — edit/load system/memory.md (agent's working notes)
    context — molt (molt with briefing)

Internal (not exposed to agent):
    _context_forget — nuclear wipe, used by auto-forget only
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "object": {
            "type": "string",
            "enum": ["memory", "context"],
            "description": (
                "memory: your working notes (system/memory.md). "
                "context: manage conversation context."
            ),
        },
        "action": {
            "type": "string",
            "enum": ["edit", "load", "molt"],
            "description": (
                "memory: edit | load.\n"
                "context: molt."
            ),
        },
        "content": {
            "type": "string",
            "description": "Text content for memory edit.",
        },
        "summary": {
            "type": "string",
            "description": (
                "For context molt: a briefing to your future self — "
                "the ONLY thing you will see after molt. "
                "Write what you are doing, what you have found, "
                "what remains to be done, and who you are working with. "
                "~10000 tokens max."
            ),
        },
    },
    "required": ["object", "action"],
}

DESCRIPTION = (
    "Core self-management — working notes and context control.\n"
    "memory: edit to write your working notes (system/memory.md), "
    "load to inject them into your active prompt.\n"
    "context: molt to molt — write a briefing to your future self, "
    "your conversation history is wiped and your summary becomes the new starting context. "
    "Before molting, save important data elsewhere first."
)


def handle(agent, args: dict) -> dict:
    """Handle eigen tool — memory and context management."""
    obj = args.get("object", "")
    action = args.get("action", "")

    if obj == "memory":
        if action == "edit":
            return _memory_edit(agent, args)
        elif action == "load":
            return _memory_load(agent, args)
        else:
            return {"error": f"Unknown memory action: {action}. Use edit or load."}
    elif obj == "context":
        if action == "molt":
            return _context_molt(agent, args)
        else:
            return {"error": f"Unknown context action: {action}. Use molt."}
    else:
        return {"error": f"Unknown object: {obj}. Use memory or context."}


def _memory_edit(agent, args: dict) -> dict:
    """Write content to system/memory.md."""
    content = args.get("content", "")
    if not content:
        return {"error": "content is required for memory edit."}

    mem_path = agent._working_dir / "system" / "memory.md"
    mem_path.parent.mkdir(parents=True, exist_ok=True)
    mem_path.write_text(content)

    agent._log("eigen_memory_edit", length=len(content))
    return {"status": "ok", "length": len(content)}


def _memory_load(agent, args: dict) -> dict:
    """Load system/memory.md into the system prompt."""
    mem_path = agent._working_dir / "system" / "memory.md"
    if not mem_path.is_file():
        return {"status": "ok", "loaded": False, "message": "No memory file found."}

    content = mem_path.read_text().strip()
    if not content:
        return {"status": "ok", "loaded": False, "message": "Memory file is empty."}

    agent.update_system_prompt("memory", content)

    # Git commit the memory state
    workdir = getattr(agent, "_workdir", None)
    if workdir is not None:
        workdir.diff_and_commit(f"memory load")

    agent._log("eigen_memory_load", length=len(content))
    return {"status": "ok", "loaded": True, "length": len(content)}


def _context_molt(agent, args: dict) -> dict:
    """Agent molt: summary IS the briefing, wipe + re-inject."""
    summary = args.get("summary")
    if summary is None:
        return {"error": "summary is required — write a briefing to your future self."}
    if not summary.strip():
        return {"error": "summary cannot be empty — write what you need to remember."}

    if agent._chat is None:
        return {"error": "No active chat session to molt."}

    before_tokens = agent._chat.interface.estimate_context_tokens()

    # Wipe context and start fresh session
    agent._session._chat = None
    agent._session._interaction_id = None
    agent._session.ensure_session()

    # Inject the agent's summary as the opening context
    from ..llm.interface import TextBlock
    iface = agent._session._chat.interface
    iface.add_user_message(f"[Previous conversation summary]\n{summary}")
    iface.add_assistant_message(
        [TextBlock(text="Understood. I have my previous context restored.")],
    )

    after_tokens = iface.estimate_context_tokens()

    # Reset molt warnings
    if hasattr(agent._session, "_molt_warnings"):
        agent._session._molt_warnings = 0

    agent._log(
        "eigen_molt",
        before_tokens=before_tokens,
        after_tokens=after_tokens,
    )

    return {
        "status": "ok",
        "before_tokens": before_tokens,
        "after_tokens": after_tokens,
    }


def context_forget(agent) -> dict:
    """Nuclear context wipe. Internal only — not exposed in SCHEMA.

    Called by base_agent auto-forget after ignored molt warnings.
    """
    if agent._chat is None:
        return {"error": "No active chat session."}

    before_tokens = agent._chat.interface.estimate_context_tokens()
    agent._session._chat = None
    agent._session._interaction_id = None
    agent._session.ensure_session()
    agent._log("eigen_forget", before_tokens=before_tokens)

    return {"status": "ok", "freed_tokens": before_tokens}
```

- [ ] **Step 2: Wire eigen into base_agent.py**

In `base_agent.py`, replace the `memory` intrinsic registration with `eigen`:

Find where intrinsics are registered (look for `"memory"` in the intrinsics dict setup) and change:
```python
# Old:
from .intrinsics import memory
self._register_intrinsic("memory", memory.SCHEMA, memory.DESCRIPTION, memory.handle)

# New:
from .intrinsics import eigen
self._register_intrinsic("eigen", eigen.SCHEMA, eigen.DESCRIPTION, eigen.handle)
```

Also update the molt auto-forget in `_handle_request` to use `eigen.context_forget(self)` instead of `anima._context_forget({})`.

- [ ] **Step 3: Write tests**

`tests/test_eigen.py` — test memory edit/load, molt, and forget:

```python
def test_eigen_memory_edit(tmp_path):
    """eigen memory edit writes to system/memory.md."""

def test_eigen_memory_load(tmp_path):
    """eigen memory load injects into system prompt."""

def test_eigen_molt_uses_summary(tmp_path):
    """molt wipes context and re-injects agent's summary."""

def test_eigen_molt_rejects_empty(tmp_path):
    """molt with empty summary returns error."""

def test_eigen_forget_wipes_context(tmp_path):
    """context_forget nuclear wipes the session."""
```

- [ ] **Step 4: Run tests**

Run: `source venv/bin/activate && python -m pytest tests/test_eigen.py -v`

- [ ] **Step 5: Smoke test**

Run: `python -c "import lingtai"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/intrinsics/eigen.py src/lingtai/base_agent.py tests/test_eigen.py
git commit -m "feat: eigen intrinsic — memory edit/load + molt"
```

---

### Task 2: Create `psyche` capability (replaces anima)

**Files:**
- Create: `src/lingtai/capabilities/psyche.py`
- Create: `tests/test_psyche.py`

- [ ] **Step 1: Create psyche.py**

Psyche provides three objects: `character`, `library`, `memory` (upgraded with `construct`).

The `construct` action replaces eigen's `memory.edit`: it accepts `ids` (library entry IDs to include) + `notes` (free text to prepend/append). It reads the library entries, combines them with the agent's notes, and writes `system/memory.md`.

```python
# memory construct:
def _memory_construct(self, args: dict) -> dict:
    ids = args.get("ids", [])
    notes = args.get("notes", "")

    parts = []
    if notes:
        parts.append(notes)

    # Load library entries by ID
    for entry_id in ids:
        entry = self._load_library_entry(entry_id)
        if entry:
            parts.append(f"### [{entry['id']}] {entry['title']}\n{entry['content']}")

    content = "\n\n".join(parts)
    mem_path = self._working_dir / "system" / "memory.md"
    mem_path.parent.mkdir(parents=True, exist_ok=True)
    mem_path.write_text(content + "\n")

    # Git commit
    self._workdir.diff_and_commit("memory construct")

    return {"status": "ok", "entries": len(ids), "length": len(content)}
```

SCHEMA adds `ids` field:
```python
"ids": {
    "type": "array",
    "items": {"type": "string"},
    "description": "Library entry IDs to include in memory (for memory construct).",
},
"notes": {
    "type": "string",
    "description": "Free text notes to include in memory (for memory construct).",
},
```

Memory actions when psyche is active: `construct` (replaces edit), `load` (inherited from eigen).

Psyche upgrades eigen via `override_intrinsic("eigen")` — takes over the tool, adds character + library + upgraded memory, but delegates molt to eigen's handler.

- [ ] **Step 2: Migrate character + library code from anima.py**

Copy the character and library logic from `anima.py` into `psyche.py`. The AnimaManager class becomes PsycheManager. Key changes:
- `object=memory, action=edit` → `object=memory, action=construct` (with ids + notes)
- `object=memory, action=load` → delegates to eigen's `_memory_load`
- `object=context, action=molt` → delegates to eigen's `_context_molt`
- `object=context, action=forget` → **removed from schema** (internal only)

- [ ] **Step 3: Update SCHEMA and DESCRIPTION**

SCHEMA enum for objects: `["character", "library", "memory", "context"]`

SCHEMA enum for actions: `["update", "diff", "load", "submit", "filter", "view", "consolidate", "delete", "construct", "molt"]`

DESCRIPTION:
```
"Self-knowledge management — identity, knowledge, memory, and context.\n"
"character: your evolving identity. update | diff | load.\n"
"library: your external brain — persists across molts, reboots, kills. "
"submit | filter | view | consolidate | delete. "
"Use filter/view to retrieve anytime.\n"
"memory: construct your active memory from library entries + notes. "
"construct(ids=[...], notes='...') builds system/memory.md from selected library entries "
"and your free text. load injects it into your prompt.\n"
"context: molt to molt — write a briefing to your future self.\n"
```

- [ ] **Step 4: Write tests**

`tests/test_psyche.py`:
```python
def test_memory_construct_with_ids_and_notes(tmp_path):
    """construct builds memory from library entries + free text."""

def test_memory_construct_notes_only(tmp_path):
    """construct with only notes (no ids) works."""

def test_memory_construct_ids_only(tmp_path):
    """construct with only ids (no notes) works."""

def test_character_update_and_load(tmp_path):
    """character update + load cycle works."""

def test_library_submit_and_filter(tmp_path):
    """library submit + filter retrieves entries."""

def test_molt_delegates_to_eigen(tmp_path):
    """psyche molt calls through to eigen's handler."""

def test_forget_not_in_schema():
    """forget is not exposed in psyche's SCHEMA actions."""
```

- [ ] **Step 5: Run tests**

Run: `source venv/bin/activate && python -m pytest tests/test_psyche.py -v`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/psyche.py tests/test_psyche.py
git commit -m "feat: psyche capability — character + library + memory construct"
```

---

### Task 3: Wire psyche capability + clean up

**Files:**
- Modify: `src/lingtai/agent.py` (register `"psyche"` capability, keep `"anima"` as alias)
- Modify: `src/lingtai/capabilities/__init__.py` (if capability registry exists)
- Delete: `src/lingtai/intrinsics/memory.py` (replaced by eigen)
- Delete: `src/lingtai/capabilities/anima.py` (replaced by psyche)
- Modify: `app/web/examples/orchestrator.py` (rename `anima` → `psyche` in capabilities)

- [ ] **Step 1: Register psyche in agent.py**

Find where capabilities are mapped to setup functions. Add `"psyche"` pointing to `psyche.setup`. Keep `"anima"` as an alias → `psyche.setup` for backward compatibility.

- [ ] **Step 2: Delete old files**

```bash
rm src/lingtai/intrinsics/memory.py
rm src/lingtai/capabilities/anima.py
rm tests/test_anima.py
```

- [ ] **Step 3: Update orchestrator example**

In `app/web/examples/orchestrator.py`, change capabilities:
```python
# Old:
"anima": {}, "conscience": {"interval": 30},
# New:
"psyche": {}, "conscience": {"interval": 30},
```

Also update CHARACTER prompt and COVENANT to reference `psyche` and `eigen` instead of `anima`.

- [ ] **Step 4: Update molt warnings in base_agent.py**

Change all `anima(object=context, action=molt, summary=...)` references in warning messages to `eigen(object=context, action=molt, summary=...)` (since molt lives in eigen, though psyche inherits it — the tool name the agent sees depends on whether psyche is active).

Actually: when psyche upgrades eigen, the tool name becomes `psyche`. When only eigen is present, the tool name is `eigen`. The warning messages should detect which is available:

```python
tool_name = "psyche" if "psyche" in cap_managers else "eigen"
f"{tool_name}(object=context, action=molt, summary=<briefing>)"
```

- [ ] **Step 5: Update all imports/references**

Grep for remaining `anima` and `memory` intrinsic references:
```bash
grep -r "anima\|from.*intrinsics.*memory" src/lingtai/ --include="*.py"
```
Fix any remaining references.

- [ ] **Step 6: Run full test suite**

Run: `source venv/bin/activate && python -m pytest tests/ -v`

- [ ] **Step 7: Smoke test**

Run: `python -c "import lingtai"`

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: anima→psyche, memory→eigen — two-layer self architecture"
```

---

### Task 4: Update CLAUDE.md and docs

**Files:**
- Modify: `CLAUDE.md` (update architecture docs: intrinsics list, capabilities list, descriptions)

- [ ] **Step 1: Update intrinsics section**

Replace `memory` with `eigen` in the intrinsics table. Update description.

- [ ] **Step 2: Update capabilities section**

Replace `anima` with `psyche` in the capabilities table. Update description to mention `construct` action and eigen relationship.

- [ ] **Step 3: Update any references**

Grep for `anima` in CLAUDE.md and update to `psyche`.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for eigen/psyche architecture"
```
