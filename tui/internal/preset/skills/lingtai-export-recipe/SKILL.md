---
name: lingtai-export-recipe
description: Export a recipe — distill the culture, skills, and behavioral patterns of the current network into a portable recipe that others can use to seed new networks. Use when the human asks you to export, share, or package a recipe.
version: 2.0.0
---

# lingtai-export-recipe: Exporting a Recipe

**Prerequisites:** Read the `lingtai-recipe` skill first — it defines what a recipe is, the directory structure, the five components (greet.md, comment.md, covenant.md, procedures.md, skills/), placeholders, i18n rules, and recipe.json format. This skill assumes you understand all of that.

A recipe is the culture of a network, distilled into a portable seed. Your job is to help the human reflect on their network's culture and package the parts worth sharing.

## How to talk to the human during this skill

**Use the `email` tool for every message to the human. Never rely on text output.**

This is a multi-round conversation with real latency between turns. The human may not be watching their terminal — they will see your messages reliably only through their inbox. Every question, status update, and confirmation goes through `email(action="send", address="human", ...)`.

## Critical: Filesystem Rules

These rules prevent silent failures. Follow them without exception.

1. **Resolve `$HOME` first.** The `write` tool does NOT expand `~`. At the start of this skill, run:
   ```bash
   echo $HOME
   ```
   Use the result (e.g., `/Users/alice`) as the prefix for ALL file paths. Never use `~` in a `write` or `file` tool call.

2. **Always use absolute paths.** Every `write` call must use a full absolute path. The `write` tool resolves relative paths from your working directory, not from the recipe directory.

3. **Always `mkdir -p` before writing.** The `write` tool may silently fail or report false success if the parent directory does not exist.

4. **Verify after writing.** After writing all files in a step, run `find <recipe-dir> -type f | sort` and confirm the output lists every file you intended to create.

5. **Never trust a write success message at face value.** Always verify with `find` or `ls`.

## Step 0: Resolve Paths + Reflect on the Network

**0a. Resolve the recipe base directory.**

```bash
echo $HOME
```

Store the result. All paths use `$HOME/lingtai-agora/recipes/` as the base. **Note: `lingtai-agora`, NOT `.lingtai-agora` — no leading dot.** The agora directory is a user-visible workspace, not a hidden config directory.

**0b. Read the `lingtai-recipe` skill** to refresh your understanding of recipe structure and components.

**0c. Reflect on the living network.** Before asking the human anything, examine the network to understand its culture:

1. Read the current recipe state: `cat .lingtai/.tui-asset/.recipe`
2. Examine the current comment.md (behavioral DNA) — find it via the recipe state
3. List all installed skills: `ls -la .lingtai/.skills/`
4. Scan the network structure: `ls .lingtai/*/` and `cat .lingtai/*/.agent.json`
5. Skim recent mail for tone and delegation patterns: `ls .lingtai/*/mailbox/archive/ | head -20`

Build a mental model of: what does this network *do*? How does it *behave*? What *skills* has it grown? What makes it distinctive?

## Step 1: Collect Metadata from the Human

Send the human **one** email introducing the export flow and collecting all key decisions upfront. This reduces round-trips.

