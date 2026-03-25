# tests/test_deep_refresh.py
"""Tests for deep refresh (full agent reconstruct from init.json)."""
from __future__ import annotations

import os


def test_resolve_env_fields_resolves_env_var(monkeypatch):
    """_resolve_env_fields replaces *_env keys with env var values."""
    from lingtai.config_resolve import _resolve_env_fields

    monkeypatch.setenv("TEST_SECRET", "hunter2")
    result = _resolve_env_fields({"api_key": None, "api_key_env": "TEST_SECRET"})
    assert result == {"api_key": "hunter2"}
    assert "api_key_env" not in result


def test_resolve_capabilities_resolves_env():
    """_resolve_capabilities applies _resolve_env_fields to each capability."""
    from lingtai.config_resolve import _resolve_capabilities

    caps = {"bash": {"policy_file": "p.json"}, "vision": {}}
    result = _resolve_capabilities(caps)
    assert result == {"bash": {"policy_file": "p.json"}, "vision": {}}


def test_resolve_addons_none():
    """_resolve_addons returns None for None/empty input."""
    from lingtai.config_resolve import _resolve_addons

    assert _resolve_addons(None) is None
    assert _resolve_addons({}) is None
