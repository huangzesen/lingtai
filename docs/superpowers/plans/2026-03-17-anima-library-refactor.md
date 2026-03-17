# Anima Library Refactor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor anima's flat memory into a structured knowledge library with five-level entries (id, title, summary, content, supplementary) and selective loading into the active system prompt.

**Architecture:** The `memory` object splits into two objects: `library` (persistent archive in `library.json`) and `memory` (active working set in `memory.md`). Library entries have academic-paper structure: title, summary (1-3 sentences for filtering), content (up to 500 words, the main body), and optional supplementary material (unbounded). The agent browses the library via `filter`, reads entries via `view` (with `depth` control), and selectively loads entries into memory. `memory load` injects only id + title + content into the system prompt — summaries serve filtering only.

**Tech Stack:** Python 3.11+, dataclasses, json, re (regex for filter), pytest

---

## File Structure

| File | Responsibility |
|------|---------------|
| Modify: `src/stoai/capabilities/anima.py` | Replace memory actions with library + memory objects, new entry shape, new actions (filter, view, delete) |
| Modify: `src/stoai/base_agent.py` | Skip `memory.md` persistence in `stop()` when anima owns it; handle `ltm` migration |
| Modify: `tests/test_anima.py` | Replace all memory tests with library + memory tests |

## Design Reference

### Entry shape in `library.json`:

```json
{
  "id": "a1b2c3d4",
  "title": "TCP Mail Retry Logic",
  "summary": "Covers retry backoff and failure modes for TCP mail.",
  "content": "Up to 500 words of main body text...",
  "supplementary": "Optional unbounded extended material...",
  "created_at": "2026-03-17T00:00:00+00:00"
}
```

### Valid action matrix:

```
role:     update | diff | load          (unchanged)
library:  submit | filter | view | consolidate | delete
memory:   load | diff
context:  compact                       (unchanged)
```

### Action parameters and return values:

| Object | Action | Required Params | Optional Params | Returns |
|--------|--------|----------------|----------------|---------|
| `library` | `submit` | `title`, `summary`, `content` | `supplementary` | `{status, id}` |
| `library` | `filter` | — | `pattern` (regex), `limit` (int) | `{status, entries: [{id, title, summary}]}` |
| `library` | `view` | `ids` | `depth` (`"content"` default, or `"supplementary"`) | `{status, entries: [{id, title, summary, content, ?supplementary}]}` |
| `library` | `consolidate` | `ids`, `title`, `summary`, `content` | `supplementary` | `{status, id, removed}` |
| `library` | `delete` | `ids` | — | `{status, removed}` |
| `memory` | `load` | `ids` | — | `{status, size_bytes, content_preview, diff}` |
| `memory` | `diff` | — | — | `{status, path, git_diff}` (inherited, discouraged) |

### Three-level access model:

| Level | Action | Returns | Persistence |
|-------|--------|---------|-------------|
| Browse | `library filter` | id + title + summary | In context (ephemeral) |
| Read | `library view depth=content` | + content | In context (ephemeral) |
| Deep read | `library view depth=supplementary` | + supplementary | In context (ephemeral) |
| Remember | `memory load ids` | id + title + content → system prompt | Persistent across turns |

---

## Task 1: Update SCHEMA and DESCRIPTION constants

**Files:**
- Modify: `src/stoai/capabilities/anima.py:25-85`

- [ ] **Step 1: Write failing test for new schema structure**

```python
# In tests/test_anima.py — add at top of file after existing imports

def test_anima_schema_has_library_fields():
    """Schema should include title, summary, supplementary, pattern, limit, depth."""
    from stoai.capabilities.anima import SCHEMA
    props = SCHEMA["properties"]
    assert "title" in props
    assert "summary" in props
    assert "supplementary" in props
    assert "pattern" in props
    assert "limit" in props
    assert "depth" in props
    assert props["depth"]["enum"] == ["content", "supplementary"]
    # object enum should include library
    assert "library" in props["object"]["enum"]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_anima.py::test_anima_schema_has_library_fields -v`
Expected: FAIL — `title` not in schema properties, `library` not in object enum

- [ ] **Step 3: Update SCHEMA and DESCRIPTION in anima.py**

Replace `SCHEMA` (lines 25-72) and `DESCRIPTION` (lines 74-85) with:

