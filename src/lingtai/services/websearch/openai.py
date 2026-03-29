"""OpenAI web search — uses OpenAI's native search API."""
from __future__ import annotations

from lingtai_kernel.logging import get_logger

from . import SearchResult, SearchService

logger = get_logger()


class OpenAISearchService(SearchService):
    """Web search via OpenAI's gpt-4o-search-preview model.

    Sends a one-shot chat completion with ``web_search_options`` enabled.

    Args:
        api_key: OpenAI API key.
        model: Model to use (default ``gpt-4o-search-preview``).
    """

    def __init__(self, api_key: str, *, model: str = "gpt-4o-search-preview") -> None:
        self._api_key = api_key
        self._model = model

    def search(self, query: str, max_results: int = 5) -> list[SearchResult]:
        import openai as openai_sdk

        client = openai_sdk.OpenAI(api_key=self._api_key)
        try:
            raw = client.chat.completions.create(
                model=self._model,
                web_search_options={},
                messages=[{"role": "user", "content": query}],
            )
        except Exception as e:
            logger.warning("OpenAI web search failed: %s", e)
            return []

        if not raw.choices:
            return []

        text = raw.choices[0].message.content or ""
        if not text:
            return []

        return [
            SearchResult(
                title="OpenAI Web Search",
                url="",
                snippet=text,
            )
        ]
