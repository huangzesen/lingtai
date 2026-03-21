# Rename 灵台 → 灵台 (lingtai) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the product from 灵台 to 灵台 (lingtai) across both `lingtai` and `lingtai-kernel` repos — package names, imports, CLI, docs, and default directories.

**Architecture:** A Python rename script handles the mechanical find-and-replace across both repos. CLAUDE.md gets a manual rewrite for the new 灵台 narrative. The script runs in dry-run mode first, then executes, then we verify with tests.

**Tech Stack:** Python (script), git mv (directory renames), pytest (verification)

**Spec:** `docs/superpowers/specs/2026-03-20-rename-to-lingtai-design.md`

---

### Task 1: Write the rename script

**Files:**
- Create: `scripts/rename.py`

- [ ] **Step 1: Write the rename script**

```python
#!/usr/bin/env python3
"""Rename lingtai → lingtai across both repos."""
from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

STOAI_REPO = Path(__file__).resolve().parent.parent          # lingtai repo
KERNEL_REPO = STOAI_REPO.parent / "lingtai-kernel"             # lingtai-kernel repo

# Ordered replacements (longest match first to avoid partial matches)
REPLACEMENTS = [
    ("lingtai_kernel", "lingtai_kernel"),   # Python module name
    ("lingtai-kernel", "lingtai-kernel"),   # PyPI package name
    ("灵台",       "灵台"),              # Brand name
    ("lingtai",       "lingtai"),           # Catch-all (imports, paths, etc.)
]

# Files/dirs to skip entirely
SKIP_DIRS = {".git", "venv", "__pycache__", "node_modules", ".egg-info"}
SKIP_FILES = {"rename-to-lingtai-design.md", "rename-to-lingtai.md", "rename.py"}

# File extensions to process
TEXT_EXTS = {
    ".py", ".md", ".toml", ".cfg", ".txt", ".json", ".yaml", ".yml",
    ".html", ".css", ".js", ".ts", ".tsx", ".jsx", ".sh", ".bash",
    ".rst", ".ini", ".env", ".example",
}

# Patterns to exclude from replacement (email addresses containing lingtai)
EMAIL_PATTERN = re.compile(r"[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+")


def should_skip(path: Path) -> bool:
    parts = set(path.parts)
    if parts & SKIP_DIRS:
        return True
    if path.name in SKIP_FILES:
        return True
    if path.suffix and path.suffix not in TEXT_EXTS:
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

    for repo, label in [(KERNEL_REPO, "lingtai-kernel"), (STOAI_REPO, "lingtai")]:
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
        if label == "lingtai-kernel":
            rename_directories(repo, [("src/lingtai_kernel", "src/lingtai_kernel")])
        else:
            rename_directories(repo, [("src/lingtai", "src/lingtai")])

        # Step 3: Replace in files
        print("\n--- Replacing in files ---")
        n = replace_in_files(repo)
        print(f"\n  {n} files updated")

    if dry:
        print("\n\nDry run complete. Run without --dry-run to apply changes.")
    else:
        print("\n\nDone! Next steps:")
        print("  1. cd ../lingtai-kernel && pip install -e .")
        print("  2. cd ../lingtai && pip install -e .")
        print("  3. python -c 'import lingtai_kernel; import lingtai'")
        print("  4. python -m pytest tests/")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Commit the script**

```bash
git add scripts/rename.py
git commit -m "chore: add rename script (lingtai → lingtai)"
```

---

### Task 2: Dry run — review what will change

- [ ] **Step 1: Run the script in dry-run mode**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
python scripts/rename.py --dry-run
```

Expected: A report showing all files grouped by replacement rule, across both repos. Review the output to confirm no unexpected matches.

- [ ] **Step 2: Verify email addresses are NOT in the match list**

Check that `stoaiagent@gmail.com` and similar email addresses do not appear in the dry-run output.

---

### Task 3: Execute the rename on lingtai-kernel

- [ ] **Step 1: Run the rename script (kernel goes first)**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
python scripts/rename.py
```

The script processes `lingtai-kernel` first (since `lingtai` depends on it), then `lingtai`.

- [ ] **Step 2: Reinstall kernel in editable mode**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
pip install -e .
```

- [ ] **Step 3: Smoke-test kernel import**

```bash
python -c "import lingtai_kernel; print('kernel OK')"
python -c "from lingtai_kernel import BaseAgent; print('BaseAgent OK')"
```

Expected: Both print OK with no errors.

- [ ] **Step 4: Run kernel tests**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
python -m pytest tests/ -v
```

Expected: All 19 test files pass.

- [ ] **Step 5: Commit kernel rename**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add -A
git commit -m "chore: rename lingtai-kernel → lingtai-kernel"
```

---

### Task 4: Verify and fix lingtai repo rename

- [ ] **Step 1: Reinstall lingtai in editable mode**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
pip install -e .
```

- [ ] **Step 2: Smoke-test lingtai imports**

```bash
python -c "import lingtai; print('lingtai OK')"
python -c "from lingtai import Agent, BaseAgent, AgentConfig; print('imports OK')"
python -c "from lingtai.llm import LLMService; print('LLMService OK')"
```

Expected: All print OK.

- [ ] **Step 3: Run lingtai tests**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
python -m pytest tests/ -v
```

Expected: All 54 test files pass.

- [ ] **Step 4: Fix any test failures**

If tests fail, inspect the errors — likely missed renames. Fix and re-run.

- [ ] **Step 5: Verify no stale references in source code**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
grep -r "lingtai" src/ tests/ app/ examples/ --include="*.py" | grep -v "lingtai" | head -20
# Also check for remaining lingtai references (excluding docs and the rename spec)
grep -rn "\blingtai\b" src/ tests/ app/ examples/ --include="*.py"
```

Expected: No matches (all `lingtai` references replaced).

- [ ] **Step 6: Commit lingtai repo rename**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
git add -A
git commit -m "chore: rename lingtai → lingtai (灵台)"
```

---

### Task 5: Rewrite CLAUDE.md with 灵台 narrative

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Rewrite the opening description**

Replace the Stoa etymology with the 灵台方寸山 narrative:

```markdown
## What is 灵台

灵台 (Língtái) is a generic agent framework — an "agent operating system" providing the minimal kernel for AI agents: thinking (LLM), perceiving (vision, search), acting (file I/O), and communicating (inter-agent email). Domain tools, coordination, and orchestration are plugged in from outside via MCP-compatible interfaces.

The name comes from 灵台方寸山 — the place where 孙悟空 learned his 72 transformations from 菩提祖师. In the framework, each agent (器灵) can spawn avatars (分身) that venture into 三千世界 and return with experiences. The self-growing network of avatars IS the agent itself — memory becomes infinite through multiplication.
```

- [ ] **Step 2: Verify all mechanical replacements in CLAUDE.md look correct**

Read through CLAUDE.md and ensure all `lingtai` / `lingtai_kernel` / `灵台` replacements read naturally. Fix any awkward phrasing from the mechanical replacement.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: rewrite CLAUDE.md with 灵台 narrative"
```

---

### Task 6: Clean up

- [ ] **Step 1: Delete the rename script**

```bash
rm scripts/rename.py
rmdir scripts/  # if empty
git add -A
git commit -m "chore: remove rename script"
```

- [ ] **Step 2: Final verification**

```bash
# Both repos
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -q
cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -q

# Smoke-test CLI entry point
lingtai --help 2>&1 | head -5
```

Expected: All tests pass, CLI responds.
