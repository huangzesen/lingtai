from __future__ import annotations

import json
import os
from pathlib import Path
from unittest.mock import patch

import pytest

from app.config import load_config, resolve_env_vars


def test_load_config_basic(tmp_path):
    cfg_file = tmp_path / "config.json"
    cfg_file.write_text(json.dumps({
        "model": {"provider": "minimax", "model": "test", "api_key_env": "TEST_KEY"},
        "agent_name": "test-agent",
    }))
    cfg = load_config(str(cfg_file))
    assert cfg["agent_name"] == "test-agent"


def test_load_config_defaults(tmp_path):
    cfg_file = tmp_path / "config.json"
    cfg_file.write_text(json.dumps({
        "model": {"provider": "minimax", "model": "test", "api_key_env": "K"},
    }))
    cfg = load_config(str(cfg_file))
    assert cfg["agent_name"] == "orchestrator"
    assert cfg["max_turns"] == 50
    assert cfg["agent_port"] == 8501
    assert cfg["cli"] is False


def test_load_config_missing_model(tmp_path):
    cfg_file = tmp_path / "config.json"
    cfg_file.write_text(json.dumps({"agent_name": "x"}))
    with pytest.raises(ValueError, match="model"):
        load_config(str(cfg_file))


def test_load_model_config_from_file(tmp_path):
    model_file = tmp_path / "model.json"
    model_file.write_text(json.dumps({
        "provider": "minimax", "model": "MiniMax-M2.7-highspeed", "api_key_env": "MINIMAX_API_KEY",
    }))
    cfg_file = tmp_path / "config.json"
    cfg_file.write_text(json.dumps({"model": "model.json"}))
    cfg = load_config(str(cfg_file))
    assert cfg["_model_config"]["provider"] == "minimax"


def test_load_model_config_inline(tmp_path):
    cfg_file = tmp_path / "config.json"
    cfg_file.write_text(json.dumps({
        "model": {"provider": "openai", "model": "gpt-4o", "api_key_env": "OAI"},
    }))
    cfg = load_config(str(cfg_file))
    assert cfg["_model_config"]["provider"] == "openai"


def test_resolve_env_vars():
    model_cfg = {"api_key_env": "TEST_KEY_1"}
    with patch.dict(os.environ, {"TEST_KEY_1": "secret123"}):
        resolved = resolve_env_vars(model_cfg, ["api_key_env"])
    assert resolved["api_key"] == "secret123"


def test_resolve_env_vars_missing():
    model_cfg = {"api_key_env": "NONEXISTENT_KEY"}
    with pytest.raises(ValueError, match="NONEXISTENT_KEY"):
        resolve_env_vars(model_cfg, ["api_key_env"])


def test_load_dotenv(tmp_path):
    env_file = tmp_path / ".env"
    env_file.write_text("MY_TEST_VAR=hello_from_dotenv\n")
    cfg_file = tmp_path / "config.json"
    cfg_file.write_text(json.dumps({
        "model": {"provider": "x", "model": "x", "api_key_env": "MY_TEST_VAR"},
    }))
    os.environ.pop("MY_TEST_VAR", None)
    cfg = load_config(str(cfg_file))
    assert os.environ.get("MY_TEST_VAR") == "hello_from_dotenv"


def test_load_config_file_not_found():
    with pytest.raises(FileNotFoundError):
        load_config("/nonexistent/config.json")


def test_load_model_file_not_found(tmp_path):
    cfg_file = tmp_path / "config.json"
    cfg_file.write_text(json.dumps({"model": "nonexistent.json"}))
    with pytest.raises(FileNotFoundError, match="nonexistent"):
        load_config(str(cfg_file))
