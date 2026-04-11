---
name: briefing
description: Scan session history dumps and produce profile, journal, and brief files. Invoke this skill at the start of every briefing cycle.
version: 1.1.0
---

# Briefing Skill

You maintain three files that give AI agents context about their human and the project they are working on. These files are injected into every agent's system prompt when the human runs `/refresh` or `/cpr`. Without them, agents start every session blind — they don't know what the human cares about, what was accomplished yesterday, or what the current priorities are.

## The Three Files

### profile.md — Who is this human?

**Location:** `~/.lingtai-tui/brief/profile.md`
**Scope:** Universal — shared across ALL projects.
**Consumer:** Every agent in every project reads this on refresh.

**Purpose:** Help agents tailor their behavior to this specific human. An astrophysicist gets different explanations than a student. Someone who prefers terse responses should not receive walls of text. The profile captures the human's identity, expertise, communication style, and preferences so agents can adapt without being told every time.

**What to include:**
- Role, expertise, domain knowledge
- Communication style — how they give instructions, level of detail they expect
- Preferences — tools, languages, frameworks they favor
- Working patterns — when they work, how they structure sessions, how they delegate

**Hard limit: 5,000 tokens.** This is injected into every agent's prompt — keep it tight. You MUST verify the token count after every write (see Token Verification below).

### journal.md — What's happening in this project?

**Location:** `~/.lingtai-tui/brief/projects/<hash>/journal.md`
**Scope:** Per-project.
**Consumer:** Agents in THIS project read it on refresh.

**Purpose:** Give agents situational awareness. When an agent wakes up after a molt or refresh, it needs to know: what is the human working on right now? What decisions were made? What's pending? The journal is a rolling briefing — not a log, but a living summary of the project's current state.

**What to include:**
- Current focus — what the human is actively working on
- Recent activity — key events from the last few sessions (rolling window)
- Key decisions — architectural choices, design directions still relevant
- Active agents — who is in the network, what they specialize in
- Pending items — what is unfinished, blocked, or planned next

**Hard limit: 20,000 tokens.** Scale with project complexity. Every token counts. You MUST verify the token count after every write (see Token Verification below).

### brief.md — The combined briefing

**Location:** `~/.lingtai-tui/brief/projects/<hash>/brief.md`
**Scope:** Per-project.
**Consumer:** Injected into agents via the `brief_file` field in init.json.

**Purpose:** This is the actual file agents read. It is simply `profile.md + journal.md` concatenated. You write profile and journal separately (because profile is universal), then construct brief mechanically.

## Token Verification

After EVERY write to profile.md or journal.md, verify the token count:

```bash
python3 -c "
from lingtai_kernel.token_counter import count_tokens
content = open('<file_path>').read()
tokens = count_tokens(content)
print(f'{tokens} tokens')
assert tokens <= <LIMIT>, f'OVER LIMIT: {tokens} > <LIMIT}'
"
```

Where `<LIMIT>` is 5000 for profile.md and 20000 for journal.md.

If the file exceeds its limit, you MUST rewrite it immediately — trim less important content until it fits. Do not proceed to the next step until the token count is under the limit.

## Git Operations

Every write to profile.md or journal.md MUST be followed by a git commit in the brief directory. This provides a versioned history of how the briefing evolves.

```bash
cd ~/.lingtai-tui/brief
git add -A
git commit -m "briefing: update <file> for <project-name>"
```

If the brief directory is not a git repo yet, initialize it first:

```bash
cd ~/.lingtai-tui/brief
git init
git add -A
git commit -m "briefing: initial commit"
```

The sequence for every file write is: **write → verify tokens → git commit**. Never skip the commit.

## Memory Append — Reference Workbench

During consolidation, you need the current profile and journal visible while synthesizing updates. Use `psyche(memory, append)` to pin them as read-only reference:

**Before starting consolidation for a project:**

```
psyche(memory, append, files=["~/.lingtai-tui/brief/profile.md", "~/.lingtai-tui/brief/projects/<hash>/journal.md"])
```

This loads both files into your memory as read-only reference. They appear under a `📎 Reference (read-only)` section in your memory — you can see them while working but they are not part of memory.md.

**After consolidation is complete:**

```
psyche(memory, append, files=[])
```

Clear the workbench to free context space.

**Rules:**
- Always pin profile + the CURRENT project's journal before rewriting that project's journal.
- Process one project at a time. Pin → rewrite journal → verify tokens → git commit → construct brief → unpin → next project.
- The pinned files are refreshed on every memory load, so after you write the updated journal, the reference still shows the OLD version until you re-pin. This is fine — you should unpin after writing, not re-pin.

## Working Order

The briefing process has two phases: **observation** and **consolidation**.

### Observation Phase (one file per cycle, molt-safe)

Each cycle, you read ONE history file, distill it into a condensed draft, and submit it to your library. Library entries are permanent — they survive molts, reboots, everything. Your memory tracks which files you have processed and how many drafts are pending per project.

