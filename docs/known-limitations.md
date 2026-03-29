# Known Limitations

## Agents Survive Ctrl-C

**This is by design.** When you close the TUI (ctrl-c, `/quit`, or closing the terminal), agent processes keep running in the background. The TUI is a management interface, not the agent runtime. Agents are independent Python processes — they continue working, checking mail, and executing tools even when the TUI is gone.

This means: **you must explicitly stop agents before deleting their directories.** If you `rm -rf` an agent's working directory while its process is still running, you create a phantom — an orphaned Python process with no way to reach it via signal files (since the directory is gone).

### How to properly shut down agents

There are three ways to suspend agents:

```bash
# 1. Inside the TUI:
/suspend        # suspend the current agent
/suspend-all    # suspend all agents in the project

# 2. From the command line (no TUI needed):
lingtai-tui suspend              # suspend all agents in current project
lingtai-tui suspend /path/to/dir # suspend all agents in specified project

# 3. Manually via signal files:
touch .lingtai/my-agent/.suspend  # suspend a specific agent
```

After suspending, you can safely remove directories:

```bash
lingtai-tui suspend
rm -rf .lingtai/my-agent/
```

### How to clean up phantom processes

If you already deleted directories without suspending:

```bash
# Find orphaned lingtai processes
ps aux | grep "lingtai run"

# Kill them
kill <pid>
```

### Why we don't auto-kill on directory deletion

A PID-based kill mechanism was considered and intentionally rejected. `SIGTERM` is Unix-only (`syscall.SIGTERM` doesn't exist on Windows), and adding platform-specific process management for a case that only occurs through manual directory deletion adds complexity without proportional benefit. The graceful shutdown flow via `/suspend-all` or `lingtai-tui suspend` handles all normal cases.

## Heavy Optional Dependencies

The `listen` capability depends on `faster-whisper` (~132 MB) and `librosa` (~202 MB). These are **not** installed with `pip install lingtai`. They are automatically installed on first use when an agent actually invokes the listen tool (transcription or music analysis). The first invocation will pause for a few seconds while the packages download.
