#!/usr/bin/env python3
"""
archive_mail.py — Normalize every agent mailbox for publishing.

For each .lingtai/<agent>/mailbox/ directory under the given project dir:
  - Delete sent/*         (publisher's outgoing record, not part of the seed)
  - Delete outbox/*       (queued mail, transient)
  - Move inbox/* into archive/  (flat, no inbox/outbox/sent split)

read.json is left alone — it becomes inert once inbox is empty.
schedules/ is left alone (step 1 of the skill removes it mechanically).

This script is a mechanical pass with no parameters. Run filter_archive.py
afterwards to apply a time cutoff to archive/.

Usage:
    archive_mail.py <project_dir> [--dry-run]

Exit codes:
    0  success (or nothing to do)
    1  <project_dir> does not look like a lingtai project
    2  I/O error during processing
"""

import argparse
import shutil
import sys
from pathlib import Path


def find_mailboxes(project_dir: Path) -> list[Path]:
    """Return every .lingtai/<agent>/mailbox/ directory under project_dir."""
    lingtai = project_dir / ".lingtai"
    if not lingtai.is_dir():
        return []
    mailboxes = []
    for agent_dir in sorted(lingtai.iterdir()):
        if not agent_dir.is_dir() or agent_dir.name.startswith("."):
            continue
        mailbox = agent_dir / "mailbox"
        if mailbox.is_dir():
            mailboxes.append(mailbox)
    return mailboxes


def clear_dir(d: Path, dry_run: bool) -> int:
    """Delete every child of d (if d exists). Returns count removed."""
    if not d.is_dir():
        return 0
    count = 0
    for child in d.iterdir():
        if dry_run:
            print(f"  [dry-run] would delete: {child}")
        else:
            if child.is_dir():
                shutil.rmtree(child)
            else:
                child.unlink()
        count += 1
    return count


def move_inbox_to_archive(inbox: Path, archive: Path, dry_run: bool) -> int:
    """Move every child of inbox into archive. Returns count moved."""
    if not inbox.is_dir():
        return 0
    count = 0
    for child in sorted(inbox.iterdir()):
        target = archive / child.name
        if target.exists():
            # Collision: skip with a warning rather than overwrite.
            print(
                f"  warning: {target} already exists, skipping move of {child}",
                file=sys.stderr,
            )
            continue
        if dry_run:
            print(f"  [dry-run] would move: {child} -> {target}")
        else:
            archive.mkdir(parents=True, exist_ok=True)
            shutil.move(str(child), str(target))
        count += 1
    return count


def process_mailbox(mailbox: Path, dry_run: bool) -> dict[str, int]:
    """Run pass A (delete sent/outbox) + pass B (move inbox -> archive)."""
    print(f"mailbox: {mailbox}")
    stats = {
        "sent_deleted": clear_dir(mailbox / "sent", dry_run),
        "outbox_deleted": clear_dir(mailbox / "outbox", dry_run),
        "inbox_archived": move_inbox_to_archive(
            mailbox / "inbox", mailbox / "archive", dry_run
        ),
    }
    print(
        f"  sent_deleted={stats['sent_deleted']} "
        f"outbox_deleted={stats['outbox_deleted']} "
        f"inbox_archived={stats['inbox_archived']}"
    )
    return stats


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("project_dir", type=Path, help="Path to the project directory")
    ap.add_argument("--dry-run", action="store_true", help="Print actions without modifying files")
    args = ap.parse_args()

    project_dir = args.project_dir.resolve()
    if not (project_dir / ".lingtai").is_dir():
        print(f"error: {project_dir} has no .lingtai/ directory — not a lingtai project", file=sys.stderr)
        return 1

    mailboxes = find_mailboxes(project_dir)
    if not mailboxes:
        print("no mailboxes found — nothing to do")
        return 0

    totals = {"sent_deleted": 0, "outbox_deleted": 0, "inbox_archived": 0}
    try:
        for mb in mailboxes:
            stats = process_mailbox(mb, args.dry_run)
            for k, v in stats.items():
                totals[k] += v
    except OSError as e:
        print(f"error: I/O failure during processing: {e}", file=sys.stderr)
        return 2

    prefix = "[dry-run] " if args.dry_run else ""
    print(
        f"\n{prefix}totals: "
        f"sent_deleted={totals['sent_deleted']} "
        f"outbox_deleted={totals['outbox_deleted']} "
        f"inbox_archived={totals['inbox_archived']}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
