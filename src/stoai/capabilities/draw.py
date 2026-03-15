"""Draw capability — text-to-image generation via LLM adapter."""
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
            "description": "Description of the image to generate",
        },
    },
    "required": ["prompt"],
}

DESCRIPTION = "Generate an image from a text description."


class DrawManager:
    def __init__(self, *, working_dir: Path, llm_service: "LLMService") -> None:
        self._working_dir = working_dir
        self._service = llm_service

    def handle(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"status": "error", "message": "Missing required parameter: prompt"}

        try:
            image_bytes = self._service.generate_image(prompt)
        except RuntimeError as exc:
            return {"status": "error", "message": str(exc)}
        except NotImplementedError:
            return {"status": "error", "message": "Provider does not support image generation"}

        if not image_bytes:
            return {"status": "error", "message": "Image generation returned empty result"}

        out_dir = self._working_dir / "media" / "images"
        out_dir.mkdir(parents=True, exist_ok=True)
        ts = int(time.time())
        short_hash = hashlib.md5(prompt.encode()).hexdigest()[:4]
        filename = f"draw_{ts}_{short_hash}.png"
        out_path = out_dir / filename
        out_path.write_bytes(image_bytes)
        return {"status": "ok", "file_path": str(out_path)}


def setup(agent: "BaseAgent", **kwargs: Any) -> DrawManager:
    """Set up the draw capability on an agent."""
    mgr = DrawManager(working_dir=agent.working_dir, llm_service=agent.service)
    agent.add_tool("draw", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt(
        "draw_instructions",
        "You can generate images via the draw tool. "
        "Provide a text prompt describing the image you want. "
        "Generated images are saved to media/images/ in your working directory.",
    )
    return mgr
