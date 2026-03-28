"""Draw capability — text-to-image generation via ImageGenService."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from lingtai_kernel.logging import get_logger

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent
    from ..services.image_gen import ImageGenService

logger = get_logger()

PROVIDERS = {
    "providers": ["minimax", "gemini"],
    "default": None,
}

def get_description(lang: str = "en") -> str:
    return t(lang, "draw.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "prompt": {
                "type": "string",
                "description": t(lang, "draw.prompt"),
            },
            "aspect_ratio": {
                "type": "string",
                "description": t(lang, "draw.aspect_ratio"),
            },
        },
        "required": ["prompt"],
    }



class DrawManager:
    """Manages text-to-image generation via ImageGenService."""

    def __init__(self, *, working_dir: Path, image_gen_service: "ImageGenService") -> None:
        self._working_dir = working_dir
        self._image_gen_service = image_gen_service

    def handle(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"status": "error", "message": "Missing required parameter: prompt"}

        aspect_ratio = args.get("aspect_ratio")
        out_dir = self._working_dir / "media" / "images"

        try:
            path = self._image_gen_service.generate(
                prompt,
                aspect_ratio=aspect_ratio,
                output_dir=out_dir,
            )
            return {"status": "ok", "file_path": str(path)}
        except Exception as exc:
            return {"status": "error", "message": str(exc)}


def setup(agent: "BaseAgent", **kwargs: Any) -> DrawManager:
    """Set up the draw capability on an agent.

    Requires either ``image_gen_service`` or ``provider`` + ``api_key``.
    Raises ``ValueError`` if neither is provided.
    """
    image_gen_service = kwargs.get("image_gen_service")

    if image_gen_service is None:
        provider = kwargs.get("provider")
        if provider is None:
            raise ValueError(
                "draw capability requires 'image_gen_service' or 'provider'. "
                "Example: capabilities={'draw': {'provider': 'minimax', 'api_key': '...'}}"
            )
        from ..services.image_gen import create_image_gen_service
        from ._media_host import resolve_media_host
        image_gen_service = create_image_gen_service(
            provider,
            api_key=kwargs.get("api_key"),
            api_host=kwargs.get("api_host") or resolve_media_host(agent),
        )

    lang = agent._config.language
    mgr = DrawManager(working_dir=agent.working_dir, image_gen_service=image_gen_service)
    agent.add_tool("draw", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang))
    return mgr
