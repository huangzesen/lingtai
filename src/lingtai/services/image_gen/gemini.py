"""GeminiImageGenService — text-to-image via Gemini's native generation.

Creates its own ``google.genai.Client`` and calls ``generate_content``
with ``response_modalities=["IMAGE"]``. Extracted from the Gemini
adapter's ``generate_image()`` method.
"""
from __future__ import annotations

import hashlib
import os
import time
from pathlib import Path
from typing import Any

from lingtai_kernel.logging import get_logger

from . import ImageGenService

logger = get_logger()


class GeminiImageGenService(ImageGenService):
    """Image generation via Gemini's native image generation API.

    Creates its own ``google.genai.Client``. Does not require a running
    Gemini adapter or LLMService.
    """

    def __init__(
        self,
        *,
        api_key: str | None = None,
        model: str = "gemini-2.5-flash-image",
    ) -> None:
        from google import genai
        from google.genai import types

        resolved_key = api_key or os.getenv("GOOGLE_API_KEY") or os.getenv("GEMINI_API_KEY")
        if not resolved_key:
            raise RuntimeError(
                "Gemini API key not provided and neither GOOGLE_API_KEY nor "
                "GEMINI_API_KEY environment variable is set."
            )

        self._client = genai.Client(
            api_key=resolved_key,
            http_options=types.HttpOptions(timeout=300_000),
        )
        self._model = model
        self._types = types

    def generate(
        self,
        prompt: str,
        *,
        aspect_ratio: str | None = None,
        output_dir: Path | None = None,
        **kwargs: Any,
    ) -> Path:
        """Generate an image via Gemini's native image generation."""
        if output_dir is None:
            output_dir = Path.cwd() / "images"
        output_dir.mkdir(parents=True, exist_ok=True)

        model = kwargs.get("model", self._model)

        raw = self._client.models.generate_content(
            model=model,
            contents=prompt,
            config=self._types.GenerateContentConfig(
                response_modalities=["IMAGE"],
            ),
        )

        # Extract image bytes from response parts
        if raw.candidates:
            for part in raw.candidates[0].content.parts:
                if part.inline_data and part.inline_data.data:
                    image_bytes = part.inline_data.data
                    # Determine extension from mime_type
                    mime = part.inline_data.mime_type or "image/png"
                    ext = ".png" if "png" in mime else ".jpeg"

                    ts = int(time.time())
                    short_hash = hashlib.md5(prompt.encode()).hexdigest()[:4]
                    filename = f"draw_{ts}_{short_hash}{ext}"
                    out_path = output_dir / filename
                    out_path.write_bytes(image_bytes)
                    return out_path

        raise RuntimeError("Gemini image generation returned no image data")
