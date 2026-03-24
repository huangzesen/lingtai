from __future__ import annotations

from unittest.mock import MagicMock, patch

from app.cli import CLIChannel


def test_construction(tmp_path):
    ch = CLIChannel(agent_address="/tmp/agent_dir", cli_dir=tmp_path / "cli")
    assert ch._agent_address == "/tmp/agent_dir"
    assert ch.address == str(tmp_path / "cli")


def test_send_message(tmp_path):
    """send() should deliver a filesystem mail to the agent address."""
    ch = CLIChannel(agent_address="/tmp/agent_dir", cli_dir=tmp_path / "cli")
    with patch("app.cli.FilesystemMailService") as MockFS:
        mock_svc = MagicMock()
        MockFS.return_value = mock_svc
        ch.send("Hello agent")
        mock_svc.send.assert_called_once()
        call_args = mock_svc.send.call_args
        assert call_args[0][0] == "/tmp/agent_dir"
        payload = call_args[0][1]
        assert payload["message"] == "Hello agent"
        assert str(tmp_path / "cli") in payload["from"]


def test_on_receive_prints(capsys, tmp_path):
    """Incoming messages should be printed to stdout."""
    ch = CLIChannel(agent_address="/tmp/agent_dir", cli_dir=tmp_path / "cli")
    ch._on_message({
        "from": "orchestrator@localhost:8501",
        "message": "I found 3 emails.",
    })
    captured = capsys.readouterr()
    assert "I found 3 emails." in captured.out
    assert "orchestrator" in captured.out


def test_on_receive_empty_message(capsys, tmp_path):
    """Empty messages should not print anything."""
    ch = CLIChannel(agent_address="/tmp/agent_dir", cli_dir=tmp_path / "cli")
    ch._on_message({"from": "agent", "message": ""})
    captured = capsys.readouterr()
    assert captured.out == ""
