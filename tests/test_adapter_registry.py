"""Tests for LLMService adapter registry."""
from __future__ import annotations
from unittest.mock import MagicMock
from lingtai_kernel.llm.service import LLMService
from lingtai_kernel.llm.base import LLMAdapter


def _make_stub_adapter(**kwargs):
    """Factory that returns a MagicMock LLMAdapter."""
    adapter = MagicMock(spec=LLMAdapter)
    adapter._init_kwargs = kwargs
    return adapter


class TestAdapterRegistry:
    def setup_method(self):
        # Save and clear registry for isolation
        self._saved = dict(LLMService._adapter_registry)
        LLMService._adapter_registry.clear()

    def teardown_method(self):
        LLMService._adapter_registry.clear()
        LLMService._adapter_registry.update(self._saved)

    def test_register_and_lookup(self):
        LLMService.register_adapter("test_provider", _make_stub_adapter)
        assert "test_provider" in LLMService._adapter_registry

    def test_register_normalizes_case(self):
        LLMService.register_adapter("TestProvider", _make_stub_adapter)
        assert "testprovider" in LLMService._adapter_registry

    def test_create_adapter_uses_registry(self):
        LLMService.register_adapter("myprovider", _make_stub_adapter)
        svc = LLMService(
            "myprovider", "my-model",
            api_key="test-key",
        )
        # The adapter should have been created via our factory
        adapter = svc.get_adapter("myprovider")
        assert adapter._init_kwargs["api_key"] == "test-key"

    def test_create_adapter_unknown_provider_raises(self):
        import pytest
        with pytest.raises(RuntimeError, match="No adapter registered"):
            LLMService("unknown_provider", "model", api_key="key")

    def test_register_overwrites(self):
        factory_a = MagicMock(return_value=MagicMock(spec=LLMAdapter))
        factory_b = MagicMock(return_value=MagicMock(spec=LLMAdapter))
        LLMService.register_adapter("prov", factory_a)
        LLMService.register_adapter("prov", factory_b)
        LLMService("prov", "model", api_key="key")
        factory_b.assert_called_once()
        factory_a.assert_not_called()


def test_default_adapters_registered():
    """All default adapters should be registered after importing lingtai.llm."""
    from lingtai.llm._register import register_all_adapters
    # Clear and re-register
    saved = dict(LLMService._adapter_registry)
    LLMService._adapter_registry.clear()
    register_all_adapters()
    expected = {"gemini", "anthropic", "openai", "minimax", "deepseek", "grok", "qwen", "glm", "kimi", "custom"}
    assert expected.issubset(set(LLMService._adapter_registry.keys()))
    LLMService._adapter_registry.clear()
    LLMService._adapter_registry.update(saved)
