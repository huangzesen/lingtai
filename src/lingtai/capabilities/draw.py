"""Draw capability — text-to-image generation via MiniMax MCP."""
from __future__ import annotations

import hashlib
import time
from pathlib import Path
from typing import TYPE_CHECKING, Any

import requests

from lingtai_kernel.logging import get_logger

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent
    from ..services.mcp import MCPClient

logger = get_logger()

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


# Backward compat
SCHEMA: dict[str, Any] = get_schema("en")
DESCRIPTION = get_description("en")


class DrawManager:
    """Manages text-to-image generation via MiniMax MCP."""

    def __init__(self, *, working_dir: Path, mcp_client: "MCPClient") -> None:
        self._working_dir = working_dir
        self._mcp = mcp_client

    def handle(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"status": "error", "message": "Missing required parameter: prompt"}

        mcp_args: dict[str, Any] = {"prompt": prompt}
        aspect_ratio = args.get("aspect_ratio")
        if aspect_ratio:
            mcp_args["aspect_ratio"] = aspect_ratio

        # Save to working_dir/media/images/
        out_dir = self._working_dir / "media" / "images"
        out_dir.mkdir(parents=True, exist_ok=True)
        mcp_args["output_directory"] = str(out_dir)

        try:
            result = self._mcp.call_tool("text_to_image", mcp_args)
        except Exception as exc:
            return {"status": "error", "message": f"MCP call failed: {exc}"}

        if isinstance(result, dict) and result.get("status") == "error":
            return {"status": "error", "message": result.get("message", "Unknown error")}

        # The MCP returns a text result with image URLs or a saved file path.
        # Parse the response to find the output file or URL.
        result_text = _extract_text(result)

        # If MCP saved to output_directory, find the file
        image_files = sorted(out_dir.glob("*.jpeg")) + sorted(out_dir.glob("*.jpg")) + sorted(out_dir.glob("*.png"))
        if image_files:
            latest = image_files[-1]
            return {"status": "ok", "file_path": str(latest)}

        # Fallback: if MCP returned a URL, download it
        url = _extract_url(result_text)
        if url:
            try:
                resp = requests.get(url, timeout=60)
                resp.raise_for_status()
                ts = int(time.time())
                short_hash = hashlib.md5(prompt.encode()).hexdigest()[:4]
                filename = f"draw_{ts}_{short_hash}.jpeg"
                out_path = out_dir / filename
                out_path.write_bytes(resp.content)
                return {"status": "ok", "file_path": str(out_path)}
            except Exception as exc:
                return {"status": "error", "message": f"Failed to download image: {exc}"}

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


def setup(agent: "BaseAgent", **kwargs: Any) -> DrawManager:
    """Set up the draw capability on an agent.

    Accepts ``mcp_client`` kwarg for an explicit MCP client.
    If not provided, auto-creates one connected to the full ``minimax-mcp``
    server (requires ``MINIMAX_API_KEY`` env var).
    """
    mcp_client = kwargs.get("mcp_client")
    if mcp_client is None:
        mcp_client = _auto_create_mcp_client(**kwargs)
    lang = agent._config.language
    mgr = DrawManager(working_dir=agent.working_dir, mcp_client=mcp_client)
    agent.add_tool("draw", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang))
    return mgr
