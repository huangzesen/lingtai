from lingtai_kernel.logging import get_logger
from ..anthropic.adapter import AnthropicAdapter
from lingtai_kernel.llm.base import ChatSession, LLMResponse

logger = get_logger()


class _GatedSession:
    """Thin proxy that routes send/send_stream through the adapter's gate."""

    def __init__(self, inner: ChatSession, gate):
        self._inner = inner
        self._gate = gate

    @property
    def interface(self):
        return self._inner.interface

    def send(self, message):
        if self._gate is not None:
            return self._gate.submit(lambda: self._inner.send(message))
        return self._inner.send(message)

    def send_stream(self, message, on_chunk=None):
        if self._gate is not None:
            return self._gate.submit(lambda: self._inner.send_stream(message, on_chunk=on_chunk))
        return self._inner.send_stream(message, on_chunk=on_chunk)

    def __getattr__(self, name):
        return getattr(self._inner, name)


class MiniMaxAdapter(AnthropicAdapter):

    def __init__(
        self, api_key: str, *, base_url: str | None = None,
        max_rpm: int = 120, timeout_ms: int = 300_000,
    ):
        effective_url = base_url or "https://api.minimaxi.com/anthropic"
        super().__init__(api_key=api_key, base_url=effective_url, timeout_ms=timeout_ms)
        self._setup_gate(max_rpm)

    def create_chat(self, *args, **kwargs):
        session = super().create_chat(*args, **kwargs)
        if self._gate is not None:
            return _GatedSession(session, self._gate)
        return session

    def generate(self, *args, **kwargs) -> LLMResponse:
        return self._gated_call(lambda: super(MiniMaxAdapter, self).generate(*args, **kwargs))