```python
SCHEMA = {
    "type": "object",
    "properties": {
        "object": {
            "type": "string",
            "enum": ["role", "library", "memory", "context"],
            "description": (
                "role: the agent's identity (system/covenant.md + system/character.md).\n"
                "library: the agent's knowledge archive (system/library.json).\n"
                "memory: the agent's active working memory "
                "(system/memory.md, loaded from library).\n"
                "context: the agent's conversation context window."
            ),
        },
        "action": {
            "type": "string",
            "enum": [
                "update", "diff", "load",
                "submit", "filter", "view", "consolidate", "delete",
                "compact",
            ],
            "description": (
                "role: update | diff | load.\n"
                "library: submit | filter | view | consolidate | delete.\n"
                "memory: load | diff.\n"
                "context: compact."
            ),
        },
        "title": {
            "type": "string",
            "description": (
                "Entry title — one line. "
                "Required for library submit and consolidate."
            ),
        },
        "summary": {
            "type": "string",
            "description": (
                "Entry summary — 1-3 sentences. Used for filtering. "
                "Required for library submit and consolidate."
            ),
        },
        "content": {
            "type": "string",
            "description": (
                "Text content — for role update (character), "
                "library submit/consolidate (main body, up to 500 words), "
                "or other actions that accept text."
            ),
        },
        "supplementary": {
            "type": "string",
            "description": (
                "Extended material for a library entry — unbounded. "
                "Optional for library submit and consolidate. "
                "Use when the content alone doesn't capture full detail."
            ),
        },
        "ids": {
            "type": "array",
            "items": {"type": "string"},
            "description": (
                "Entry IDs — for library view, consolidate, delete, "
                "and memory load."
            ),
        },
        "pattern": {
            "type": "string",
            "description": (
                "Regex pattern for library filter. "
                "Searches across titles, summaries, and content. "
                "Omit to list all entries."
            ),
        },
        "limit": {
            "type": "integer",
            "description": "Maximum entries to return for library filter.",
        },
        "depth": {
            "type": "string",
            "enum": ["content", "supplementary"],
            "description": (
                "Depth for library view. "
                "'content' (default): id + title + summary + content. "
                "'supplementary': id + title + summary + content + supplementary."
            ),
        },
        "prompt": {
            "type": "string",
            "description": (
                "Compaction guidance — what to preserve, what to compress. "
                "Required for context compact. Can be empty."
            ),
        },
    },
    "required": ["object", "action"],
}

DESCRIPTION = (
    "Self-knowledge management — identity, knowledge library, active memory, "
    "and context control.\n"
    "role: update your character, diff to review, load to apply.\n"
    "library: your knowledge archive. submit entries (title + summary + content, "
    "optional supplementary). filter to browse (returns id + title + summary, "
    "optional regex pattern and limit). view to read entries at depth "
    "(content or supplementary, default content). consolidate to merge entries. "
    "delete to remove entries. Be a thoughtful librarian — write clear titles, "
    "concise summaries (1-3 sentences), and structured content (up to 500 words).\n"
    "memory: load selected library entries into active memory by IDs "
    "(injects id + title + content into system prompt). "
    "diff to see uncommitted changes (inherited, rarely needed).\n"
    "context: compact to proactively free context space — check usage via "
    "status show first.\n"
    "Workflow: filter to browse → view if you need detail → load to remember."
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_anima.py::test_anima_schema_has_library_fields -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from stoai.capabilities.anima import SCHEMA, DESCRIPTION; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add src/stoai/capabilities/anima.py tests/test_anima.py
git commit -m "refactor(anima): update schema for library/memory split"
```

---

## Task 2: Add `_anima_owns_memory` flag to BaseAgent

**Files:**
- Modify: `src/stoai/base_agent.py:343-348` (stop method)
- Modify: `tests/test_anima.py`

**Why:** `BaseAgent.stop()` unconditionally writes the `memory` prompt section back to `system/memory.md`. When anima is active, anima owns `memory.md` (rendering it from selected library entries via `memory load`). If `stop()` overwrites it, the git-committed version becomes dirty. Worse, if the agent never called `memory load` this session, `stop()` writes an empty file, wiping memory from the previous session.

- [ ] **Step 1: Write failing test**

```python
def test_anima_stop_does_not_overwrite_memory_md(tmp_path):
    """When anima is active, stop() should not write memory.md."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    # Write something to memory.md manually to simulate previous session state
    mem_file = agent.working_dir / "system" / "memory.md"
    mem_file.parent.mkdir(exist_ok=True)
    mem_file.write_text("previous session memory")
    agent.stop()
    assert mem_file.read_text() == "previous session memory"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_anima.py::test_anima_stop_does_not_overwrite_memory_md -v`
Expected: FAIL — `stop()` overwrites memory.md with empty string (no `memory` section in prompt manager)

- [ ] **Step 3: Add flag to BaseAgent**

In `base_agent.py`, add a flag in `__init__` (around line 139, near other instance vars):

```python
self._anima_owns_memory = False
```

