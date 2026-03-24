from __future__ import annotations
import json
import os
from pathlib import Path
from unittest.mock import MagicMock, patch
import pytest


def test_build_capabilities_always():
    from app import _build_capabilities
    caps = _build_capabilities({})
    assert "file" in caps
    assert "psyche" in caps
    assert "avatar" in caps
    assert "email" in caps


def test_build_capabilities_with_bash():
    from app import _build_capabilities
    caps = _build_capabilities({"bash_policy": "policy.json"})
    assert "bash" in caps


def test_build_capabilities_no_bash():
    from app import _build_capabilities
    caps = _build_capabilities({})
    assert "bash" not in caps


def test_build_addons_imap():
    from app import _build_addons
    cfg = {"imap": {"email_address": "a@b.com"}}
    addons = _build_addons(cfg)
    assert "imap" in addons
    assert addons["imap"]["email_address"] == "a@b.com"


def test_build_addons_telegram():
    from app import _build_addons
    cfg = {"telegram": {"bot_token": "123:ABC"}}
    addons = _build_addons(cfg)
    assert "telegram" in addons


def test_build_addons_empty():
    from app import _build_addons
    addons = _build_addons({})
    assert addons == {}


def test_print_meta(capsys):
    from app import _print_meta
    _print_meta({
        "agent_id": "abc123",
        "agent_name": "test",
        "base_dir": "/tmp/lingtai",
        "imap": {"email_address": "a@b.com"},
        "cli": True,
        "agent_port": 8501,
    })
    out = capsys.readouterr().out
    assert "test" in out
    assert "a@b.com" in out


def test_send_message(tmp_path):
    from app import send_message
    cfg_file = tmp_path / "config.json"
    cfg_file.write_text(json.dumps({
        "model": {"provider": "x", "model": "x", "api_key_env": "K"},
        "agent_id": "abc123",
        "base_dir": str(tmp_path / "agents"),
    }))
    (tmp_path / "agents").mkdir()
    with patch.dict(os.environ, {"K": "v"}), \
         patch("app.FilesystemMailService") as MockFSMail:
        mock_svc = MagicMock()
        MockFSMail.return_value = mock_svc
        send_message(str(cfg_file), "hello")
        mock_svc.send.assert_called_once()
        assert "abc123" in mock_svc.send.call_args[0][0]
