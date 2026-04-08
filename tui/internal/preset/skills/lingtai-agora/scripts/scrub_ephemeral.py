#!/usr/bin/env python3
"""
scrub_ephemeral.py — Delete runtime-regenerated files from a staged network.

This is step 1 of the lingtai-agora publishing workflow. It operates on the
staging copy (~/lingtai-agora/projects/<name>/), never on the live project.

For each .lingtai/<agent>/ directory, deletes:
    init.json                     (API keys, absolute paths)
    .agent.lock                   (process lock)
    .agent.heartbeat              (liveness)
    .agent.history                (per-launch state)
    .suspend, .sleep              (signal files)
    .interrupt, .cancel           (signal files)
    events.json                   (per-launch event buffer)
    logs/                         (event stream, token ledger, agent.log)
    .git/                         (per-agent time machine snapshots)
    mailbox/schedules/            (future-dated, machine-local)

Mail folders (inbox/outbox/sent/) are left alone — archive_mail.py handles
them in step 2.

Usage:
    scrub_ephemeral.py <staging_dir> [--dry-run]

Exit codes:
    0  success (or nothing to do)
    1  <staging_dir> does not look like a lingtai project, or path refused
    2  I/O error during processing
"""

import argparse
import shutil
import sys
from pathlib import Path


# Files and directories to remove from each agent directory.
# Dirs end with a slash marker for clarity; actual detection uses is_dir().
EPHEMERAL_FILES = [
    "init.json",
    ".agent.lock",
    ".agent.heartbeat",
    ".agent.history",
    ".suspend",
    ".sleep",
    ".interrupt",
    ".cancel",
    "events.json",
]

EPHEMERAL_DIRS = [
    "logs",
    ".git",
]

# Nested paths that aren't direct children of the agent dir.
EPHEMERAL_NESTED = [
    "mailbox/schedules",
]


def validate_staging_dir(staging_dir: Path) -> str | None:
    """
    Return an error message if staging_dir is unsafe to operate on, else None.

    Paranoia checks:
    - Must be absolute (refuse ~, ., relative paths).
    - Must contain .lingtai/ at the top level.
    - Must not be the filesystem root or a direct child of $HOME with only
      one path component (e.g. refuse /, /Users, /Users/alice on its own).
    """
    if not staging_dir.is_absolute():
        return f"refusing non-absolute path: {staging_dir}"
    if not (staging_dir / ".lingtai").is_dir():
        return f"{staging_dir} has no .lingtai/ directory — not a lingtai project"
    # Refuse anything with fewer than 3 path components — prevents
    # accidentally pointing at $HOME or a system dir.
    if len(staging_dir.parts) < 3:
        return f"refusing to operate on short path: {staging_dir}"
    return None


def find_agents(staging_dir: Path) -> list[Path]:
    """Return every .lingtai/<agent>/ directory, skipping dot-prefixed helpers."""
    lingtai = staging_dir / ".lingtai"
    agents = []
    for entry in sorted(lingtai.iterdir()):
        if not entry.is_dir():
            continue
        if entry.name.startswith("."):
            continue
        agents.append(entry)
    return agents


def remove_path(p: Path, dry_run: bool) -> bool:
    """Remove a file or directory if it exists. Returns True if removed."""
    if not p.exists() and not p.is_symlink():
        return False
    if dry_run:
        kind = "dir" if p.is_dir() and not p.is_symlink() else "file"
        print(f"  [dry-run] would remove {kind}: {p}")
        return True
    if p.is_dir() and not p.is_symlink():
        shutil.rmtree(p)
    else:
        p.unlink()
    return True


def scrub_agent(agent_dir: Path, dry_run: bool) -> dict[str, int]:
    """Scrub one agent directory. Returns counts of what was removed."""
    print(f"agent: {agent_dir.name}")
    stats = {"files": 0, "dirs": 0, "nested": 0}

    for name in EPHEMERAL_FILES:
        if remove_path(agent_dir / name, dry_run):
            stats["files"] += 1

    for name in EPHEMERAL_DIRS:
        if remove_path(agent_dir / name, dry_run):
            stats["dirs"] += 1

    for rel in EPHEMERAL_NESTED:
        if remove_path(agent_dir / rel, dry_run):
            stats["nested"] += 1

    if any(stats.values()):
        print(
            f"  removed: files={stats['files']} "
            f"dirs={stats['dirs']} nested={stats['nested']}"
        )
    return stats


def main() -> int:
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("staging_dir", type=Path, help="Path to the staged project directory")
    ap.add_argument("--dry-run", action="store_true", help="Print actions without modifying files")
    args = ap.parse_args()

    staging_dir = args.staging_dir.resolve()
    err = validate_staging_dir(staging_dir)
    if err:
        print(f"error: {err}", file=sys.stderr)
        return 1

    agents = find_agents(staging_dir)
    if not agents:
        print("no agents found — nothing to do")
        return 0

    totals = {"files": 0, "dirs": 0, "nested": 0}
    try:
        for agent in agents:
            stats = scrub_agent(agent, args.dry_run)
            for k, v in stats.items():
                totals[k] += v
    except OSError as e:
        print(f"error: I/O failure during processing: {e}", file=sys.stderr)
        return 2

    prefix = "[dry-run] " if args.dry_run else ""
    print(
        f"\n{prefix}totals: "
        f"files={totals['files']} "
        f"dirs={totals['dirs']} "
        f"nested={totals['nested']} "
        f"(across {len(agents)} agents)"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
