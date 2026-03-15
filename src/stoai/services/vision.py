"""VisionService — abstract image understanding backing the vision intrinsic.

First implementation: LLMVisionService (wraps multimodal LLM).
Future: dedicated vision models, OCR services, etc.
"""
from __future__ import annotations

from abc import ABC, abstractmethod
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..llm.service import LLMService


class VisionService(ABC):
    """Abstract vision service.

    Backs the vision intrinsic. Implementations provide image understanding
    via LLM multimodal input, dedicated vision models, or other backends.
    """

    @abstractmethod
    def analyze_image(self, image_path: str, prompt: str | None = None) -> str:
        """Analyze an image and return a text description.

        Args:
            image_path: Path to the image file.
            prompt: Optional prompt to guide the analysis (e.g., "describe the chart").

        Returns:
            Text description/analysis of the image.
        """
        ...


class LLMVisionService(VisionService):
    """Uses a multimodal LLM for image understanding.

    This is the first implementation — delegates to the LLMService's
    multimodal capabilities. Requires an LLM that supports image input.
    """

    def __init__(self, llm: LLMService):
        self._llm = llm

    def analyze_image(self, image_path: str, prompt: str | None = None) -> str:
        # TODO: implement using LLMService multimodal API
        # For now, return a placeholder indicating the service is available
        # but the actual LLM vision call needs to be wired through ChatSession
        raise NotImplementedError(
            "LLMVisionService.analyze_image requires multimodal LLM support — "
            "wire through ChatSession when ready"
        )
