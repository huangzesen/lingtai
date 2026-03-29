"""Anthropic web search — uses Claude's native web_search tool."""
from __future__ import annotations

from lingtai_kernel.logging import get_logger

from . import SearchResult, SearchService

logger = get_logger()


class AnthropicSearchService(SearchService):
    """Web search via Anthropic's native web_search_20250305 tool.

    Sends a one-shot request with the web search tool enabled and
    extracts text from the response.

    Args:
        api_key: Anthropic API key.
        model: Model to use (default ``claude-sonnet-4-20250514``).
    """

    def __init__(self, api_key: str, *, model: str = "claude-sonnet-4-20250514") -> None:
        self._api_key = api_key
        self._model = model

    def search(self, query: str, max_results: int = 5) -> list[SearchResult]:
        import anthropic

        client = anthropic.Anthropic(api_key=self._api_key)
        try:
            raw = client.messages.create(
                model=self._model,
                max_tokens=4096,
                messages=[{"role": "user", "content": query}],
                tools=[
                    {
                        "type": "web_search_20250305",
                        "name": "web_search",
                        "max_uses": max_results,
                    }
                ],
            )
        except Exception as e:
            logger.warning("Anthropic web search failed: %s", e)
            return []

        # Extract text blocks as a single result
        text_parts: list[str] = []
        for block in raw.content:
            if block.type == "text":
                text_parts.append(block.text)

        if not text_parts:
            return []

        return [
            SearchResult(
                title="Anthropic Web Search",
                url="",
                snippet="\n".join(text_parts),
            )
        ]
