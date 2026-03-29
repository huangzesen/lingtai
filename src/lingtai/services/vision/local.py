"""Local vision service — on-device image analysis via mlx-vlm (Apple Silicon).

Runs a quantized VLM locally with no API key and no network calls (after
initial model download from Hugging Face Hub).

Requires ``pip install mlx-vlm`` (or ``pip install lingtai[local-vision]``).
macOS with Apple Silicon only — mlx uses Metal for acceleration.

Usage:
    from lingtai.services.vision.local import LocalVisionService

    svc = LocalVisionService(model="mlx-community/paligemma2-3b-ft-docci-448-8bit")
    result = svc.analyze_image("/path/to/image.png", prompt="describe this")
"""
from __future__ import annotations

from . import VisionService


class LocalVisionService(VisionService):
    """Image understanding via mlx-vlm on Apple Silicon.

    The model is loaded lazily on first ``analyze_image`` call so
    construction is instant and import-time cost is zero.
    """

    def __init__(
        self,
        *,
        model: str = "mlx-community/paligemma2-3b-ft-docci-448-8bit",
        max_tokens: int = 512,
    ) -> None:
        self._model_name = model
        self._max_tokens = max_tokens
        self._model = None
        self._processor = None
        self._config = None

    def _ensure_loaded(self) -> None:
        """Lazily load the model on first use."""
        if self._model is not None:
            return

        from mlx_vlm import load
        from mlx_vlm.utils import load_config

        self._model, self._processor = load(self._model_name)
        self._config = load_config(self._model_name)

    def analyze_image(self, image_path: str, prompt: str | None = None) -> str:
        """Analyze an image using a local VLM on Apple Silicon."""
        from mlx_vlm import generate
        from mlx_vlm.prompt_utils import apply_chat_template

        self._ensure_loaded()

        question = prompt or "Describe this image."
        formatted_prompt = apply_chat_template(
            self._processor, self._config, question, num_images=1,
        )
        result = generate(
            self._model,
            self._processor,
            formatted_prompt,
            [image_path],
            max_tokens=self._max_tokens,
            verbose=False,
        )
        # generate() returns GenerationResult with .text attribute
        if hasattr(result, "text"):
            return result.text
        return str(result)
