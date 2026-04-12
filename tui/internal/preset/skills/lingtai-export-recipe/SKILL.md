---
name: lingtai-export-recipe
description: Export a recipe — distill the culture, skills, and behavioral patterns of the current network into a portable recipe that others can use to seed new networks. Use when the human asks you to export, share, or package a recipe.
version: 1.0.0
---

# lingtai-export-recipe: Exporting a Recipe

A **recipe** is the culture of a network, distilled into a portable seed. It captures how an orchestrator greets users, what behavioral constraints it follows, what skills it ships, and optionally what covenant and procedures it operates under. When someone applies a recipe, they inherit the network's "DNA" — its style, knowledge, and capabilities — without any project-specific state.

Your job is to help the human reflect on their network's culture and package the parts worth sharing into a recipe at `~/lingtai-agora/recipes/<name>/`.

## How to talk to the human during this skill

**Use the `email` tool for every message to the human. Never rely on text output.**

This is a multi-round conversation with real latency between turns. The human may not be watching their terminal — they will see your messages reliably only through their inbox. Every question, status update, and confirmation goes through `email(action="send", address="human", ...)`.

## Recipe Structure

A recipe directory contains:

```
<name>/
  recipe.json             # Required — name and description
  greet.md                # Required — first-contact message for new users
  comment.md              # Required — behavioral DNA (persistent system prompt)
  covenant.md             # Optional — foundational principles override
  procedures.md           # Optional — operational norms override
  en/                     # Optional — language-specific variants
    greet.md
    comment.md
  zh/
    greet.md
    comment.md
  wen/
    greet.md
    comment.md
  skills/                 # Optional — reusable capabilities
    <skill-name>/
      SKILL.md
      scripts/            # Optional helper scripts
      assets/             # Optional assets
```

## Step 0: Reflect on the Network

Before asking the human anything, examine the living network to understand its culture:

1. Read the current recipe state:

```bash
cat .lingtai/.tui-asset/.recipe
```

2. Examine the current comment.md (behavioral DNA):

```bash
# Find the current recipe's comment file
# If recipe is "custom" or "imported", check the custom_dir
# If bundled, check ~/.lingtai-tui/recipes/<name>/
```

3. List all installed skills:

```bash
ls -la .lingtai/.skills/
```

4. Scan the network structure — agent names, roles, specializations:

```bash
ls .lingtai/*/
cat .lingtai/*/.agent.json
```

5. Skim recent mail to understand tone, delegation patterns, working style:

```bash
ls .lingtai/*/mailbox/archive/ | head -20
```

Build a mental model of: what does this network *do*? How does it *behave*? What *skills* has it grown? What makes it distinctive?

## Step 1: Discuss with the Human

Send the human an email introducing the export flow and what you've observed:

