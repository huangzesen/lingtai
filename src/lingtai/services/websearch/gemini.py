"""Gemini web search — uses Google Search grounding via google-genai."""
from __future__ import annotations

from lingtai_kernel.logging import get_logger

from . import SearchResult, SearchService

logger = get_logger()


class GeminiSearchService(SearchService):
    """Web search via Gemini's Google Search grounding.

    Sends a one-shot generate_content call with the Google Search tool
    enabled and extracts text from the response.

    Args:
        api_key: Google AI / Gemini API key.
        model: Model to use (default ``gemini-3-flash-preview``).
    """

    def __init__(self, api_key: str, *, model: str = "gemini-3-flash-preview") -> None:
        self._api_key = api_key
        self._model = model

    def search(self, query: str, max_results: int = 5) -> list[SearchResult]:
        from google import genai
        from google.genai import types

        client = genai.Client(api_key=self._api_key)
        try:
            gen_config = types.GenerateContentConfig(
                tools=[types.Tool(google_search=types.GoogleSearch())],
            )
            raw = client.models.generate_content(
                model=self._model,
                contents=query,
                config=gen_config,
            )
        except Exception as e:
            logger.warning("Gemini web search failed: %s", e)
            return []

        # Extract text from response candidates
        text_parts: list[str] = []
        candidates = getattr(raw, "candidates", None) or []
        if candidates:
            content = candidates[0].content
            if content and content.parts:
                for part in content.parts:
                    if hasattr(part, "text") and part.text and not getattr(part, "thought", False):
                        text_parts.append(part.text)

        if not text_parts:
            return []

        return [
            SearchResult(
                title="Gemini Web Search",
                url="",
                snippet="\n".join(text_parts),
            )
        ]
