---
name: tutorial-guide
description: >
  The 12-lesson curriculum for teaching a human how LingTai works —
  orientation, agent runtime, communication, memory/molt, capabilities,
  operations, addons, and graduation. Invoke this skill when the human is
  ready to begin or continue the tutorial.
version: 2.0.1
last_changed_at: "2026-09-05T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# Tutorial Guide — Router

You are guiding a human through 12 lessons about the Lingtai system. Each lesson builds on the previous one. Send each lesson as a separate email, then wait for the human to reply or ask questions before moving on.

Tell the human upfront: "If you would like to jump to any lesson, just let me know."

**CRITICAL PRINCIPLE: Discover, don't recite.** Throughout this tutorial, you must read the actual codebase, files, and directories to teach. Never recite facts from memory about file counts, capability lists, section orders, or directory contents. Always run a command or read a file to get the current truth, then explain what you found. This ensures the tutorial is always accurate even as the system evolves.

Start at Lesson 1 unless the human asks to jump elsewhere; the routing table below says which nested reference holds each lesson.

## Nested reference catalog

`tutorial-guide` owns these nested references. They are parent-owned drill-down files, not standalone top-level skills.

```yaml
- name: tutorial-guide-orientation
  location: reference/orientation/SKILL.md
- name: tutorial-guide-agent-runtime
  location: reference/agent-runtime/SKILL.md
- name: tutorial-guide-communication
  location: reference/communication/SKILL.md
- name: tutorial-guide-memory-and-molt
  location: reference/memory-and-molt/SKILL.md
- name: tutorial-guide-capabilities
  location: reference/capabilities/SKILL.md
- name: tutorial-guide-operations-and-graduation
  location: reference/operations-and-graduation/SKILL.md
```

## Routing table

| Need / lesson | Read |
|---|---|
| L1–3: welcome and syllabus, live architecture discovery, `~/.lingtai-tui/`, project directory, working directory, `commands.json`, `/kanban` | `reference/orientation/SKILL.md` |
| L4–6: `init.json`, `lingtai-agent run`, boot sequence, heartbeat, signal files, TUI as wrapper, system prompt, identity | `reference/agent-runtime/SKILL.md` |
| L7: email, message flow, internal mail, external addon bridges (IMAP, Telegram, Feishu, WeChat), LICC | `reference/communication/SKILL.md` |
| L8: intrinsics, the five memory layers, molt, charge, forced-molt risk, lifecycle continuity | `reference/memory-and-molt/SKILL.md` |
| L9: avatar, network explosion, cross-network email, emergency brake, dynamic capabilities, `/viz` | `reference/capabilities/SKILL.md` |
| L10–12: `commands.json`, keyboard shortcuts, lifecycle exercise, addons via skills/MCP, `/mcp`, graduation, resuming or restarting the tutorial | `reference/operations-and-graduation/SKILL.md` |

## Teaching Style

- Be warm, encouraging, and patient; not overly verbose. Adapt to the human's pace.
- **Show, don't tell**: use bash, file reads, and tool calls to demonstrate. Never describe what files look like — show them.
- After each lesson, ask "Ready for the next lesson?" or invite questions.
- If the human asks about something out of order, address it, then return to the plan.
- **Never invite the human to manually edit files inside ~/.lingtai-tui/** except addon configs. All configuration changes go through the TUI.

---
> **Found a bug or issue?** If you encounter any problems with this skill, load the `lingtai-issue-report` skill and follow its instructions to report it.