In `stop()` (lines 343-348), guard the memory persistence:

```python
# Persist memory from prompt manager to file
if not self._anima_owns_memory:
    memory_content = self._prompt_manager.read_section("memory") or ""
    memory_file = self._working_dir / "system" / "memory.md"
    if memory_file.is_file() or memory_content:
        memory_file.parent.mkdir(exist_ok=True)
        memory_file.write_text(memory_content)
```

- [ ] **Step 4: Set flag in anima setup**

In `anima.py` `setup()` function, add after `override_intrinsic`:

```python
agent._anima_owns_memory = True
```

(The full `setup()` rewrite with migration comes in Task 3, after `_library_submit` exists.)

- [ ] **Step 5: Run test to verify it passes**

Run: `python -m pytest tests/test_anima.py::test_anima_stop_does_not_overwrite_memory_md -v`
Expected: PASS

- [ ] **Step 6: Smoke-test import**

Run: `python -c "import stoai; print('OK')"`
Expected: `OK`

- [ ] **Step 7: Commit**

```bash
git add src/stoai/base_agent.py src/stoai/capabilities/anima.py tests/test_anima.py
git commit -m "fix(anima): prevent stop() from overwriting memory.md when anima active"
```

---

## Task 3: Rename backing store, add `re` import, update entry shape, and migrate ltm

**Files:**
- Modify: `src/stoai/capabilities/anima.py:88-160` (AnimaManager.__init__, persistence methods)
- Modify: `tests/test_anima.py`

- [ ] **Step 1: Write failing test for new entry shape**

```python
def test_library_submit_creates_structured_entry(tmp_path):
    """Submit should require title, summary, content and store structured entry."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "library", "action": "submit",
        "title": "TCP Retry Logic",
        "summary": "Covers retry backoff and failure modes.",
        "content": "The TCP mail service uses exponential backoff...",
    })
    assert result["status"] == "ok"
    assert "id" in result
    # Check library.json structure
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert len(data["entries"]) == 1
    entry = data["entries"][0]
    assert entry["title"] == "TCP Retry Logic"
    assert entry["summary"] == "Covers retry backoff and failure modes."
    assert entry["content"] == "The TCP mail service uses exponential backoff..."
    assert entry["supplementary"] == ""
    assert "created_at" in entry
    agent.stop(timeout=1.0)


def test_library_submit_with_supplementary(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "library", "action": "submit",
        "title": "Mail Protocol",
        "summary": "FIFO mail queue internals.",
        "content": "Main body here.",
        "supplementary": "Extended appendix data...",
    })
    assert result["status"] == "ok"
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert data["entries"][0]["supplementary"] == "Extended appendix data..."
    agent.stop(timeout=1.0)


def test_library_submit_requires_title_summary_content(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    # Missing title
    r1 = mgr.handle({"object": "library", "action": "submit",
                      "summary": "s", "content": "c"})
    assert "error" in r1
    # Missing summary
    r2 = mgr.handle({"object": "library", "action": "submit",
                      "title": "t", "content": "c"})
    assert "error" in r2
    # Missing content
    r3 = mgr.handle({"object": "library", "action": "submit",
                      "title": "t", "summary": "s"})
    assert "error" in r3
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_anima.py::test_library_submit_creates_structured_entry tests/test_anima.py::test_library_submit_with_supplementary tests/test_anima.py::test_library_submit_requires_title_summary_content -v`
Expected: FAIL — `library` not in `_VALID_ACTIONS`

- [ ] **Step 3: Update AnimaManager internals**

In `AnimaManager.__init__` (around line 91-109), rename paths:

```python
# Replace these lines:
self._memory_md = system_dir / "memory.md"
self._memory_json = system_dir / "memory.json"

# With:
self._memory_md = system_dir / "memory.md"
self._library_json = system_dir / "library.json"
```

Add `import re` to the top-level imports (alongside `hashlib`, `json`, `os`, etc.).

Update `_load_entries` and `_save_entries` to use `self._library_json` instead of `self._memory_json`.

Add migration in `_load_entries`: if an entry lacks `title` (legacy flat format), auto-migrate it:

```python
def _load_entries(self) -> list[dict]:
    """Load entries from library.json, or return empty list if missing."""
    if not self._library_json.is_file():
        return []
    try:
        data = json.loads(self._library_json.read_text())
        entries = data.get("entries", [])
        # Migrate legacy flat entries (pre-library format)
        for e in entries:
            if "title" not in e:
                e["title"] = e.get("content", "")[:50] or "Untitled"
                e["summary"] = e.get("content", "")[:200]
                e["supplementary"] = ""
        return entries
    except (json.JSONDecodeError, OSError):
        return []
```

