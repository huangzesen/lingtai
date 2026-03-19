"""Compose capability — music generation via MiniMax MCP."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from stoai_kernel.logging import get_logger

from ..i18n import t

if TYPE_CHECKING:
    from stoai_kernel.base_agent import BaseAgent
    from ..services.mcp import MCPClient

logger = get_logger()

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


# Backward compat
SCHEMA: dict[str, Any] = get_schema("en")
DESCRIPTION = get_description("en")


class ComposeManager:
    """Manages music generation via MiniMax MCP."""

    def __init__(self, *, working_dir: Path, mcp_client: "MCPClient") -> None:
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


def _auto_create_mcp_client(**kwargs: Any) -> "MCPClient":
    """Create a MiniMax media MCP client for this capability."""
    from ..llm.minimax.mcp_media_client import create_minimax_media_client
    return create_minimax_media_client(
        api_key=kwargs.get("api_key"),
        api_host=kwargs.get("api_host"),
    )


def setup(agent: "BaseAgent", **kwargs: Any) -> ComposeManager:
    """Set up the compose capability on an agent.

    Accepts ``mcp_client`` kwarg for an explicit MCP client.
    If not provided, auto-creates one connected to the full ``minimax-mcp``
    server (requires ``MINIMAX_API_KEY`` env var).
    """
    mcp_client = kwargs.get("mcp_client")
    if mcp_client is None:
        mcp_client = _auto_create_mcp_client(**kwargs)
    lang = agent._config.language
    mgr = ComposeManager(working_dir=agent.working_dir, mcp_client=mcp_client)
    agent.add_tool("compose", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang))
    return mgr
