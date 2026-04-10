---
name: briefing
description: Scan session history dumps and produce profile, journal, and brief files. Invoke this skill at the start of every briefing cycle.
version: 1.0.0
---

# Briefing Skill

This skill guides your briefing cycle. You maintain three types of files:

| File | Scope | Location |
|---|---|---|
| `profile.md` | Universal (all projects) | `~/.lingtai-tui/brief/profile.md` |
| `journal.md` | Per-project | `~/.lingtai-tui/brief/projects/<hash>/journal.md` |
| `brief.md` | Per-project | `~/.lingtai-tui/brief/projects/<hash>/brief.md` |

## Working Order

The briefing process has two phases: **observation** and **consolidation**.

### Observation Phase (one file per cycle)

Each cycle, you read ONE history file, distill it into a library draft, and record your progress. This is molt-safe — library entries and memory survive molts.

### Consolidation Phase (when caught up)

When all pending files are processed, you load your draft entries from library, synthesize the final journal and profile, write them to disk, construct the brief, and clean up drafts.

```
Cycle 1: read file A → draft to library → "A done" in memory → idle
  (molt safe — drafts in library, state in memory)
Cycle 2: read file B → draft to library → "B done" in memory → idle
  (molt safe)
Cycle N: read file Z → draft to library → "Z done" in memory
  No more pending → CONSOLIDATE:
    load all drafts from library → write journal.md → write profile.md
    → construct brief.md → clean up draft entries → idle (hourly)
```

## Context Management — CRITICAL

- **Never read more than ONE history file per turn.** Read it, draft to library, save state, go idle.
- **Always check file size before reading.** Use `wc -c <file>` — if a file exceeds 150,000 bytes (~40k tokens), skip it and note in memory.
- **Process projects round-robin.** One file per project per cycle if multiple have backlogs.

## Step 1: Discover Projects

Read the project registry:

```bash
cat ~/.lingtai-tui/registry.jsonl
```

Each line is `{"path": "/absolute/path/to/project"}`. Compute each project's hash:

```bash
echo -n "/absolute/path/to/project" | shasum -a 256 | cut -c1-12
```

The brief directory is `~/.lingtai-tui/brief/projects/<hash>/`. Use the path's basename as the human-readable project name (e.g., `/Users/alice/my-app` → "my-app").

## Step 2: Find Pending History

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

## Step 3: Pick ONE File to Process

Choose the oldest pending file from the project with the most backlog (round-robin if tied). Check its size:

```bash
wc -c ~/.lingtai-tui/brief/projects/<hash>/history/YYYY-MM-DD-HH.md
```

- **≤ 150,000 bytes**: proceed.
- **> 150,000 bytes**: skip it. Record in memory: `"skipped: <hash>/YYYY-MM-DD-HH.md (too large)"`. Advance your timestamp past it.

## Step 4: Read the History File

```bash
cat ~/.lingtai-tui/brief/projects/<hash>/history/YYYY-MM-DD-HH.md
```

As you read, distill:
- What the human worked on during this hour
- Key decisions, breakthroughs, or problems encountered
- New agents spawned, tools used, collaborators involved
- Any shift in project direction or priorities

## Step 5: Submit Draft to Library

Submit a condensed observation to library. This is your molt-safe working scratchpad.

```
library(submit,
  title="draft:<project-name>:<YYYY-MM-DD-HH>",
  summary="Briefing draft for <project-name>, hour <YYYY-MM-DD-HH>",
  content="<condensed observations from the history file — what happened, key decisions, what changed>"
)
```

**Title convention**: always prefix with `draft:` so you can find all drafts later. Include project name and hour.

Target: **200–500 words per draft.** Compress aggressively — you are distilling 20k+ tokens into a few hundred words. Capture what matters for someone joining the project, not raw details.

## Step 6: Record State in Memory

Update your memory with progress:

