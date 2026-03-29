"""DuckDuckGo web search — zero-API-key search via the ddgs package."""
from __future__ import annotations

from . import SearchResult, SearchService


class DuckDuckGoSearchService(SearchService):
    """Zero-API-key web search via DuckDuckGo.

    Uses the ``ddgs`` package to scrape DuckDuckGo results.
    Install with ``pip install lingtai[duckduckgo]`` or
    ``pip install ddgs``.
    """

    def search(self, query: str, max_results: int = 5) -> list[SearchResult]:
        from ddgs import DDGS  # type: ignore[import-untyped]
        raw = DDGS().text(query, max_results=max_results)
        return [
            SearchResult(
                title=r.get("title", ""),
                url=r.get("href", ""),
                snippet=r.get("body", ""),
            )
            for r in raw
        ]
