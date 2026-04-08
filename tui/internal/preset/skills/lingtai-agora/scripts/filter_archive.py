#!/usr/bin/env python3
"""
filter_archive.py — Drop archived mail older than a cutoff date.

For each .lingtai/<agent>/mailbox/archive/<uuid>/message.json under the
given project dir, read the `received_at` field and delete the <uuid>/
directory if its timestamp is strictly before --before.

Malformed messages (no message.json, missing received_at, unparseable
timestamp) are ALSO deleted — a publishing tool should never ship data
it cannot verify.

This script is re-runnable: running with a later cutoff drops more mail,
running with the same cutoff is a no-op, running with an earlier cutoff
is also a no-op (filtering is one-way — earlier mail has already been
removed and cannot be restored from this script).

Usage:
    filter_archive.py <project_dir> --before YYYY-MM-DD [--dry-run]

Exit codes:
    0  success (or nothing to do)
    1  <project_dir> does not look like a lingtai project, or bad --before
    2  I/O error during processing
"""

import argparse
import json
import shutil
import sys
from datetime import datetime, timezone
from pathlib import Path


def parse_cutoff(s: str) -> datetime:
    """Parse YYYY-MM-DD as UTC midnight. Raises ValueError on bad input."""
    dt = datetime.strptime(s, "%Y-%m-%d")
    return dt.replace(tzinfo=timezone.utc)


def parse_received_at(ts: str) -> datetime:
    """Parse an ISO-8601 timestamp (with or without trailing Z) to a tz-aware datetime."""
    # Accept both "2026-04-03T18:42:23Z" and "2026-04-03T18:42:23+00:00"
    if ts.endswith("Z"):
        ts = ts[:-1] + "+00:00"
    return datetime.fromisoformat(ts)


def find_archives(project_dir: Path) -> list[Path]:
    """Return every .lingtai/<agent>/mailbox/archive/ directory under project_dir."""
    lingtai = project_dir / ".lingtai"
    if not lingtai.is_dir():
        return []
    archives = []
    for agent_dir in sorted(lingtai.iterdir()):
        if not agent_dir.is_dir() or agent_dir.name.startswith("."):
            continue
        archive = agent_dir / "mailbox" / "archive"
        if archive.is_dir():
            archives.append(archive)
    return archives


def should_drop(msg_dir: Path, cutoff: datetime) -> tuple[bool, str]:
    """
    Decide whether msg_dir should be deleted.

    Returns (drop, reason). drop=True means delete. reason is a short
    human-readable explanation suitable for dry-run output or logs.
    """
    msg_file = msg_dir / "message.json"
    if not msg_file.is_file():
        return True, "no message.json"
    try:
        data = json.loads(msg_file.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as e:
        return True, f"unreadable message.json ({e.__class__.__name__})"
    ts = data.get("received_at")
    if not isinstance(ts, str):
        return True, "missing received_at"
    try:
        dt = parse_received_at(ts)
    except ValueError:
        return True, f"unparseable received_at={ts!r}"
    if dt < cutoff:
        return True, f"older than cutoff ({ts})"
    return False, f"kept ({ts})"


def process_archive(archive: Path, cutoff: datetime, dry_run: bool) -> dict[str, int]:
    """Walk archive/<uuid>/ and drop anything older than cutoff or malformed."""
    print(f"archive: {archive}")
    stats = {"dropped_old": 0, "dropped_malformed": 0, "kept": 0}
    for msg_dir in sorted(archive.iterdir()):
        if not msg_dir.is_dir():
            continue
        drop, reason = should_drop(msg_dir, cutoff)
        if drop:
            if reason.startswith("older than cutoff"):
                stats["dropped_old"] += 1
            else:
                stats["dropped_malformed"] += 1
            if dry_run:
                print(f"  [dry-run] would drop {msg_dir.name}: {reason}")
            else:
                shutil.rmtree(msg_dir)
        else:
            stats["kept"] += 1
    print(
        f"  dropped_old={stats['dropped_old']} "
        f"dropped_malformed={stats['dropped_malformed']} "
        f"kept={stats['kept']}"
    )
    return stats


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("project_dir", type=Path, help="Path to the project directory")
    ap.add_argument(
        "--before",
        required=True,
        help="Drop mail with received_at strictly before this date (YYYY-MM-DD, UTC)",
    )
    ap.add_argument("--dry-run", action="store_true", help="Print actions without modifying files")
    args = ap.parse_args()

    try:
        cutoff = parse_cutoff(args.before)
    except ValueError:
        print(f"error: --before must be YYYY-MM-DD, got {args.before!r}", file=sys.stderr)
        return 1

    project_dir = args.project_dir.resolve()
    if not (project_dir / ".lingtai").is_dir():
        print(f"error: {project_dir} has no .lingtai/ directory — not a lingtai project", file=sys.stderr)
        return 1

    archives = find_archives(project_dir)
    if not archives:
        print("no archive/ directories found — run archive_mail.py first, or nothing to filter")
        return 0

    print(f"cutoff: {cutoff.isoformat()}")
    totals = {"dropped_old": 0, "dropped_malformed": 0, "kept": 0}
    try:
        for a in archives:
            stats = process_archive(a, cutoff, args.dry_run)
            for k, v in stats.items():
                totals[k] += v
    except OSError as e:
        print(f"error: I/O failure during processing: {e}", file=sys.stderr)
        return 2

    prefix = "[dry-run] " if args.dry_run else ""
    print(
        f"\n{prefix}totals: "
        f"dropped_old={totals['dropped_old']} "
        f"dropped_malformed={totals['dropped_malformed']} "
        f"kept={totals['kept']}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
