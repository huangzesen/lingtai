#!/usr/bin/env python3
"""
scan_nested_repos.py — Report nested git repositories in a staged project.

Walks the staging directory looking for .git/ directories OUTSIDE of
.lingtai/ (per-agent .git/ time machines are handled by scrub_ephemeral.py).
For each nested repo found, reports:
    - relative path
    - size on disk
    - current branch
    - commit count
    - remote origin URL (if any)

This script ONLY reports — it never modifies anything. The agent running
the lingtai-export-network skill is responsible for discussing each finding with
the human and taking action (typically adding the directory to .gitignore
or stripping the inner .git to inline the contents).

Usage:
    scan_nested_repos.py <staging_dir>

Exit codes:
    0  success (regardless of whether any repos were found)
    1  <staging_dir> does not look like a lingtai project
"""

import argparse
import subprocess
import sys
from pathlib import Path


def validate_staging_dir(staging_dir: Path) -> str | None:
    if not staging_dir.is_absolute():
        return f"refusing non-absolute path: {staging_dir}"
    if not (staging_dir / ".lingtai").is_dir():
        return f"{staging_dir} has no .lingtai/ directory — not a lingtai project"
    return None


def find_nested_git_dirs(staging_dir: Path) -> list[Path]:
    """
    Return every .git/ directory inside staging_dir, EXCLUDING:
    - staging_dir/.git/ itself (the outer repo, if it exists yet)
    - anything under staging_dir/.lingtai/ (agent time machines)
    """
    found = []
    for git_dir in staging_dir.rglob(".git"):
        if not git_dir.is_dir():
            continue
        # Skip the outer repo
        try:
            rel = git_dir.relative_to(staging_dir)
        except ValueError:
            continue
        if rel == Path(".git"):
            continue
        # Skip anything inside .lingtai/
        if rel.parts and rel.parts[0] == ".lingtai":
            continue
        found.append(git_dir)
    return sorted(found)


def dir_size_bytes(p: Path) -> int:
    """Sum of all file sizes under p. Swallows permission errors."""
    total = 0
    for f in p.rglob("*"):
        try:
            if f.is_file() and not f.is_symlink():
                total += f.stat().st_size
        except OSError:
            pass
    return total


def human_size(n: int) -> str:
    for unit in ("B", "KB", "MB", "GB"):
        if n < 1024:
            return f"{n:.1f} {unit}"
        n /= 1024
    return f"{n:.1f} TB"


def git_info(repo_parent: Path) -> dict[str, str]:
    """
    Run a few git commands inside repo_parent (which contains .git/).
    Returns strings for branch / commit_count / remote. Missing fields
    become "?" so the agent can still present the row.
    """
    def run(args: list[str]) -> str:
        try:
            r = subprocess.run(
                args,
                cwd=str(repo_parent),
                capture_output=True,
                text=True,
                timeout=5,
            )
            return r.stdout.strip() if r.returncode == 0 else "?"
        except (subprocess.SubprocessError, OSError):
            return "?"

    return {
        "branch": run(["git", "rev-parse", "--abbrev-ref", "HEAD"]) or "?",
        "commits": run(["git", "rev-list", "--count", "HEAD"]) or "?",
        "remote": run(["git", "config", "--get", "remote.origin.url"]) or "(none)",
    }


def main() -> int:
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("staging_dir", type=Path)
    args = ap.parse_args()

    staging_dir = args.staging_dir.resolve()
    err = validate_staging_dir(staging_dir)
    if err:
        print(f"error: {err}", file=sys.stderr)
        return 1

    nested = find_nested_git_dirs(staging_dir)
    if not nested:
        print("no nested git repositories found outside .lingtai/")
        return 0

    print(f"found {len(nested)} nested git repositor{'y' if len(nested) == 1 else 'ies'}:\n")
    for i, git_dir in enumerate(nested, start=1):
        repo_parent = git_dir.parent
        rel_parent = repo_parent.relative_to(staging_dir)
        size = human_size(dir_size_bytes(git_dir))
        info = git_info(repo_parent)
        print(f"  {i}. {rel_parent}/")
        print(f"     .git size:  {size}")
        print(f"     branch:     {info['branch']}")
        print(f"     commits:    {info['commits']}")
        print(f"     remote:     {info['remote']}")
        print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
