---
name: lingtai-changelog
description: Chronicle of breaking changes, renames, and migrations in the LingTai system. Load this when you encounter unfamiliar names, deprecated references, or confusion about what things are called and where they live. Entries are prepended — newest first.
version: 1.0.0
---

# LingTai Changelog

A living chronicle of system-level changes that affect how you work. When something doesn't match what you remember, check here first.

---

## 2026-04-16 — Addon Secrets Move to Admin's `.secrets/`

### What changed

Addon configs (IMAP, Feishu, Telegram, WeChat) can now live inside the orchestrator agent's own working directory at `.secrets/<addon>.json`, in plaintext JSON without `*_env` indirection. The old project-shared path keeps working — nothing is forced to move.

### New path

| Addon | New path (inside admin's working dir) |
|-------|----------------------------------------|
| imap | `.secrets/imap.json` |
| feishu | `.secrets/feishu.json` |
| telegram | `.secrets/telegram.json` |
| wechat | `.secrets/wechat.json` (+ `.secrets/credentials.json` after QR login) |

### Old path (still works, no migration required)

| Addon | Old path (relative to project root) |
|-------|--------------------------------------|
| imap | `.lingtai/.addons/imap/config.json` |
| feishu | `.lingtai/.addons/feishu/config.json` |
| telegram | `.lingtai/.addons/telegram/config.json` |
| wechat | `.lingtai/.addons/wechat/config.json` |

### Why

Addons are an admin-only responsibility — avatars must not configure them. Keeping addon secrets inside the orchestrator's own directory makes that ownership explicit, removes the `*_env` / `.env` indirection, and keeps each agent's secrets self-contained.

### What you should do

- **New setups:** use the new path. See `lingtai-imap-setup`, `lingtai-feishu-setup`, `lingtai-telegram-setup`, or `lingtai-wechat-setup` skills for full instructions.
- **Existing setups:** leave them alone unless the human asks to migrate. Only the `lingtai-imap-setup` skill ships migration instructions; for other addons, the human should migrate manually.
- **Avatars:** you should never be configuring addons. If an addon tool is missing from your tool list, that is by design — ask your orchestrator.

---

## 2026-04-13 — The Pad / Codex / Library Rename

### What changed

Three core concepts were renamed to better reflect what they actually are:

| Before | After | What it is | System prompt presence |
|--------|-------|-----------|----------------------|
| `memory` (psyche sub-action) | **pad** | Your working notes — always in front of you | FULL — entire content injected |
| `library` (tool) | **codex** | Your personal knowledge archive — structured entries you curate | SEMI — summaries, load on demand |
| `skills` (capability) | **library** | The skill library — a shelf of playbooks you consult | ROUTING — XML catalog only |

### New names in each language

| Level | English | 中文 | 文言 |
|-------|---------|------|------|
| 1 | pad | 手记 | 简 |
| 2 | codex | 典集 | 典 |
| 3 | library | 藏经阁 | 藏经阁 |

### What moved on disk

| Old path | New path |
|----------|----------|
| `system/memory.md` | `system/pad.md` |
| `system/memory_append.json` | `system/pad_append.json` |
| `library/library.json` | `codex/codex.json` |
| `.lingtai/.skills/` | `.lingtai/.library/` |

A TUI migration (m015) handles the filesystem renames automatically for existing agents.

### Tool call changes

**Psyche / eigen:**
```
# Old:
psyche(memory, edit, content=...)
psyche(memory, load)
psyche(memory, append, files=[...])

# New:
psyche(pad, edit, content=...)
psyche(pad, load)
psyche(pad, append, files=[...])
```

**Knowledge archive (was library, now codex):**
```
# Old:
library(submit, title=..., summary=..., content=...)
library(filter, pattern=...)
library(view, ids=[...])
library(export, ids=[...])

# New:
codex(submit, title=..., summary=..., content=...)
codex(filter, pattern=...)
codex(view, ids=[...])
codex(export, ids=[...])
```

**Skill library (was skills, now library):**
```
# Old:
skills(action='register')
skills(action='refresh')

# New:
library(action='register')
library(action='refresh')
```

### Why the rename

The old names were misleading:

- **"memory"** implied persistence and recall, but it's really a scratchpad — working notes you jot down, always visible, always editable. **Pad** says what it is.
- **"library"** implied a public reference you browse, but it's really your personal knowledge manuscript — structured entries you curate over time, heavy and durable. **Codex** captures the weight and personal ownership.
- **"skills"** were already called "skills" inside, but the container was also called "skills." Now the container is a **library** — a library of skills. You walk to the 藏经阁 (hall of scriptures), find the right 功法 (technique manual), and bring it back to your desk.

The three levels form a gradient of context presence:
1. **Pad** — hot, always in your prompt, your working surface
2. **Codex** — warm, structured entries you pull into your pad when needed
3. **Library** — cold, an XML routing table; you load a skill's full SKILL.md on demand

### If you see old names

If you encounter `system/memory.md`, `library/library.json`, `.skills/`, or tool calls using the old names in existing files, notes, or emails from before this rename — they refer to `pad`, `codex`, and `library` respectively. The TUI migration renamed the files, but references in your own pad notes, codex entries, or old email may still use the old names.
