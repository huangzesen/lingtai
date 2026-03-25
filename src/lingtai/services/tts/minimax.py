"""MiniMaxTTSService — text-to-speech via MiniMax MCP server."""
from __future__ import annotations

import hashlib
import re
import time
from pathlib import Path
from typing import Any

from lingtai_kernel.logging import get_logger

from . import TTSService

logger = get_logger()


def _extract_text(result: Any) -> str:
    """Extract text from an MCP call result."""
    if isinstance(result, dict):
        return result.get("text", str(result))
    return str(result)


def _extract_url(text: str) -> str | None:
    """Extract the first HTTP(S) URL from text."""
    match = re.search(r"https?://\S+", text)
    return match.group(0).rstrip("']") if match else None


class MiniMaxTTSService(TTSService):
    """TTS via the full ``minimax-mcp`` MCP server.

    Creates and owns its own :class:`MCPClient` subprocess.  The client
    is started lazily on the first call to :meth:`synthesize` and shut
    down when :meth:`close` is called.

    Args:
        api_key: MiniMax API key.  Falls back to ``MINIMAX_API_KEY``.
        api_host: API host URL.  Falls back to ``MINIMAX_API_HOST``,
            then ``https://api.minimaxi.com``.
    """

    def __init__(
        self,
        *,
        api_key: str | None = None,
        api_host: str | None = None,
    ) -> None:
        self._api_key = api_key
        self._api_host = api_host
        self._mcp: Any | None = None  # MCPClient, created lazily

    # -- lazy MCP lifecycle ---------------------------------------------------

    def _ensure_mcp(self) -> Any:
        if self._mcp is None:
            from ...llm.minimax.mcp_media_client import create_minimax_media_client

            self._mcp = create_minimax_media_client(
                api_key=self._api_key,
                api_host=self._api_host,
            )
        return self._mcp

    def close(self) -> None:
        """Shut down the MCP client if running."""
        if self._mcp is not None:
            try:
                self._mcp.close()
            except Exception:
                pass
            self._mcp = None

    # -- TTSService -----------------------------------------------------------

    def synthesize(
        self,
        text: str,
        *,
        voice: str | None = None,
        output_dir: Path | None = None,
        **kwargs: object,
    ) -> Path:
        """Synthesize speech via MiniMax MCP ``text_to_audio`` tool.

        Args:
            text: Text to convert to speech.
            voice: MiniMax voice ID (passed as ``voice_id``).
            output_dir: Directory to save audio files.
            **kwargs: Extra MiniMax params (``emotion``, ``speed``).

        Returns:
            Path to the saved audio file.

        Raises:
            RuntimeError: If the MCP call fails or produces no file.
        """
        if output_dir is None:
            output_dir = Path.cwd() / "media" / "audio"
        output_dir.mkdir(parents=True, exist_ok=True)

        mcp = self._ensure_mcp()

        mcp_args: dict[str, Any] = {
            "text": text,
            "output_directory": str(output_dir),
        }
        if voice is not None:
            mcp_args["voice_id"] = voice
        for key in ("emotion", "speed", "voice_id"):
            val = kwargs.get(key)
            if val is not None:
                mcp_args[key] = val

        try:
            result = mcp.call_tool("text_to_audio", mcp_args)
        except Exception as exc:
            raise RuntimeError(f"MiniMax MCP call failed: {exc}") from exc

        if isinstance(result, dict) and result.get("status") == "error":
            raise RuntimeError(
                f"MiniMax MCP error: {result.get('message', 'Unknown error')}"
            )

        # Check if MCP saved a file to output_directory
        audio_files = sorted(output_dir.glob("*.mp3")) + sorted(
            output_dir.glob("*.wav")
        )
        if audio_files:
            return audio_files[-1]

        # Fallback: MCP may have returned a URL in text
        result_text = _extract_text(result)
        url = _extract_url(result_text)
        if url:
            try:
                import requests  # type: ignore[import-untyped]

                resp = requests.get(url, timeout=60)
                resp.raise_for_status()
                ts = int(time.time())
                short_hash = hashlib.md5(text.encode()).hexdigest()[:4]
                filename = f"talk_{ts}_{short_hash}.mp3"
                out_path = output_dir / filename
                out_path.write_bytes(resp.content)
                return out_path
            except Exception as exc:
                raise RuntimeError(f"Failed to download audio: {exc}") from exc

        raise RuntimeError(f"Unexpected MCP response: {result_text}")
