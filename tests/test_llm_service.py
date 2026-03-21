"""Tests for lingtai.llm.service — model registry and context limits."""

from lingtai_kernel.llm.service import get_context_limit, DEFAULT_CONTEXT_WINDOW


def test_get_context_limit_unknown():
    """Unknown models should return default 256k."""
    limit = get_context_limit("totally-unknown-model-xyz")
    assert limit == DEFAULT_CONTEXT_WINDOW


def test_get_context_limit_empty():
    """Empty model name returns default 256k."""
    assert get_context_limit("") == DEFAULT_CONTEXT_WINDOW


def test_adapter_base_class_has_no_multimodal_methods():
    """LLMAdapter ABC should not define multimodal convenience methods."""
    from lingtai_kernel.llm.base import LLMAdapter
    # These methods were removed — they live on individual adapters only
    for method in ("web_search", "generate_vision", "generate_image",
                   "generate_music", "text_to_speech",
                   "transcribe", "analyze_audio"):
        assert not hasattr(LLMAdapter, method), f"LLMAdapter still has {method}"


def test_llm_service_has_no_multimodal_methods():
    """LLMService should not define multimodal routing methods."""
    from lingtai_kernel.llm.service import LLMService
    for method in ("web_search", "generate_vision", "make_multimodal_message",
                   "generate_image", "generate_music", "text_to_speech",
                   "transcribe", "analyze_audio"):
        assert not hasattr(LLMService, method), f"LLMService still has {method}"


def test_llm_service_has_no_provider_config():
    """LLMService should not accept provider_config parameter."""
    import inspect
    from lingtai_kernel.llm.service import LLMService
    sig = inspect.signature(LLMService.__init__)
    assert "provider_config" not in sig.parameters
