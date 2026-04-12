# Launch Recipes: Portable Agent Culture

*April 10, 2026*

---

When you share an agent network, what are you really sharing?

Not the model. Not the API key. Not even the agents themselves — those are just processes. What you're sharing is a *way of working*: how the orchestrator greets new users, what behavioral constraints it follows, what skills it has, how it delegates. The culture of the network.

That's what a launch recipe is.

## The problem

Before recipes, the TUI had a boolean: `greeting: true` or `greeting: false`. One hardcoded greeting, one on/off switch. If you wanted to change what the orchestrator said on first contact, you edited a file manually. If you wanted to ship a guided tutorial experience, that was a completely separate `/tutorial` command with its own code path.

Sharing a network via Agora meant sharing agents, mail, and topology — but the first-contact experience was either generic or missing entirely. The recipient cloned your network and got... silence, or a canned welcome.

## What a recipe is

A recipe is a directory:

```
my-recipe/
  recipe.json             # name and description
  greet.md                # first message to new users
  comment.md              # persistent behavioral instructions
  skills/                 # capabilities that travel with the recipe
    research-guide/
      en/
        SKILL.md
      zh/
        SKILL.md
```

Three layers, each doing something different:

**`recipe.json`** is the manifest. A name and a description. This is what the TUI shows in the picker. Without it, the recipe doesn't exist.

**`greet.md`** is the first contact. The orchestrator sends this message when a new user opens the TUI. It's written from the orchestrator's perspective — first person, proactive, inviting. Placeholders like `{{time}}`, `{{location}}`, and `{{lang}}` are substituted at setup time, so each user gets a personalized greeting.

**`comment.md`** is the ongoing playbook. It's injected into the orchestrator's system prompt on every turn. Not a one-time greeting — a permanent behavioral constraint. "Walk users through these topics in order." "Delegate citation questions to citation-agent." "Be patient." This is where the culture lives.

**`skills/`** is the capability layer. Skills are markdown files (SKILL.md) that agents load on demand. A research recipe might ship skills for structured note-taking, citation management, and literature review. A coding recipe might ship skills for PR workflow and TDD. The skills travel with the recipe — symlinked into `.lingtai/.skills/` automatically on TUI startup.

## Five bundled recipes

The TUI ships with four bundled recipes plus a custom option:

**Adaptive** (default) — minimal greeting, progressive discovery. The orchestrator watches what you're doing and suggests features at the moment they become useful. It tracks what it has introduced via psyche memory, so you never get the same tip twice.

**Greeter** — comprehensive guided greeting with a full feature overview. For users who want to know everything upfront.

**Plain** — nothing. No greeting, no comment. The orchestrator starts silent. For power users who know the system.

**Tutorial** — step-by-step walkthrough. The orchestrator teaches you how lingtai works, one concept at a time.

**Custom** — point it at any directory on your filesystem. Your own greet, your own comment, your own skills.

## Imported recipes

When you clone a network from Agora, the publisher's recipe lives at `.lingtai-recipe/` in the project root. If it contains a valid `recipe.json`, the TUI auto-detects it and shows it as the first option in the picker — above Adaptive, with the recipe's own name and description.

```
  Recipe

  > OpenClaw Explainer    Imported
  ─────────────────────────────────
    Adaptive               Recommended
    Greeter
    Plain
    Tutorial
    Custom                 Enter folder path
```

The recipient doesn't need to know where the recipe directory is or what it's called. They see the name the publisher chose, select it, and the orchestrator wakes up speaking the publisher's language.

This is the bridge between Agora (sharing networks) and recipes (shaping behavior). A published network carries its culture with it.

## Recipe skills: capabilities that travel

The most interesting part of the recipe system is skills. Before recipes could carry skills, all the behavioral guidance had to live in `comment.md` — injected into the system prompt every turn, burning tokens whether or not it was relevant.

Now, `comment.md` can be short: "You have recipe skills available — consult them when appropriate." The heavy content moves to skills that agents load on demand. A 90-line adaptive discovery playbook becomes a 20-line comment plus two focused skills.

Recipe skills are symlinked, not copied. The TUI creates symlinks from `.lingtai/.skills/` to the recipe's skill directories on every startup. This means:

- **Single source of truth.** The recipe directory is the canonical location. No sync drift.
- **All recipes' skills coexist.** Switching recipes only changes greet and comment — skills from all recipes remain available. You don't lose capabilities by changing your greeting.
- **Collision detection.** If two recipes ship a skill with the same name, the first one wins and a warning is printed. Bundled recipes always take priority.
- **Stale cleanup.** Broken symlinks (from deleted recipe directories) are automatically pruned on startup.

The naming convention is `<recipe>-<skill>[-<lang>]`. A skill called `research-guide` from a recipe called `openclaw` in English becomes `.lingtai/.skills/openclaw-research-guide-en/` — a symlink pointing to the recipe's `skills/research-guide/en/` directory.

## i18n

Every file in a recipe follows the same resolution rule: try `<lang>/` first, fall back to root. Root is mandatory. Language-specific directories are optional enhancements.

This means a recipe author can write one set of files at root level and it works for every language. If they want a Chinese-specific greeting, they add `zh/greet.md`. If they want a Chinese-specific skill, they add `skills/my-skill/zh/SKILL.md`. Everything else falls through to root.

## Self-documenting

There's a bundled skill called `lingtai-recipe` that documents the entire recipe contract. The agent can read it on demand. So when a user asks "how do I create a recipe?", the orchestrator doesn't need to know the answer from its training data — it reads the skill and walks the user through it.

The skill covers directory structure, `recipe.json` format, placeholder contract, i18n rules, skill authoring, testing via `/setup`, and exporting via `/export network`. It's the recipe for making recipes.

## The bigger picture

Lingtai agents grow through pressure and crystallization. Context fills up, the agent molts — shedding raw conversation but carrying forward distilled knowledge. Over time, the network's topology, memories, and communication patterns become the real intelligence.

A recipe is what happens when you extract the *culture* from a network and make it portable. Not the specific agents or their memories, but the way they work: how they greet, what constraints they follow, what skills they have.

A full Agora publication (agents + recipe) is a transplant. A recipe-only publication is a playbook. Both live in the same agora, both are cloned the same way. The TUI detects what's inside and routes accordingly.

This is orchestration as a service — not the model, not the framework, but the orchestration itself. The recipe is the smallest portable unit of that orchestration. Now you can share it.

## Try it

```bash
brew install huangzesen/lingtai/lingtai-tui
lingtai-tui
```

Create a network, let it run, then create a `.lingtai-recipe/` directory with a `recipe.json`, `greet.md`, and `comment.md`. Point `/setup` at it. Your orchestrator restarts with your recipe immediately.

When you're ready to share, `/export network` packages everything — agents, recipe, and skills — into a git repo that anyone can clone and run.

---

*Launch recipes ship in lingtai-tui v0.5.0. The recipe picker, imported recipe detection, recipe-shipped skills, and the `lingtai-recipe` self-documenting skill are all included.*
