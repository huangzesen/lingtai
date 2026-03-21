"""MCPClient — async-to-sync bridge for MCP stdio servers.

Spawns a subprocess running an MCP server, manages a persistent session
on a background thread, and provides a synchronous call_tool() interface.

The subprocess is started lazily on first call_tool() and kept alive
until close() is called. A background daemon thread runs the async event loop.
"""
from __future__ import annotations

import json
import threading
from datetime import datetime, timedelta, timezone
from typing import Any

from lingtai_kernel.logging import get_logger

logger = get_logger()


class MCPClient:
    """Async-to-sync bridge for any MCP stdio server.

    Args:
        command: Executable to run (e.g., "uvx").
        args: Arguments to the command (e.g., ["minimax-coding-plan-mcp", "-y"]).
        env: Environment variables for the subprocess. If None, inherits
            the current process environment.
    """

    def __init__(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
    ):
        self._command = command
        self._args = args or []
        self._env = env

        self._session: Any = None
        self._read_stream: Any = None
        self._write_stream: Any = None
        self._loop: Any = None
        self._thread: threading.Thread | None = None
        self._ready = threading.Event()
        self._error: str | None = None
        self._lock = threading.Lock()
        self._closed = False
        self._stdio_cm: Any = None
        self._session_cm: Any = None

        # Activity log for debugging — last 50 calls
        self._activity_log: list[dict[str, Any]] = []
        self._activity_lock = threading.Lock()

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        """Spawn the background thread and connect to the MCP server.

        Called automatically by call_tool() if not yet connected.
        """
        if self.is_connected():
            return
        self._thread = threading.Thread(target=self._run_loop, daemon=True)
        self._thread.start()
        self._ready.wait(timeout=30)
        if self._error:
            raise RuntimeError(f"MCP server failed to start: {self._error}")

    def close(self) -> None:
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

    # ------------------------------------------------------------------
    # Tool calls
    # ------------------------------------------------------------------

    def call_tool(self, name: str, args: dict, timeout: float = 120) -> dict:
        """Call an MCP tool synchronously.

        Starts the connection lazily if not yet connected.

        Args:
            name: Tool name (e.g., "web_search").
            args: Tool arguments dict.
            timeout: Timeout in seconds.

        Returns:
            Parsed dict from the tool's JSON response.

        Raises:
            RuntimeError: If the client is closed or connection fails.
        """
        import asyncio

        if self._closed:
            raise RuntimeError("MCP client has been closed")

        # Lazy start
        if not self.is_connected():
            self.start()

        if self._session is None or self._loop is None:
            raise RuntimeError("MCP client not connected")

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

        # Log activity
        with self._activity_lock:
            self._activity_log.append({
                "tool": name,
                "args": args,
                "result": result,
                "timestamp": datetime.now(timezone.utc).isoformat(),
            })
            if len(self._activity_log) > 50:
                self._activity_log[:] = self._activity_log[-50:]

        return result

    def list_tools(self, timeout: float = 10) -> list[dict]:
        """List available tools from the MCP server.

        Returns a list of dicts with 'name', 'description', and 'schema' keys.
        """
        import asyncio

        if not self.is_connected():
            self.start()

        if self._session is None or self._loop is None:
            raise RuntimeError("MCP client not connected")

        async def _list():
            result = await self._session.list_tools()
            tools = []
            for tool in result.tools:
                schema = {}
                if tool.inputSchema:
                    schema = tool.inputSchema if isinstance(tool.inputSchema, dict) else {}
                tools.append({
                    "name": tool.name,
                    "description": tool.description or "",
                    "schema": schema,
                })
            return tools

        future = asyncio.run_coroutine_threadsafe(_list(), self._loop)
        return future.result(timeout=timeout)

    def get_activity_log(self) -> list[dict[str, Any]]:
        """Get recent MCP tool calls for debugging."""
        with self._activity_lock:
            return list(self._activity_log)

    # ------------------------------------------------------------------
    # Internal — background thread
    # ------------------------------------------------------------------

    def _run_loop(self) -> None:
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

    async def _async_connect(self) -> None:
        """Establish the MCP stdio connection (runs in background thread)."""
        from mcp.client.stdio import stdio_client, StdioServerParameters
        from mcp.client.session import ClientSession

        server_params = StdioServerParameters(
            command=self._command,
            args=self._args,
            env=self._env,
        )

        self._stdio_cm = stdio_client(server_params)
        self._read_stream, self._write_stream = await self._stdio_cm.__aenter__()

        self._session_cm = ClientSession(self._read_stream, self._write_stream)
        self._session = await self._session_cm.__aenter__()

        await self._session.initialize()

        self._ready.set()

    async def _async_cleanup(self) -> None:
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
