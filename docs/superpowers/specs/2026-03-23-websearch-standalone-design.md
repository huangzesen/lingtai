# Web Search as Standalone SearchService

**Date:** 2026-03-23
**Scope:** lingtai (services, capabilities, adapters)
**Status:** Draft

## Problem

Web search is currently split across two layers:

1. **SearchService** (capability-level) — `DuckDuckGoSearchService` in `services/search.py`, plus a stub `LLMSearchService`.
2. **Adapter fallback** — each LLM adapter implements `web_search()` directly. Anthropic uses its native tool, OpenAI uses a search model, Gemini uses `GoogleSearch()`, MiniMax uses an MCP subprocess.

This conflates "LLM thinking" with "web searching." The MiniMax adapter's `web_search()` spins up an MCP subprocess needing its own API key (`MINIMAX_API_KEY`), but that key is resolved through `LLMService.get_adapter()` — designed for LLM sessions. If you use MiniMax only for web search (not as your LLM provider), there's no clean way to pass the key. The adapter shouldn't own search concerns.

## Design Principles

**Adapters are for LLM calls only.** Web search is a capability backed by a service, not an adapter feature. The adapter ABC should have no knowledge of search.

**Explicit configuration.** The capability declares its `provider` and `api_key` in init.json. No silent fallback to `agent.service.provider`. If the config is wrong, fail at setup time, not at call time.

**One interface, one routing path.** Every search backend implements `SearchService`. The capability only talks to `SearchService`. No two-tier fallback logic.

## Package Structure

```
services/websearch/
    __init__.py          # ABC, SearchResult, registry, factory
    duckduckgo.py        # DuckDuckGoSearchService
    minimax.py           # MiniMaxSearchService
    anthropic.py         # AnthropicSearchService
    openai.py            # OpenAISearchService
    gemini.py            # GeminiSearchService
```

Old `services/search.py` is deleted. Everything moves to `services/websearch/`.

## `__init__.py` — ABC + Factory

Contains:

- `SearchResult` dataclass (moved from `services/search.py`): `title`, `url`, `snippet`
- `SearchService` ABC (moved from `services/search.py`): `search(query, max_results=5) -> list[SearchResult]`
- Provider registry mapping provider name to module path
- Factory function:

```python
def create_search_service(provider: str, **kwargs) -> SearchService:
    """Create a SearchService by provider name.

    Lazy-imports the implementation class. Passes api_key and
    any other kwargs to the constructor.

    Raises ValueError for unknown provider.
    Raises RuntimeError if provider needs api_key and it's missing.
    """
```

Public API: `from lingtai.services.websearch import SearchService, SearchResult, create_search_service`

## Provider Implementations

Each file exports a single class. Constructor takes `api_key: str | None = None` plus any provider-specific kwargs. Validates key requirement at `__init__` time — not at `search()` time.

| Provider | Class | Needs `api_key` | How it searches |
|----------|-------|-----------------|-----------------|
| `duckduckgo` | `DuckDuckGoSearchService` | No | `ddgs` package scraping |
| `minimax` | `MiniMaxSearchService` | Yes | MCP subprocess (`minimax-coding-plan-mcp`) |
| `anthropic` | `AnthropicSearchService` | Yes | Anthropic API `web_search_20250305` tool |
| `openai` | `OpenAISearchService` | Yes | `gpt-4o-search-preview` model call |
| `gemini` | `GeminiSearchService` | Yes | `google.genai` with `GoogleSearch()` tool |

The search logic currently inside each adapter's `web_search()` moves into these classes. Same SDK calls, different home.

### MiniMax specifics

`MiniMaxSearchService` manages its own MCP client lifecycle. It does not share the singleton in `llm/minimax/mcp_client.py` — that module stays for adapter-level MCP tools (talk, compose, draw). The search service creates its own `MCPClient` with the provided `api_key`.

Alternatively, if talk/compose/draw also migrate to standalone services later, the MCP client could be shared via a utility. But for now, isolation is cleaner.

## Capability Changes (`capabilities/web_search.py`)

### `setup()` signature

```python
def setup(
    agent: BaseAgent,
    provider: str | None = None,
    api_key: str | None = None,
    search_service: SearchService | None = None,
    **kwargs,
) -> WebSearchManager:
```

Resolution order:
1. `search_service` passed directly → use it (programmatic API for custom implementations)
2. `provider` passed → `create_search_service(provider, api_key=api_key, **kwargs)`
3. Neither → `ValueError` at setup time

### `WebSearchManager` simplification

The class shrinks significantly. No more adapter fallback path:

```python
class WebSearchManager:
    def __init__(self, agent, search_service: SearchService) -> None:
        self._agent = agent
        self._search_service = search_service

    def handle(self, args: dict) -> dict:
        query = args.get("query")
        if not query:
            return {"status": "error", "message": "Missing required parameter: query"}
        results = self._search_service.search(query)
        formatted = "\n\n".join(
            f"**{r.title}**\n{r.url}\n{r.snippet}" for r in results
        )
        return {"status": "ok", "results": formatted or "No results found."}
```

## Adapter Cleanup

Remove from all adapters (Anthropic, OpenAI, Gemini, MiniMax):
- `supports_web_search` property
- `web_search()` method

Remove from `lingtai_kernel/llm/base.py`:
- `supports_web_search` property on `LLMAdapter` ABC

MiniMax adapter: `web_search()` removed. Other MCP tool methods (talk, compose, draw) stay on the adapter for now.

## init.json Schema

`manifest.capabilities.web_search` validates:
- `provider`: required `str`
- `api_key`: optional `str | null`

Example configs:

```json
// MiniMax — needs a key
"capabilities": {
    "web_search": {"provider": "minimax", "api_key": "sk-..."}
}

// DuckDuckGo — no key needed
"capabilities": {
    "web_search": {"provider": "duckduckgo"}
}

// Anthropic — needs a key
"capabilities": {
    "web_search": {"provider": "anthropic", "api_key": "sk-ant-..."}
}
```

## Backward Compatibility

### Import paths

`from lingtai.services.search import SearchService, SearchResult` → broken. New path: `from lingtai.services.websearch import SearchService, SearchResult`.

Check all internal imports and update. No re-export shim from old path — clean break per project conventions.

### Programmatic API

`Agent(capabilities={"web_search": {"search_service": my_svc}})` still works — `search_service` takes precedence.

`Agent(capabilities={"web_search": {"provider": "minimax"}})` without `api_key` raises at setup time for providers that need it.

`Agent(capabilities=["web_search"])` without any config → raises `ValueError` (no implicit fallback).

## Tests

Update `tests/test_web_search_capability.py`:
- Test `create_search_service()` factory with valid and invalid providers
- Test each provider validates `api_key` at construction
- Test `WebSearchManager` routes through `SearchService` only
- Test `setup()` error cases (no provider, missing required key)
- Mock SDK calls in provider-specific tests

## Migration Checklist

1. Create `services/websearch/` package with ABC, factory, and all 5 providers
2. Update `capabilities/web_search.py` to use `SearchService` exclusively
3. Remove `web_search()` and `supports_web_search` from all adapters
4. Remove `supports_web_search` from kernel `LLMAdapter` ABC
5. Delete `services/search.py`
6. Update all internal imports
7. Update init.json schema validation
8. Update tests
9. Smoke-test: `python -c "import lingtai"`
