"""Vision capability — image understanding via LLM or VisionService.

Adds the ability to analyze images. Uses VisionService if provided,
otherwise falls back to the LLM's multimodal vision endpoint.

Usage:
    agent.add_capability("vision")  # uses LLM fallback
    agent.add_capability("vision", vision_service=my_svc)  # uses dedicated service
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

def get_description(lang: str = "en") -> str:
    return t(lang, "vision.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "image_path": {"type": "string", "description": t(lang, "vision.image_path")},
            "question": {
                "type": "string",
                "description": t(lang, "vision.question"),
                "default": "Describe this image.",
            },
        },
        "required": ["image_path"],
    }


# Backward compat
SCHEMA = get_schema("en")
DESCRIPTION = get_description("en")

_MIME_BY_EXT: dict[str, str] = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".webp": "image/webp",
}


class VisionManager:
    """Handles vision tool calls."""

    def __init__(
        self,
        agent: "BaseAgent",
        vision_service: Any | None = None,
        vision_provider: str | None = None,
    ) -> None:
        self._agent = agent
        self._vision_service = vision_service
        self._vision_provider = vision_provider

    def handle(self, args: dict) -> dict:
        image_path = args.get("image_path", "")
        question = args.get("question", "Describe what you see in this image.")

        if not image_path:
            return {"status": "error", "message": "Provide image_path"}

        path = Path(image_path)
        if not path.is_absolute():
            path = self._agent._working_dir / path

        if not path.is_file():
            return {"status": "error", "message": f"Image file not found: {path}"}

        # Try VisionService first
        if self._vision_service is not None:
            try:
                analysis = self._vision_service.analyze_image(str(path), prompt=question)
                return {"status": "ok", "analysis": analysis}
            except NotImplementedError:
                pass  # Fall through to direct LLM call

        # Fall back to direct adapter call
        provider = self._vision_provider or self._agent.service.provider
        if provider is None:
            return {
                "status": "error",
                "message": "Vision provider not configured. Pass provider='...' in capability kwargs.",
            }
        try:
            adapter = self._agent.service.get_adapter(provider)
        except RuntimeError:
            return {
                "status": "error",
                "message": f"Vision provider {provider!r} not available.",
            }
        image_bytes = path.read_bytes()
        mime = _MIME_BY_EXT.get(path.suffix.lower(), "image/png")
        defaults = self._agent.service._get_provider_defaults(provider)
        model = defaults.get("model", "") if defaults else ""
        response = adapter.generate_vision(question, image_bytes, model=model, mime_type=mime)
        if not response.text:
            return {
                "status": "error",
                "message": "Vision analysis returned no response.",
            }
        return {"status": "ok", "analysis": response.text}


def setup(agent: "BaseAgent", vision_service: Any | None = None,
          provider: str | None = None, **kwargs: Any) -> VisionManager:
    """Set up the vision capability on an agent."""
    lang = agent._config.language
    mgr = VisionManager(agent, vision_service=vision_service, vision_provider=provider)
    agent.add_tool("vision", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang))
    return mgr
