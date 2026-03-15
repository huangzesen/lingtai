"""Talk capability — text-to-speech via LLM adapter."""
from __future__ import annotations

import hashlib
import time
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..agent import BaseAgent
    from ..llm.service import LLMService

SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "text": {
            "type": "string",
            "description": "Text to convert to speech",
        },
    },
    "required": ["text"],
}

DESCRIPTION = "Convert text to speech audio."


class TalkManager:
    def __init__(self, *, working_dir: Path, llm_service: "LLMService") -> None:
        self._working_dir = working_dir
        self._service = llm_service

    def handle(self, args: dict) -> dict:
        text = args.get("text")
        if not text:
            return {"status": "error", "message": "Missing required parameter: text"}

        try:
            audio_bytes = self._service.text_to_speech(text)
        except RuntimeError as exc:
            return {"status": "error", "message": str(exc)}
        except NotImplementedError:
            return {"status": "error", "message": "Provider does not support text-to-speech"}

        if not audio_bytes:
            return {"status": "error", "message": "Text-to-speech returned empty result"}

        out_dir = self._working_dir / "media" / "audio"
        out_dir.mkdir(parents=True, exist_ok=True)
        ts = int(time.time())
        short_hash = hashlib.md5(text.encode()).hexdigest()[:4]
        filename = f"talk_{ts}_{short_hash}.mp3"
        out_path = out_dir / filename
        out_path.write_bytes(audio_bytes)
        return {"status": "ok", "file_path": str(out_path)}


def setup(agent: "BaseAgent", **kwargs: Any) -> TalkManager:
    """Set up the talk capability on an agent."""
    mgr = TalkManager(working_dir=agent.working_dir, llm_service=agent.service)
    agent.add_tool("talk", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt(
        "talk_instructions",
        "You can convert text to speech via the talk tool. "
        "Provide the text you want spoken. "
        "Generated audio is saved to media/audio/ in your working directory.",
    )
    return mgr
