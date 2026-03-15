"""Listen capability — speech transcription and audio analysis via LLM adapter."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..agent import BaseAgent
    from ..llm.service import LLMService

SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "audio_path": {
            "type": "string",
            "description": "Path to the audio file",
        },
        "mode": {
            "type": "string",
            "enum": ["transcribe", "analyze"],
            "description": "Transcribe speech or analyze audio content",
            "default": "transcribe",
        },
        "prompt": {
            "type": "string",
            "description": "Question about the audio (for analyze mode)",
        },
    },
    "required": ["audio_path"],
}

DESCRIPTION = "Transcribe speech or analyze audio content from a file."


class ListenManager:
    def __init__(self, *, working_dir: Path, llm_service: "LLMService") -> None:
        self._working_dir = working_dir
        self._service = llm_service

    def handle(self, args: dict) -> dict:
        audio_path = args.get("audio_path")
        if not audio_path:
            return {"status": "error", "message": "Missing required parameter: audio_path"}

        path = Path(audio_path)
        if not path.is_absolute():
            path = self._working_dir / path

        if not path.is_file():
            return {"status": "error", "message": f"Audio file not found: {path}"}

        audio_bytes = path.read_bytes()
        mode = args.get("mode", "transcribe")

        try:
            if mode == "analyze":
                prompt = args.get("prompt", "Describe this audio.")
                text = self._service.analyze_audio(audio_bytes, prompt)
            else:
                text = self._service.transcribe(audio_bytes)
        except RuntimeError as exc:
            return {"status": "error", "message": str(exc)}
        except NotImplementedError:
            return {"status": "error", "message": f"Provider does not support audio {mode}"}

        return {"status": "ok", "text": text}


def setup(agent: "BaseAgent", **kwargs: Any) -> ListenManager:
    """Set up the listen capability on an agent."""
    mgr = ListenManager(working_dir=agent.working_dir, llm_service=agent.service)
    agent.add_tool("listen", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt(
        "listen_instructions",
        "You can transcribe speech or analyze audio via the listen tool. "
        "Provide the audio file path. Use mode='transcribe' for speech-to-text "
        "or mode='analyze' with a prompt to ask questions about the audio.",
    )
    return mgr