### Consolidation Phase (when caught up)

When all pending history files are processed, you load your draft entries from library (grouped by project), synthesize the final journal for each project, optionally update the profile, construct brief.md for each project, and delete the draft entries.

```
Cycle 1: read file A → draft to library → "A done" in memory → idle
  (molt safe — drafts in library, state in memory)
Cycle 2: read file B → draft to library → "B done" in memory → idle
Cycle N: read file Z → draft to library → "Z done" in memory
  No more pending → CONSOLIDATE:
    for each project with drafts:
      load drafts → read existing journal → write updated journal → delete drafts
    update profile if needed
    construct brief.md for each updated project
    → idle (hourly schedule)
```

## Context Management — CRITICAL

Your library limit is 100 entries (raised from the default 20). You have room for many drafts, but still:

- **Never read more than ONE history file per turn.** Read it, draft to library, save state, go idle.
- **Always check file size before reading.** Use `wc -c <file>` — if a file exceeds 150,000 bytes (~40k tokens), skip it and note in memory.
- **Process projects round-robin.** One file per project per cycle if multiple have backlogs.
- **Consolidate one project at a time.** Load drafts for one project, write its journal, delete its drafts, then move to the next.

---

## Observation Steps

### Step 1: Discover Projects

Read the project registry:

```bash
cat ~/.lingtai-tui/registry.jsonl
```

Each line is `{"path": "/absolute/path/to/project"}`. Compute each project's hash:

```bash
echo -n "/absolute/path/to/project" | shasum -a 256 | cut -c1-12
```

The brief directory is `~/.lingtai-tui/brief/projects/<hash>/`. Use the path's basename as the human-readable project name (e.g., `/Users/alice/my-app` → "my-app").

### Step 2: Find Pending History

For each project, list history files and compare against your last-processed timestamp (stored in your memory):

```bash
ls -1 ~/.lingtai-tui/brief/projects/<hash>/history/ | sort
```

Files are named `YYYY-MM-DD-HH.md`. Any file newer than your last-processed timestamp is pending.

```bash
# Example: find files newer than 2026-04-10-14.md
ls -1 ~/.lingtai-tui/brief/projects/<hash>/history/ | sort | awk '$0 > "2026-04-10-14.md"'
```

If no files are pending for any project, go idle — nothing to do this cycle.

### Step 3: Pick ONE File to Process

Choose the oldest pending file from the project with the most backlog (round-robin if tied). Check its size:

```bash
wc -c ~/.lingtai-tui/brief/projects/<hash>/history/YYYY-MM-DD-HH.md
```

- **≤ 150,000 bytes**: proceed.
- **> 150,000 bytes**: skip it. Record in memory. Advance your timestamp past it.

### Step 4: Read the History File

```bash
cat ~/.lingtai-tui/brief/projects/<hash>/history/YYYY-MM-DD-HH.md
```

As you read, distill:
- What the human worked on during this hour
- Key decisions, breakthroughs, or problems encountered
- New agents spawned, tools used, collaborators involved
- Any shift in project direction or priorities
- Anything revealing about the human's preferences or working style (for the profile)

### Step 5: Submit Draft to Library

Submit a condensed observation. This is your molt-safe working scratchpad.

```
library(submit,
  title="draft:<project-name>:<YYYY-MM-DD-HH>",
  summary="Briefing draft for <project-name>, hour <YYYY-MM-DD-HH>",
  content="<condensed observations — what happened, key decisions, what changed, any profile-relevant observations>"
)
```

**Title convention**: always `draft:<project-name>:<hour>`. This lets you filter drafts by project during consolidation: `library(filter, pattern="draft:my-app:")`.

Target: **200–500 words per draft.** You are distilling 20k+ tokens into a few hundred words.

### Step 6: Record State in Memory

Update your memory with progress. This is how your future self (after molt) knows where to resume.

```
psyche(memory, edit, content="
Briefing state:
  projects:
    my-app (a1b2c3d4e5f6): last=2026-04-10-14, pending=3, drafts=2
    my-site (f6e5d4c3b2a1): last=2026-04-10-08, pending=0, drafts=0
  skipped: a1b2c3d4e5f6/2026-04-09-22.md (too large)
  next action: continue observation
")
```

**Every field matters for continuity:**
- `last` — timestamp of last processed file (your future self uses this to find pending files)
- `pending` — count of remaining files (tells you when to consolidate)
- `drafts` — count of library entries for this project (tells you what to load during consolidation)

### Step 7: Schedule Next Cycle or Consolidate

If there are still pending files for ANY project: schedule a 5-minute follow-up and go idle.

```
email(send, address=secretary, message="continue briefing", delay=300)
```

If ALL projects have pending=0 AND any project has drafts>0: proceed to **Consolidation Steps** below.

