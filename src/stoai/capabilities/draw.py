"""Draw capability — text-to-image generation via MiniMax MCP."""
from __future__ import annotations

import hashlib
import time
from pathlib import Path
from typing import TYPE_CHECKING, Any

import requests

from ..logging import get_logger

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

logger = get_logger()

SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "prompt": {
            "type": "string",
            "description": "Description of the image to generate",
        },
        "aspect_ratio": {
            "type": "string",
            "description": "Aspect ratio (e.g. '1:1', '16:9', '9:16'). Default: '1:1'",
        },
    },
    "required": ["prompt"],
}

DESCRIPTION = (
    "Generate an image from a text description. "
    "Provide a detailed prompt — the more specific, the better the result. "
    "Supports various aspect ratios (1:1, 16:9, 9:16, 4:3, etc.). "
    "Output: JPEG image saved to media/images/ in your working directory. "
    "Combine with vision to generate an image and then analyze or critique it."
)


class DrawManager:
    """Manages text-to-image generation via MiniMax MCP."""

    def __init__(self, *, working_dir: Path, mcp_client: Any) -> None:
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


def setup(agent: "BaseAgent", **kwargs: Any) -> DrawManager:
    """Set up the draw capability on an agent.

    Requires ``mcp_client`` kwarg — a connected MiniMax MCP client instance.
    """
    mcp_client = kwargs.get("mcp_client")
    if mcp_client is None:
        raise ValueError(
            "draw capability requires mcp_client kwarg — "
            "pass a connected MiniMax MCP client"
        )
    mgr = DrawManager(working_dir=agent.working_dir, mcp_client=mcp_client)
    agent.add_tool("draw", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    return mgr
