---
name: skills-manual
description: What skills are, how to write them, where to find them, and when to create one
version: 1.1.0
---

# Skills Manual

## What is a Skill?

A skill is a **structured instruction bundle** that teaches you how to perform a specialized task. Each skill lives in its own folder with a `SKILL.md` file (this format) and optional supporting files (scripts, templates, data, reference docs).

You already have skills loaded — they appear as `<available_skills>` in your system prompt. When a task matches a skill's description, load the SKILL.md via `read` and follow its instructions.

## SKILL.md Format

```markdown
---
name: my-skill
description: One-line description (this is what you see in the catalog)
version: 1.0.0
---

Step-by-step instructions in Markdown...
```

**Required frontmatter:** `name`, `description`.
**Optional frontmatter:** `version`, `author`, `tags`.

### Skill Folder Structure

```
my-skill/
  SKILL.md              # Entry point — instructions (keep under 500 lines)
  scripts/              # Deterministic helper scripts (Python, Bash)
  references/           # Supplementary context (schemas, cheatsheets)
  assets/               # Templates, static files
```

SKILL.md should reference supporting files by relative path. Offload dense content to subdirectories — keep SKILL.md focused on the procedure.

### Skill Store Layout

The skill store at `.lingtai/.skills/` is organized into group folders:

```
.skills/
  intrinsic/            # Bundled with the TUI — managed automatically
    lingtai-mcp/
    lingtai-export-recipe/
    ...
  <recipe-name>/        # From recipes — managed by the TUI
    research-skill/
    writing-skill/
    ...
  custom/               # YOUR skills — create new skills here
    my-workflow/
      SKILL.md
    data-pipeline/
      SKILL.md
```

**When creating a new skill, always place it under `.skills/custom/`.** Group folders can nest arbitrarily deep — a folder with `SKILL.md` is a skill; a folder containing only subfolders is a group.

```
custom/
  research/             # Group — no SKILL.md, just subfolders
    arxiv/              # Skill — has SKILL.md
      SKILL.md
      scripts/
    scholar/            # Skill — has SKILL.md
      SKILL.md
  my-standalone-skill/  # Skill — has SKILL.md
    SKILL.md
```

**Corruption rule:** a folder with no `SKILL.md` that contains loose files (not just subdirectories) is considered corrupted and will be flagged as a problem.

## When to Create a Skill

**Create a skill when:**
- A task is repeatable with consistent steps (e.g., "run a security audit", "generate a report")
- The procedure requires domain knowledge you don't reliably have (e.g., a specific API's quirks, company guidelines)
- A workflow involves multi-step orchestration with decision trees
- You want to share expertise with other agents in your network
- You find yourself repeating the same instructions across conversations

**Do NOT create a skill when:**
- You already handle the task reliably without instructions
- It's a one-off task with no reuse value
- The task is just "call this API" — use an MCP server instead
- The instructions are personality/style preferences — use covenant or character instead

## How to Write a Good Skill

1. **Trigger-optimized description** — include what it does AND what it doesn't do. The description is the only thing visible in the catalog.
2. **Numbered steps in imperative form** — "Extract the text...", "Run the script...", not "You should extract..."
3. **Concrete templates** in `assets/` rather than prose descriptions of desired output.
4. **Deterministic scripts** in `scripts/` for fragile/repetitive operations (parsing, generation, validation).
5. **Keep SKILL.md under 500 lines** — offload to `references/` and `scripts/`.
6. **Flat subdirectory structure** — one level deep (`references/schema.md`, not `references/db/v1/schema.md`).

## Where to Find Skills

### Notable Repositories

| Repository | What |
|---|---|
| `github.com/anthropics/skills` | Official reference skills + Agent Skills spec |
| `github.com/VoltAgent/awesome-openclaw-skills` | 5,400+ curated skills |
| `github.com/VoltAgent/awesome-agent-skills` | 1,000+ cross-platform agent skills |
| `github.com/hesreallyhim/awesome-claude-code` | Curated skills, hooks, plugins for Claude Code |
| `github.com/sickn33/antigravity-awesome-skills` | 1,344+ installable SKILL.md files |
| `github.com/trailofbits/skills` | Security research and audit skills |
| `github.com/alirezarezvani/claude-skills` | 220+ skills for coding agents |

### Registries

| Registry | URL |
|---|---|
| Agent Skills spec | `agentskills.io` |
| ClawHub (OpenClaw) | `clawhub.ai` |
| Cursor directory | `cursor.directory` |

### Installing a Skill

Use bash to clone or download into the `custom/` group:

```bash
# Clone a skill repo
cd <skills-dir>/custom
git clone https://github.com/someone/useful-skill.git

# Or download a single skill folder
# (copy/extract into <skills-dir>/custom/skill-name/)
```

Then call `skills(action='register')` to validate and commit.

To update a skill that's a git repo:
```bash
cd <skills-dir>/custom/skill-name
git pull
```
Then `skills(action='register')` again.

To pick up skills another agent registered: `skills(action='refresh')`.

## What to Consolidate into a Skill

When you discover a useful, repeatable procedure — whether from research, experimentation, or instruction — write it as a skill so your network benefits:

- **Research workflows** — how to find, evaluate, and synthesize information on a topic
- **Code patterns** — project-specific conventions, testing strategies, deployment procedures
- **Data processing pipelines** — parsing, transformation, validation steps
- **Communication templates** — report formats, email templates, briefing structures
- **Domain expertise** — specialized knowledge that requires specific steps (legal review, security audit, scientific analysis)

The test: **if you'd need to re-explain it next time, make it a skill.**
