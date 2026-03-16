"""Web search capability — web lookup via LLM or SearchService.

Adds the ability to search the web. Uses SearchService if provided,
otherwise falls back to the LLM's grounding/search endpoint.

Usage:
    agent.add_capability("web_search")  # uses LLM fallback
    agent.add_capability("web_search", search_service=my_svc)  # uses dedicated service
"""
from __future__ import annotations

from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "query": {"type": "string", "description": "Search query"},
    },
    "required": ["query"],
}

DESCRIPTION = (
    "Search the web for current information. "
    "Use for real-time data, recent events, documentation, "
    "or anything beyond your training knowledge. "
    "Returns ranked search results with titles, URLs, and snippets."
)


class WebSearchManager:
    """Handles web_search tool calls."""

    def __init__(self, agent: "BaseAgent", search_service: Any | None = None) -> None:
        self._agent = agent
        self._search_service = search_service

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

        # Fall back to direct LLM grounding call
        resp = self._agent.service.web_search(query)
        if not resp.text:
            return {
                "status": "error",
                "message": "Web search returned no results. The web search provider may not be configured.",
            }
        return {"status": "ok", "results": resp.text}


def setup(agent: "BaseAgent", search_service: Any | None = None, **kwargs: Any) -> WebSearchManager:
    """Set up the web_search capability on an agent."""
    mgr = WebSearchManager(agent, search_service=search_service)
    agent.add_tool("web_search", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    return mgr