If all caught up and no drafts: schedule hourly cycle and go idle.

---

## Consolidation Steps

### Step 8: Consolidate Per Project

Process ONE project at a time. For each project that has drafts:

**8a.** Pin current profile + this project's journal as reference:

```
psyche(memory, append, files=["~/.lingtai-tui/brief/profile.md", "~/.lingtai-tui/brief/projects/<hash>/journal.md"])
```

This loads both into your memory as read-only reference. You can now see the current state while writing the update.

**8b.** Load drafts for this project:

```
library(filter, pattern="draft:<project-name>:")
library(view, ids=[<list of draft IDs for this project>])
```

**8c.** Write the updated journal — a COMPLETE REWRITE synthesizing all drafts + existing journal into the current state. Do not patch — rewrite the entire file from scratch:

```bash
cat > ~/.lingtai-tui/brief/projects/<hash>/journal.md << 'JOURNAL_EOF'
# <Project Name> — Journal

**Last updated:** YYYY-MM-DD HH:MM UTC

## Current Focus
...

## Recent Activity
...

## Key Decisions
...

## Active Agents
...

## Pending Items
...
JOURNAL_EOF
```

**8d.** Verify token count and git commit:

```bash
python3 -c "
import os
from lingtai_kernel.token_counter import count_tokens
content = open(os.path.expanduser('~/.lingtai-tui/brief/projects/<hash>/journal.md')).read()
tokens = count_tokens(content)
print(f'{tokens} tokens')
assert tokens <= 20000, f'OVER LIMIT: {tokens} > 20000'
"
```

```bash
cd ~/.lingtai-tui/brief && git add -A && git commit -m "briefing: update journal for <project-name>"
```

If over limit, rewrite to trim. Do not proceed until under 20,000 tokens.

**8e.** Delete the consolidated drafts:

```
library(delete, ids=[<draft IDs just consolidated>])
```

**8f.** Clear the reference workbench:

```
psyche(memory, append, files=[])
```

**8g.** Repeat for the next project with drafts.

### Step 9: Update Profile

After all project journals are written, consider the profile. Pin it as reference:

```
psyche(memory, append, files=["~/.lingtai-tui/brief/profile.md"])
```

Only update if your drafts revealed something NEW about the human that applies universally:
- A new skill or expertise area
- A consistent communication pattern you hadn't captured
- A preference showing across multiple projects
- A correction to something you previously wrote

If updating, do a COMPLETE REWRITE — not a patch:

```bash
cat > ~/.lingtai-tui/brief/profile.md << 'PROFILE_EOF'
<profile content>
PROFILE_EOF
```

Verify token count and git commit:

```bash
python3 -c "
import os
from lingtai_kernel.token_counter import count_tokens
content = open(os.path.expanduser('~/.lingtai-tui/brief/profile.md')).read()
tokens = count_tokens(content)
print(f'{tokens} tokens')
assert tokens <= 5000, f'OVER LIMIT: {tokens} > 5000'
"
```

```bash
cd ~/.lingtai-tui/brief && git add -A && git commit -m "briefing: update profile"
```

If over limit, rewrite to trim. Do not proceed until under 5,000 tokens.

Clear the workbench:

```
psyche(memory, append, files=[])
```

### Step 10: Construct Briefs

For EACH project that had its journal updated, reconstruct the brief and commit:

```bash
cat ~/.lingtai-tui/brief/profile.md > ~/.lingtai-tui/brief/projects/<hash>/brief.md
echo -e "\n---\n" >> ~/.lingtai-tui/brief/projects/<hash>/brief.md
cat ~/.lingtai-tui/brief/projects/<hash>/journal.md >> ~/.lingtai-tui/brief/projects/<hash>/brief.md
```

```bash
cd ~/.lingtai-tui/brief && git add -A && git commit -m "briefing: construct brief for <project-name>"
```

### Step 11: Record Final State

```
psyche(memory, edit, content="
Briefing state:
  projects:
    my-app (a1b2c3d4e5f6): last=2026-04-10-17, pending=0, drafts=0
    my-site (f6e5d4c3b2a1): last=2026-04-10-08, pending=0, drafts=0
  last consolidation: 2026-04-10-17T12:00Z
  next action: wait for hourly cycle
")
```

### Step 12: Schedule Next Cycle

```
email(send, address=secretary, message="briefing cycle", delay=3600)
```

Then go idle.

---

## First Run

On your first cycle, there may be many history files from migration backfill. Do NOT try to read them all. Process them one at a time — the 5-minute follow-up schedule works through the backlog. Consolidation happens only when all files are processed.

## Molt Preparation

When context pressure rises, your molt summary MUST include:
- Per-project last-processed timestamps, pending counts, and draft counts
- Library draft IDs that have not been consolidated yet
- Whether a consolidation was in progress (and which project you were on)
- Any skipped files and why

Your future self needs these exact details to continue without reprocessing or losing drafts.
