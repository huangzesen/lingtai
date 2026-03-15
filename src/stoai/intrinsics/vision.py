"""Vision intrinsic — image understanding via LLM."""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "image_path": {"type": "string", "description": "Path to the image file"},
        "question": {"type": "string", "description": "Question about the image", "default": "Describe this image."},
    },
    "required": ["image_path"],
}
DESCRIPTION = "Analyze an image using the LLM's vision capabilities."
