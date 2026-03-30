"""Shared config resolution helpers — env vars, capabilities, addons."""
from __future__ import annotations

import json
import os
import re
from pathlib import Path


def load_jsonc(path: str | Path) -> dict:
    """Load a JSON or JSONC file (strips // comments and trailing commas)."""
    text = Path(path).read_text(encoding="utf-8")
    text = re.sub(r'//.*$', '', text, flags=re.MULTILINE)
    text = re.sub(r',\s*([}\]])', r'\1', text)
    return json.loads(text)


def resolve_env(value: str | None, env_name: str | None) -> str | None:
    """Resolve a value from env var name, falling back to raw value."""
    if env_name:
        env_val = os.environ.get(env_name)
        if env_val:
            return env_val
    return value


def load_env_file(path: str | Path) -> None:
    """Load a .env file into os.environ. Existing vars are not overwritten."""
    env_path = Path(path).expanduser()
    if not env_path.is_file():
        return
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, _, val = line.partition("=")
        if not _:
            continue
        key = key.strip()
        val = val.strip().strip("'\"")
        if key not in os.environ:
            os.environ[key] = val


def resolve_file(value: str | None, file_path: str | None) -> str | None:
    """Resolve a value from a file path, falling back to raw value."""
    if file_path:
        p = Path(file_path).expanduser()
        if p.is_file():
            return p.read_text(encoding="utf-8")
    return value


def _resolve_env_fields(d: dict) -> dict:
    """Resolve ``*_env`` keys in a dict using ``resolve_env``."""
    result = dict(d)
    env_keys = [k for k in result if k.endswith("_env")]
    for env_key in env_keys:
        base_key = env_key[: -len("_env")]
        result[base_key] = resolve_env(result.get(base_key), result.pop(env_key))
    return result


def _resolve_file_fields(d: dict) -> dict:
    """Resolve ``*_file`` keys in a dict using ``resolve_file``."""
    result = dict(d)
    file_keys = [k for k in result if k.endswith("_file")]
    for file_key in file_keys:
        base_key = file_key[: -len("_file")]
        result[base_key] = resolve_file(result.get(base_key), result.pop(file_key))
    return result


def resolve_paths(data: dict, working_dir: str | Path) -> None:
    """Make every path field in init.json absolute, resolved against working_dir.

    Mutates *data* in place. Handles:
    - Top-level: env_file, venv_path, *_file (covenant_file, etc.)
    - Addon configs: addons.*.config
    """
    wd = Path(working_dir)

    # Top-level path fields
    for key in ("env_file", "venv_path",
                "covenant_file", "principle_file", "memory_file",
                "prompt_file", "comment_file", "soul_file"):
        if key in data and isinstance(data[key], str) and data[key]:
            p = Path(data[key]).expanduser()
            if not p.is_absolute():
                p = wd / p
            data[key] = str(p)

    # Addon config paths
    addons = data.get("addons")
    if isinstance(addons, dict):
        for cfg in addons.values():
            if isinstance(cfg, dict) and "config" in cfg:
                p = Path(cfg["config"]).expanduser()
                if not p.is_absolute():
                    p = wd / p
                cfg["config"] = str(p)


def _resolve_capabilities(capabilities: dict) -> dict:
    """Resolve ``*_env`` fields in each capability's kwargs."""
    resolved = {}
    for name, kwargs in capabilities.items():
        if isinstance(kwargs, dict) and kwargs:
            resolved[name] = _resolve_env_fields(kwargs)
        else:
            resolved[name] = kwargs
    return resolved


def _resolve_addons(addons: dict | None) -> dict | None:
    """Resolve *_env fields in addon configs to actual values."""
    if not addons:
        return None
    resolved = {}
    for name, cfg in addons.items():
        if isinstance(cfg, dict):
            resolved[name] = _resolve_env_fields(cfg)
    return resolved or None
