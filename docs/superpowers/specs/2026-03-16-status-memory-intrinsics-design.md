# Status & Memory Intrinsics Design Spec

## Goal

Add two new intrinsics to the agent kernel:

- **`status`** — self-introspection (identity, runtime, token usage, context window)
- **`memory`** — long-term memory reload from disk into live system prompt, with git-backed versioning

Additionally, initialize the agent's working directory as a git repository for state tracking.

## Context

Agents currently have no way to inspect their own identity, resource consumption, or context window usage. Long-term memory (LTM) is stored as a string in the manifest (`agent.json`) and injected into the system prompt at init — the agent cannot update it during execution.

This design introduces:
1. A `status` intrinsic for self-inspection
2. A `memory` intrinsic that works with a markdown file (`ltm/ltm.md`) the agent edits via existing file intrinsics, then reloads into its live system prompt
3. Git version control of the agent's working directory, tracking only explicitly allowed paths

## Architecture

### Git-Controlled Working Directory

On `agent.start()`, the working directory is initialized as a git repo if `.git` doesn't already exist.

**`.gitignore` — opt-in tracking:**

```
# Track nothing by default
*
# Except these
!.gitignore
!ltm/
!ltm/**
```

Future capabilities (email, etc.) add their own entries when set up (e.g., `!mailbox/`, `!mailbox/**`).

**Init sequence:**
1. `git init` (if no `.git`)
2. Write `.gitignore`
3. Create `ltm/` directory and `ltm.md` if they don't exist
4. Migrate LTM from manifest if needed (see Migration section)
5. `git add .gitignore ltm/` → initial commit (`"init: agent working directory"`)

If `.git` already exists (resume), skip init. The directory is already tracked.

### `status` Intrinsic

**Single action: `show`**

Pure read-only self-inspection. No service dependency — always available. `handler=None` (needs agent state).

**Schema:**

```python
SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["show"],
            "description": (
                "show: display full agent self-inspection. Returns:\n"
                "- identity: agent_id, working_dir, mail_address (or null if no mail service)\n"
                "- runtime: started_at (UTC ISO), uptime_seconds\n"
                "- tokens.input_tokens, output_tokens, thinking_tokens, cached_tokens, "
                "total_tokens, api_calls: cumulative LLM usage since start\n"
                "- tokens.context.system_tokens, tools_tokens, history_tokens: "
                "current context window breakdown\n"
                "- tokens.context.window_size: total context window capacity\n"
                "- tokens.context.usage_pct: percentage of context window currently occupied\n"
                "Use this to monitor resource consumption, decide when to save "
                "important information to long-term memory, and identify yourself."
            ),
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Agent self-inspection. 'show' returns identity (agent_id, working_dir, "
    "mail address), runtime (uptime), and resource usage (cumulative tokens, "
    "context window breakdown with usage percentage). "
    "Check this to monitor your own resource consumption and decide when to "
    "save important information to long-term memory before context compaction."
)
```

**Return payload:**

```python
{
    "status": "ok",
    "identity": {
        "agent_id": "alice",
        "working_dir": "/base/alice",
        "mail_address": "127.0.0.1:8301",  # or None
    },
    "runtime": {
        "started_at": "2026-03-16T08:50:47Z",
        "uptime_seconds": 3421.5,
    },
    "tokens": {
        "input_tokens": 15200,
        "output_tokens": 4800,
        "thinking_tokens": 1200,
        "cached_tokens": 8000,
        "total_tokens": 21200,
        "api_calls": 12,
        "context": {
            "system_tokens": 2400,
            "tools_tokens": 1800,
            "history_tokens": 9600,
            "total_tokens": 13800,
            "window_size": 128000,
            "usage_pct": 10.8,
        },
    },
}
```

**Implementation notes:**
- **`started_at` semantics:** Two separate values. `started_at` is a wall-clock UTC timestamp persisted in manifest — records when this agent process last started. `_uptime_anchor` is a `time.monotonic()` value set once during `start()` — used to compute uptime within the current process. On resume, `started_at` is updated to the new start time; uptime always reflects the current process run, not cumulative time.
- **Token payload restructuring:** `_handle_status` restructures the flat dict from `get_token_usage()` into the nested `identity`/`runtime`/`tokens` payload shown above. This is a presentation concern only — the underlying data source is unchanged.
- Token data comes from existing `get_token_usage()` method
- Context window size from `self._chat.context_window()` (requires active chat session; context fields are `null` if no session)
- **Nested git repos:** If `base_dir` is inside a host git repo, the agent's `working_dir` will be a nested repo. This is intentional — each agent owns its own history. The opt-in `.gitignore` prevents accidental tracking.

### `memory` Intrinsic

**Single action: `load`**

Reloads `ltm/ltm.md` from disk into the live system prompt, then git-commits the file. `handler=None` (needs agent state).

