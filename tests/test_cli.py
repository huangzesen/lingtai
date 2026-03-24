import json
import os
import pytest
from pathlib import Path
from unittest.mock import MagicMock, patch


def _write_init(tmp_path: Path, overrides: dict | None = None) -> Path:
    """Write a valid init.json to tmp_path and return the path."""
    data = {
        "manifest": {
            "agent_name": "test-agent",
            "language": "en",
            "llm": {
                "provider": "anthropic",
                "model": "test-model",
                "api_key": "test-key",
                "base_url": None,
            },
            "capabilities": {},
            "soul": {"delay": 30},
            "stamina": 60,
            "context_limit": None,
            "molt_pressure": 0.8,
            "molt_prompt": "",
            "max_turns": 10,
            "admin": {"karma": True},
            "streaming": False,
        },
        "principle": "",
        "covenant": "Be helpful.",
        "memory": "I remember nothing.",
        "prompt": "",
    }
    if overrides:
        # Deep merge manifest if provided
        for k, v in overrides.items():
            if k == "manifest" and isinstance(v, dict):
                data["manifest"].update(v)
            else:
                data[k] = v
    init_path = tmp_path / "init.json"
    init_path.write_text(json.dumps(data))
    return tmp_path


def test_load_init_reads_file(tmp_path):
    from lingtai.cli import load_init
    _write_init(tmp_path)
    data = load_init(tmp_path)
    assert data["manifest"]["agent_name"] == "test-agent"


def test_load_init_missing_file(tmp_path):
    from lingtai.cli import load_init
    with pytest.raises(SystemExit):
        load_init(tmp_path)


def test_load_init_invalid_json(tmp_path):
    (tmp_path / "init.json").write_text("{bad json")
    from lingtai.cli import load_init
    with pytest.raises(SystemExit):
        load_init(tmp_path)


def test_load_init_validation_error(tmp_path):
    (tmp_path / "init.json").write_text(json.dumps({"manifest": {}}))
    from lingtai.cli import load_init
    with pytest.raises(SystemExit):
        load_init(tmp_path)


@patch("lingtai.cli.LLMService")
@patch("lingtai.cli.Agent")
@patch("lingtai.cli.FilesystemMailService")
def test_build_agent_constructs_correctly(mock_mail, mock_agent, mock_llm, tmp_path):
    from lingtai.cli import load_init, build_agent
    _write_init(tmp_path)
    data = load_init(tmp_path)
    build_agent(data, tmp_path)

    mock_llm.assert_called_once_with(
        provider="anthropic",
        model="test-model",
        api_key="test-key",
        base_url=None,
    )
    mock_mail.assert_called_once_with(working_dir=tmp_path)
    mock_agent.assert_called_once()
    call_kwargs = mock_agent.call_args
    assert call_kwargs.kwargs["agent_name"] == "test-agent"
    assert call_kwargs.kwargs["working_dir"] == tmp_path
    assert call_kwargs.kwargs["covenant"] == "Be helpful."
    assert call_kwargs.kwargs["memory"] == "I remember nothing."
    assert call_kwargs.kwargs["streaming"] is False


# --- env file and env var resolution ---


def test_load_env_file(tmp_path):
    from lingtai.cli import load_env_file
    env_path = tmp_path / ".env"
    env_path.write_text("TEST_CLI_KEY=secret123\nTEST_CLI_OTHER='quoted'\n")

    # Clean up after test
    for k in ("TEST_CLI_KEY", "TEST_CLI_OTHER"):
        os.environ.pop(k, None)

    load_env_file(env_path)
    assert os.environ["TEST_CLI_KEY"] == "secret123"
    assert os.environ["TEST_CLI_OTHER"] == "quoted"

    # Does not overwrite existing
    os.environ["TEST_CLI_KEY"] = "original"
    load_env_file(env_path)
    assert os.environ["TEST_CLI_KEY"] == "original"

    # Clean up
    os.environ.pop("TEST_CLI_KEY", None)
    os.environ.pop("TEST_CLI_OTHER", None)


def test_load_env_file_missing():
    from lingtai.cli import load_env_file
    # Should not raise on missing file
    load_env_file("/nonexistent/.env")


def test_resolve_env_prefers_env_var():
    from lingtai.cli import resolve_env
    os.environ["TEST_RESOLVE_KEY"] = "from-env"
    try:
        assert resolve_env("raw-value", "TEST_RESOLVE_KEY") == "from-env"
    finally:
        os.environ.pop("TEST_RESOLVE_KEY", None)


def test_resolve_env_falls_back_to_raw():
    from lingtai.cli import resolve_env
    os.environ.pop("NONEXISTENT_KEY_12345", None)
    assert resolve_env("raw-value", "NONEXISTENT_KEY_12345") == "raw-value"


