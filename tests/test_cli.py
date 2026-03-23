import json
import os
import signal
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
            "vigil": 60,
            "soul_delay": 30,
            "max_turns": 10,
            "admin": {"karma": True},
            "streaming": False,
        },
        "covenant": "Be helpful.",
        "memory": "I remember nothing.",
    }
    if overrides:
        data.update(overrides)
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


def test_pid_file_written_and_cleaned(tmp_path):
    from lingtai.cli import write_pid, remove_pid
    write_pid(tmp_path)
    pid_file = tmp_path / ".agent.pid"
    assert pid_file.is_file()
    assert pid_file.read_text().strip() == str(os.getpid())
    remove_pid(tmp_path)
    assert not pid_file.is_file()
