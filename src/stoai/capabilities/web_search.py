"""Web search capability — web lookup via LLM or SearchService.

Adds the ability to search the web. Uses SearchService if provided,
otherwise falls back to the LLM's grounding/search endpoint.

Usage:
    agent.add_capability("web_search")  # uses LLM fallback
    agent.add_capability("web_search", search_service=my_svc)  # uses dedicated service
"""
from __future__ import annotations

from typing import TYPE_CHECKING, Any

from ..i18n import t

if TYPE_CHECKING:
    from stoai_kernel.base_agent import BaseAgent

def get_description(lang: str = "en") -> str:
    return t(lang, "web_search.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "query": {"type": "string", "description": t(lang, "web_search.query")},
        },
        "required": ["query"],
    }


# Backward compat
SCHEMA = get_schema("en")
DESCRIPTION = get_description("en")


class WebSearchManager:
    """Handles web_search tool calls."""

    def __init__(
        self,
        agent: "BaseAgent",
        search_service: Any | None = None,
        web_search_provider: str | None = None,
    ) -> None:
        self._agent = agent
        self._search_service = search_service
        self._web_search_provider = web_search_provider

    def handle(self, args: dict) -> dict:
        query = args.get("query")
        if not query:
            return {"status": "error", "message": "Missing required parameter: query"}

        # Try SearchService first
        if self._search_service is not None:
            try:
                results = self._search_service.search(query)
                formatted = "\n\n".join(
                    f"**{r.title}**\n{r.url}\n{r.snippet}" for r in results
                )
                return {"status": "ok", "results": formatted or "No results found."}
            except NotImplementedError:
                pass  # Fall through to direct LLM call

        # Fall back to direct adapter call
        provider = self._web_search_provider or self._agent.service.provider
        if provider is None:
            return {
                "status": "error",
                "message": "Web search provider not configured. Pass provider='...' in capability kwargs.",
            }
        try:
            adapter = self._agent.service.get_adapter(provider)
        except RuntimeError:
            return {
                "status": "error",
                "message": f"Web search provider {provider!r} not available.",
            }
        defaults = self._agent.service._get_provider_defaults(provider)
        model = defaults.get("model", "") if defaults else ""
        resp = adapter.web_search(query, model=model)
        if not resp.text:
            return {
                "status": "error",
                "message": "Web search returned no results.",
            }
        return {"status": "ok", "results": resp.text}


def setup(agent: "BaseAgent", search_service: Any | None = None,
          provider: str | None = None, **kwargs: Any) -> WebSearchManager:
    """Set up the web_search capability on an agent."""
    lang = agent._config.language
    mgr = WebSearchManager(agent, search_service=search_service, web_search_provider=provider)
    agent.add_tool("web_search", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang))
    return mgr
