from stoai_kernel.logging import get_logger
from ..anthropic.adapter import AnthropicAdapter
from stoai_kernel.llm.base import ChatSession, LLMResponse

logger = get_logger()

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


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
    supports_web_search = True
    supports_vision = True

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

    def web_search(self, query: str, model: str) -> LLMResponse:
        def _do_search():
            try:
                from .mcp_client import get_minimax_mcp_client
                client = get_minimax_mcp_client()
                result = client.call_tool("web_search", {"query": query})
                if result.get("status") == "error":
                    logger.warning("MiniMax MCP web search error: %s", result.get("message"))
                    return LLMResponse(text="")
                text = result.get("text", "") or result.get("answer", "")
                if not text and "organic" in result:
                    parts = []
                    for item in result["organic"]:
                        title = item.get("title", "")
                        snippet = item.get("snippet", "")
                        link = item.get("link", "")
                        parts.append(f"**{title}**\n{snippet}\nSource: {link}")
                    text = "\n\n".join(parts)
                if not text:
                    text = str(result)
                return LLMResponse(text=text)
            except Exception as e:
                logger.warning("MiniMax MCP web search failed: %s", e)
                return LLMResponse(text="")
        return self._gated_call(_do_search)

    def generate_vision(
        self, question: str, image_bytes: bytes, *, model: str = "",
        mime_type: str = "image/png",
    ) -> LLMResponse:
        """Vision via MiniMax MCP understand_image tool."""
        import base64
        def _do_vision():
            try:
                from .mcp_client import get_minimax_mcp_client
                client = get_minimax_mcp_client()
                b64 = base64.b64encode(image_bytes).decode("ascii")
                result = client.call_tool("understand_image", {
                    "image_source": f"data:{mime_type};base64,{b64}",
                    "prompt": question,
                })
                if result.get("status") == "error":
                    logger.warning("MiniMax MCP vision error: %s", result.get("message"))
                    return LLMResponse(text="")
                return LLMResponse(text=result.get("text", ""))
            except Exception as e:
                logger.warning("MiniMax MCP vision failed: %s", e)
                return LLMResponse(text="")
        return self._gated_call(_do_vision)

