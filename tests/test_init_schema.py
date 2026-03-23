import json
import pytest
from lingtai.init_schema import validate_init


def _valid_init() -> dict:
    """Return a minimal valid init.json dict."""
    return {
        "manifest": {
            "agent_name": "alice",
            "language": "en",
            "llm": {
                "provider": "anthropic",
                "model": "claude-sonnet-4-20250514",
                "api_key": None,
                "base_url": None,
            },
            "capabilities": {},
            "soul": {"delay": 120},
            "vigil": 3600,
            "max_turns": 50,
            "admin": {"karma": True},
            "streaming": False,
        },
        "covenant": "",
        "memory": "",
        "prompt": "",
    }


def test_valid_init_passes():
    validate_init(_valid_init())  # should not raise


def test_missing_top_level_key():
    data = _valid_init()
    del data["covenant"]
    with pytest.raises(ValueError, match="covenant"):
        validate_init(data)


def test_missing_manifest_field():
    data = _valid_init()
    del data["manifest"]["agent_name"]
    with pytest.raises(ValueError, match="manifest.agent_name"):
        validate_init(data)


def test_missing_llm_field():
    data = _valid_init()
    del data["manifest"]["llm"]["provider"]
    with pytest.raises(ValueError, match="manifest.llm.provider"):
        validate_init(data)


def test_wrong_type_top_level():
    data = _valid_init()
    data["covenant"] = 123
    with pytest.raises(ValueError, match="covenant.*str"):
        validate_init(data)


def test_wrong_type_manifest_field():
    data = _valid_init()
    data["manifest"]["vigil"] = "one hour"
    with pytest.raises(ValueError, match="manifest.vigil.*(int|float|number)"):
        validate_init(data)


def test_wrong_type_capabilities():
    data = _valid_init()
    data["manifest"]["capabilities"] = ["file", "bash"]
    with pytest.raises(ValueError, match="manifest.capabilities.*object"):
        validate_init(data)


def test_wrong_type_streaming():
    data = _valid_init()
    data["manifest"]["streaming"] = "yes"
    with pytest.raises(ValueError, match="manifest.streaming.*bool"):
        validate_init(data)


def test_bool_rejected_for_numeric_field():
    """bool is a subclass of int in Python — must be rejected for numeric fields."""
    data = _valid_init()
    data["manifest"]["vigil"] = True
    with pytest.raises(ValueError, match="manifest.vigil.*number.*bool"):
        validate_init(data)