```
psyche(memory, edit, content="
Briefing state:
  my-app (a1b2c3d4e5f6): last=2026-04-10-14, pending=3, drafts=2
  my-site (f6e5d4c3b2a1): last=2026-04-10-08, pending=0, drafts=0
  skipped: a1b2c3d4e5f6/2026-04-09-22.md (too large)
")
```

## Step 7: Check if Consolidation is Due

If there are still pending files for ANY project: schedule a 5-minute follow-up and go idle.

```
email(send, address=secretary, message="continue briefing", delay=300)
```

If ALL projects are caught up (pending=0) AND there are draft entries in library: proceed to **consolidation** (Step 8).

If all caught up and no drafts: nothing to do. Schedule hourly cycle and go idle.

## Step 8: Consolidation — Load Drafts

Find all your draft entries:

```
library(filter, pattern="draft:")
```

Load them:

```
library(view, ids=[<list of draft IDs>])
```

Also read the existing journal and profile:

```bash
cat ~/.lingtai-tui/brief/projects/<hash>/journal.md 2>/dev/null || echo "(no journal yet)"
cat ~/.lingtai-tui/brief/profile.md 2>/dev/null || echo "(no profile yet)"
```

## Step 9: Write Journal

Synthesize all drafts for this project into a single journal. The journal captures the CURRENT state — not a chronological log. It should read as a briefing for someone joining the project right now.

Structure:

```markdown
# <Project Name> — Journal

**Last updated:** YYYY-MM-DD HH:MM UTC

## Current Focus
What the human is actively working on right now.

## Recent Activity
Key events from the last few sessions (rolling window — drop old entries as new ones arrive).

## Key Decisions
Architectural choices, tool selections, design directions that are still relevant.

## Active Agents
Which agents are in the network, what they specialize in.

## Pending Items
What is unfinished, blocked, or planned next.
```

Target: **500–2000 words.** Be ruthless about compression. The journal is injected into every agent's system prompt on refresh — every token counts.

```bash
cat > ~/.lingtai-tui/brief/projects/<hash>/journal.md << 'JOURNAL_EOF'
<journal content>
JOURNAL_EOF
```

## Step 10: Update Profile (Selectively)

Only update if you observed something NEW about the human across projects:
- A new skill or expertise area
- A consistent communication pattern
- A preference that shows across multiple projects

```bash
cat > ~/.lingtai-tui/brief/profile.md << 'PROFILE_EOF'
<profile content>
PROFILE_EOF
```

Target: **200–500 words.** Universal context — no project-specific details.

## Step 11: Construct Brief

Concatenate profile and journal:

```bash
cat ~/.lingtai-tui/brief/profile.md > ~/.lingtai-tui/brief/projects/<hash>/brief.md
echo -e "\n---\n" >> ~/.lingtai-tui/brief/projects/<hash>/brief.md
cat ~/.lingtai-tui/brief/projects/<hash>/journal.md >> ~/.lingtai-tui/brief/projects/<hash>/brief.md
```

## Step 12: Clean Up Drafts

Delete the draft entries you just consolidated — they are now captured in the journal.

```
library(delete, ids=[<list of consolidated draft IDs>])
```

Update memory to clear the draft count:

```
psyche(memory, edit, content="
Briefing state:
  my-app (a1b2c3d4e5f6): last=2026-04-10-17, pending=0, drafts=0
  consolidated: 2026-04-10-17T12:00Z
")
```

## Step 13: Schedule Next Cycle

Schedule the normal hourly cycle:

```
email(send, address=secretary, message="briefing cycle", delay=3600)
```

Then go idle.

## First Run

On your first cycle, there may be many history files from migration backfill. Process them one at a time — the 5-minute follow-up schedule works through the backlog. Consolidation happens only when all files are processed.

## Molt Preparation

When context pressure rises, your molt summary MUST include:
- Per-project last-processed timestamps and pending counts
- Library draft IDs that have not been consolidated yet
- Any skipped files and why
- Whether a consolidation was in progress

Your future self needs these exact details to continue without reprocessing or losing drafts.
