"""Compose capability — music generation via MiniMax MCP."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from ..logging import get_logger

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

logger = get_logger()

SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "prompt": {
            "type": "string",
            "description": "Description of the music style, mood, and genre to generate",
        },
        "lyrics": {
            "type": "string",
            "description": (
                "Lyrics for the song. Required by the music model. "
                "Use empty string or instrumental placeholders like "
                "'La la la' for instrumental-style tracks."
            ),
        },
    },
    "required": ["prompt", "lyrics"],
}

DESCRIPTION = (
    "Generate music from a text description and lyrics. "
    "Provide a prompt describing the style/mood/genre and lyrics for the vocals. "
    "For instrumental-style tracks, use placeholder lyrics like 'La la la'. "
    "Output: MP3 file (up to 5 minutes) saved to media/music/ in your working directory. "
    "Combine with listen (appreciate) to analyze the generated music."
)


class ComposeManager:
    """Manages music generation via MiniMax MCP."""

    def __init__(self, *, working_dir: Path, mcp_client: Any) -> None:
        self._working_dir = working_dir
        self._mcp = mcp_client

    def handle(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"status": "error", "message": "Missing required parameter: prompt"}

        lyrics = args.get("lyrics")
        if lyrics is None:
            return {"status": "error", "message": "Missing required parameter: lyrics"}

        # Save to working_dir/media/music/
        out_dir = self._working_dir / "media" / "music"
        out_dir.mkdir(parents=True, exist_ok=True)

        mcp_args: dict[str, Any] = {
            "prompt": prompt,
            "lyrics": lyrics,
            "output_directory": str(out_dir),
        }

        try:
            result = self._mcp.call_tool("music_generation", mcp_args)
        except Exception as exc:
            return {"status": "error", "message": f"MCP call failed: {exc}"}

        if isinstance(result, dict) and result.get("status") == "error":
            return {"status": "error", "message": result.get("message", "Unknown error")}

        # Check if MCP saved a file to output_directory
        music_files = sorted(out_dir.glob("*.mp3")) + sorted(out_dir.glob("*.wav"))
        if music_files:
            latest = music_files[-1]
            return {"status": "ok", "file_path": str(latest)}

        # Fallback: MCP may have returned a URL in text
        result_text = _extract_text(result)
        url = _extract_url(result_text)
        if url:
            try:
                import hashlib
                import time

                import requests
                resp = requests.get(url, timeout=120)
                resp.raise_for_status()
                ts = int(time.time())
                short_hash = hashlib.md5(prompt.encode()).hexdigest()[:4]
                filename = f"compose_{ts}_{short_hash}.mp3"
                out_path = out_dir / filename
                out_path.write_bytes(resp.content)
                return {"status": "ok", "file_path": str(out_path)}
            except Exception as exc:
                return {"status": "error", "message": f"Failed to download music: {exc}"}

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


def setup(agent: "BaseAgent", **kwargs: Any) -> ComposeManager:
    """Set up the compose capability on an agent.

    Requires ``mcp_client`` kwarg — a connected MiniMax MCP client instance.
    """
    mcp_client = kwargs.get("mcp_client")
    if mcp_client is None:
        raise ValueError(
            "compose capability requires mcp_client kwarg — "
            "pass a connected MiniMax MCP client"
        )
    mgr = ComposeManager(working_dir=agent.working_dir, mcp_client=mcp_client)
    agent.add_tool("compose", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    return mgr
