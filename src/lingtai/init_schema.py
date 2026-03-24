"""init.json validation — every field required, no defaults, fail loudly."""
from __future__ import annotations


def validate_init(data: dict) -> None:
    """Validate an init.json dict. Raises ValueError with field path on failure."""

    _require_keys(data, {
        "manifest": dict,
        "principle": str,
        "covenant": str,
        "memory": str,
        "prompt": str,
    }, prefix="")

    # Optional top-level fields
    _optional_keys(data, {
        "env_file": str,
        "addons": dict,
    }, prefix="")

    manifest = data["manifest"]
    _require_keys(manifest, {
        "agent_name": str,
        "language": str,
        "llm": dict,
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

    soul = manifest["soul"]
    _require_keys(soul, {
        "delay": (int, float),
    }, prefix="manifest.soul")

    llm = manifest["llm"]
    _require_keys(llm, {
        "provider": str,
        "model": str,
        "api_key": (str, type(None)),
        "base_url": (str, type(None)),
    }, prefix="manifest.llm")
    _optional_keys(llm, {
        "api_key_env": str,
    }, prefix="manifest.llm")

    # Validate addons if present
    addons = data.get("addons")
    if addons is not None:
        if "imap" in addons:
            _validate_imap_addon(addons["imap"])
        if "telegram" in addons:
            _validate_telegram_addon(addons["telegram"])


def _validate_imap_addon(cfg: dict) -> None:
    """Validate imap addon config within init.json."""
    if not isinstance(cfg, dict):
        raise ValueError("addons.imap: expected object")
    _require_keys(cfg, {
        "email_address": str,
    }, prefix="addons.imap")
    _optional_keys(cfg, {
        "email_password": str,
        "email_password_env": str,
        "imap_host": str,
        "imap_port": int,
        "smtp_host": str,
        "smtp_port": int,
        "allowed_senders": list,
        "poll_interval": int,
    }, prefix="addons.imap")
    # Must have at least one of email_password or email_password_env
    if "email_password" not in cfg and "email_password_env" not in cfg:
        raise ValueError(
            "addons.imap: requires 'email_password' or 'email_password_env'"
        )


def _validate_telegram_addon(cfg: dict) -> None:
    """Validate telegram addon config within init.json."""
    if not isinstance(cfg, dict):
        raise ValueError("addons.telegram: expected object")
    _optional_keys(cfg, {
        "bot_token": str,
        "bot_token_env": str,
        "allowed_users": list,
        "poll_interval": (int, float),
    }, prefix="addons.telegram")
    # Must have at least one of bot_token or bot_token_env
    if "bot_token" not in cfg and "bot_token_env" not in cfg:
        raise ValueError(
            "addons.telegram: requires 'bot_token' or 'bot_token_env'"
        )


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
