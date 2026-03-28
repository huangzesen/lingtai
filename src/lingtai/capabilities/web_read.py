"""Web read capability — fetch and extract readable content from URLs.

Uses WebReadService if provided, otherwise auto-creates TrafilaturaWebReadService.

Usage:
    agent.add_capability("web_read")  # uses trafilatura (default)
    agent.add_capability("web_read", web_read_service=my_svc)  # uses custom service
"""
from __future__ import annotations

from typing import TYPE_CHECKING, Any

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

PROVIDERS = {"providers": [], "default": "builtin"}


def get_description(lang: str = "en") -> str:
    return t(lang, "web_read.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "url": {"type": "string", "description": t(lang, "web_read.url")},
            "output_format": {
                "type": "string",
                "enum": ["text", "markdown"],
                "description": t(lang, "web_read.output_format"),
            },
        },
        "required": ["url"],
    }



class WebReadManager:
    """Handles web_read tool calls."""

    def __init__(
        self,
        web_read_service: Any | None = None,
    ) -> None:
        self._service = web_read_service

    def handle(self, args: dict) -> dict:
        url = args.get("url")
        if not url:
            return {"status": "error", "message": "Missing required parameter: url"}

        output_format = args.get("output_format", "markdown")

        # Auto-create default service if none provided
        if self._service is None:
            from ..services.web_read import TrafilaturaWebReadService
            self._service = TrafilaturaWebReadService()

        try:
            result = self._service.read(url, output_format=output_format)
            return {"status": "ok", "url": url, "content": result.content, "title": result.title}
        except ImportError as e:
            return {"status": "error", "message": str(e)}
        except RuntimeError as e:
            return {"status": "error", "message": str(e)}


def setup(agent: "BaseAgent", web_read_service: Any | None = None,
          **kwargs: Any) -> WebReadManager:
    """Set up the web_read capability on an agent."""
    lang = agent._config.language
    mgr = WebReadManager(web_read_service=web_read_service)
    agent.add_tool("web_read", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang))
    return mgr
