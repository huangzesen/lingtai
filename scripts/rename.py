#!/usr/bin/env python3
"""Rename stoai → lingtai across both repos."""
from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

STOAI_REPO = Path(__file__).resolve().parent.parent          # stoai repo
KERNEL_REPO = STOAI_REPO.parent / "stoai-kernel"             # stoai-kernel repo

# Ordered replacements (longest match first to avoid partial matches)
REPLACEMENTS = [
    ("stoai_kernel", "lingtai_kernel"),   # Python module name
    ("stoai-kernel", "lingtai-kernel"),   # PyPI package name
    ("StoAI",       "灵台"),              # Brand name
    ("stoai",       "lingtai"),           # Catch-all (imports, paths, etc.)
]

# Files/dirs to skip entirely
SKIP_DIRS = {".git", "venv", "__pycache__", "node_modules", ".egg-info",
             ".worktrees", ".pytest_cache", ".superpowers", ".claude"}
SKIP_FILES = {"rename-to-lingtai-design.md", "rename-to-lingtai.md", "rename.py"}

# File extensions to process
TEXT_EXTS = {
    ".py", ".md", ".toml", ".cfg", ".txt", ".json", ".yaml", ".yml",
    ".html", ".css", ".js", ".ts", ".tsx", ".jsx", ".sh", ".bash",
    ".rst", ".ini", ".env", ".example",
}

# Patterns to exclude from replacement (email addresses containing stoai)
EMAIL_PATTERN = re.compile(r"[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+")


def should_skip(path: Path) -> bool:
    parts = set(path.parts)
    if parts & SKIP_DIRS:
        return True
    if path.name in SKIP_FILES:
        return True
    # Files with no extension: skip if binary-looking name, process otherwise
    if not path.suffix:
        return False
    if path.suffix not in TEXT_EXTS:
        return True
    return False


def replace_content(text: str) -> str:
    """Apply replacements, but protect email addresses."""
    # Find all email addresses and their positions
    emails = [(m.start(), m.end(), m.group()) for m in EMAIL_PATTERN.finditer(text)]

    # Build result by processing non-email regions
    result = []
    last_end = 0
    for start, end, email in emails:
        # Process text before this email
        chunk = text[last_end:start]
        for old, new in REPLACEMENTS:
            chunk = chunk.replace(old, new)
        result.append(chunk)
        # Keep email as-is
        result.append(email)
        last_end = end

    # Process remaining text after last email
    chunk = text[last_end:]
    for old, new in REPLACEMENTS:
        chunk = chunk.replace(old, new)
    result.append(chunk)

    return "".join(result)


def dry_run(repo: Path) -> dict[str, list[str]]:
    """Report what would change, grouped by replacement rule."""
    changes: dict[str, list[str]] = {old: [] for old, _ in REPLACEMENTS}

    for root, dirs, files in os.walk(repo):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS and not d.endswith(".egg-info")]
        for fname in files:
            fpath = Path(root) / fname
            if should_skip(fpath):
                continue
            try:
                text = fpath.read_text(encoding="utf-8")
            except (UnicodeDecodeError, PermissionError):
                continue
            for old, _ in REPLACEMENTS:
                if old in text:
                    rel = fpath.relative_to(repo)
                    changes[old].append(str(rel))

    return changes


def rename_directories(repo: Path, dir_renames: list[tuple[str, str]]):
    """Rename directories using git mv."""
    for old_dir, new_dir in dir_renames:
        old_path = repo / old_dir
        new_path = repo / new_dir
        if old_path.exists():
            print(f"  git mv {old_dir} → {new_dir}")
            subprocess.run(["git", "mv", str(old_path), str(new_path)],
                           cwd=repo, check=True)


def delete_egg_info(repo: Path):
    """Delete .egg-info directories (regenerated on install)."""
    src = repo / "src"
    if not src.exists():
        return
    for d in src.iterdir():
        if d.is_dir() and d.name.endswith(".egg-info"):
            print(f"  rm -rf {d.relative_to(repo)}")
            shutil.rmtree(d)


def replace_in_files(repo: Path):
    """Apply find-and-replace in all text files."""
    count = 0
    for root, dirs, files in os.walk(repo):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS and not d.endswith(".egg-info")]
        for fname in files:
            fpath = Path(root) / fname
            if should_skip(fpath):
                continue
            try:
                text = fpath.read_text(encoding="utf-8")
            except (UnicodeDecodeError, PermissionError):
                continue
            new_text = replace_content(text)
            if new_text != text:
                fpath.write_text(new_text, encoding="utf-8")
                print(f"  ✓ {fpath.relative_to(repo)}")
                count += 1
    return count


def main():
    dry = "--dry-run" in sys.argv

    for repo, label in [(KERNEL_REPO, "stoai-kernel"), (STOAI_REPO, "stoai")]:
        print(f"\n{'='*60}")
        print(f"  {label} repo: {repo}")
        print(f"{'='*60}")

        if not repo.exists():
            print(f"  ⚠ repo not found, skipping")
            continue

        if dry:
            changes = dry_run(repo)
            for old, files in changes.items():
                if files:
                    print(f"\n  '{old}' found in {len(files)} files:")
                    for f in sorted(files):
                        print(f"    {f}")
            continue

        # Step 1: Delete egg-info
        print("\n--- Deleting .egg-info ---")
        delete_egg_info(repo)

        # Step 2: Rename directories
        print("\n--- Renaming directories ---")
        if label == "stoai-kernel":
            rename_directories(repo, [("src/stoai_kernel", "src/lingtai_kernel")])
        else:
            rename_directories(repo, [("src/stoai", "src/lingtai")])

        # Step 3: Replace in files
        print("\n--- Replacing in files ---")
        n = replace_in_files(repo)
        print(f"\n  {n} files updated")

    if dry:
        print("\n\nDry run complete. Run without --dry-run to apply changes.")
    else:
        print("\n\nDone! Next steps:")
        print("  1. cd ../stoai-kernel && pip install -e .")
        print("  2. cd ../stoai && pip install -e .")
        print("  3. python -c 'import lingtai_kernel; import lingtai'")
        print("  4. python -m pytest tests/")


if __name__ == "__main__":
    main()
