"""MiniMax Media MCP factory — builds MCPClient instances for the full minimax-mcp server.

Unlike ``mcp_client.py`` (which manages a singleton for the coding-plan server
with web_search + understand_image), this module creates *per-capability*
MCPClient instances backed by the full ``minimax-mcp`` package that provides
media-generation tools: text_to_audio, music_generation, text_to_image, etc.

Each capability (talk, compose, draw) can create its own client so that
different capabilities can point to different servers or configurations.
"""
from __future__ import annotations

import atexit
import os
import shutil
import threading
from typing import Any

from ...logging import get_logger
from ...services.mcp import MCPClient

logger = get_logger()

# Track all media clients for cleanup at exit
_all_clients: list[MCPClient] = []
_cleanup_lock = threading.Lock()


def create_minimax_media_client(
    *,
    api_key: str | None = None,
    api_host: str | None = None,
    resource_mode: str = "url",
) -> MCPClient:
    """Create a new MCPClient connected to the full ``minimax-mcp`` server.

    Args:
        api_key: MiniMax API key. Falls back to ``MINIMAX_API_KEY`` env var.
        api_host: API host URL. Falls back to ``MINIMAX_API_HOST`` env var,
            then to ``https://api.minimaxi.com``.
        resource_mode: ``"url"`` (return download URLs) or ``"local"``
            (save files in subprocess). Default ``"url"`` — capabilities
            handle downloading themselves.

    Returns:
        A connected MCPClient instance. Caller is responsible for calling
        ``close()`` when done (though atexit cleanup is also registered).

    Raises:
        RuntimeError: If uvx is not found or API key is missing.
    """
    uvx_path = shutil.which("uvx")
    if not uvx_path:
        raise RuntimeError(
            "uvx not found. Please install uv: "
            "https://docs.astral.sh/uv/getting-started/installation/"
        )

    resolved_key = api_key or os.getenv("MINIMAX_API_KEY")
    if not resolved_key:
        raise RuntimeError(
            "MINIMAX_API_KEY not provided and environment variable not set."
        )

    resolved_host = api_host or os.getenv("MINIMAX_API_HOST") or "https://api.minimaxi.com"

    env = {
        **os.environ,
        "MINIMAX_API_KEY": resolved_key,
        "MINIMAX_API_HOST": resolved_host,
        "MINIMAX_API_RESOURCE_MODE": resource_mode,
    }

    logger.debug("MiniMaxMediaMCP: creating client (host=%s, mode=%s)", resolved_host, resource_mode)
    client = MCPClient(
        command=uvx_path,
        args=["minimax-mcp"],
        env=env,
    )
    client.start()

    with _cleanup_lock:
        _all_clients.append(client)

    logger.debug("MiniMaxMediaMCP: client connected")
    return client


def _cleanup_at_exit() -> None:
    """atexit handler — close all media MCP clients."""
    with _cleanup_lock:
        for client in _all_clients:
            try:
                client.close()
            except Exception:
                pass
        _all_clients.clear()


atexit.register(_cleanup_at_exit)
