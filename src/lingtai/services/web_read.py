"""WebReadService — abstract URL content extraction backing the web_read capability.

Implementations:
- TrafilaturaWebReadService — zero-API-key content extraction via trafilatura.
"""
from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass


@dataclass
class WebReadResult:
    """Extracted content from a URL."""
    title: str
    content: str
    url: str


class WebReadService(ABC):
    """Abstract web content extraction service.

    Backs the web_read capability. Implementations fetch a URL and
    extract the main readable content.
    """

    @abstractmethod
    def read(self, url: str, output_format: str = "text") -> WebReadResult:
        """Fetch a URL and extract readable content.

        Args:
            url: The URL to read.
            output_format: Output format — 'text' or 'markdown'.

        Returns:
            Extracted content with title and URL.
        """
        ...


class TrafilaturaWebReadService(WebReadService):
    """Zero-API-key web content extraction via trafilatura.

    Uses the ``trafilatura`` package to fetch and extract main content.
    Install with ``pip install lingtai[trafilatura]`` or
    ``pip install trafilatura``.
    """

    def read(self, url: str, output_format: str = "text") -> WebReadResult:
        import trafilatura  # type: ignore[import-untyped]

        downloaded = trafilatura.fetch_url(url)
        if downloaded is None:
            raise RuntimeError(f"Failed to fetch URL: {url}")

        fmt = "markdown" if output_format == "markdown" else "txt"
        content = trafilatura.extract(downloaded, output_format=fmt)
        if not content:
            raise RuntimeError(f"No readable content extracted from: {url}")

        # Extract title from metadata
        title = ""
        metadata = trafilatura.bare_extraction(downloaded)
        if metadata and isinstance(metadata, dict):
            title = metadata.get("title", "")

        return WebReadResult(title=title, content=content, url=url)
