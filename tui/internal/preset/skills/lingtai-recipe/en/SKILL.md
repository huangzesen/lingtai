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
  recipe.json             # Required — name and description
  en/
    recipe.json           # Optional — lang-specific override
    greet.md
    comment.md
    covenant.md           # Optional — overrides system-wide covenant
    procedures.md         # Optional — overrides system-wide procedures
  zh/
    greet.md
    comment.md
    covenant.md           # Optional
    procedures.md         # Optional
  skills/                 # Optional: recipe-shipped skills
    my-skill/
      en/
        SKILL.md
        scripts/          # Optional helper scripts
        assets/           # Optional assets
      zh/
        SKILL.md
```

## The Five Components

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

### 3. `covenant.md` — Covenant Override (Optional)

Overrides the system-wide covenant (`~/.lingtai-tui/covenant/<lang>/covenant.md`) for agents created with this recipe. When present, the recipe's covenant is used instead of the global one.

**Purpose:** Some recipes need a fundamentally different covenant. For example, a utility agent that should never spawn avatars or participate in networks needs a simpler covenant than the default.

**Rules:**
- No placeholders — static text
- If absent, the system-wide covenant is used (no change in behavior)
- Follows the same i18n fallback as greet.md and comment.md

### 4. `procedures.md` — Procedures Override (Optional)

Overrides the system-wide procedures (`~/.lingtai-tui/procedures/<lang>/procedures.md`) for agents created with this recipe. When present, the recipe's procedures are used instead of the global ones.

**Purpose:** Some recipes need different operational procedures. For example, a utility agent may need simplified or entirely different procedures than the default.

**Rules:**
- No placeholders — static text
- If absent, the system-wide procedures are used (no change in behavior)
- Follows the same i18n fallback as greet.md and comment.md

### 5. `skills/` — Recipe-Shipped Skills

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

**i18n:** Each skill can have language-specific versions. Resolution: try `<lang>/` first, fall back to root. See fallback rules below.

**Symlink naming:** The TUI creates symlinks in `.lingtai/.skills/` named `<recipe>-<skill>-<lang>` (lang-specific) or `<recipe>-<skill>` (root fallback). This prevents collisions across recipes.

**Scripts and assets:** Place them alongside `SKILL.md` in the same language directory. They are self-contained per language.

## recipe.json — Recipe Manifest

Every recipe must contain a `recipe.json` at root level (language-specific overrides are optional):

```json
{
  "name": "My Recipe Name",
  "description": "One-line description of what this recipe does"
}
```

- `name` — **required**, displayed in the TUI recipe picker
- `description` — **required**, shown as hint text in the picker
- Extra fields are ignored but tolerated (forward-compatible)

Without a valid `recipe.json`, the recipe will not be recognized as importable. The TUI only auto-detects `.lingtai-recipe/` directories that contain a valid manifest.

## i18n Fallback Rules

All recipe files (greet.md, comment.md, covenant.md, procedures.md, skill directories) use the same resolution:

1. Try `<lang>/` — language-specific version
2. Fall back to root

**Root is mandatory.** Every recipe file that exists must have a root-level version. Language-specific directories are optional enhancements. If only root exists, all languages get the same content.

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

When you run `/export network` (or `/export recipe` for a recipe-only export), the export flow includes a step to create a launch recipe at `.lingtai-recipe/` in the project root. This recipe travels with the published network and is automatically used by recipients who clone it.

## Testing

Point `/setup`'s custom recipe picker at your directory. The orchestrator restarts with your greet, comment, and skills immediately. Iterate until satisfied, then publish.