> "I've looked at your network and here's what I see as its culture:
>
> [2-3 sentence summary of the network's identity, style, and capabilities]
>
> A recipe distills this into something portable. To get started, I need a few things:
>
> 1. **Recipe name** — something that captures its essence (not the project name — the recipe is about culture, not the project)
> 2. **One-line description** — what does this recipe give someone?
> 3. **Audience** — who is this recipe for?
> 4. **Greeting style** — what tone should the first-contact message have?
> 5. **Skills to include** — which of the installed skills should ship with the recipe?
> 6. **Any behavioral constraints** — what should carry over from this network's culture?
>
> Here are the installed skills:
> [list from ls .lingtai/.skills/]
>
> Answer as many as you can in one message and I'll draft everything in one pass."

If `$HOME/lingtai-agora/recipes/<name>/` already exists, ask before overwriting.

## Step 2: Author the Recipe Files

Once you have the human's input, author all files in one pass. You WRITE the content (not copy) — the recipe should be a distillation, not a raw dump of existing files. Refer to the `lingtai-recipe` skill for the exact format and rules of each component.

### Pre-flight: Create all directories first

```bash
RECIPE_DIR="$HOME/lingtai-agora/recipes/<name>"
mkdir -p "$RECIPE_DIR"
mkdir -p "$RECIPE_DIR/skills/<skill-1>"
mkdir -p "$RECIPE_DIR/skills/<skill-2>"
```

### 2a. recipe.json

Write `name` and `description` (see `lingtai-recipe` skill for format).

### 2b. greet.md — First Contact

Write a greeting from the orchestrator's perspective. Follow the rules and placeholders documented in `lingtai-recipe`. Write fresh recipe-specific content — do NOT copy templates or include `[system]` prefixes.

### 2c. comment.md — Behavioral DNA

This is the heart of the recipe. **Draw from the living network** — look at how the orchestrator actually behaves and distill that into portable instructions. See `lingtai-recipe` for the format rules (no placeholders, static text, injected every turn).

**What to distill.** Walk through each of these areas and extract what's worth keeping:

- **Delegation and avatar rules** — how does the orchestrator decide when to spawn avatars vs handle things itself? What avatar blueprints does it use? If there are specific naming conventions, specialization patterns, or spawn-on-demand rules, capture them. Reflect on the avatar rules you've set in this network — if they work well, they belong in comment.md.
- **Communication norms** — does the network enforce deposit-before-email (write findings to a file before sending a summary)? Are there conventions about email length, format, or frequency between agents?
- **Workflow patterns** — is there a specific order of operations? Does the orchestrator follow a pipeline (research → draft → review → publish)? Are there quality gates or checkpoints?
- **Tool usage conventions** — any rules about which tools to prefer, when to use bash vs file tools, when to use web search? Any cost-awareness rules (e.g., avoid redundant API calls)?
- **Tone and style** — formal vs casual? Terse vs detailed? Does the orchestrator have a persona or voice?
- **Guardrails** — what does the orchestrator explicitly avoid? Topics it won't engage with? Actions it won't take without human approval?
- **Skill references** — if the recipe ships skills, how and when should the orchestrator invoke them? What triggers each skill?
- **Network topology** — how many agents does this network typically grow to? Is there a hierarchy (orchestrator → specialists → workers)? Any rules about network size or structure?

**Where to look:**
- The current `comment.md` — what's already codified
- The orchestrator's recent mail — how it actually delegates and responds
- Avatar `.agent.json` blueprints — what specialized agents exist and why
- The covenant and procedures — any custom overrides already in place
- The human's feedback patterns — what corrections has the human made repeatedly?

**Distillation technique:** For each behavioral norm you observe (e.g., "agents always deposit findings before emailing"), write it as an explicit rule (e.g., "Always write your findings to a file before sending an email summary"). Transform living behavior → explicit rule → readable prose.

### 2d. skills/ — Reusable Capabilities (Optional)

For each skill the human wants to include:

1. Check if it's an intrinsic skill (in `.skills/intrinsic/`) — if so, don't copy it; it's already available everywhere
2. If it's a custom or recipe skill, copy the skill directory:

```bash
mkdir -p $HOME/lingtai-agora/recipes/<name>/skills/<skill-name>
cp -R .lingtai/.skills/custom/<skill-name>/* $HOME/lingtai-agora/recipes/<name>/skills/<skill-name>/
```

3. Verify each skill has a valid `SKILL.md` with proper frontmatter

### 2e–2f. covenant.md / procedures.md (Optional)

Only create these if the network's principles or procedures fundamentally differ from the system default. Most recipes don't need them. See `lingtai-recipe` for details.

### Post-write verification (MANDATORY)

```bash
find $HOME/lingtai-agora/recipes/<name>/ -type f | sort
```

**Check the output against your intended file list.** If any file is missing, re-create its parent directory and re-write it. **Do not proceed until all files are confirmed on disk.**

## Step 3: Review with the Human

Show the human the `find` output and read back each file's content via email. Iterate until the human approves.

## Step 4: Multi-Language Variants (Optional)

If the human mentions a multi-language audience, create per-language subdirectories for greet.md and comment.md. See `lingtai-recipe` for i18n fallback rules.

## Step 5: git init + commit

```bash
cd $HOME/lingtai-agora/recipes/<name>/
git init -b main
git add .
git status
```

Show `git status` to the human. Get confirmation. Then: `git commit -m "Recipe: <name>"`

## Step 6: Push to GitHub (Optional)

Check `gh auth status` and follow the three-branch pattern:

- **Branch A (gh ready):** Ask if they want to push, confirm repo name and visibility, run `gh repo create`
- **Branch B (gh installed but not authenticated):** Guide through `gh auth login`
- **Branch C (gh not installed):** Offer install instructions

## Things to Watch Out For

**Don't copy blindly.** The recipe should be authored, not dumped. A raw copy of the current comment.md might reference project-specific agents, paths, or context that won't exist in the recipient's network.

**Skills must be self-contained.** Each skill directory should work independently. Check that scripts don't reference absolute paths or project-specific resources.

**The recipe is a seed, not a clone.** It shapes behavior — it does NOT reproduce the network's state, history, or data. That's what `/export network` is for.

**Intrinsic skills don't need copying.** Skills under `.skills/intrinsic/` are shipped with the TUI and already available in every installation.
