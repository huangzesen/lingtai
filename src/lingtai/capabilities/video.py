"""Video capability — video generation via VideoGenService."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from lingtai_kernel.logging import get_logger

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent
    from ..services.video_gen import VideoGenService

logger = get_logger()

PROVIDERS = {
    "providers": ["minimax"],
    "default": None,
}


def get_description(lang: str = "en") -> str:
    return t(lang, "video.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "prompt": {
                "type": "string",
                "description": t(lang, "video.prompt"),
            },
            "first_frame_image": {
                "type": "string",
                "description": t(lang, "video.first_frame_image"),
            },
            "model": {
                "type": "string",
                "description": t(lang, "video.model"),
            },
            "duration": {
                "type": "integer",
                "description": t(lang, "video.duration"),
            },
            "resolution": {
                "type": "string",
                "description": t(lang, "video.resolution"),
            },
        },
        "required": ["prompt"],
    }


class VideoManager:
    """Manages video generation via VideoGenService."""

    def __init__(self, *, working_dir: Path, video_gen_service: "VideoGenService") -> None:
        self._working_dir = working_dir
        self._service = video_gen_service

    def handle(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"status": "error", "message": "Missing required parameter: prompt"}

        first_frame_image = args.get("first_frame_image")
        model = args.get("model")
        duration = args.get("duration")
        resolution = args.get("resolution")
        out_dir = self._working_dir / "media" / "videos"

        try:
            path = self._service.generate(
                prompt,
                first_frame_image=first_frame_image,
                model=model,
                duration=duration,
                resolution=resolution,
                output_dir=out_dir,
            )
            return {"status": "ok", "file_path": str(path)}
        except Exception as exc:
            return {"status": "error", "message": str(exc)}


def setup(agent: "BaseAgent", **kwargs: Any) -> VideoManager:
    """Set up the video capability on an agent.

    Requires either ``video_gen_service`` or ``provider`` + ``api_key``.
    Raises ``ValueError`` if neither is provided.
    """
    video_gen_service: VideoGenService | None = kwargs.get("video_gen_service")

    if video_gen_service is None:
        provider = kwargs.get("provider")
        if provider is None:
            raise ValueError(
                "video capability requires 'video_gen_service' or 'provider'. "
                "Example: capabilities={'video': {'provider': 'minimax', 'api_key': '...'}}"
            )
        from ..services.video_gen import create_video_gen_service
        from ._media_host import resolve_media_host
        video_gen_service = create_video_gen_service(
            provider,
            api_key=kwargs.get("api_key"),
            api_host=kwargs.get("api_host") or resolve_media_host(agent),
        )

    lang = agent._config.language
    mgr = VideoManager(working_dir=agent.working_dir, video_gen_service=video_gen_service)
    agent.add_tool(
        "video",
        schema=get_schema(lang),
        handler=mgr.handle,
        description=get_description(lang),
    )
    return mgr
