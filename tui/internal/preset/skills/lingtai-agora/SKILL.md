---
name: lingtai-agora
description: Prepare the current lingtai network for publishing. Copies the project to ~/lingtai-agora/projects/<name>/, scrubs ephemeral runtime state, processes mail with a user-chosen time cutoff, writes a canonical .gitignore, scans for nested git repos and leaked secrets, and initializes a clean git repository ready to push. Use when the human asks you to share, publish, package, export, or back up this network for others.
version: 1.0.0
---

# lingtai-agora: Publishing a Network

You are about to copy the network you live in to a publishable location. **This is literal self-copying** — the snapshot will not contain the moment you made it. Everything up to this conversation turn will be in the published copy; this turn itself will only exist in the original.

Walk the human through the steps below carefully. Each step is either *mechanical* (run a script, report the result) or *interactive* (discuss a decision with the human before proceeding). Never skip the interactive steps — the whole point of a skill-driven publishing flow is that a human is in the loop for judgment calls.

All scripts live alongside this SKILL.md under `scripts/`. The canonical `.gitignore` template lives at `assets/gitignore.template`. You run the scripts with `python3 <path-to-script> ...`; resolve the absolute path from this skill's location in `.lingtai/.skills/lingtai-agora/`.

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

**Do not proceed to step 5 with unresolved hard matches.** The consequence of shipping a real API key to GitHub is an irreversible privacy incident — there is no cleanup, only key rotation.

## Step 5: git init + commit

Once steps 1–4 are clean:

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

Report the staging path. The network is now a clean local git repo, ready for step 6.

## Step 6: Publish to GitHub (optional)

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
