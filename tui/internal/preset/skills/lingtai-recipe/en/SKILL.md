---
name: lingtai-recipe
description: Guide for creating and understanding launch recipes — the mechanism that shapes how an orchestrator greets users, what behavioral constraints it follows, and what skills it ships. Use when the human asks about recipes, wants to create a custom recipe, or needs to understand how recipes work.
version: 1.0.0
---

# lingtai-recipe: Creating Launch Recipes

A **launch recipe** is a named directory that shapes an orchestrator's first-contact behavior, ongoing constraints, and available skills. Every lingtai project uses a recipe — selected during `/setup` or inherited from a published network via `/agora`.

## Recipe Directory Structure

```
my-recipe/
  en/
    greet.md              # First message to new users
    comment.md            # Persistent behavioral instructions
  zh/
    greet.md
    comment.md
  skills/                 # Optional: recipe-shipped skills
    my-skill/
      en/
        SKILL.md
        scripts/          # Optional helper scripts
        assets/           # Optional assets
      zh/
        SKILL.md
```

## The Three Components

### 1. `greet.md` — First Contact

The first message the orchestrator sends when a new user opens the TUI. Written from the orchestrator's perspective (first person).

**Purpose:** Set the tone, introduce the network, tell the user what they can do, offer guidance.

**Placeholders** (substituted at setup time):

| Placeholder | Value |
|---|---|
| `{{time}}` | Current date and time (2006-01-02 15:04) |
| `{{addr}}` | Human's email address in the network |
| `{{lang}}` | Language code (en, zh, wen) |
| `{{location}}` | Human's geographic location (City, Region, Country) |
| `{{soul_delay}}` | Soul cycle interval in seconds |

**Example:**

```markdown
Welcome to the OpenClaw Explainer Network! It's {{time}}.

I'm the lead orchestrator of a team of 10 agents. Type /cpr all
to wake everyone up, then tell me what you'd like to explore.
```

**Rules:**
- Keep it short (5-10 sentences max)
- Be proactive — introduce yourself, don't wait to be asked
- Always remind users to `/cpr all` to wake the full team (if the network has multiple agents)
- Use `{{time}}` and `{{location}}` to make the greeting feel alive

### 2. `comment.md` — Ongoing Behavioral Constraints

Injected into the orchestrator's system prompt on every turn. The persistent playbook.

**Purpose:** Define what topics to cover, how to delegate, constraints, tone. Think of it as a covenant extension specific to this recipe.

**Rules:**
- No placeholders — this is static text
- Keep it focused and concise — it's injected every turn, so every token counts
- Reference skills by name if the recipe ships skills (the agent can load them on demand)

### 3. `skills/` — Recipe-Shipped Skills

Optional. Skills that travel with the recipe and are automatically symlinked into `.lingtai/.skills/` when the TUI starts.

Each skill follows the standard SKILL.md contract:

```markdown
---
name: my-skill-name
description: One-line description of what this skill does
version: 1.0.0
---

# Skill content here...
```

**i18n:** Each skill can have language-specific versions. The TUI resolves:
1. `skills/<name>/<lang>/SKILL.md` — language-specific (preferred)
2. `skills/<name>/SKILL.md` — root fallback (language-agnostic)

**Symlink naming:** The TUI creates symlinks in `.lingtai/.skills/` named `<recipe>-<skill>-<lang>` (lang-specific) or `<recipe>-<skill>` (root fallback). This prevents collisions across recipes.

**Scripts and assets:** Place them alongside `SKILL.md` in the same language directory. They are self-contained per language.

## i18n Fallback Rules

All recipe files use the same resolution pattern:

1. Try `<recipe>/<lang>/<file>` — language-specific version
2. Try `<recipe>/<file>` — root fallback
3. Skip if neither exists

This applies to `greet.md`, `comment.md`, and skill directories.

## Recipe Types

| Type | Location | When Linked |
|---|---|---|
| Bundled | `~/.lingtai-tui/recipes/<name>/` | Always (shipped with TUI) |
| Custom | User-specified directory | When set via `/setup` |
| Agora | `<project>/.lingtai-recipe/` | When agora project exists |

All types follow the same directory structure and rules.

## How to Create a Custom Recipe

1. Create a directory with the structure above
2. Write at least a `greet.md` (comment.md and skills/ are optional)
3. In the TUI, run `/setup`, select "Custom" recipe, and enter the path to your directory
4. The orchestrator will restart and use your recipe

## How to Publish a Recipe

When you run `/agora publish`, the publishing flow includes a step to create a launch recipe at `.lingtai-recipe/` in the project root. This recipe travels with the published network and is automatically used by recipients who clone it.

## Testing

Point `/setup`'s custom recipe picker at your directory. The orchestrator restarts with your greet, comment, and skills immediately. Iterate until satisfied, then publish.
