"""Vision capability — image understanding via VisionService.

Adds the ability to analyze images. Requires a VisionService instance,
created either explicitly or via the ``provider``/``api_key`` factory.

Usage:
    agent.add_capability("vision", vision_service=my_svc)
    agent.add_capability("vision", provider="anthropic", api_key="sk-...")
    agent.add_capability("vision", provider="local")  # mlx-vlm, no API key
    agent.add_capability("vision", provider="local", model="mlx-community/Qwen2-VL-2B-Instruct-4bit")
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from ..i18n import t
from ..services.vision import VisionService, create_vision_service

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

PROVIDERS = {
    "providers": ["minimax", "gemini", "anthropic", "openai", "local"],
    "default": None,
}

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



class VisionManager:
    """Handles vision tool calls via a VisionService."""

    def __init__(
        self,
        agent: "BaseAgent",
        vision_service: VisionService,
    ) -> None:
        self._agent = agent
        self._vision_service = vision_service

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

        try:
            analysis = self._vision_service.analyze_image(str(path), prompt=question)
            if not analysis:
                return {
                    "status": "error",
                    "message": "Vision analysis returned no response.",
                }
            return {"status": "ok", "analysis": analysis}
        except Exception as e:
            return {"status": "error", "message": f"Vision analysis failed: {e}"}


def setup(
    agent: "BaseAgent",
    vision_service: VisionService | None = None,
    provider: str | None = None,
    api_key: str | None = None,
    **kwargs: Any,
) -> VisionManager:
    """Set up the vision capability on an agent.

    Requires either ``vision_service`` or ``provider`` + ``api_key``.
    Raises ``ValueError`` if neither is provided.
    """
    if vision_service is None and provider is not None:
        from ._media_host import resolve_media_host
        if "api_host" not in kwargs:
            kwargs["api_host"] = resolve_media_host(agent)
        vision_service = create_vision_service(provider, api_key=api_key, **kwargs)
    elif vision_service is None:
        raise ValueError(
            "vision capability requires 'vision_service' or 'provider' + 'api_key'. "
            "Example: capabilities={'vision': {'provider': 'gemini', 'api_key': '...'}}"
        )

    lang = agent._config.language
    mgr = VisionManager(agent, vision_service=vision_service)
    agent.add_tool("vision", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang))
    return mgr