def test_resolve_env_no_env_name():
    from lingtai.cli import resolve_env
    assert resolve_env("raw-value", None) == "raw-value"
    assert resolve_env(None, None) is None


@patch("lingtai.cli.LLMService")
@patch("lingtai.cli.Agent")
@patch("lingtai.cli.FilesystemMailService")
def test_build_agent_resolves_api_key_env(mock_mail, mock_agent, mock_llm, tmp_path):
    """api_key_env resolves from environment, overriding raw api_key."""
    from lingtai.cli import load_init, build_agent

    _write_init(tmp_path)
    data = load_init(tmp_path)
    data["manifest"]["llm"]["api_key_env"] = "TEST_LLM_KEY"
    data["manifest"]["llm"]["api_key"] = "fallback-key"

    os.environ["TEST_LLM_KEY"] = "env-key-value"
    try:
        build_agent(data, tmp_path)
    finally:
        os.environ.pop("TEST_LLM_KEY", None)

    mock_llm.assert_called_once_with(
        provider="anthropic",
        model="test-model",
        api_key="env-key-value",
        base_url=None,
    )


@patch("lingtai.cli.LLMService")
@patch("lingtai.cli.Agent")
@patch("lingtai.cli.FilesystemMailService")
def test_build_agent_env_file_loaded(mock_mail, mock_agent, mock_llm, tmp_path):
    """env_file is loaded before resolving env vars."""
    from lingtai.cli import load_init, build_agent

    env_path = tmp_path / "secrets.env"
    env_path.write_text("TEST_ENV_FILE_KEY=from-file\n")

    _write_init(tmp_path)
    data = load_init(tmp_path)
    data["env_file"] = str(env_path)
    data["manifest"]["llm"]["api_key_env"] = "TEST_ENV_FILE_KEY"

    os.environ.pop("TEST_ENV_FILE_KEY", None)
    try:
        build_agent(data, tmp_path)
    finally:
        os.environ.pop("TEST_ENV_FILE_KEY", None)

    mock_llm.assert_called_once_with(
        provider="anthropic",
        model="test-model",
        api_key="from-file",
        base_url=None,
    )


# --- addons ---


@patch("lingtai.cli.LLMService")
@patch("lingtai.cli.Agent")
@patch("lingtai.cli.FilesystemMailService")
def test_build_agent_passes_addons(mock_mail, mock_agent, mock_llm, tmp_path):
    """Addons from init.json are passed through to Agent."""
    from lingtai.cli import load_init, build_agent

    _write_init(tmp_path)
    data = load_init(tmp_path)
    data["addons"] = {
        "imap": {
            "email_address": "test@gmail.com",
            "email_password": "secret",
            "imap_host": "imap.gmail.com",
            "smtp_host": "smtp.gmail.com",
        },
    }

    build_agent(data, tmp_path)

    call_kwargs = mock_agent.call_args.kwargs
    assert call_kwargs["addons"] is not None
    assert "imap" in call_kwargs["addons"]
    assert call_kwargs["addons"]["imap"]["email_address"] == "test@gmail.com"
    assert call_kwargs["addons"]["imap"]["email_password"] == "secret"


@patch("lingtai.cli.LLMService")
@patch("lingtai.cli.Agent")
@patch("lingtai.cli.FilesystemMailService")
def test_build_agent_resolves_addon_env(mock_mail, mock_agent, mock_llm, tmp_path):
    """Addon *_env fields are resolved from environment."""
    from lingtai.cli import load_init, build_agent

    _write_init(tmp_path)
    data = load_init(tmp_path)
    data["addons"] = {
        "imap": {
            "email_address": "test@gmail.com",
            "email_password_env": "TEST_IMAP_PASS",
        },
        "telegram": {
            "bot_token_env": "TEST_TG_TOKEN",
        },
    }

    os.environ["TEST_IMAP_PASS"] = "imap-secret"
    os.environ["TEST_TG_TOKEN"] = "tg-secret"
    try:
        build_agent(data, tmp_path)
    finally:
        os.environ.pop("TEST_IMAP_PASS", None)
        os.environ.pop("TEST_TG_TOKEN", None)

    addons = mock_agent.call_args.kwargs["addons"]
    assert addons["imap"]["email_password"] == "imap-secret"
    assert "email_password_env" not in addons["imap"]
    assert addons["telegram"]["bot_token"] == "tg-secret"
    assert "bot_token_env" not in addons["telegram"]


@patch("lingtai.cli.LLMService")
@patch("lingtai.cli.Agent")
@patch("lingtai.cli.FilesystemMailService")
def test_build_agent_no_addons(mock_mail, mock_agent, mock_llm, tmp_path):
    """No addons field means addons=None."""
    from lingtai.cli import load_init, build_agent

    _write_init(tmp_path)
    data = load_init(tmp_path)
    build_agent(data, tmp_path)

    assert mock_agent.call_args.kwargs["addons"] is None