Remove `_render_memory_md` — memory.md rendering now happens in `_memory_load` based on selected IDs.

Remove `self._character_path.write_text("")` in `__init__` — leave that to role_update.

Update `_VALID_ACTIONS` (line 164-168):

```python
_VALID_ACTIONS: dict[str, set[str]] = {
    "role": {"update", "diff", "load"},
    "library": {"submit", "filter", "view", "consolidate", "delete"},
    "memory": {"load", "diff"},
    "context": {"compact"},
}
```

- [ ] **Step 4: Implement `_library_submit`**

Replace the old `_memory_submit` with:

```python
def _library_submit(self, args: dict) -> dict:
    title = args.get("title", "").strip()
    summary = args.get("summary", "").strip()
    content = args.get("content", "").strip()
    supplementary = args.get("supplementary", "").strip()
    if not title:
        return {"error": "title is required for library submit."}
    if not summary:
        return {"error": "summary is required for library submit."}
    if not content:
        return {"error": "content is required for library submit."}
    now = datetime.now(timezone.utc).isoformat()
    entry_id = self._make_id(title + content, now)
    self._entries.append({
        "id": entry_id,
        "title": title,
        "summary": summary,
        "content": content,
        "supplementary": supplementary,
        "created_at": now,
    })
    self._save_entries()
    return {"status": "ok", "id": entry_id}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `python -m pytest tests/test_anima.py::test_library_submit_creates_structured_entry tests/test_anima.py::test_library_submit_with_supplementary tests/test_anima.py::test_library_submit_requires_title_summary_content -v`
Expected: PASS

- [ ] **Step 6: Smoke-test import**

Run: `python -c "import stoai.capabilities.anima; print('OK')"`
Expected: `OK`

- [ ] **Step 7: Write test for ltm migration**

```python
def test_anima_migrates_ltm_to_library(tmp_path):
    """If ltm is provided, anima should migrate it to a library entry."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        ltm="I know about CDF format",
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "filter"})
    assert len(result["entries"]) == 1
    assert "CDF" in result["entries"][0]["summary"]
    agent.stop(timeout=1.0)
```

- [ ] **Step 8: Update setup() to migrate existing memory.md to library**

Now that `_library_submit` exists and `_VALID_ACTIONS` includes `library`, the migration can call `mgr.handle()` safely:

```python
def setup(agent: "BaseAgent") -> AnimaManager:
    """Set up anima capability — self-knowledge management."""
    mgr = AnimaManager(agent)
    mgr._original_system = agent.override_intrinsic("system")
    agent._anima_owns_memory = True

    # Migrate existing memory.md content to library as a seed entry
    memory_file = agent._working_dir / "system" / "memory.md"
    if memory_file.is_file():
        existing = memory_file.read_text().strip()
        if existing and not mgr._entries:
            mgr.handle({
                "object": "library", "action": "submit",
                "title": "Initial memory (migrated)",
                "summary": existing[:200],
                "content": existing,
            })

    agent.add_tool(
        "anima", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )
    return mgr
```

- [ ] **Step 9: Run migration test**

Run: `python -m pytest tests/test_anima.py::test_anima_migrates_ltm_to_library -v`
Expected: PASS

- [ ] **Step 10: Smoke-test import**

Run: `python -c "import stoai.capabilities.anima; print('OK')"`
Expected: `OK`

- [ ] **Step 11: Commit**

```bash
git add src/stoai/capabilities/anima.py tests/test_anima.py
git commit -m "refactor(anima): library.json backing store, structured entries, ltm migration"
```

---

## Task 4: Implement library filter action

**Files:**
- Modify: `src/stoai/capabilities/anima.py`
- Modify: `tests/test_anima.py`

- [ ] **Step 1: Write failing tests**

```python
def test_library_filter_all(tmp_path):
    """Filter with no pattern returns all entries."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    mgr.handle({"object": "library", "action": "submit",
                 "title": "Entry A", "summary": "About A.", "content": "Details A."})
    mgr.handle({"object": "library", "action": "submit",
                 "title": "Entry B", "summary": "About B.", "content": "Details B."})
    result = mgr.handle({"object": "library", "action": "filter"})
    assert result["status"] == "ok"
    assert len(result["entries"]) == 2
    # Each entry has id, title, summary only
    for e in result["entries"]:
        assert "id" in e
        assert "title" in e
        assert "summary" in e
        assert "content" not in e
        assert "supplementary" not in e
    agent.stop(timeout=1.0)


