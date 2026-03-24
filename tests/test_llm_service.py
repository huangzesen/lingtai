"""Tests for lingtai.llm.service — model registry and context limits."""

import pytest

from lingtai.llm.service import get_context_limit, CONTEXT_WINDOWS


def test_get_context_limit_unknown():
    """Unknown models should raise ValueError."""
    with pytest.raises(ValueError, match="Unknown model"):
        get_context_limit("totally-unknown-model-xyz")


def test_get_context_limit_empty():
    """Empty model name raises ValueError."""
    with pytest.raises(ValueError, match="model_name is required"):
        get_context_limit("")


def test_get_context_limit_exact_match():
    """Known models return their registered context window."""
    assert get_context_limit("claude-opus-4") == 200_000


def test_get_context_limit_prefix_match():
    """Dated model variants match via prefix."""
    assert get_context_limit("claude-sonnet-4-20250514") == 200_000


def test_adapter_base_class_has_no_multimodal_methods():
    """LLMAdapter ABC should not define multimodal convenience methods."""
    from lingtai.llm.base import LLMAdapter
    # These methods were removed — they live on individual adapters only
    for method in ("web_search", "generate_vision", "generate_image",
                   "generate_music", "text_to_speech",
                   "transcribe", "analyze_audio"):
        assert not hasattr(LLMAdapter, method), f"LLMAdapter still has {method}"


def test_llm_service_has_no_multimodal_methods():
    """LLMService should not define multimodal routing methods."""
    from lingtai.llm.service import LLMService
    for method in ("web_search", "generate_vision", "make_multimodal_message",
                   "generate_image", "generate_music", "text_to_speech",
                   "transcribe", "analyze_audio"):
        assert not hasattr(LLMService, method), f"LLMService still has {method}"


def test_llm_service_has_no_provider_config():
    """LLMService should not accept provider_config parameter."""
    import inspect
    from lingtai.llm.service import LLMService
    sig = inspect.signature(LLMService.__init__)
    assert "provider_config" not in sig.parameters
