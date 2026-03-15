"""Compose capability — music generation via LLM adapter."""
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
        "prompt": {
            "type": "string",
            "description": "Description of the music to generate",
        },
        "duration_seconds": {
            "type": "number",
            "description": "Desired duration in seconds",
        },
    },
    "required": ["prompt"],
}

DESCRIPTION = "Generate music from a text description."


class ComposeManager:
    def __init__(self, *, working_dir: Path, llm_service: "LLMService") -> None:
        self._working_dir = working_dir
        self._service = llm_service

    def handle(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"status": "error", "message": "Missing required parameter: prompt"}

        duration = args.get("duration_seconds")

        try:
            audio_bytes = self._service.generate_music(prompt, duration_seconds=duration)
        except RuntimeError as exc:
            return {"status": "error", "message": str(exc)}
        except NotImplementedError:
            return {"status": "error", "message": "Provider does not support music generation"}

        if not audio_bytes:
            return {"status": "error", "message": "Music generation returned empty result"}

        out_dir = self._working_dir / "media" / "music"
        out_dir.mkdir(parents=True, exist_ok=True)
        ts = int(time.time())
        short_hash = hashlib.md5(prompt.encode()).hexdigest()[:4]
        filename = f"compose_{ts}_{short_hash}.mp3"
        out_path = out_dir / filename
        out_path.write_bytes(audio_bytes)
        return {"status": "ok", "file_path": str(out_path)}


def setup(agent: "BaseAgent", **kwargs: Any) -> ComposeManager:
    """Set up the compose capability on an agent."""
    mgr = ComposeManager(working_dir=agent.working_dir, llm_service=agent.service)
    agent.add_tool("compose", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt(
        "compose_instructions",
        "You can generate music via the compose tool. "
        "Provide a text prompt describing the music you want. "
        "Optionally specify duration_seconds. "
        "Generated music is saved to media/music/ in your working directory.",
    )
    return mgr