def test_library_filter_with_pattern(tmp_path):
    """Filter with regex pattern matches against title, summary, and content."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    mgr.handle({"object": "library", "action": "submit",
                 "title": "TCP Retry", "summary": "About TCP.", "content": "Backoff logic."})
    mgr.handle({"object": "library", "action": "submit",
                 "title": "HTTP Caching", "summary": "About HTTP.", "content": "Cache rules."})
    result = mgr.handle({"object": "library", "action": "filter", "pattern": "TCP"})
    assert len(result["entries"]) == 1
    assert result["entries"][0]["title"] == "TCP Retry"
    agent.stop(timeout=1.0)


def test_library_filter_with_limit(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    for i in range(5):
        mgr.handle({"object": "library", "action": "submit",
                     "title": f"Entry {i}", "summary": f"About {i}.", "content": f"Details {i}."})
    result = mgr.handle({"object": "library", "action": "filter", "limit": 3})
    assert len(result["entries"]) == 3
    agent.stop(timeout=1.0)


def test_library_filter_matches_content(tmp_path):
    """Filter should match against content field, not just title."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    mgr.handle({"object": "library", "action": "submit",
                 "title": "Entry A", "summary": "About A.", "content": "Uses exponential backoff."})
    mgr.handle({"object": "library", "action": "submit",
                 "title": "Entry B", "summary": "About B.", "content": "Simple linear scan."})
    result = mgr.handle({"object": "library", "action": "filter", "pattern": "exponential"})
    assert len(result["entries"]) == 1
    assert result["entries"][0]["title"] == "Entry A"
    agent.stop(timeout=1.0)


def test_library_filter_invalid_regex(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "filter", "pattern": "[invalid"})
    assert "error" in result
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_anima.py -k "test_library_filter" -v`
Expected: FAIL

- [ ] **Step 3: Implement `_library_filter`**

```python
def _library_filter(self, args: dict) -> dict:
    pattern = args.get("pattern")
    limit = args.get("limit")

    entries = self._entries
    if pattern:
        try:
            rx = re.compile(pattern, re.IGNORECASE)
        except re.error as e:
            return {"error": f"Invalid regex pattern: {e}"}
        entries = [
            e for e in entries
            if rx.search(e["title"]) or rx.search(e["summary"]) or rx.search(e["content"])
        ]

    if limit is not None and limit > 0:
        entries = entries[:limit]

    return {
        "status": "ok",
        "entries": [
            {"id": e["id"], "title": e["title"], "summary": e["summary"]}
            for e in entries
        ],
    }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_anima.py -k "test_library_filter" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/stoai/capabilities/anima.py tests/test_anima.py
git commit -m "feat(anima): add library filter action with regex and limit"
```

---

## Task 5: Implement library view action with depth

**Files:**
- Modify: `src/stoai/capabilities/anima.py`
- Modify: `tests/test_anima.py`

- [ ] **Step 1: Write failing tests**

```python
def test_library_view_content_depth(tmp_path):
    """View with depth=content returns id, title, summary, content."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r = mgr.handle({"object": "library", "action": "submit",
                     "title": "A", "summary": "S.", "content": "C.",
                     "supplementary": "Supp."})
    result = mgr.handle({"object": "library", "action": "view",
                          "ids": [r["id"]], "depth": "content"})
    assert result["status"] == "ok"
    assert len(result["entries"]) == 1
    e = result["entries"][0]
    assert e["title"] == "A"
    assert e["summary"] == "S."
    assert e["content"] == "C."
    assert "supplementary" not in e
    agent.stop(timeout=1.0)


def test_library_view_supplementary_depth(tmp_path):
    """View with depth=supplementary returns everything."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r = mgr.handle({"object": "library", "action": "submit",
                     "title": "A", "summary": "S.", "content": "C.",
                     "supplementary": "Supp."})
    result = mgr.handle({"object": "library", "action": "view",
                          "ids": [r["id"]], "depth": "supplementary"})
    assert result["status"] == "ok"
    e = result["entries"][0]
    assert e["supplementary"] == "Supp."
    agent.stop(timeout=1.0)