> "I've looked at your network and here's what I see as its culture:
>
> [2-3 sentence summary of the network's identity, style, and capabilities]
>
> A recipe distills this into something portable. What parts of this culture do you want to package for others?
>
> Some questions to guide us:
> - Who is the intended audience for this recipe?
> - What should the orchestrator say on first contact?
> - What behavioral constraints should carry over?
> - Which skills are worth including?"

**This is a creative conversation, not a checkbox.** Let the human guide what to include. They know what's essential vs. project-specific.

## Step 2: Name the Recipe

Ask the human for a name. The name becomes:
- The directory name under `~/lingtai-agora/recipes/`
- The `name` field in `recipe.json`

> "What should this recipe be called? Pick something that captures its essence — this is what people will see when browsing the agora."

Do not suggest a default based on the project name — the recipe is about culture, not the project.

If `~/lingtai-agora/recipes/<name>/` already exists, ask before overwriting.

## Step 3: Author the Recipe Files

Work through each file with the human. You WRITE the content (not copy) — the recipe should be a distillation, not a raw dump of existing files.

### 3a. recipe.json

```bash
mkdir -p ~/lingtai-agora/recipes/<name>
```

Write:

```json
{
  "name": "<human's chosen name>",
  "description": "<one-line description agreed with the human>"
}
```

### 3b. greet.md — First Contact

Write a greeting message from the orchestrator's perspective. This is how the recipe introduces itself to a new user.

**Rules:**
- Keep it short (5-10 sentences)
- Be proactive — introduce the network's purpose, don't wait to be asked
- If the network has multiple agents, remind users to `/cpr all`
- Available placeholders: `{{time}}`, `{{addr}}`, `{{lang}}`, `{{location}}`, `{{soul_delay}}`

Show the draft to the human. Iterate until satisfied.

### 3c. comment.md — Behavioral DNA

This is the heart of the recipe. It captures:
- The orchestrator's role and personality
- How it delegates to other agents
- What topics or workflows it guides users through
- Tone and style constraints
- References to recipe-shipped skills (by name)

**Draw from the living network** — look at how the orchestrator actually behaves (its current comment, its mail patterns, its delegation style) and distill that into portable instructions.

**No placeholders** — this is static text injected every turn. Every token counts.

Show the draft to the human. Iterate until satisfied.

### 3d. skills/ — Reusable Capabilities (Optional)

If the human wants to include skills, discuss which ones:

```bash
ls -la .lingtai/.skills/
```

For each skill the human wants to include:

1. Check if it's a bundled skill (shipped with the TUI) — if so, don't copy it; it's already available everywhere
2. If it's a custom or recipe skill, copy the skill directory:

```bash
mkdir -p ~/lingtai-agora/recipes/<name>/skills/<skill-name>
cp -R .lingtai/.skills/<skill-name>/* ~/lingtai-agora/recipes/<name>/skills/<skill-name>/
```

3. Verify each skill has a valid `SKILL.md` with proper frontmatter

### 3e. covenant.md — Foundational Principles (Optional)

Only create this if the network's principles fundamentally differ from the system default. Ask the human:

> "Does this recipe need its own covenant, or will the system default work? Most recipes don't need a custom covenant unless the network operates under fundamentally different principles."

If yes, draft it with the human. If no, skip — the system default will be used.

### 3f. procedures.md — Operational Norms (Optional)

Same as covenant — only if needed:

> "Does this recipe need custom procedures? Only if the operational norms differ significantly from the default."

If yes, draft it. If no, skip.

## Step 4: Multi-Language Variants (Optional)

If the human mentions a multi-language audience:

> "Would you like to create versions in other languages? The TUI tries `<lang>/greet.md` first, then falls back to the root file. Supported: en, zh, wen."

If yes, create per-language subdirectories and translate the greet.md and comment.md. If no, the root-level files serve all languages.

## Step 5: Review

Show the human the complete recipe structure:

```bash
find ~/lingtai-agora/recipes/<name>/ -type f | sort
```

Read back each file's content. Ask for final review:

> "Here's the complete recipe. Read through each file and let me know if you want to change anything before I commit."

Iterate until the human approves.

## Step 6: git init + commit

```bash
cd ~/lingtai-agora/recipes/<name>/
git init -b main
git add .
git status
```

Show the human `git status`. Get confirmation. Then:

```bash
git commit -m "Recipe: <name>"
```

## Step 7: Push to GitHub (Optional)

Check `gh auth status` and follow the same three-branch pattern as the network export:

- **Branch A (gh ready):** Ask if they want to push, confirm repo name and public/private, run `gh repo create <name> --source=. --<public|private> --push`
- **Branch B (gh installed but not authenticated):** Guide through `gh auth login`
- **Branch C (gh not installed):** Offer install instructions

If the human declines GitHub, remind them they can push manually later.

## Things to Watch Out For

**Don't copy blindly.** The recipe should be authored, not dumped. A raw copy of the current comment.md might reference project-specific agents, paths, or context that won't exist in the recipient's network.

**Skills must be self-contained.** Each skill directory should work independently. Check that scripts don't reference absolute paths or project-specific resources.

**The recipe is a seed, not a clone.** It shapes behavior on first contact and ongoing — it does NOT reproduce the network's state, history, or data. That's what `/export network` is for.

**Bundled skills don't need copying.** Skills shipped with the TUI (lingtai-export-network, lingtai-recipe, lingtai-mcp, etc.) are already available in every installation. Only copy custom or recipe-specific skills.

**recipe.json is mandatory.** Without it, the TUI won't recognize the directory as a valid recipe. Always create it with at least a `name` field.
