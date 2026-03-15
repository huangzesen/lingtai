"""
MCP client singleton for the MiniMax Coding Plan MCP server.

Manages a persistent subprocess running `minimax-coding-plan-mcp` and provides
a synchronous call_tool() interface for the MiniMax adapter.

The subprocess is spawned lazily on first use and kept alive for the
agent's lifetime. A background daemon thread runs the async event loop.
"""

import atexit
import json
import os
import shutil
import sys
import threading
from datetime import datetime, timedelta
from typing import Any

from ...logging import get_logger

logger = get_logger()

# Activity log for debugging - last 50 calls
_ACTIVITY_LOG: list[dict[str, Any]] = []
_ACTIVITY_LOCK = threading.Lock()


_default_enabled = True
_default_api_host: str | None = None


class MiniMaxMCPClient:
    """Manages a persistent MCP connection to the minimax-coding-plan-mcp server."""

    # Class-level state for enable/disable and config (loaded from config.json)
    _enabled: bool = _default_enabled
    _api_host: str | None = _default_api_host

    def __init__(self):
        self._session = None
        self._read_stream = None
        self._write_stream = None
        self._loop = None
        self._thread = None
        self._ready = threading.Event()
        self._error = None
        self._lock = threading.Lock()
        self._closed = False
        self._stdio_cm = None
        self._session_cm = None

    @classmethod
    def set_enabled(cls, enabled: bool) -> None:
        """Enable or disable the MCP client."""
        cls._enabled = enabled
        logger.debug("MiniMaxMCP: MCP client %s", "enabled" if enabled else "disabled")

    @classmethod
    def is_enabled(cls) -> bool:
        """Check if the MCP client is enabled."""
        return cls._enabled

    @classmethod
    def set_api_host(cls, host: str) -> None:
        """Set the API host for MCP calls."""
        cls._api_host = host
        logger.debug("MiniMaxMCP: MCP API host set to: %s", host)

    @classmethod
    def get_api_host(cls) -> str | None:
        """Get the current API host."""
        return cls._api_host

    @classmethod
    def get_status(cls) -> dict:
        """Get the MCP client status."""
        return {
            "enabled": cls._enabled,
            "connected": _client is not None and _client.is_connected() if _client else False,
            "error": _client._error if _client and _client._error else None,
            "api_host": cls._api_host,
        }

    @classmethod
    def get_activity_log(cls) -> list[dict[str, Any]]:
        """Get recent MCP tool calls for debugging."""
        with _ACTIVITY_LOCK:
            return list(_ACTIVITY_LOG)

    def _start(self):
        """Spawn the background thread that runs the MCP session."""
        self._thread = threading.Thread(target=self._run_loop, daemon=True)
        self._thread.start()
        self._ready.wait(timeout=30)
        if self._error:
            raise RuntimeError(f"MiniMax MCP server failed to start: {self._error}")

    def _run_loop(self):
        """Background thread: run the async event loop with the MCP session."""
        import asyncio

        loop = asyncio.new_event_loop()
        self._loop = loop
        asyncio.set_event_loop(loop)
        try:
            loop.run_until_complete(self._async_connect())
            loop.run_forever()
        except Exception as e:
            self._error = str(e)
            self._ready.set()
        finally:
            try:
                loop.run_until_complete(self._async_cleanup())
            except Exception:
                pass
            loop.close()

    async def _async_connect(self):
        """Establish the MCP stdio connection (runs in background thread)."""
        from mcp.client.stdio import stdio_client, StdioServerParameters
        from mcp.client.session import ClientSession

        uvx_path = shutil.which("uvx")
        if not uvx_path:
            raise RuntimeError(
                "uvx not found. Please install uv: "
                "https://docs.astral.sh/uv/getting-started/installation/"
            )

        api_key = os.getenv("MINIMAX_API_KEY")
        if not api_key:
            raise RuntimeError(
                "MINIMAX_API_KEY environment variable not set. "
                "Please set it in your .env file."
            )

        # Use class-level API host if set, otherwise use default
        api_host = self._api_host
        if not api_host:
            api_host = "https://api.minimaxi.com"

        env = {**os.environ, "MINIMAX_API_KEY": api_key, "MINIMAX_API_HOST": api_host}

        server_params = StdioServerParameters(
            command=uvx_path,
            args=["minimax-coding-plan-mcp", "-y"],
            env=env,
        )

        self._stdio_cm = stdio_client(server_params)
        self._read_stream, self._write_stream = await self._stdio_cm.__aenter__()

        self._session_cm = ClientSession(self._read_stream, self._write_stream)
        self._session = await self._session_cm.__aenter__()

        await self._session.initialize()

        self._ready.set()

    async def _async_cleanup(self):
        """Clean up MCP session and stdio transport."""
        if self._session_cm:
            try:
                await self._session_cm.__aexit__(None, None, None)
            except Exception:
                pass
        if self._stdio_cm:
            try:
                await self._stdio_cm.__aexit__(None, None, None)
            except Exception:
                pass

    def call_tool(self, name: str, args: dict, timeout: float = 120) -> dict:
        """Call an MCP tool synchronously.

        Args:
            name: Tool name (e.g., "web_search")
            args: Tool arguments dict
            timeout: Timeout in seconds

        Returns:
            Parsed dict from the tool's JSON response.

        Raises:
            RuntimeError: If the MCP session is not connected or tool call fails.
        """
        import asyncio

        if self._closed:
            raise RuntimeError("MiniMax MCP client has been closed")

        if self._session is None or self._loop is None:
            raise RuntimeError("MiniMax MCP client not connected")

        async def _call():
            result = await self._session.call_tool(
                name=name,
                arguments=args,
                read_timeout_seconds=timedelta(seconds=timeout),
            )
            if result.isError:
                error_text = (
                    result.content[0].text if result.content else "Unknown MCP error"
                )
                return {"status": "error", "message": error_text}

            if result.content:
                for block in result.content:
                    if hasattr(block, "text"):
                        try:
                            return json.loads(block.text)
                        except (json.JSONDecodeError, TypeError):
                            return {"status": "success", "text": block.text}

            return {"status": "success", "text": ""}

        future = asyncio.run_coroutine_threadsafe(_call(), self._loop)
        result = future.result(timeout=timeout)

        # Log to activity
        with _ACTIVITY_LOCK:
            _ACTIVITY_LOG.append({
                "tool": name,
                "args": args,
                "result": result,
                "timestamp": datetime.utcnow().isoformat() + "Z",
            })
            # Keep only last 50
            if len(_ACTIVITY_LOG) > 50:
                _ACTIVITY_LOG[:] = _ACTIVITY_LOG[-50:]

        return result

    def close(self):
        """Shut down the MCP session and background thread."""
        if self._closed:
            return
        self._closed = True
        if self._loop and self._loop.is_running():
            self._loop.call_soon_threadsafe(self._loop.stop)
        if self._thread:
            self._thread.join(timeout=5)

    def is_connected(self) -> bool:
        """Check if the client has an active session."""
        return (
            self._session is not None
            and self._loop is not None
            and self._loop.is_running()
            and not self._closed
        )


_client: MiniMaxMCPClient | None = None
_client_lock = threading.Lock()


def get_minimax_mcp_client() -> MiniMaxMCPClient:
    """Get or create the MiniMax MCP client singleton.

    The client is lazily initialized on first call. The subprocess is kept
    alive for the lifetime of the agent process.
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

        logger.debug("MiniMaxMCP: Starting MiniMax MCP client subprocess...")
        _client = MiniMaxMCPClient()
        _client._start()
        logger.debug("MiniMaxMCP: MiniMax MCP client connected")
        return _client


def _cleanup_at_exit():
    """atexit handler to clean up the MCP client."""
    global _client
    if _client is not None:
        try:
            _client.close()
        except Exception:
            pass
        _client = None


atexit.register(_cleanup_at_exit)