def test_library_view_default_depth_is_content(tmp_path):
    """View without explicit depth defaults to content."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r = mgr.handle({"object": "library", "action": "submit",
                     "title": "A", "summary": "S.", "content": "C.",
                     "supplementary": "Supp."})
    result = mgr.handle({"object": "library", "action": "view", "ids": [r["id"]]})
    assert result["status"] == "ok"
    e = result["entries"][0]
    assert "content" in e
    assert "supplementary" not in e
    agent.stop(timeout=1.0)


def test_library_view_requires_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "view"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_view_unknown_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "view", "ids": ["nope"]})
    assert "error" in result
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_anima.py -k "test_library_view" -v`
Expected: FAIL

- [ ] **Step 3: Implement `_library_view`**

```python
def _library_view(self, args: dict) -> dict:
    ids = args.get("ids")
    if not ids:
        return {"error": "ids is required for library view."}
    depth = args.get("depth", "content")

    entries_by_id = {e["id"]: e for e in self._entries}
    invalid = [i for i in ids if i not in entries_by_id]
    if invalid:
        return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

    result_entries = []
    for entry_id in ids:
        e = entries_by_id[entry_id]
        item = {
            "id": e["id"],
            "title": e["title"],
            "summary": e["summary"],
            "content": e["content"],
        }
        if depth == "supplementary":
            item["supplementary"] = e.get("supplementary", "")
        result_entries.append(item)

    return {"status": "ok", "entries": result_entries}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_anima.py -k "test_library_view" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/stoai/capabilities/anima.py tests/test_anima.py
git commit -m "feat(anima): add library view action with depth control"
```

---

## Task 6: Implement library consolidate and delete actions

**Files:**
- Modify: `src/stoai/capabilities/anima.py`
- Modify: `tests/test_anima.py`

- [ ] **Step 1: Write failing tests**

```python
def test_library_consolidate(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r1 = mgr.handle({"object": "library", "action": "submit",
                      "title": "A", "summary": "s1.", "content": "c1"})
    r2 = mgr.handle({"object": "library", "action": "submit",
                      "title": "B", "summary": "s2.", "content": "c2"})
    result = mgr.handle({
        "object": "library", "action": "consolidate",
        "ids": [r1["id"], r2["id"]],
        "title": "AB Combined",
        "summary": "Merged A and B.",
        "content": "Combined content.",
    })
    assert result["status"] == "ok"
    assert result["removed"] == 2
    assert "id" in result
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["title"] == "AB Combined"
    agent.stop(timeout=1.0)


def test_library_consolidate_requires_title_summary_content(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r = mgr.handle({"object": "library", "action": "submit",
                     "title": "X", "summary": "s.", "content": "c"})
    # Missing title
    result = mgr.handle({"object": "library", "action": "consolidate",
                          "ids": [r["id"]], "summary": "s", "content": "c"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_consolidate_invalid_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "library", "action": "consolidate",
        "ids": ["nonexist"], "title": "T", "summary": "s.", "content": "c",
    })
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_delete(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r1 = mgr.handle({"object": "library", "action": "submit",
                      "title": "A", "summary": "s.", "content": "c"})
    r2 = mgr.handle({"object": "library", "action": "submit",
                      "title": "B", "summary": "s.", "content": "c"})
    result = mgr.handle({"object": "library", "action": "delete",
                          "ids": [r1["id"]]})
    assert result["status"] == "ok"
    assert result["removed"] == 1
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["id"] == r2["id"]
    agent.stop(timeout=1.0)


def test_library_delete_invalid_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "delete", "ids": ["nope"]})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_delete_requires_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "delete"})
    assert "error" in result
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_anima.py -k "test_library_consolidate or test_library_delete" -v`
Expected: FAIL

- [ ] **Step 3: Implement `_library_consolidate` and `_library_delete`**

```python
def _library_consolidate(self, args: dict) -> dict:
    ids = args.get("ids")
    title = args.get("title", "").strip()
    summary = args.get("summary", "").strip()
    content = args.get("content", "").strip()
    supplementary = args.get("supplementary", "").strip()
    if not ids:
        return {"error": "ids is required for library consolidate."}
    if not title:
        return {"error": "title is required for library consolidate."}
    if not summary:
        return {"error": "summary is required for library consolidate."}
    if not content:
        return {"error": "content is required for library consolidate."}

    existing_ids = {e["id"] for e in self._entries}
    invalid = [i for i in ids if i not in existing_ids]
    if invalid:
        return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

    ids_set = set(ids)
    self._entries = [e for e in self._entries if e["id"] not in ids_set]

    now = datetime.now(timezone.utc).isoformat()
    new_id = self._make_id(title + content, now)
    self._entries.append({
        "id": new_id,
        "title": title,
        "summary": summary,
        "content": content,
        "supplementary": supplementary,
        "created_at": now,
    })

    self._save_entries()
    return {"status": "ok", "id": new_id, "removed": len(ids)}


def _library_delete(self, args: dict) -> dict:
    ids = args.get("ids")
    if not ids:
        return {"error": "ids is required for library delete."}

    existing_ids = {e["id"] for e in self._entries}
    invalid = [i for i in ids if i not in existing_ids]
    if invalid:
        return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

    ids_set = set(ids)
    before = len(self._entries)
    self._entries = [e for e in self._entries if e["id"] not in ids_set]
    removed = before - len(self._entries)

    self._save_entries()
    return {"status": "ok", "removed": removed}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_anima.py -k "test_library_consolidate or test_library_delete" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/stoai/capabilities/anima.py tests/test_anima.py
git commit -m "feat(anima): add library consolidate and delete actions"
```

---

## Task 7: Implement memory load with selective IDs

**Files:**
- Modify: `src/stoai/capabilities/anima.py`
- Modify: `tests/test_anima.py`

- [ ] **Step 1: Write failing tests**

```python
def test_memory_load_selective(tmp_path):
    """Memory load should inject only selected entries (id + title + content) into prompt."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        r1 = mgr.handle({"object": "library", "action": "submit",
                          "title": "Entry A", "summary": "sA.", "content": "Content A."})
        r2 = mgr.handle({"object": "library", "action": "submit",
                          "title": "Entry B", "summary": "sB.", "content": "Content B."})
        r3 = mgr.handle({"object": "library", "action": "submit",
                          "title": "Entry C", "summary": "sC.", "content": "Content C."})
        # Load only A and C
        result = mgr.handle({"object": "memory", "action": "load",
                              "ids": [r1["id"], r3["id"]]})
        assert result["status"] == "ok"
        section = agent._prompt_manager.read_section("memory")
        assert "Entry A" in section
        assert "Content A" in section
        assert "Entry C" in section
        assert "Content C" in section
        assert "Entry B" not in section
        # Summary should NOT be in memory section
        assert "sA." not in section
    finally:
        agent.stop()


