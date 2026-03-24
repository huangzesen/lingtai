"""Tests for lingtai.llm.service."""

import inspect

from lingtai.llm.service import LLMService


def test_context_window_stored():
    """context_window should be accepted and stored."""
    sig = inspect.signature(LLMService.__init__)
    assert "context_window" in sig.parameters


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
    for method in ("web_search", "generate_vision", "make_multimodal_message",
                   "generate_image", "generate_music", "text_to_speech",
                   "transcribe", "analyze_audio"):
        assert not hasattr(LLMService, method), f"LLMService still has {method}"


def test_llm_service_has_no_provider_config():
    """LLMService should not accept provider_config parameter."""
    sig = inspect.signature(LLMService.__init__)
    assert "provider_config" not in sig.parameters


def test_no_get_context_limit():
    """get_context_limit should no longer exist — context window is caller-provided."""
    import lingtai.llm.service as mod
    assert not hasattr(mod, "get_context_limit")
    assert not hasattr(mod, "CONTEXT_WINDOWS")
    assert not hasattr(mod, "DEFAULT_CONTEXT_WINDOW")
