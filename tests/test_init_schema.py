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
            "stamina": 3600,
            "context_limit": None,
            "molt_pressure": 0.8,
            "molt_prompt": "",
            "max_turns": 50,
            "admin": {"karma": True},
            "streaming": False,
        },
        "principle": "",
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
    """Only manifest.llm is truly required — other fields are optional."""
    data = _valid_init()
    del data["manifest"]["llm"]
    with pytest.raises(ValueError, match="manifest.llm"):
        validate_init(data)


def test_minimal_init_passes():
    """Bare-minimum init.json: only manifest.llm with provider+model."""
    data = {
        "manifest": {
            "llm": {
                "provider": "anthropic",
                "model": "claude-sonnet-4-20250514",
            },
        },
        "principle": "",
        "covenant": "",
        "memory": "",
        "prompt": "",
    }
    validate_init(data)  # should not raise


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
    data["manifest"]["stamina"] = "one hour"
    with pytest.raises(ValueError, match="manifest.stamina.*(int|float|number)"):
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
    data["manifest"]["stamina"] = True
    with pytest.raises(ValueError, match="manifest.stamina.*number.*bool"):
        validate_init(data)


# --- optional fields ---


def test_env_file_optional():
    data = _valid_init()
    validate_init(data)  # no env_file — should pass
    data["env_file"] = "~/.lingtai/.env"
    validate_init(data)  # with env_file — should pass


def test_env_file_wrong_type():
    data = _valid_init()
    data["env_file"] = 123
    with pytest.raises(ValueError, match="env_file.*str"):
        validate_init(data)


def test_api_key_env_optional():
    data = _valid_init()
    data["manifest"]["llm"]["api_key_env"] = "MY_KEY"
    validate_init(data)


def test_api_key_env_wrong_type():
    data = _valid_init()
    data["manifest"]["llm"]["api_key_env"] = 123
    with pytest.raises(ValueError, match="api_key_env.*str"):
        validate_init(data)


# --- addons ---


def test_addons_optional():
    data = _valid_init()
    validate_init(data)  # no addons — should pass


def test_addons_imap_valid():
    data = _valid_init()
    data["addons"] = {
        "imap": {
            "email_address": "test@gmail.com",
            "email_password": "secret",
        },
    }
    validate_init(data)


def test_addons_imap_with_env():
    data = _valid_init()
    data["addons"] = {
        "imap": {
            "email_address": "test@gmail.com",
            "email_password_env": "IMAP_PASS",
        },
    }
    validate_init(data)


def test_addons_imap_missing_password():
    data = _valid_init()
    data["addons"] = {
        "imap": {
            "email_address": "test@gmail.com",
        },
    }
    with pytest.raises(ValueError, match="email_password"):
        validate_init(data)


def test_addons_imap_missing_email():
    data = _valid_init()
    data["addons"] = {
        "imap": {
            "email_password": "secret",
        },
    }
    with pytest.raises(ValueError, match="email_address"):
        validate_init(data)


def test_addons_telegram_valid():
    data = _valid_init()
    data["addons"] = {
        "telegram": {
            "bot_token": "123:ABC",
        },
    }
    validate_init(data)


def test_addons_telegram_with_env():
    data = _valid_init()
    data["addons"] = {
        "telegram": {
            "bot_token_env": "TG_TOKEN",
        },
    }
    validate_init(data)


def test_addons_telegram_missing_token():
    data = _valid_init()
    data["addons"] = {
        "telegram": {},
    }
    with pytest.raises(ValueError, match="bot_token"):
        validate_init(data)