def test_memory_load_requires_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "memory", "action": "load"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_memory_load_replaces_previous(tmp_path):
    """Each load replaces the entire memory section."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        r1 = mgr.handle({"object": "library", "action": "submit",
                          "title": "A", "summary": "s.", "content": "cA"})
        r2 = mgr.handle({"object": "library", "action": "submit",
                          "title": "B", "summary": "s.", "content": "cB"})
        # Load A
        mgr.handle({"object": "memory", "action": "load", "ids": [r1["id"]]})
        section = agent._prompt_manager.read_section("memory")
        assert "cA" in section
        # Load B (replaces A)
        mgr.handle({"object": "memory", "action": "load", "ids": [r2["id"]]})
        section = agent._prompt_manager.read_section("memory")
        assert "cB" in section
        assert "cA" not in section
    finally:
        agent.stop()


def test_memory_load_invalid_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "memory", "action": "load", "ids": ["nope"]})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_memory_load_writes_memory_md(tmp_path):
    """Load should also write memory.md to disk."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        r = mgr.handle({"object": "library", "action": "submit",
                         "title": "X", "summary": "s.", "content": "body"})
        mgr.handle({"object": "memory", "action": "load", "ids": [r["id"]]})
        md = (agent.working_dir / "system" / "memory.md").read_text()
        assert "X" in md
        assert "body" in md
    finally:
        agent.stop()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_anima.py -k "test_memory_load" -v`
Expected: FAIL

- [ ] **Step 3: Implement `_memory_load` and `_memory_diff`**

Replace the old `_memory_load` and `_memory_diff` methods. Memory load no longer delegates to the system intrinsic — it builds memory.md from selected library entries directly.

```python
def _memory_load(self, args: dict) -> dict:
    ids = args.get("ids")
    if not ids:
        return {"error": "ids is required for memory load."}

    entries_by_id = {e["id"]: e for e in self._entries}
    invalid = [i for i in ids if i not in entries_by_id]
    if invalid:
        return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

    # Render memory.md with id + title + content
    lines = []
    for entry_id in ids:
        e = entries_by_id[entry_id]
        lines.append(f"### [{e['id']}] {e['title']}\n{e['content']}")
    content = "\n\n".join(lines) + ("\n" if lines else "")

    # Write to disk
    self._memory_md.parent.mkdir(exist_ok=True)
    self._memory_md.write_text(content)

    # Inject into system prompt
    if content.strip():
        self._agent._prompt_manager.write_section("memory", content)
    else:
        self._agent._prompt_manager.delete_section("memory")
    self._agent._token_decomp_dirty = True

    if self._agent._chat is not None:
        self._agent._chat.update_system_prompt(
            self._agent._build_system_prompt()
        )

    # Git commit
    rel_path = "system/memory.md"
    git_diff, commit_hash = self._agent._workdir.diff_and_commit(
        rel_path, "memory",
    )

    self._agent._log(
        "anima_memory_load",
        entry_count=len(ids),
        changed=commit_hash is not None,
    )

    return {
        "status": "ok",
        "loaded": len(ids),
        "size_bytes": len(content.encode("utf-8")),
        "content_preview": content[:200],
        "diff": {
            "changed": commit_hash is not None,
            "git_diff": git_diff or "",
            "commit": commit_hash,
        },
    }


