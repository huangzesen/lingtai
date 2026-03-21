"""Config loading, model config, env var resolution, validation."""
from __future__ import annotations

import json
import os
from pathlib import Path

_DEFAULTS = {
    "agent_name": "orchestrator",
    "max_turns": 50,
    "agent_port": 8501,
    "cli": False,
    "language": "en",
}


def load_dotenv(config_dir: Path) -> None:
    """Load .env from config directory into os.environ (setdefault)."""
    env_path = config_dir / ".env"
    if not env_path.exists():
        return
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))


def load_config(config_path: str) -> dict:
    """Load and validate config.json. Returns config dict with _model_config added."""
    path = Path(config_path)
    if not path.is_file():
        raise FileNotFoundError(f"Config not found: {config_path}")

    config_dir = path.parent
    load_dotenv(config_dir)

    cfg = json.loads(path.read_text(encoding="utf-8"))

    # Derive base_dir from config path: configs/config.json -> project dir
    project_dir = config_dir.parent
    cfg["base_dir"] = str(project_dir)

    # Apply defaults
    for key, default in _DEFAULTS.items():
        cfg.setdefault(key, default)

    # Resolve model config
    model_raw = cfg.get("model")
    if model_raw is None:
        raise ValueError("'model' field is required in config.json")

    if isinstance(model_raw, str):
        # File path — must end in .json
        model_path = config_dir / model_raw
        if not model_path.is_file():
            raise FileNotFoundError(f"Model config not found: {model_path}")
        model_cfg = json.loads(model_path.read_text(encoding="utf-8"))
    elif isinstance(model_raw, dict):
        model_cfg = model_raw
    else:
        raise ValueError("'model' must be a file path (string ending in .json) or inline object")

    cfg["_model_config"] = model_cfg
    return cfg


def resolve_env_vars(cfg: dict, env_keys: list[str]) -> dict:
    """Resolve *_env fields to their values.

    For each key in env_keys (e.g. "api_key_env"), looks up the env var
    and stores the value under the non-_env key (e.g. "api_key").
    Returns a new dict with resolved values added.
    """
    result = dict(cfg)
    for env_key in env_keys:
        if env_key not in cfg:
            continue
        var_name = cfg[env_key]
        value = os.environ.get(var_name)
        if not value:
            raise ValueError(
                f"Environment variable {var_name!r} (from {env_key!r}) is not set. "
                f"Set it in your environment or .env file."
            )
        base_key = env_key.removesuffix("_env")
        result[base_key] = value
    return result
