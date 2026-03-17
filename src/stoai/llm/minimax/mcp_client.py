"""MiniMax MCP factory — builds an MCPClient for the MiniMax MCP server.

Manages a singleton MCPClient instance and MiniMax-specific config
(API key, API host, enable/disable).
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

# ------------------------------------------------------------------
# Module-level config
# ------------------------------------------------------------------

_enabled: bool = True
_api_host: str | None = None


def set_enabled(enabled: bool) -> None:
    """Enable or disable the MiniMax MCP client."""
    global _enabled
    _enabled = enabled
    logger.debug("MiniMaxMCP: client %s", "enabled" if enabled else "disabled")


def is_enabled() -> bool:
    """Check if the MiniMax MCP client is enabled."""
    return _enabled


def set_api_host(host: str) -> None:
    """Set the API host for MiniMax MCP calls."""
    global _api_host
    _api_host = host
    logger.debug("MiniMaxMCP: API host set to: %s", host)


def get_api_host() -> str | None:
    """Get the current API host."""
    return _api_host


def get_status() -> dict[str, Any]:
    """Get the MiniMax MCP client status."""
    return {
        "enabled": _enabled,
        "connected": _client is not None and _client.is_connected() if _client else False,
        "error": _client._error if _client and _client._error else None,
        "api_host": _api_host,
    }


# ------------------------------------------------------------------
# Singleton
# ------------------------------------------------------------------

_client: MCPClient | None = None
_client_lock = threading.Lock()


def get_minimax_mcp_client() -> MCPClient:
    """Get or create the MiniMax MCP client singleton.

    Lazily initialized on first call. The subprocess is kept alive
    for the lifetime of the agent process.
    """
    global _client
    if _client is not None and _client.is_connected():
        return _client

    with _client_lock:
        if _client is not None and _client.is_connected():
            return _client

        if _client is not None:
            try:
                _client.close()
            except Exception:
                pass

        # Resolve uvx
        uvx_path = shutil.which("uvx")
        if not uvx_path:
            raise RuntimeError(
                "uvx not found. Please install uv: "
                "https://docs.astral.sh/uv/getting-started/installation/"
            )

        # Resolve API key
        api_key = os.getenv("MINIMAX_API_KEY")
        if not api_key:
            raise RuntimeError(
                "MINIMAX_API_KEY environment variable not set. "
                "Please set it in your .env file."
            )

        # Resolve API host
        host = _api_host or "https://api.minimaxi.com"
        env = {**os.environ, "MINIMAX_API_KEY": api_key, "MINIMAX_API_HOST": host}

        logger.debug("MiniMaxMCP: starting MCP client subprocess...")
        _client = MCPClient(
            command=uvx_path,
            args=["minimax-coding-plan-mcp", "-y"],
            env=env,
        )
        _client.start()
        logger.debug("MiniMaxMCP: connected")
        return _client


def _cleanup_at_exit() -> None:
    """atexit handler to clean up the MCP client."""
    global _client
    if _client is not None:
        try:
            _client.close()
        except Exception:
            pass
        _client = None


atexit.register(_cleanup_at_exit)