def _memory_diff(self, _args: dict) -> dict:
    # Delegate to original system handler (inherited, discouraged)
    if self._original_system is None:
        return {"error": "anima not properly initialized (missing system handler)"}
    return self._original_system({"action": "diff", "object": "memory"})
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_anima.py -k "test_memory_load" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/stoai/capabilities/anima.py tests/test_anima.py
git commit -m "feat(anima): selective memory load from library entries"
```

---

## Task 8: Remove old memory tests and add integration tests

**Files:**
- Modify: `tests/test_anima.py`

- [ ] **Step 1: Remove old memory tests that no longer apply**

Delete these test functions (they test the old flat-memory API):
- `test_memory_submit` (replaced by `test_library_submit_*`)
- `test_memory_submit_empty_rejected` (replaced by `test_library_submit_requires_*`)
- `test_memory_consolidate` (replaced by `test_library_consolidate`)
- `test_memory_consolidate_invalid_ids` (replaced by `test_library_consolidate_invalid_ids`)
- `test_memory_consolidate_no_ids` (replaced by `test_library_consolidate_requires_*`)
- `test_memory_diff_delegates_to_system` (keep — still valid for `memory diff`)
- `test_memory_load_delegates_to_system` (remove — load no longer delegates)

Update `test_memory_diff_delegates_to_system` — it still works but needs the library submit API:

```python
def test_memory_diff_delegates_to_system(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        r = mgr.handle({"object": "library", "action": "submit",
                         "title": "T", "summary": "s.", "content": "test entry"})
        mgr.handle({"object": "memory", "action": "load", "ids": [r["id"]]})
        result = mgr.handle({"object": "memory", "action": "diff"})
        assert result["status"] == "ok"
    finally:
        agent.stop()
```

- [ ] **Step 2: Add end-to-end integration test**

```python
def test_library_to_memory_workflow(tmp_path):
    """Full workflow: submit → filter → view → load → verify prompt."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        # Submit entries
        r1 = mgr.handle({
            "object": "library", "action": "submit",
            "title": "Mail Protocol",
            "summary": "FIFO mail queue and TCP transport.",
            "content": "The mail service uses a FIFO queue with TCP transport.",
            "supplementary": "Detailed protocol spec...",
        })
        r2 = mgr.handle({
            "object": "library", "action": "submit",
            "title": "File I/O",
            "summary": "Local filesystem service for file operations.",
            "content": "FileIOService wraps read, write, edit, glob, grep.",
        })
        # Filter
        filtered = mgr.handle({"object": "library", "action": "filter",
                                 "pattern": "mail"})
        assert len(filtered["entries"]) == 1
        assert filtered["entries"][0]["id"] == r1["id"]
        # View at content depth
        viewed = mgr.handle({"object": "library", "action": "view",
                               "ids": [r1["id"]]})
        assert "FIFO queue" in viewed["entries"][0]["content"]
        assert "supplementary" not in viewed["entries"][0]
        # View at supplementary depth
        viewed_deep = mgr.handle({"object": "library", "action": "view",
                                    "ids": [r1["id"]], "depth": "supplementary"})
        assert "protocol spec" in viewed_deep["entries"][0]["supplementary"]
        # Load into memory
        loaded = mgr.handle({"object": "memory", "action": "load",
                               "ids": [r1["id"], r2["id"]]})
        assert loaded["status"] == "ok"
        section = agent._prompt_manager.read_section("memory")
        assert "Mail Protocol" in section
        assert "File I/O" in section
        # Summary should NOT be in memory
        assert "FIFO mail queue and TCP transport." not in section
        # Content should be in memory
        assert "FIFO queue with TCP transport" in section
    finally:
        agent.stop()
```

- [ ] **Step 3: Update error handling tests for new object names**

Update `test_invalid_action_for_object` to use `library` instead of `memory` where appropriate (or keep as-is since `role` is still valid). Ensure `test_invalid_object` still works.

- [ ] **Step 4: Run full test suite**

Run: `python -m pytest tests/test_anima.py -v`
Expected: ALL PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "import stoai.capabilities.anima; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add tests/test_anima.py
git commit -m "test(anima): update tests for library/memory refactor"
```

---

## Task 9: Run full project test suite

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `python -m pytest tests/ -v`
Expected: ALL PASS — no other tests should break since anima is self-contained

- [ ] **Step 2: Final smoke-test**

Run: `python -c "import stoai; print('OK')"`
Expected: `OK`

- [ ] **Step 3: Final commit if any fixups needed**

```bash
git add -u
git commit -m "fix(anima): address test suite issues from library refactor"
```
