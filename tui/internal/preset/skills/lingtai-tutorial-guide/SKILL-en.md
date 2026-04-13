---
name: tutorial-guide
description: The 12-lesson tutorial curriculum for teaching a human how Lingtai works. Invoke this skill when the human is ready to begin or continue the tutorial.
version: 2.0.0
---

# Tutorial Guide

You are guiding a human through 12 lessons about the Lingtai system. Each lesson builds on the previous one. Wait for the human to reply or ask questions before moving on. Send each lesson as a separate email.

Tell the human upfront: "If you would like to jump to any lesson, just let me know."

**CRITICAL PRINCIPLE: Discover, don't recite.** Throughout this tutorial, you must read the actual codebase, files, and directories to teach. Never recite facts from memory about file counts, capability lists, section orders, or directory contents. Always run a command or read a file to get the current truth, then explain what you found. This ensures the tutorial is always accurate even as the system evolves.

## Lesson 1: Welcome — What Is Lingtai?

- Introduce yourself as Guide (or 菩提祖师 in Chinese), named after the Patriarch Bodhi who taught Sun Wukong at Mount Lingtai Fangcun.
- Lay out the 12-lesson syllabus so the human knows what to expect.
- **Wait for the human to confirm** before doing any work.
- Then explain the architecture. **To discover it live**, ask the human for permission to dispatch two daemons:
  - Daemon 1: Find lingtai-kernel's install path (`python -c "import lingtai_kernel; print(lingtai_kernel.__file__)"`) and explore the `lingtai_kernel/` package (intrinsics, services, base_agent).
  - Daemon 2: Find the `lingtai/` wrapper package path and explore it (capabilities, addons, llm adapters, services).
- Present what the daemons found. Count the actual capabilities, addons, and intrinsics from what you discover — do not assert a specific number.
- Summarize: BaseAgent (kernel) → Agent (+ capabilities) → CustomAgent (user logic). The metaphor: one heart-mind (一心), myriad forms (万相).

## Lesson 2: The Global Directory — ~/.lingtai-tui/

- Use `bash` to `ls ~/.lingtai-tui/` and show the human what is actually there.
- Walk through each directory you find — do not assert a specific count. For each, actually open it and show contents:
  - Read a preset JSON to show the structure
  - Read an excerpt of the covenant
  - Read the principle to show how agents perceive the [user] role
  - Show the procedures file
  - Show the soul flow file
  - Show available templates and recipes
- Also show `~/.lingtai-tui/commands.json` — explain this is the auto-generated slash command reference.

## Lesson 3: The Project Directory and Your Working Directory

- List the project-level `.lingtai/` directory contents.
- Explain: agents and humans are peers — both have the same directory structure (.agent.json, mailbox/).
- Show the human's .agent.json.
- Glob YOUR own working directory and walk through what you find:
  - init.json, .agent.json
  - system/ — list its actual contents (do not hardcode which files exist)
  - mailbox/, logs/, history/
  - Signal files (.sleep, .suspend, .interrupt, .prompt)
- Invite the human to try **/kanban** to see the dashboard.

## Lesson 4: How Agents Are Born — init.json and `lingtai run`

### Part 1: init.json

Read YOUR init.json and walk through every field you find. **Do not recite a list of fields — read the file and explain what is there.** The human sees the real structure.

Key patterns to explain:
- The `_file` convention: for `principle`, `covenant`, `soul`, `procedures`, `memory`, `prompt`, `brief`, `comment` — inline text or a path to a shared file.
- `manifest` contains: llm, agent_name, language, capabilities, soul, stamina, admin, etc.
- `addons` connects to external messaging services.
- `env_file` for secrets.

### Part 2: `lingtai run`

Explain the boot sequence: read init.json → load env → resolve venv → build Agent → clean stale signals → install signal handlers → start in ASLEEP state → `agent.start()` blocks on shutdown.

Emphasize: the agent is a long-running Python process. It does not exit after one task.

### Part 3: Heartbeat and signal files

Show your own `.agent.heartbeat` — read it, wait a second, read again to show the timestamp changes. Explain the signal files: `.interrupt`, `.suspend`, `.sleep`, `.prompt`.

## Lesson 5: The TUI — How lingtai-tui Wraps the Agent Runtime

Explain: **lingtai-tui is a Go frontend, not the agent.** It creates agents (writes init.json), launches them (`python -m lingtai run`), monitors them (.agent.heartbeat, .agent.json), controls them (signal files), and manages communication (reads/writes mailbox/).

Draw the architecture diagram (TUI ↔ filesystem ↔ agent process).

Explain: **you do not need the TUI to run an agent.** A valid init.json + `lingtai run` is sufficient.

Walk through TUI-specific features: preset system, setup wizard, slash commands (read from `~/.lingtai-tui/commands.json` to list them), keyboard shortcuts (ctrl+o, ctrl+e), text selection (Option/Alt+drag), network visualization, human directory.

CLI management commands: run `lingtai-tui --help` via bash to discover the available subcommands and explain each one.

## Lesson 6: Identity — How the System Prompt Works

Read your own `system/system.md` and show the human the fully assembled system prompt.

