"""init.json validation — required fields are strict, unknown fields warn."""
from __future__ import annotations

import logging

log = logging.getLogger(__name__)


def validate_init(data: dict) -> list[str]:
    """Validate an init.json dict.

    Raises ValueError for missing required fields or wrong types on known fields.
    Returns a list of warning strings for unknown/unexpected fields.
    """
    warnings: list[str] = []

    _require_keys(data, {
        "manifest": dict,
    }, prefix="")

    # Text fields: inline value OR _file path (at least one required)
    for key in ("principle", "covenant", "memory", "prompt", "soul"):
        file_key = f"{key}_file"
        has_inline = key in data
        has_file = file_key in data
        if not has_inline and not has_file:
            raise ValueError(f"missing required field: {key} (or {file_key})")
        if has_inline and not isinstance(data[key], str):
            raise ValueError(f"{key}: expected str, got {type(data[key]).__name__}")
        if has_file and not isinstance(data[file_key], str):
            raise ValueError(f"{file_key}: expected str, got {type(data[file_key]).__name__}")

    # Comment: optional app-level system prompt section (inline or _file)
    for key in ("comment",):
        file_key = f"{key}_file"
        if key in data and not isinstance(data[key], str):
            raise ValueError(f"{key}: expected str, got {type(data[key]).__name__}")
        if file_key in data and not isinstance(data[file_key], str):
            raise ValueError(f"{file_key}: expected str, got {type(data[file_key]).__name__}")

    # Optional top-level fields — check types for known ones
    _optional_keys(data, {
        "env_file": str,
        "venv_path": str,
        "addons": dict,
    }, prefix="")

    # Warn about unknown top-level keys
    _known_top = {
        "manifest", "env_file", "venv_path", "addons",
        "principle", "principle_file", "covenant", "covenant_file",
        "memory", "memory_file", "prompt", "prompt_file",
        "soul", "soul_file", "comment", "comment_file",
    }
    for key in data:
        if key not in _known_top:
            warnings.append(f"unknown top-level field: {key}")

    manifest = data["manifest"]
    _require_keys(manifest, {
        "llm": dict,
    }, prefix="manifest")
    _optional_keys(manifest, {
        "agent_name": (str, type(None)),
        "language": str,
        "capabilities": dict,
        "soul": dict,
        "stamina": (int, float),
        "context_limit": (int, type(None)),
        "molt_pressure": (int, float),
        "molt_prompt": str,
        "max_turns": int,
        "admin": dict,
        "streaming": bool,
    }, prefix="manifest")

    _known_manifest = {
        "llm", "agent_name", "language", "capabilities", "soul",
        "stamina", "context_limit", "molt_pressure", "molt_prompt",
        "max_turns", "admin", "streaming",
    }
    for key in manifest:
        if key not in _known_manifest:
            warnings.append(f"unknown field: manifest.{key}")

    soul = manifest.get("soul")
    if soul is not None:
        _optional_keys(soul, {
            "delay": (int, float),
        }, prefix="manifest.soul")

    llm = manifest["llm"]
    _require_keys(llm, {
        "provider": str,
        "model": str,
    }, prefix="manifest.llm")
    _optional_keys(llm, {
        "api_key": (str, type(None)),
        "api_key_env": str,
        "base_url": (str, type(None)),
    }, prefix="manifest.llm")

    # If api_key_env is set without api_key, env_file must be provided
    if llm.get("api_key_env") and not llm.get("api_key"):
        if not data.get("env_file"):
            raise ValueError(
                "manifest.llm.api_key_env is set but no env_file provided "
                "— the agent cannot resolve the API key without it"
            )

    # Validate addons if present
    addons = data.get("addons")
    if addons is not None:
        if "imap" in addons:
            warnings.extend(_validate_imap_addon(addons["imap"]))
        if "telegram" in addons:
            warnings.extend(_validate_telegram_addon(addons["telegram"]))

    return warnings


def _validate_imap_addon(cfg: dict) -> list[str]:
    """Validate imap addon config within init.json.

    Expects ``{"config": "<path>"}``. Inline fields are accepted
    but produce warnings (credentials belong in config files).
    """
    warnings: list[str] = []
    if not isinstance(cfg, dict):
        raise ValueError("addons.imap: expected object")
    if "config" not in cfg:
        warnings.append(
            "addons.imap: missing 'config' — "
            "use {\"config\": \"imap.json\"} and put credentials in the config file"
        )
    else:
        if not isinstance(cfg["config"], str):
            raise ValueError("addons.imap.config: expected str")
    return warnings


def _validate_telegram_addon(cfg: dict) -> list[str]:
    """Validate telegram addon config within init.json.

    Expects ``{"config": "<path>"}``. Inline fields are accepted
    but produce warnings (credentials belong in config files).
    """
    warnings: list[str] = []
    if not isinstance(cfg, dict):
        raise ValueError("addons.telegram: expected object")
    if "config" not in cfg:
        warnings.append(
            "addons.telegram: missing 'config' — "
            "use {\"config\": \"telegram.json\"} and put credentials in the config file"
        )
    else:
        if not isinstance(cfg["config"], str):
            raise ValueError("addons.telegram.config: expected str")
    return warnings


def _require_keys(
    data: dict,
    schema: dict[str, type | tuple[type, ...]],
    prefix: str,
) -> None:
    """Check that all keys exist in data with correct types."""
    for key, expected_type in schema.items():
        path = f"{prefix}.{key}" if prefix else key

        if key not in data:
            raise ValueError(f"missing required field: {path}")

        _check_type(data[key], expected_type, path)


def _optional_keys(
    data: dict,
    schema: dict[str, type | tuple[type, ...]],
    prefix: str,
) -> None:
    """Check types for keys that are present but not required."""
    for key, expected_type in schema.items():
        if key not in data:
            continue
        path = f"{prefix}.{key}" if prefix else key
        _check_type(data[key], expected_type, path)


def _check_type(
    value: object,
    expected_type: type | tuple[type, ...],
    path: str,
) -> None:
    """Validate a single value's type."""
    # bool is a subclass of int in Python — reject bools for numeric fields
    if isinstance(value, bool) and expected_type in (int, (int, float)):
        raise ValueError(f"{path}: expected number, got bool")

    if not isinstance(value, expected_type):
        if isinstance(expected_type, tuple):
            names = [t.__name__ for t in expected_type if t is not type(None)]
            type_str = (
                (" | ".join(names) + " | null")
                if type(None) in expected_type
                else " | ".join(names)
            )
        else:
            type_str = expected_type.__name__
            if expected_type is dict:
                type_str = "object"
        raise ValueError(
            f"{path}: expected {type_str}, got {type(value).__name__}"
        )
