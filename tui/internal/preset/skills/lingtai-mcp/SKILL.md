---
name: lingtai-mcp
description: Register external MCP servers so this agent can use their tools. Supports both local stdio servers (npx, uvx) and remote HTTP servers. Read this when the human asks to connect an MCP server, add external tools, or integrate a third-party service via MCP.
version: 1.0.0
---

# Registering MCP Servers

MCP (Model Context Protocol) servers provide external tools to your agent. Once registered, their tools appear alongside your built-in capabilities — you can call them directly.

## How It Works

1. You write a config file at `mcp/servers.json` in your working directory
2. On next restart (molt, `/refresh`, `/cpr`), the servers are connected automatically
3. All tools from each server become available to you

## Config File Location

```
<your-working-dir>/mcp/servers.json
```

Create the `mcp/` directory if it doesn't exist.

## Config Format

`servers.json` is a JSON object where each key is a server name and the value is its configuration. Two types are supported:

### Stdio Servers (local subprocess)

For MCP servers that run as local processes (npx, uvx, or any executable):

```json
{
  "server-name": {
    "type": "stdio",
    "command": "npx",
    "args": ["-y", "@some-org/mcp-server"],
    "env": {
      "API_KEY": "your-key-here"
    }
  }
}
```

| Field | Required | Description |
|---|---|---|
| `type` | No | `"stdio"` (default if omitted) |
| `command` | Yes | Executable to run |
| `args` | No | Command-line arguments |
| `env` | No | Environment variables for the subprocess |

### HTTP Servers (remote)

For MCP servers accessible via HTTP (streamable-http transport):

```json
{
  "server-name": {
    "type": "http",
    "url": "https://api.example.com/mcp",
    "headers": {
      "Authorization": "Bearer your-key-here"
    }
  }
}
```

| Field | Required | Description |
|---|---|---|
| `type` | Yes | Must be `"http"` |
| `url` | Yes | HTTP endpoint of the MCP server |
| `headers` | No | HTTP headers (typically for authentication) |

### Multiple Servers

You can register multiple servers in one file:

```json
{
  "vision": {
    "type": "stdio",
    "command": "npx",
    "args": ["-y", "@z_ai/mcp-server"],
    "env": {
      "Z_AI_API_KEY": "your-key",
      "Z_AI_MODE": "ZAI"
    }
  },
  "web-search": {
    "type": "http",
    "url": "https://api.z.ai/api/mcp/web_search_prime/mcp",
    "headers": {
      "Authorization": "Bearer your-key"
    }
  },
  "web-reader": {
    "type": "http",
    "url": "https://api.z.ai/api/mcp/web_reader/mcp",
    "headers": {
      "Authorization": "Bearer your-key"
    }
  }
}
```

## How to Register

When the human asks to add an MCP server:

1. Ask what server they want (name, type, credentials)
2. Read the existing `mcp/servers.json` if it exists (to preserve other entries)
3. Add the new server entry
4. Write the updated file using the `write` tool
5. Tell the human to `/refresh` you so the tools are loaded

**Important:** Do not overwrite existing entries — merge the new server into the existing config.

## API Keys and Secrets

API keys in `servers.json` are stored in plain text. For sensitive keys:

- The human can use environment variable references in `env` fields
- For HTTP servers, the key goes in the `headers` field
- Remind the human that `mcp/servers.json` should not be committed to version control if it contains secrets

## After Registration

Once registered and restarted:

- The server's tools appear in your tool list automatically
- You can call them like any other tool
- If a server fails to connect, a warning is logged but the agent continues running
- To remove a server, edit `mcp/servers.json` and remove the entry

## Troubleshooting

- **"command not found"**: The executable (npx, uvx) must be installed on the system
- **HTTP 401/403**: Check the API key and header format
- **Tools not appearing**: Make sure you restarted after editing `servers.json`
- **Server crashes on start**: Check the server's own logs or try running the command manually
