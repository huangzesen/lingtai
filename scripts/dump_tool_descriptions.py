#!/usr/bin/env python3
"""Dump all tool descriptions from i18n files as a single markdown with all locales.

Usage:
    python scripts/dump_tool_descriptions.py

Reads from both lingtai-kernel and lingtai i18n JSONs.
Outputs one markdown with en / zh / wen columns per tool.
"""
from __future__ import annotations

import json
from pathlib import Path

LANGS = ["en", "zh", "wen"]
LANG_LABELS = {"en": "English", "zh": "中文", "wen": "文言"}

BASE = Path(__file__).resolve().parent.parent
KERNEL_I18N = BASE.parent / "lingtai-kernel" / "src" / "lingtai_kernel" / "i18n"
LINGTAI_I18N = BASE / "src" / "lingtai" / "i18n"


def load_descriptions(path: Path) -> dict[str, str]:
    """Load i18n JSON and return only keys ending with '.description'."""
    if not path.is_file():
        return {}
    data = json.loads(path.read_text())
    return {k: v for k, v in data.items() if k.endswith(".description")}


def tool_name(key: str) -> str:
    return key.rsplit(".description", 1)[0]


def collect(i18n_dir: Path) -> dict[str, dict[str, str]]:
    """Return {tool_name: {lang: description}} for all langs."""
    tools: dict[str, dict[str, str]] = {}
    for lang in LANGS:
        descs = load_descriptions(i18n_dir / f"{lang}.json")
        for key, value in descs.items():
            name = tool_name(key)
            tools.setdefault(name, {})[lang] = value
    return tools


def print_section(title: str, tools: dict[str, dict[str, str]]) -> None:
    if not tools:
        return
    print(f"## {title}\n")
    for name, langs in tools.items():
        print(f"### {name}\n")
        for lang in LANGS:
            desc = langs.get(lang, "—")
            print(f"**{LANG_LABELS[lang]}:**\n{desc}\n")


def main() -> None:
    kernel = collect(KERNEL_I18N)
    lingtai = collect(LINGTAI_I18N)

    print("# Tool Descriptions\n")
    print_section("Kernel Intrinsics", kernel)
    print_section("Capabilities", lingtai)


if __name__ == "__main__":
    main()
