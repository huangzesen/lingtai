# MCP Client Extraction Design

**Date:** 2026-03-17
**Status:** Draft

## Overview

Extract the generic MCP stdio client from `llm/minimax/mcp_client.py` into `services/mcp.py`. Remove the `MCPTool` dataclass and `tools=` parameter — domain tools come from MCP servers, not local wrappers.

## Problem

The MCP client currently lives in `llm/minimax/mcp_client.py` (297 lines) even though it's generic infrastructure — an async-to-sync bridge for any MCP stdio server. It has zero MiniMax-specific logic in the core class; only the singleton factory and env var handling are MiniMax-specific.

Meanwhile, `MCPTool` in `types.py` is a local tool wrapper that pretends to be MCP but doesn't use the protocol. It creates confusion: "MCP" means two different things in the codebase.

## Design

### 1. Generic MCPClient → services/mcp.py

Extract the core `MCPClient` class into `services/mcp.py`. It takes a server command at construction — no provider knowledge.

```python
class MCPClient:
    """Async-to-sync bridge for MCP stdio servers.

    Spawns a subprocess running an MCP server, manages a persistent
    session on a background thread, and provides a synchronous
    call_tool() interface.

    Connection happens lazily on first call_tool(), not at construction.
    """

    def __init__(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
    ):
        ...

    def start(self) -> None:
        """Spawn the background thread and connect. Called lazily by call_tool()."""
        ...

    def call_tool(self, name: str, args: dict, timeout: float = 120) -> dict:
        """Call an MCP tool synchronously. Starts connection if not yet connected."""
        ...

    def close(self) -> None:
        """Shut down the MCP session and background thread."""
        ...

    def is_connected(self) -> bool:
        """Check if the client has an active session."""
        ...
```

This class contains:
- Background thread with asyncio event loop
- Async MCP session lifecycle (connect, cleanup)
- `call_tool()` sync wrapper via `run_coroutine_threadsafe`
- Activity logging (last 50 calls)
- Thread-safe close and connection checks

It does NOT contain:
- Singleton management
- Provider-specific config (API keys, hosts)
- Enable/disable toggle
- `get_status()` or `set_api_host()`

### 2. MiniMax factory → llm/minimax/mcp_client.py (shrunk)

The MiniMax-specific file shrinks to a thin factory:

```python
from ...services.mcp import MCPClient

def get_minimax_mcp_client() -> MCPClient:
    """Singleton factory for MiniMax's MCP server."""
    # Resolve uvx path, MINIMAX_API_KEY, MINIMAX_API_HOST
    # Return MCPClient(command=uvx_path, args=[...], env={...})
```

Keeps:
- Singleton management (`_client`, `_client_lock`) as module-level state
- Module-level functions: `set_enabled()`, `is_enabled()`, `set_api_host()`, `get_status()`
- `atexit` cleanup

No more `MiniMaxMCPClient` class — the class-level config becomes module-level functions around the singleton `MCPClient` instance.

### 3. Remove MCPTool and tools= parameter

**Delete from `types.py`:**
- `MCPTool` dataclass

**Update `agent.py`:**
- Remove `tools: list[MCPTool] | None = None` parameter
- Remove the domain tools registration loop
- Remove `_mcp_tool_names` tracking

**Update `base_agent.py`:**
- Remove `_mcp_tool_names` set
- Remove `[MCP]` prefix logic in `_build_tool_schemas()`
- `add_tool()` stays (capabilities use it) but it's no longer called for domain tools

### 4. Capabilities unchanged

`draw.py`, `talk.py`, `compose.py` still:
- Accept `mcp_client` kwarg in `setup()`
- Call `mcp_client.call_tool(name, args)`
- Handle output directories, file management, error parsing

The only change: `mcp_client` is now typed as `MCPClient` from `services.mcp` instead of `Any`.

## Impact

| Component | Change |
|-----------|--------|
| `services/mcp.py` | NEW — generic MCPClient |
| `llm/minimax/mcp_client.py` | Shrink to factory, import MCPClient from services |
| `types.py` | Remove MCPTool |
| `agent.py` | Remove tools= param, remove _mcp_tool_names |
| `base_agent.py` | Remove _mcp_tool_names, remove [MCP] prefix |
| `capabilities/draw.py` | Type hint only |
| `capabilities/talk.py` | Type hint only |
| `capabilities/compose.py` | Type hint only |
| `__init__.py` | Remove MCPTool from public API |
| `CLAUDE.md` | Update three-tier tool model docs |
| `tests/test_types.py` | Remove MCPTool tests |
| `tests/test_agent_capabilities.py` | Remove tools= and _mcp_tool_names tests |
| `tests/test_agent.py` | Remove MCPTool imports/usage |

## Implementation Order

1. Create `services/mcp.py` — extract generic MCPClient
2. Shrink `llm/minimax/mcp_client.py` — use new MCPClient
3. Remove MCPTool and tools= — clean up types.py, agent.py, base_agent.py
4. Update CLAUDE.md
