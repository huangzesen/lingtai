"""Talk capability — text-to-speech via TTSService."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from lingtai_kernel.logging import get_logger

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

    from ..services.tts import TTSService

logger = get_logger()

PROVIDERS = {
    "providers": ["minimax", "gemini"],
    "default": None,
}

def get_description(lang: str = "en") -> str:
    return t(lang, "talk.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "text": {
                "type": "string",
                "description": t(lang, "talk.text"),
            },
            "voice_id": {
                "type": "string",
                "description": t(lang, "talk.voice_id"),
            },
            "emotion": {
                "type": "string",
                "description": t(lang, "talk.emotion"),
            },
            "speed": {
                "type": "number",
                "description": t(lang, "talk.speed"),
            },
        },
        "required": ["text"],
    }



class TalkManager:
    """Manages text-to-speech via TTSService."""

    def __init__(self, *, working_dir: Path, tts_service: "TTSService") -> None:
        self._working_dir = working_dir
        self._tts_service = tts_service

    def handle(self, args: dict) -> dict:
        text = args.get("text")
        if not text:
            return {"status": "error", "message": "Missing required parameter: text"}

        # Save to working_dir/media/audio/
        out_dir = self._working_dir / "media" / "audio"

        # Collect provider-specific kwargs
        extra: dict[str, Any] = {}
        for key in ("emotion", "speed"):
            val = args.get(key)
            if val is not None:
                extra[key] = val

        voice = args.get("voice_id")

        try:
            file_path = self._tts_service.synthesize(
                text, voice=voice, output_dir=out_dir, **extra
            )
        except Exception as exc:
            return {"status": "error", "message": str(exc)}

        return {"status": "ok", "file_path": str(file_path)}


def setup(
    agent: "BaseAgent",
    *,
    provider: str | None = None,
    api_key: str | None = None,
    tts_service: "TTSService | None" = None,
    **kwargs: Any,
) -> TalkManager:
    """Set up the talk capability on an agent.

    Resolution order:
    1. ``tts_service`` passed directly -- use it.
    2. ``provider`` passed -- create via factory.
    3. Neither -- ``ValueError``.
    """
    if tts_service is None:
        if provider is None:
            raise ValueError(
                "talk capability requires either 'tts_service' or 'provider' "
                "(e.g. 'minimax', 'gemini'). Example: "
                "capabilities={'talk': {'provider': 'minimax', 'api_key': '...'}}"
            )
        from ..services.tts import create_tts_service
        from ._media_host import resolve_media_host

        if "api_host" not in kwargs:
            kwargs["api_host"] = resolve_media_host(agent)
        tts_service = create_tts_service(provider, api_key=api_key, **kwargs)

    lang = agent._config.language
    mgr = TalkManager(working_dir=agent.working_dir, tts_service=tts_service)
    agent.add_tool(
        "talk",
        schema=get_schema(lang),
        handler=mgr.handle,
        description=get_description(lang),
    )
    return mgr
