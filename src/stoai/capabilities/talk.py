"""Talk capability — text-to-speech via MiniMax MCP."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from ..logging import get_logger

if TYPE_CHECKING:
    from ..base_agent import BaseAgent
    from ..services.mcp import MCPClient

logger = get_logger()

SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "text": {
            "type": "string",
            "description": "Text to convert to speech",
        },
        "voice_id": {
            "type": "string",
            "description": "Voice to use (optional). Use list_voices to see options.",
        },
        "emotion": {
            "type": "string",
            "description": "Emotion for the speech (e.g. 'happy', 'sad', 'neutral'). Default: 'happy'",
        },
        "speed": {
            "type": "number",
            "description": "Speech speed multiplier (e.g. 0.5 for slow, 2.0 for fast). Default: 1.0",
        },
    },
    "required": ["text"],
}

DESCRIPTION = (
    "Convert text to speech audio. "
    "Produces natural-sounding speech in multiple voices and emotions. "
    "Output: MP3 file saved to media/audio/ in your working directory. "
    "Supports voice selection, emotion control (happy, sad, neutral), "
    "and speed adjustment. Use for narration, announcements, or "
    "giving your output a voice."
)


class TalkManager:
    """Manages text-to-speech via MiniMax MCP."""

    def __init__(self, *, working_dir: Path, mcp_client: "MCPClient") -> None:
        self._working_dir = working_dir
        self._mcp = mcp_client

    def handle(self, args: dict) -> dict:
        text = args.get("text")
        if not text:
            return {"status": "error", "message": "Missing required parameter: text"}

        # Save to working_dir/media/audio/
        out_dir = self._working_dir / "media" / "audio"
        out_dir.mkdir(parents=True, exist_ok=True)

        mcp_args: dict[str, Any] = {
            "text": text,
            "output_directory": str(out_dir),
        }
        for key in ("voice_id", "emotion", "speed"):
            val = args.get(key)
            if val is not None:
                mcp_args[key] = val

        try:
            result = self._mcp.call_tool("text_to_audio", mcp_args)
        except Exception as exc:
            return {"status": "error", "message": f"MCP call failed: {exc}"}

        if isinstance(result, dict) and result.get("status") == "error":
            return {"status": "error", "message": result.get("message", "Unknown error")}

        # Check if MCP saved a file to output_directory
        audio_files = sorted(out_dir.glob("*.mp3")) + sorted(out_dir.glob("*.wav"))
        if audio_files:
            latest = audio_files[-1]
            return {"status": "ok", "file_path": str(latest)}

        # Fallback: MCP may have returned a URL in text
        result_text = _extract_text(result)
        url = _extract_url(result_text)
        if url:
            try:
                import hashlib
                import time

                import requests
                resp = requests.get(url, timeout=60)
                resp.raise_for_status()
                ts = int(time.time())
                short_hash = hashlib.md5(text.encode()).hexdigest()[:4]
                filename = f"talk_{ts}_{short_hash}.mp3"
                out_path = out_dir / filename
                out_path.write_bytes(resp.content)
                return {"status": "ok", "file_path": str(out_path)}
            except Exception as exc:
                return {"status": "error", "message": f"Failed to download audio: {exc}"}

        return {"status": "error", "message": f"Unexpected MCP response: {result_text}"}


def _extract_text(result: Any) -> str:
    """Extract text from an MCP call result."""
    if isinstance(result, dict):
        return result.get("text", str(result))
    return str(result)


def _extract_url(text: str) -> str | None:
    """Extract the first HTTP(S) URL from text."""
    import re
    match = re.search(r"https?://\S+", text)
    return match.group(0).rstrip("']") if match else None


def _auto_create_mcp_client(**kwargs: Any) -> "MCPClient":
    """Create a MiniMax media MCP client for this capability."""
    from ..llm.minimax.mcp_media_client import create_minimax_media_client
    return create_minimax_media_client(
        api_key=kwargs.get("api_key"),
        api_host=kwargs.get("api_host"),
    )


def setup(agent: "BaseAgent", **kwargs: Any) -> TalkManager:
    """Set up the talk capability on an agent.

    Accepts ``mcp_client`` kwarg for an explicit MCP client.
    If not provided, auto-creates one connected to the full ``minimax-mcp``
    server (requires ``MINIMAX_API_KEY`` env var).
    """
    mcp_client = kwargs.get("mcp_client")
    if mcp_client is None:
        mcp_client = _auto_create_mcp_client(**kwargs)
    mgr = TalkManager(working_dir=agent.working_dir, mcp_client=mcp_client)
    agent.add_tool("talk", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    return mgr
