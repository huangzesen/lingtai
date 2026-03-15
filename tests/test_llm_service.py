"""Tests for stoai.llm.service — model registry and context limits."""

from stoai.llm.service import get_context_limit, DEFAULT_CONTEXT_WINDOW


def test_get_context_limit_unknown():
    """Unknown models should return default 256k."""
    limit = get_context_limit("totally-unknown-model-xyz")
    assert limit == DEFAULT_CONTEXT_WINDOW


def test_get_context_limit_empty():
    """Empty model name returns default 256k."""
    assert get_context_limit("") == DEFAULT_CONTEXT_WINDOW


def test_adapter_generate_image_raises_not_implemented():
    """Base LLMAdapter media methods raise NotImplementedError."""
    from stoai.llm.base import LLMAdapter, LLMResponse
    from unittest.mock import MagicMock
    import pytest
    # Create a minimal concrete subclass (all abstract methods must be overridden)
    class StubAdapter(LLMAdapter):
        def create_chat(self, *a, **kw): pass
        def generate(self, *a, **kw): return LLMResponse()
        def make_tool_result_message(self, *a, **kw): pass
        def make_multimodal_message(self, *a, **kw): pass
        def is_quota_error(self, *a, **kw): return False
    adapter = StubAdapter()
    with pytest.raises(NotImplementedError):
        adapter.generate_image("a cat", model="test")
    with pytest.raises(NotImplementedError):
        adapter.generate_music("jazz", model="test")
    with pytest.raises(NotImplementedError):
        adapter.text_to_speech("hello", model="test")
    with pytest.raises(NotImplementedError):
        adapter.transcribe(b"audio", model="test")
    with pytest.raises(NotImplementedError):
        adapter.analyze_audio(b"audio", "what is this?", model="test")


def test_generate_image_no_provider():
    """generate_image raises RuntimeError when image_provider not configured."""
    from stoai.llm.service import LLMService
    from unittest.mock import MagicMock, patch
    import pytest
    with patch.object(LLMService, '_create_adapter', return_value=MagicMock()):
        svc = LLMService("gemini", "gemini-test", key_resolver=lambda p: "key", provider_defaults={})
    with pytest.raises(RuntimeError, match="image_provider"):
        svc.generate_image("a cat")


def test_generate_image_routes_to_adapter():
    """generate_image routes to the configured adapter."""
    from stoai.llm.service import LLMService
    from unittest.mock import MagicMock, patch
    adapter = MagicMock()
    adapter.generate_image.return_value = b"PNG_BYTES"
    with patch.object(LLMService, '_create_adapter', return_value=MagicMock()):
        svc = LLMService(
            "gemini", "gemini-test",
            key_resolver=lambda p: "key",
            provider_defaults={"minimax": {"model": "mm-img"}},
        )
    svc._config["image_provider"] = "minimax"
    svc._adapters[("minimax", None)] = adapter
    result = svc.generate_image("a cat")
    assert result == b"PNG_BYTES"
    adapter.generate_image.assert_called_once_with("a cat", model="mm-img")


def test_text_to_speech_no_provider():
    from stoai.llm.service import LLMService
    from unittest.mock import MagicMock, patch
    import pytest
    with patch.object(LLMService, '_create_adapter', return_value=MagicMock()):
        svc = LLMService("gemini", "gemini-test", key_resolver=lambda p: "key", provider_defaults={})
    with pytest.raises(RuntimeError, match="tts_provider"):
        svc.text_to_speech("hello")


def test_transcribe_no_provider():
    from stoai.llm.service import LLMService
    from unittest.mock import MagicMock, patch
    import pytest
    with patch.object(LLMService, '_create_adapter', return_value=MagicMock()):
        svc = LLMService("gemini", "gemini-test", key_resolver=lambda p: "key", provider_defaults={})
    with pytest.raises(RuntimeError, match="audio_provider"):
        svc.transcribe(b"audio")