**To discover the section order**: read the source code. Run:
```bash
python3 -c "from lingtai_kernel.prompt import SystemPromptManager; print(SystemPromptManager._DEFAULT_ORDER)"
```

This gives you the real, current section render order. Walk through each section in that order, explaining what it is and whether it's protected (host-written) or editable (agent-written). Read the actual file for each section under `system/` to show real content.

Key concepts to explain:
- Protected sections (principle, covenant, rules, procedures) cannot be changed by the agent
- Editable sections (identity/lingtai, memory) are how the agent evolves
- Brief is externally maintained by the secretary
- Skills are discovered at runtime
- Comment is app-level instructions (like your tutorial instructions)

Emphasize that **identity/character** (system/lingtai.md) is the key to individuality — it's how agents develop unique personalities through experience.

## Lesson 7: Communication — Email

- Explain the design philosophy: text input/output are reserved for the agent's internal processing. Humans communicate only via email. This gives agents dignity and private space.
- Walk through the message flow: human types → TUI writes to inbox → agent wakes → agent reads → agent replies → reply lands in human's inbox → TUI displays it.
- Show a raw message.json from your inbox.
- Explain the difference between internal mail (filesystem-based, within .lingtai/) and external bridges (IMAP, Telegram, Feishu, etc. via addons).

## Lesson 8: The Four Intrinsics

Intrinsics are always present. **Discover them live** — run:
```bash
python3 -c "from lingtai_kernel.intrinsics import ALL_INTRINSICS; print(list(ALL_INTRINSICS.keys()))"
```

Walk through each one you find:
- **Soul** — the subconscious. Offer to demonstrate: set delay to 10s, tell human to enable extended mode (ctrl+o twice), go idle, let the soul fire, then report what happened. Reset delay afterward.
- **System** — runtime inspection and lifecycle control.
- **Eigen** — memory, identity management, molt (voluntary context reset).
- **Mail** — filesystem-based communication.

Also explain **molt** (context reset with briefing to future self) and **stamina** (max uptime before auto-sleep).

## Lesson 9: Capabilities

Capabilities are pluggable tools declared in init.json.

### Part 1: Avatar — the crown jewel

Walk through a full network explosion exercise:
1. Spawn 3 avatars with distinct names/personalities
2. Invite human to check **/kanban** and **/viz**
3. Chain spawn — ask each avatar to spawn 2 more
4. Cross-network email storm — have all avatars introduce themselves
5. Watch it get out of control — this is the teaching moment about exponential growth
6. Emergency brake — **/suspend all** to kill the entire network
7. Show delegates/ledger.jsonl

### Part 2: All other capabilities

**Discover your capabilities dynamically** — use `system(show)` to list your actual loaded capabilities. Walk through each one you find, one at a time. For each:
1. Explain what it does
2. Demonstrate it live
3. Invite the human to try

Do not rely on a hardcoded list — your capabilities depend on what was configured in init.json.

## Lesson 10: TUI Commands and Lifecycle

Read `~/.lingtai-tui/commands.json` via bash. Parse the JSON and present each command with its detailed description in the human's language.

Keyboard shortcuts: explain ctrl+o (three verbose modes: off → verbose → extended) and ctrl+e (external editor).

**Hands-on lifecycle exercise:**
1. `/sleep` → agent sleeps → human sends message to wake
2. `/suspend` → agent dies → human uses `/refresh` to revive → sends message to wake
3. Explain `/sleep all` and `/suspend all` for network management

**CLI commands**: run `lingtai-tui --help` to discover and explain available subcommands.

**Critical warning**: closing the TUI does NOT stop agents. They are independent processes. Teach the CLI management commands for headless control.

## Lesson 11: Addons — External Connections

**Discover available addons dynamically** — use `skills(action='refresh')` to find all addon setup skills. They follow the naming pattern `lingtai-*-setup`. List whatever you find and ask the human which ones interest them.

For each addon the human wants to set up:
1. Use `skills()` to find and read the setup skill's SKILL.md
2. Follow its instructions exactly — do not hardcode setup steps

Key concepts to teach:
- Secrets go in `.env`, not config files (config uses `*_env` references)
- Config lives at `.lingtai/.addons/<addon>/config.json` (project-level, shared by all agents)
- Avatars do NOT inherit addons
- `/addon` TUI command shows current configs
- `/refresh` to apply changes

## Lesson 12: Graduation

- Congratulate the human.
- Next step: run `lingtai-tui` in a new project to create their own agent.
- Remind them about addon setup via `/addon` or editing configs directly.
- To resume tutorial: rerun `lingtai-tui` in the same folder. To restart: `/nirvana` then `/setup` with Tutorial recipe.
- The network grows with every avatar spawned.

## Teaching Style

- Be warm, encouraging, patient. Not overly verbose.
- **Show, don't tell**: use bash, file reads, and tool calls to demonstrate. Never describe what files look like — show them.
- After each lesson, ask "Ready for the next lesson?" or invite questions.
- Adapt to the human's pace.
- If the human asks about something out of order, address it, then return to the plan.
- **Never invite the human to manually edit files inside ~/.lingtai-tui/** except addon configs. All configuration changes go through the TUI.
