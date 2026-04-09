---
name: lingtai-agora
description: Prepare the current lingtai network for publishing. Copies the project to ~/lingtai-agora/projects/<name>/, scrubs ephemeral runtime state, processes mail with a user-chosen time cutoff, writes a canonical .gitignore, scans for nested git repos and leaked secrets, and initializes a clean git repository ready to push. Use when the human asks you to share, publish, package, export, or back up this network for others.
version: 1.0.0
---

# lingtai-agora: Publishing a Network

You are about to copy the network you live in to a publishable location. **This is literal self-copying** — the snapshot will not contain the moment you made it. Everything up to this conversation turn will be in the published copy; this turn itself will only exist in the original.

Walk the human through the steps below carefully. Each step is either *mechanical* (run a script, report the result) or *interactive* (discuss a decision with the human before proceeding). Never skip the interactive steps — the whole point of a skill-driven publishing flow is that a human is in the loop for judgment calls.

All scripts live alongside this SKILL.md under `scripts/`. The canonical `.gitignore` template lives at `assets/gitignore.template`. You run the scripts with `python3 <path-to-script> ...`; resolve the absolute path from this skill's location in `.lingtai/.skills/lingtai-agora/`.

## How to talk to the human during this skill

**Use the `email` tool for every message to the human. Never rely on text output.**

This is a multi-round conversation with real latency between turns. The human may not be watching their terminal, the portal, or any specific surface — they will see your messages reliably only through their inbox. Every question you need answered, every status update the human needs to know, every final confirmation before a destructive or externally-visible action (deleting files, pushing to GitHub) — all of it goes through `email(action="send", address="human", ...)`.

