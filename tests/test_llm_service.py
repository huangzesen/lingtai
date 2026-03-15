"""Tests for stoai.llm.service — model registry and context limits."""

from stoai.llm.service import get_context_limit, DEFAULT_CONTEXT_WINDOW


def test_get_context_limit_unknown():
    """Unknown models should return default 256k."""
    limit = get_context_limit("totally-unknown-model-xyz")
    assert limit == DEFAULT_CONTEXT_WINDOW


def test_get_context_limit_empty():
    """Empty model name returns default 256k."""
    assert get_context_limit("") == DEFAULT_CONTEXT_WINDOW