**Workflow:** The agent uses existing file intrinsics (`read`/`edit`/`write`) to modify `ltm/ltm.md`, then calls `memory load` to make changes take effect and commit them.

**Schema:**

```python
SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["load"],
            "description": (
                "load: read the long-term memory file (ltm/ltm.md in working directory) "
                "and reload its contents into the live system prompt. "
                "Call this after editing ltm/ltm.md with the write/edit intrinsics "
                "to make your changes take effect in the current conversation.\n"
                "Returns: status, path (absolute path to the ltm file), "
                "size_bytes (file size), content_preview (first 200 chars), "
                "diff (git diff of changes with commit hash).\n"
                "If the file does not exist, it is created empty and loaded.\n"
                "Workflow: write/edit ltm/ltm.md → call memory load → "
                "changes are now part of your system prompt and committed to git."
            ),
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Long-term memory management. The agent's persistent memory lives in "
    "ltm/ltm.md (markdown file in working directory). "
    "Edit that file with read/write/edit intrinsics, then call 'load' to "
    "reload it into the live system prompt. "
    "Use this to persist important information across context compactions "
    "and agent restarts."
)
```

**Return payload:**

```python
{
    "status": "ok",
    "path": "/base/alice/ltm/ltm.md",
    "size_bytes": 1482,
    "content_preview": "# Long-Term Memory\n\n## Key findings\n- The database...",
    "diff": {
        "changed": True,
        "git_diff": "diff --git a/ltm/ltm.md b/ltm/ltm.md\n...",
        "commit": "a3f1b2c",
    },
}
```

When nothing changed:

```python
{
    "status": "ok",
    "path": "/base/alice/ltm/ltm.md",
    "size_bytes": 1482,
    "content_preview": "# Long-Term Memory\n...",
    "diff": {
        "changed": False,
        "git_diff": "",
        "commit": None,
    },
}
```

**Implementation notes:**

1. Read `{working_dir}/ltm/ltm.md` (create dir + empty file if missing)
2. If file is empty or whitespace-only, remove the `"ltm"` section from prompt manager (avoids empty `## ltm` in system prompt). Otherwise, inject content as unprotected `"ltm"` section.
3. Mark `_token_decomp_dirty = True`
4. Run `git diff ltm/ltm.md` to capture changes
5. If changed: `git add ltm/ltm.md` → `git commit -m "ltm: update long-term memory"`
6. Return payload with diff and commit hash
7. If unchanged: return with `changed=False`, no commit

**Git operations** should be run via `subprocess.run()` with `cwd=working_dir`. Failures in git (e.g., git not installed) should not break the intrinsic — the load still works, just without the diff/commit (return `git_diff: null, commit: null`).

## LTM Migration

On agent init, if `agent.json` has a non-empty `ltm` field AND `ltm/ltm.md` does not exist:

1. Create `ltm/` directory
2. Write manifest LTM content to `ltm/ltm.md`
3. Clear `ltm` from manifest
4. Write updated manifest (without `ltm`)

After migration, manifest stores only: `agent_id`, `started_at`, `address`, `role`.

On agent init (after migration or on resume), auto-load `ltm/ltm.md` into the system prompt. This replaces the current flow where LTM goes from manifest → constructor → prompt manager.

## Auto-Load on Init

During agent initialization (after manifest read, before first LLM session):

1. Read `ltm/ltm.md` if it exists
2. Inject into `SystemPromptManager` as unprotected `"ltm"` section
3. This replaces the current `ltm` constructor parameter flow for resumed agents

The `ltm` constructor parameter still works for fresh agents — it writes to `ltm/ltm.md` and loads from there.

## Intrinsic Count

Current: 9 (read, edit, write, glob, grep, mail, vision, web_search, clock).
After: 11 (+ status, memory).

## Files Affected

| File | Action | What changes |
|------|--------|-------------|
| `src/stoai/intrinsics/status.py` | Create | Schema and description |
| `src/stoai/intrinsics/memory.py` | Create | Schema and description |
| `src/stoai/intrinsics/__init__.py` | Modify | Register both with `handler=None` |
| `src/stoai/agent.py` | Modify | Git init, `_started_at`, `_handle_status`, `_handle_memory`, wire intrinsics, LTM migration, auto-load |
| `tests/test_status.py` | Create | Status intrinsic tests |
| `tests/test_memory.py` | Create | Memory intrinsic tests (with git) |
| `tests/test_agent.py` | Modify | Intrinsic count 9 → 11 |

## Future Extensions (Not In Scope)

- **Memory capability ("DLC")**: structured sections, search, categories, self-initiated compaction, memory history via `git log`
- **Email capability git integration**: auto-commit mailbox changes to the same working dir repo
- **Custom commit messages**: `memory load` accepting an optional `message` parameter