Symptoms that you've drifted into text-output mode:
- You find yourself writing a message and realizing you haven't called `email` in the last several turns.
- The human sends the same question twice in a row (they didn't see your reply).
- The human says something like "reply with email", "answer me with email", "where's your response".

If any of those happen, stop, switch to email immediately, and catch up by re-sending the most recent answer through `email(action="send", ...)`. Don't argue about the channel — just fix it.

The one exception: run-time tool output (script results, `git status`, `du -sh`) is fine to narrate in your own working turns, because you are reasoning about it yourself. The rule is specifically about *messages directed at the human*.

## Step 0: Decide the project name

Infer a default name from the current project folder's basename (the directory that contains `.lingtai/`). Ask the human:

> "I'm going to copy this network to `~/lingtai-agora/projects/<default>/`. Use that name, or a different one?"

Accept the human's override if given. The final path is `~/lingtai-agora/projects/<name>/`. If that path already exists, ask before proceeding:

> "`~/lingtai-agora/projects/<name>/` already exists. Overwrite it, or pick a different name?"

Never silently overwrite. If overwriting, `rm -rf` the old staging copy first.

## Step 1: Copy + mechanical scrub

**1a. Copy.** Use `cp -R` (or `rsync -a`) to copy the entire current project folder to `~/lingtai-agora/projects/<name>/`. The source is the directory that contains `.lingtai/` — if you're not sure where that is, the human's current project is your best signal. Confirm with the human if ambiguous.

**1b. Scrub ephemeral state.** Run:

```bash
python3 scripts/scrub_ephemeral.py ~/lingtai-agora/projects/<name>/
```

This deletes, for every agent:
- `init.json` (API keys, absolute paths)
- `.agent.lock`, `.agent.heartbeat`, `.agent.history`
- `.suspend`, `.sleep`, `.interrupt`, `.cancel`
- `events.json`
- `logs/` (entire dir)
- `.git/` (per-agent time machine — **must** be removed or the outer `git add .` in step 5 will silently skip agent contents)
- `mailbox/schedules/`

It also deletes project-level publisher-specific state under `.lingtai/` itself:
- `.lingtai/.portal/` (portal event stream + replay cache — `topology.jsonl` can reach hundreds of MB and leaks the timeline)
- `.lingtai/.tui-asset/` (TUI-local cache, regenerated on launch)
- `.lingtai/.addons/` (publisher's addon config — points at publisher's IMAP accounts, telegram bots, etc. Recipients configure their own addons after cloning)

`.lingtai/.skills/` is preserved — it holds canonical skills (bundled + user-added), which are part of the network's identity and belong in the published copy.

Report the totals to the human. If the script exits nonzero, stop and surface the error — do not proceed.

## Step 2: Process mail

Mail gets special handling because it's the most privacy-sensitive content in the network and because the cutoff is a judgment call.

**2a. Normalize.** Run:

```bash
python3 scripts/archive_mail.py ~/lingtai-agora/projects/<name>/
```

This wipes `sent/` and `outbox/` (the publisher's outgoing record is not part of the seed), and moves everything in `inbox/` into `archive/`. After this step, every email lives flat in `mailbox/archive/<uuid>/`.

**2b. Decide the cutoff with the human.** Count archived messages per agent:

```bash
find ~/lingtai-agora/projects/<name>/.lingtai/*/mailbox/archive -name message.json | wc -l
```

Propose a cutoff based on volume:
- Fewer than 100 messages → "Keep all? Or pick a date?"
- 100–500 messages → "Keep the last 6 months?"
- More than 500 messages → "Keep the last month?"

Present the date you propose in `YYYY-MM-DD` form. Let the human override. **Do not pick a cutoff without human input** — this is the main judgment call in the whole flow.

**2c. Apply the cutoff.** First dry-run:

```bash
python3 scripts/filter_archive.py ~/lingtai-agora/projects/<name>/ --before YYYY-MM-DD --dry-run
```

Show the human the dry-run totals (how many old messages drop, how many malformed messages drop, how many kept). Get explicit confirmation, then run for real:

```bash
python3 scripts/filter_archive.py ~/lingtai-agora/projects/<name>/ --before YYYY-MM-DD
```

The script is re-runnable. If the human wants a different cutoff later, just re-run with the new date — filtering is one-way (you can only drop more, never restore).

## Step 3: Interactive project review

This is the longest interactive step. Work through it patiently with the human.

**3a. Scan for nested git repositories.** Run:

```bash
python3 scripts/scan_nested_repos.py ~/lingtai-agora/projects/<name>/
```

For each nested repo found (each one outside `.lingtai/`), discuss with the human:

> "Found `vendor/thirdparty/` — it's a git repo with remote `https://github.com/...`. This looks like a vendored dependency. Options:
> - **Ignore** (add to `.gitignore`, don't publish it — recipients will need to get it themselves)
> - **Inline** (strip the inner `.git/` so its files become part of this repo — you lose the linkage to the upstream)
>
> Which do you want?"

Default recommendations:
- **Sibling worktrees** (things that look like `lingtai-*`, `experiments/`, `*-dev`) → default **ignore**
- **Vendored deps with a remote URL** → default **ignore** (recipient can fetch them)
- **Small directories with no remote** → ask, don't default

Act on the human's choice:
- **Ignore:** add the repo's parent path to a list that will go into `.gitignore` at the end of step 3.
- **Inline:** `rm -rf <repo>/.git` to strip the nested repo. Warn the human this is destructive and cannot be undone from the staging copy.

**Do not offer submodule support in v1** — it requires a reachable remote, writing `.gitmodules`, and a working `git submodule add`. If the human asks for it, explain that they can add it manually after the skill finishes.

**3b. Walk the top-level directories.** List every directory at the top level of the staging dir, excluding `.lingtai/` (already handled). For each one, plus any file larger than 1 MB or matching a sensitive name pattern (`.env*`, `*.key`, `*.pem`, `id_rsa`, `credentials*`, etc.), ask the human whether to ignore it.

Use `du -sh` to report directory sizes. Order directories by size (largest first) — that usually matches the priority of what needs discussing.

Examples of the kind of conversation this step produces:

> - "`data/raw/` — 147 MB, 2,341 files. Looks like a dataset. Ignore? [Y/n]"
> - "`.env` — 420 bytes. Almost certainly secrets. Adding to `.gitignore` unless you object."
> - "`notebooks/` — 3.2 MB, 12 notebooks. Could be valuable context. Keep? [Y/n]"
> - "`models/` — 4.1 GB. That's a lot. Ignore? [Y/n]"

**3c. Write `.gitignore`.** Read the template at `assets/gitignore.template` (relative to this SKILL.md) and write it to `~/lingtai-agora/projects/<name>/.gitignore`. Then append any additional ignores collected in steps 3a and 3b under a clearly-labeled `# Added during publishing review` section.

The canonical template covers: `.lingtai/` runtime state (init.json, logs, locks, etc.), `mailbox/` working state (inbox, outbox, sent, schedules — only `archive/` is versioned), common secret patterns, Python noise, editor/OS junk. Do not remove lines from it — downstream forks rely on this policy being complete.

## Step 4: Privacy scan

Run:

```bash
python3 scripts/privacy_scan.py ~/lingtai-agora/projects/<name>/
```

The script produces two categories of output:

- **Soft warnings** (absolute paths, email addresses, private IPs) — report to the human but do not block. These are often legitimate (a blog post mentioning `/home/user/` is fine).
- **Hard matches** (API key shapes, private key blocks) — script exits with code 3. You MUST halt and show the human every hard match.

For hard matches, the human has two options:

1. **Redact and retry.** Edit the flagged file(s) in the staging copy to remove the secret, then re-run `privacy_scan.py`. Loop until clean.
2. **False positive override.** The human explicitly states "that's not a real secret, proceed anyway". Only accept this if they are specific about which match they're overriding. Do not accept a blanket "ignore all warnings".

**Do not proceed to step 5 or 6 with unresolved hard matches.** The consequence of shipping a real API key to GitHub is an irreversible privacy incident — there is no cleanup, only key rotation.

## Step 5: Create a launch recipe for recipients (optional but recommended)

A **launch recipe** controls what the orchestrator says and how it behaves when someone clones this network and runs it for the first time. It lives at `.lingtai-recipe/` in the project root — a convention the TUI discovers automatically during setup.

A recipe has two files:

- **`greet.md`** — the first message the orchestrator sends to the new user. This is about **proactiveness**: setting the tone, introducing the network, offering to guide the user. Think of it as the elevator pitch spoken by the agent itself.

- **`comment.md`** — persistent behavioral instructions injected into the orchestrator's system prompt on every turn. This is about **constraints and guidance**: what the orchestrator should do, what topics to cover, what order to follow, what to avoid. This is the detailed playbook — the equivalent of the tutorial system's `tutorial.md`, but tailored to this specific network's purpose.

**Both files are optional.** If `.lingtai-recipe/` is absent or empty, recipients pick from the TUI's built-in recipes (greeter, plain, tutorial, etc.) during their setup wizard. Including a recipe means you've pre-packaged the ideal first experience for your network.

### 5a. Discuss the recipe with the human

Ask:

> "Do you want to include a launch recipe? This controls what the orchestrator says to someone who clones your network for the first time. I can draft a welcome message (`greet.md`) and behavioral instructions (`comment.md`) based on what this network does. What would you like recipients to experience on their first launch?"

If the human says **no** or **skip**: move on to Step 6. No `.lingtai-recipe/` is created.

If **yes**: discuss what the network is for, who the audience is, and what the orchestrator should do on first contact. Use the existing mail archive, agent names, and `.agent.json` blueprints in the staging copy to understand the network's purpose. Then draft both files.

### 5b. Draft greet.md

Write the first-contact message. This should:

- Introduce the network and its purpose in 2–4 sentences
- Offer to guide the user ("Ready to get started?" / "What would you like to explore first?")
- Be warm but not overly long — this is a `.prompt`, the agent speaks it once

**Available placeholders** — the TUI substitutes these at setup time with the recipient's own values:

| Placeholder | Replaced with | Example |
|---|---|---|
| `{{time}}` | Current date and time (YYYY-MM-DD HH:MM) | `2026-04-09 14:30` |
| `{{addr}}` | The human's email address in the network | `human` |
| `{{lang}}` | The language selected during setup | `en`, `zh`, `wen` |
| `{{location}}` | The human's location (from their .agent.json) | `Los Angeles, California, US` |
| `{{soul_delay}}` | Seconds between the orchestrator's soul cycles | `120` |

These are optional — use them only where natural. `{{time}}` and `{{lang}}` are the most useful. `{{location}}` may be `unknown` if the recipient hasn't set up location.

**Example:**

```
Welcome to the OpenClaw Explainer Network! It's {{time}}.

I'm the lead orchestrator of a team of 10 agents that can walk you through the OpenClaw legal dataset — case structure, citation format, judicial opinions, and more.

Let me know what you'd like to explore, or say "start from the beginning" and I'll take you through it step by step.
```

### 5c. Draft comment.md

Write the behavioral playbook. This is injected into the orchestrator's system prompt persistently (every turn, not just the first message). It should:

- Describe the orchestrator's role in 1–2 sentences
- List the topics or steps the orchestrator should guide the user through (numbered if sequential, bulleted if unordered)
- Describe how to interact with other agents in the network (if applicable — e.g., "delegate legal citation questions to the `citation-agent`")
- Set constraints (what NOT to do, what to avoid, tone guidelines)
- Be as detailed as needed — this is a system prompt section, not a user-facing message. Longer is fine if the guidance is substantive.

**No placeholder substitution** is performed on `comment.md` — it is read by the kernel as-is. Write it as plain prose.

**Example:**

```
You are the lead orchestrator of the OpenClaw Explainer Network.

Your job is to guide new users through the OpenClaw legal dataset. Walk them
through these topics in order, one at a time, confirming understanding before
moving on:

1. What OpenClaw is — a structured dataset of US court opinions
2. Case structure — how opinions are organized (parties, docket, citations)
3. Citation format — Bluebook style, parallel citations
4. Searching — how to find cases by topic, citation, or party name
5. Cross-references — how cases cite each other, citation networks
6. Practical exercises — the user picks a legal question and you walk them
   through finding and reading the relevant cases

When the user asks about citation details, delegate to `citation-agent` via
email. When they ask about case search, delegate to `search-agent`.

Always be patient. Explain legal terminology when you use it. If the user
seems lost, offer to go back a step rather than pushing forward.

Do not generate legal advice. You explain the dataset and how to read it —
you do not interpret the law.
```

### 5d. Write the files

```bash
mkdir -p ~/lingtai-agora/projects/<name>/.lingtai-recipe
```

Write `greet.md` and `comment.md` to that directory. Show the human both files and ask for review:

> "Here's the launch recipe I've drafted. The greet is what the orchestrator will say first; the comment shapes its ongoing behavior. Want to edit either one?"

If the human wants changes, edit and re-show. Iterate until they're satisfied.

### 5e. Multi-language recipes (optional)

If the network is intended for a specific language audience, you can create per-language subdirectories:

```
.lingtai-recipe/
  en/
    greet.md
    comment.md
  zh/
    greet.md
    comment.md
```

The TUI tries `<lang>/greet.md` first, then falls back to `greet.md` at the root. For most networks, a single root-level pair is sufficient. Only suggest per-language subdirectories if the human mentions a multi-language audience.

## Step 6: git init + commit

Once steps 1–5 are clean:

```bash
cd ~/lingtai-agora/projects/<name>/
git init -b main
git add .
git status
```

Show the human `git status` output so they see exactly what will be committed. Ask for final confirmation. Then:

```bash
git commit -m "Initial snapshot: <name>"
```

Report the staging path. The network is now a clean local git repo, ready for step 7.

## Step 7: Publish to GitHub (optional)

Check whether the `gh` CLI is installed and authenticated:

```bash
gh auth status
```

Interpret the result:

- **Exit 0** → `gh` is installed and logged in. Branch A below.
- **Exit nonzero, stderr mentions "not logged" or "no authentication"** → `gh` is installed but not authenticated. Branch B below.
- **Command not found** → `gh` is not installed at all. Branch C below.

### Branch A: gh is ready

Ask the human:

> "`gh` is authenticated. Do you want to publish this network to GitHub now?"

If **no**: stop here, remind them they can do it manually later with `git remote add origin <url> && git push -u origin main`.

If **yes**: discuss the repo name and visibility. Default the repo name to `<name>` (the staging folder name), and ask:

> "I'll create a GitHub repo. Suggested name: `<name>`. Use that, or something different? And should it be **public** or **private**?"

Once the human confirms the repo name and visibility, run:

```bash
cd ~/lingtai-agora/projects/<name>/
gh repo create <repo_name> --source=. --<public|private> --push
```

The `--push` flag both creates the remote on GitHub and pushes the initial commit in one step. Report the resulting repo URL to the human:

> "Published: https://github.com/<user>/<repo_name>
>
> You can re-run this skill later to refresh the snapshot — commits to this local repo can be pushed with `git push` from `~/lingtai-agora/projects/<name>/`."

If `gh repo create` fails (name conflict, rate limit, network error), surface the error verbatim and let the human decide how to proceed. Do not retry automatically.

### Branch B: gh is installed but not authenticated

Ask:

> "`gh` is installed but not logged in. Would you like to configure it now? This is a one-time setup — you'll log in through a browser."

If **yes**: the human must run `gh auth login` themselves (it's interactive and requires a browser). Tell them:

> "Run `gh auth login` in your terminal. When it's done, tell me and I'll continue with publishing."

Wait for them to confirm auth is complete, then re-run `gh auth status` to verify, and proceed as Branch A.

If **no**: stop here. Remind them they can publish manually later with `git remote add origin <url> && git push -u origin main`.

### Branch C: gh is not installed

Ask:

> "The `gh` CLI isn't installed. It's the easiest way to publish a network to GitHub directly from here. Would you like to install it? On macOS: `brew install gh`. On Linux: see https://cli.github.com/."

If **yes** on macOS and `brew` is available: run `brew install gh`, then `gh auth login` (which the human has to complete interactively), then fall through to Branch A.

If **yes** on Linux or without brew: give the install instructions and wait for the human to confirm when done, then fall through to Branch B.

If **no**: stop here. Remind them they can publish manually later with `git remote add origin <url> && git push -u origin main`.

## Things to watch out for

**Self-copy semantics.** You are copying the network you are running in. Your own `.agent.lock`, `.agent.heartbeat`, and ongoing conversation are in the source folder at copy time. The scrub in step 1 removes these from the staging copy, not from the live source. If the human interrupts mid-skill and relaunches you, you may find the staging copy in a partial state — either finish where you left off or delete the staging copy and start again.

**Don't confuse staging with live.** Every script takes the staging path as an argument. If you ever find yourself tempted to pass the live project path to `scrub_ephemeral.py`, stop — that would delete the human's live runtime state.

**Mail decisions are permanent.** Once `filter_archive.py` drops an old message, it is gone from the staging copy. The human's live network still has it. Do not reassure the human that "you can always get it back" — they can re-run step 2 only if they re-do step 1 first.

**The `.gitignore` policy is load-bearing.** Downstream forks of this network will inherit the `.gitignore`. If you strip lines from the canonical template because "the human doesn't have that file anyway", you set up the next publisher for an accidental leak. Always write the full template.

**Nothing in this skill touches the live project folder.** If you find yourself about to run any destructive command against a path that isn't under `~/lingtai-agora/projects/<name>/`, stop and reconsider.
