"""init.json validation — every field required, no defaults, fail loudly."""
from __future__ import annotations


def validate_init(data: dict) -> None:
    """Validate an init.json dict. Raises ValueError with field path on failure."""

    _require_keys(data, {
        "manifest": dict,
        "covenant": str,
        "memory": str,
        "prompt": str,
    }, prefix="")

    manifest = data["manifest"]
    _require_keys(manifest, {
        "agent_name": str,
        "language": str,
        "llm": dict,
        "capabilities": dict,
        "soul": dict,
        "vigil": (int, float),
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

        value = data[key]

        # bool is a subclass of int in Python — reject bools for numeric fields
        if isinstance(value, bool) and expected_type in (int, (int, float)):
            raise ValueError(f"{path}: expected number, got bool")

        if not isinstance(value, expected_type):
            # Build readable type name
            if isinstance(expected_type, tuple):
                names = [t.__name__ for t in expected_type if t is not type(None)]
                type_str = (" | ".join(names) + " | null") if type(None) in expected_type else " | ".join(names)
            else:
                type_str = expected_type.__name__
                if expected_type is dict:
                    type_str = "object"
            raise ValueError(
                f"{path}: expected {type_str}, got {type(value).__name__}"
            )
