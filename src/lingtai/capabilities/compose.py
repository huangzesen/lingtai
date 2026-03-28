"""Compose capability — music generation via MusicGenService."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from lingtai_kernel.logging import get_logger

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent
    from ..services.music_gen import MusicGenService

logger = get_logger()

PROVIDERS = {
    "providers": ["minimax"],
    "default": None,
}

def get_description(lang: str = "en") -> str:
    return t(lang, "compose.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "prompt": {
                "type": "string",
                "description": t(lang, "compose.prompt"),
            },
            "lyrics": {
                "type": "string",
                "description": t(lang, "compose.lyrics"),
            },
        },
        "required": ["prompt", "lyrics"],
    }



class ComposeManager:
    """Manages music generation via MusicGenService."""

    def __init__(
        self,
        *,
        working_dir: Path,
        music_gen_service: "MusicGenService",
    ) -> None:
        self._working_dir = working_dir
        self._service = music_gen_service

    def handle(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"status": "error", "message": "Missing required parameter: prompt"}

        lyrics = args.get("lyrics")
        if lyrics is None:
            return {"status": "error", "message": "Missing required parameter: lyrics"}

        out_dir = self._working_dir / "media" / "music"

        try:
            path = self._service.generate(
                prompt, lyrics=lyrics, output_dir=out_dir,
            )
            return {"status": "ok", "file_path": str(path)}
        except Exception as exc:
            return {"status": "error", "message": str(exc)}


def setup(agent: "BaseAgent", **kwargs: Any) -> ComposeManager:
    """Set up the compose capability on an agent.

    Requires either ``music_gen_service`` or ``provider`` + ``api_key``.
    Raises ``ValueError`` if neither is provided.
    """
    music_gen_service: MusicGenService | None = kwargs.get("music_gen_service")

    if music_gen_service is None:
        provider = kwargs.get("provider")
        if provider is None:
            raise ValueError(
                "compose capability requires 'music_gen_service' or 'provider'. "
                "Example: capabilities={'compose': {'provider': 'minimax', 'api_key': '...'}}"
            )
        from ..services.music_gen import create_music_gen_service
        from ._media_host import resolve_media_host
        music_gen_service = create_music_gen_service(
            provider,
            api_key=kwargs.get("api_key"),
            api_host=kwargs.get("api_host") or resolve_media_host(agent),
        )

    lang = agent._config.language
    mgr = ComposeManager(
        working_dir=agent.working_dir,
        music_gen_service=music_gen_service,
    )
    agent.add_tool(
        "compose",
        schema=get_schema(lang),
        handler=mgr.handle,
        description=get_description(lang),
    )
    return mgr
