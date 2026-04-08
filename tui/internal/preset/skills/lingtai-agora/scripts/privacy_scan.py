#!/usr/bin/env python3
"""
privacy_scan.py — Scan a staged project for secrets and absolute paths.

Walks every text file under <staging_dir> looking for two categories of
potential leaks:

HARD matches (exit code 3, agent must halt and confirm with human):
    - API keys: sk-ant-…, sk-proj-…, sk-…, ghp_…, gho_…, ghs_…, ghu_…,
                AKIA[0-9A-Z]{16}, xoxb-…, xoxp-…
    - Private keys: -----BEGIN (RSA |DSA |EC |OPENSSH |)PRIVATE KEY-----

SOFT matches (exit code 0, reported as warnings):
    - Absolute user paths: /Users/<name>/, /home/<name>/, C:\\Users\\<name>\\
    - Email addresses
    - Private IP addresses (10.*, 192.168.*, 172.16-31.*)

Binary files, files in .git/, and files larger than --max-size (default 5 MB)
are skipped. The scan walks the whole staging tree, including files that
would be excluded by .gitignore — a privacy tool should see everything the
publisher has on disk, not just what they're about to commit.

Usage:
    privacy_scan.py <staging_dir> [--max-size MB]

Exit codes:
    0  no hard matches (soft warnings may still be printed)
    1  <staging_dir> does not look like a lingtai project
    2  I/O error
    3  hard matches found — caller must halt
"""

import argparse
import re
import sys
from pathlib import Path

# ─── Hard patterns: secrets that must never ship ───────────────────────
HARD_PATTERNS: list[tuple[str, re.Pattern[str]]] = [
    ("anthropic_api_key", re.compile(r"sk-ant-[A-Za-z0-9_\-]{20,}")),
    ("openai_api_key", re.compile(r"sk-proj-[A-Za-z0-9_\-]{20,}|sk-[A-Za-z0-9]{40,}")),
    ("github_token", re.compile(r"gh[pousr]_[A-Za-z0-9]{36,}")),
    ("aws_access_key", re.compile(r"AKIA[0-9A-Z]{16}")),
    ("slack_token", re.compile(r"xox[bpars]-[A-Za-z0-9\-]{10,}")),
    (
        "private_key_block",
        re.compile(r"-----BEGIN (?:RSA |DSA |EC |OPENSSH )?PRIVATE KEY-----"),
    ),
]

# ─── Soft patterns: likely but not always problematic ──────────────────
SOFT_PATTERNS: list[tuple[str, re.Pattern[str]]] = [
    ("abs_unix_path", re.compile(r"/(?:Users|home)/[A-Za-z0-9._\-]+/")),
    ("abs_windows_path", re.compile(r"[Cc]:\\Users\\[A-Za-z0-9._\-]+\\")),
    (
        "email_address",
        re.compile(r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}"),
    ),
    (
        "private_ipv4",
        re.compile(
            r"\b(?:10\.\d{1,3}\.\d{1,3}\.\d{1,3}"
            r"|192\.168\.\d{1,3}\.\d{1,3}"
            r"|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3})\b"
        ),
    ),
]

# Skip entirely — either meaningless to scan or performance sinkholes.
SKIP_DIR_NAMES = {".git", "__pycache__", "node_modules", ".venv", "venv"}


def validate_staging_dir(staging_dir: Path) -> str | None:
    if not staging_dir.is_absolute():
        return f"refusing non-absolute path: {staging_dir}"
    if not (staging_dir / ".lingtai").is_dir():
        return f"{staging_dir} has no .lingtai/ directory — not a lingtai project"
    return None


def is_binary(path: Path, sample_bytes: int = 2048) -> bool:
    """Cheap binary detection: look for NUL in the first 2 KB."""
    try:
        with path.open("rb") as f:
            chunk = f.read(sample_bytes)
    except OSError:
        return True
    return b"\x00" in chunk


def iter_text_files(root: Path, max_size: int):
    """Yield every readable text file under root, skipping binaries and junk."""
    for p in root.rglob("*"):
        # Skip if any ancestor is in the skip set.
        try:
            rel = p.relative_to(root)
        except ValueError:
            continue
        if any(part in SKIP_DIR_NAMES for part in rel.parts):
            continue
        if not p.is_file() or p.is_symlink():
            continue
        try:
            if p.stat().st_size > max_size:
                continue
        except OSError:
            continue
        if is_binary(p):
            continue
        yield p


def scan_file(path: Path) -> tuple[list[tuple[str, int, str]], list[tuple[str, int, str]]]:
    """
    Scan one file. Returns (hard_hits, soft_hits) where each hit is
    (pattern_name, line_number, matched_text).
    """
    hard: list[tuple[str, int, str]] = []
    soft: list[tuple[str, int, str]] = []
    try:
        text = path.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return hard, soft

    for lineno, line in enumerate(text.splitlines(), start=1):
        for name, pat in HARD_PATTERNS:
            m = pat.search(line)
            if m:
                hard.append((name, lineno, m.group(0)[:80]))
        for name, pat in SOFT_PATTERNS:
            m = pat.search(line)
            if m:
                soft.append((name, lineno, m.group(0)[:80]))
    return hard, soft


def main() -> int:
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("staging_dir", type=Path)
    ap.add_argument(
        "--max-size",
        type=float,
        default=5.0,
        help="Skip files larger than N MB (default: 5)",
    )
    args = ap.parse_args()

    staging_dir = args.staging_dir.resolve()
    err = validate_staging_dir(staging_dir)
    if err:
        print(f"error: {err}", file=sys.stderr)
        return 1

    max_bytes = int(args.max_size * 1024 * 1024)

    total_files = 0
    hard_hits: list[tuple[Path, list[tuple[str, int, str]]]] = []
    soft_counts: dict[str, int] = {}
    soft_sample: dict[str, tuple[Path, int, str]] = {}

    try:
        for f in iter_text_files(staging_dir, max_bytes):
            total_files += 1
            hard, soft = scan_file(f)
            if hard:
                hard_hits.append((f, hard))
            for name, lineno, text in soft:
                soft_counts[name] = soft_counts.get(name, 0) + 1
                if name not in soft_sample:
                    soft_sample[name] = (f, lineno, text)
    except OSError as e:
        print(f"error: I/O failure during scan: {e}", file=sys.stderr)
        return 2

    print(f"scanned {total_files} text files under {staging_dir}\n")

    # ─── Soft warnings (don't block) ───────────────────────────────
    if soft_counts:
        print("─── soft warnings (review, do not block) ───")
        for name, count in sorted(soft_counts.items()):
            sample_path, lineno, text = soft_sample[name]
            rel = sample_path.relative_to(staging_dir)
            print(f"  {name}: {count} match(es)")
            print(f"    e.g. {rel}:{lineno}  →  {text}")
        print()

    # ─── Hard hits (block) ─────────────────────────────────────────
    if hard_hits:
        print("─── HARD MATCHES — halt and confirm with human ───")
        for path, hits in hard_hits:
            rel = path.relative_to(staging_dir)
            print(f"  {rel}")
            for name, lineno, text in hits:
                print(f"    line {lineno}  [{name}]  →  {text}")
        print(f"\ntotal: {sum(len(h) for _, h in hard_hits)} hard match(es) "
              f"in {len(hard_hits)} file(s)")
        return 3

    print("no hard matches — safe to proceed (soft warnings above, if any, are advisory)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
