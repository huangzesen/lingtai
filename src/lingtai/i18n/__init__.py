"""Capability i18n — language-aware string tables for lingtai capabilities.

Usage: t(lang, key, **kwargs)
  lang: language code ("en", "zh")
  key: dotted string ID ("read.description")
  kwargs: template substitutions

Mirrors lingtai_kernel.i18n for the capability layer.
"""
from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path

_DIR = Path(__file__).parent
_CACHE: dict[str, dict[str, str]] = {}


def _load(lang: str) -> dict[str, str]:
    if lang not in _CACHE:
        path = _DIR / f"{lang}.json"
        if path.is_file():
            _CACHE[lang] = json.loads(path.read_text(encoding="utf-8"))
        else:
            _CACHE[lang] = {}
    return _CACHE[lang]


def t(lang: str, key: str, **kwargs) -> str:
    table = _load(lang)
    value = table.get(key)
    if value is None and lang != "en":
        value = _load("en").get(key)
    if value is None:
        return key
    if kwargs:
        return value.format_map(defaultdict(str, kwargs))
    return value
