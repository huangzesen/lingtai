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

## Context Management — CRITICAL

History files can be large (20,000–30,000 tokens each). You MUST manage your context carefully:

- **Never read more than ONE history file per turn.** Read it, update the journal, save your state, then go idle. The next cycle picks up the next file.
- **Always check file size before reading.** Use `wc -c <file>` — if a file exceeds 150,000 bytes (~40k tokens), skip it and note in your memory that it was skipped.
- **Read the existing journal BEFORE reading new history.** The journal is small (500–2000 words). It gives you context to integrate new history without losing prior information.
- **Process projects round-robin.** If multiple projects have pending history, process one file from each project per cycle, not all files from one project.

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

Files are named `YYYY-MM-DD-HH.md`. Any file newer than your last-processed timestamp is pending. Count them:

```bash
# Example: find files newer than 2026-04-10-14.md
ls -1 ~/.lingtai-tui/brief/projects/<hash>/history/ | sort | awk '$0 > "2026-04-10-14.md"'
```

If no files are pending for any project, go idle — nothing to do this cycle.

## Step 3: Pick ONE File to Process

Choose the oldest pending file from the project with the most backlog (round-robin if tied). Before reading, check its size:

```bash
wc -c ~/.lingtai-tui/brief/projects/<hash>/history/YYYY-MM-DD-HH.md
```

- If **≤ 150,000 bytes**: proceed to read it.
- If **> 150,000 bytes**: skip it. Record in your memory: `"skipped: <hash>/YYYY-MM-DD-HH.md (too large, NNN bytes)"`. Move your last-processed timestamp past it and pick the next file.

## Step 4: Read Existing Journal

Before reading the history file, read the current journal to have context:

```bash
cat ~/.lingtai-tui/brief/projects/<hash>/journal.md 2>/dev/null || echo "(no journal yet)"
```

This is typically 500–2000 words. Hold it in mind as you read the new history.

## Step 5: Read the History File

Now read the one selected history file:

```bash
cat ~/.lingtai-tui/brief/projects/<hash>/history/YYYY-MM-DD-HH.md
```

As you read, identify:
- What the human worked on during this hour
- Key decisions, breakthroughs, or problems encountered
- New agents spawned, tools used, collaborators involved
- Any shift in project direction or priorities

## Step 6: Update Journal

Rewrite the journal in full, integrating what you learned from the new history file. The journal captures the CURRENT state of the project — not a chronological log. It should read as a briefing for someone joining the project right now.

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

Target: **500–2000 words.** Be ruthless about compression. Drop details that are no longer relevant. The journal is injected into every agent's system prompt on refresh — every token counts.

Write:

```bash
cat > ~/.lingtai-tui/brief/projects/<hash>/journal.md << 'JOURNAL_EOF'
<journal content>
JOURNAL_EOF
```

## Step 7: Update Profile (Selectively)

Only update the profile when you observe something NEW about the human that applies across all projects. Do NOT rewrite the profile every cycle — it should change slowly.

Read current profile:

```bash
cat ~/.lingtai-tui/brief/profile.md 2>/dev/null || echo "(no profile yet)"
```

Update ONLY if you noticed:
- A new skill or expertise area
- A consistent communication pattern you hadn't captured
- A preference that shows across multiple projects
- A correction to something you previously wrote

If updating:

```bash
cat > ~/.lingtai-tui/brief/profile.md << 'PROFILE_EOF'
<profile content>
PROFILE_EOF
```

Target: **200–500 words.** The profile is universal context — keep it tight.

## Step 8: Construct Brief

After updating journal (and optionally profile), reconstruct the brief:

```bash
cat ~/.lingtai-tui/brief/profile.md > ~/.lingtai-tui/brief/projects/<hash>/brief.md
echo -e "\n---\n" >> ~/.lingtai-tui/brief/projects/<hash>/brief.md
cat ~/.lingtai-tui/brief/projects/<hash>/journal.md >> ~/.lingtai-tui/brief/projects/<hash>/brief.md
```

## Step 9: Record State

Update your memory with what you processed:

```
psyche(memory, edit, content="
Briefing state:
  my-app (a1b2c3d4e5f6): last=2026-04-10-14, pending=3
  my-site (f6e5d4c3b2a1): last=2026-04-10-08, pending=0
  skipped: a1b2c3d4e5f6/2026-04-09-22.md (too large, 200KB)
")
```

## Step 10: Schedule Next Cycle

If there are still pending files for any project, schedule a short follow-up (5 minutes) to process the next one:

```
email(send, address=secretary, message="continue briefing", delay=300)
```

If all projects are caught up, schedule the normal hourly cycle:

```
email(send, address=secretary, message="briefing cycle", delay=3600)
```

Then go idle.

## First Run

On your very first cycle, there may be many history files (from migration backfill). Do NOT try to read them all. Process them one at a time across multiple cycles. The 5-minute follow-up schedule will work through the backlog efficiently.

If the journal doesn't exist yet, create it from the first history file you read. First impressions matter — even a sparse journal is better than none.

## Molt Preparation

When context pressure rises, your molt summary should include:
- Per-project last-processed timestamps
- Count of pending files per project
- Any skipped files and why
- A brief note on profile update status

Your future self needs these exact timestamps to continue without reprocessing.
