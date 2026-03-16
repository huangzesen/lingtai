# Move Vision & Web Search to Capabilities — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `vision` and `web_search` from intrinsics to capabilities, simplifying the base agent constructor and making perception opt-in.

**Architecture:** Remove vision/web_search from `ALL_INTRINSICS` and `_wire_intrinsics`. Remove `vision=` and `search=` constructor params and `_vision_service`/`_search_service` fields. Create capability modules that call `agent.add_tool()` with the same handler logic. The capabilities access `agent.service` (LLMService) directly for fallback LLM calls, and optionally accept a dedicated service via kwargs.

**Tech Stack:** Python stdlib only

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/stoai/capabilities/vision.py` | Modify | Already exists (MiniMax draw). WAIT — this is `draw.py`. `vision.py` doesn't exist. Create it. |
| `src/stoai/capabilities/web_search.py` | Create | Web search capability module |
| `src/stoai/intrinsics/vision.py` | Delete | No longer an intrinsic |
| `src/stoai/intrinsics/web_search.py` | Delete | No longer an intrinsic |
| `src/stoai/intrinsics/__init__.py` | Modify | Remove vision and web_search from registry |
| `src/stoai/agent.py` | Modify | Remove `vision=`/`search=` params, `_vision_service`/`_search_service`, `_handle_vision`/`_handle_web_search`, MIME dict, wiring |
| `tests/test_agent.py` | Modify | Update intrinsic count, remove vision/web_search assertions |
| `tests/test_vision_capability.py` | Create | Tests for vision capability |
| `tests/test_web_search_capability.py` | Create | Tests for web_search capability |

---

## Chunk 1: Create capabilities + remove intrinsics

### Task 1: Create vision capability

**Files:**
- Create: `src/stoai/capabilities/vision.py`

- [ ] **Step 1: Write failing test**

Create `tests/test_vision_capability.py`:

```python
"""Tests for vision capability."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from stoai.agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_vision_not_intrinsic(tmp_path):
    """Vision should NOT be an intrinsic anymore."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "vision" not in agent._intrinsics


def test_vision_added_by_capability(tmp_path):
    """add_capability('vision') should register the vision tool."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_capability("vision")
    assert "vision" in agent._mcp_handlers


def test_vision_analyzes_image(tmp_path):
    """Vision capability should analyze an image file."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    # Mock generate_vision on the LLM service
    mock_response = MagicMock()
    mock_response.text = "A cat sitting on a table"
    agent.service.generate_vision = MagicMock(return_value=mock_response)

    agent.add_capability("vision")

    # Create a fake image file
    img_path = agent.working_dir / "test.png"
    img_path.write_bytes(b"\x89PNG fake image data")

    result = agent._mcp_handlers["vision"]({"image_path": str(img_path)})
    assert result["status"] == "ok"
    assert "cat" in result["analysis"]


def test_vision_with_dedicated_service(tmp_path):
    """Vision capability should use VisionService if provided."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    mock_vision_svc = MagicMock()
    mock_vision_svc.analyze_image.return_value = "A dog in the park"

    agent.add_capability("vision", vision_service=mock_vision_svc)

    img_path = agent.working_dir / "test.jpg"
    img_path.write_bytes(b"\xff\xd8\xff fake jpeg")

    result = agent._mcp_handlers["vision"]({"image_path": str(img_path)})
    assert result["status"] == "ok"
    assert "dog" in result["analysis"]
    mock_vision_svc.analyze_image.assert_called_once()


def test_vision_missing_image(tmp_path):
    """Vision should return error for missing image file."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_capability("vision")

    result = agent._mcp_handlers["vision"]({"image_path": "/nonexistent/image.png"})
    assert "error" in result or result.get("status") == "error"


def test_vision_relative_path(tmp_path):
    """Vision should resolve relative paths against working directory."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    mock_response = MagicMock()
    mock_response.text = "An image"
    agent.service.generate_vision = MagicMock(return_value=mock_response)

    agent.add_capability("vision")

    img_path = agent.working_dir / "photo.png"
    img_path.write_bytes(b"\x89PNG fake")

    result = agent._mcp_handlers["vision"]({"image_path": "photo.png"})
    assert result["status"] == "ok"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_vision_capability.py -v`
Expected: FAIL — vision is still an intrinsic, capability module doesn't exist

- [ ] **Step 3: Create vision capability module**

Create `src/stoai/capabilities/vision.py`:

```python
"""Vision capability — image understanding via LLM or VisionService.

Adds the ability to analyze images. Uses VisionService if provided,
otherwise falls back to the LLM's multimodal vision endpoint.

Usage:
    agent.add_capability("vision")  # uses LLM fallback
    agent.add_capability("vision", vision_service=my_svc)  # uses dedicated service
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "image_path": {"type": "string", "description": "Path to the image file"},
        "question": {
            "type": "string",
            "description": "Question about the image",
            "default": "Describe this image.",
        },
    },
    "required": ["image_path"],
}

DESCRIPTION = (
    "Analyze an image using the LLM's vision capabilities. "
    "Supports JPEG, PNG, and WebP. Ask any question about the image — "
    "describe contents, read text, interpret charts, identify objects, "
    "assess style or mood. Combine with draw to generate then analyze images."
)

_MIME_BY_EXT: dict[str, str] = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".webp": "image/webp",
}


class VisionManager:
    """Handles vision tool calls."""

    def __init__(self, agent: "BaseAgent", vision_service: Any | None = None) -> None:
        self._agent = agent
        self._vision_service = vision_service

    def handle(self, args: dict) -> dict:
        image_path = args.get("image_path", "")
        question = args.get("question", "Describe what you see in this image.")

        if not image_path:
            return {"status": "error", "message": "Provide image_path"}

        path = Path(image_path)
        if not path.is_absolute():
            path = self._agent._working_dir / path

        if not path.is_file():
            return {"status": "error", "message": f"Image file not found: {path}"}

        # Try VisionService first
        if self._vision_service is not None:
            try:
                analysis = self._vision_service.analyze_image(str(path), prompt=question)
                return {"status": "ok", "analysis": analysis}
            except NotImplementedError:
                pass  # Fall through to direct LLM call

        # Fall back to direct LLM multimodal call
        image_bytes = path.read_bytes()
        mime = _MIME_BY_EXT.get(path.suffix.lower(), "image/png")

        response = self._agent.service.generate_vision(question, image_bytes, mime_type=mime)
        if not response.text:
            return {
                "status": "error",
                "message": "Vision analysis returned no response — vision provider may not be configured.",
            }
        return {"status": "ok", "analysis": response.text}


def setup(agent: "BaseAgent", vision_service: Any | None = None, **kwargs: Any) -> VisionManager:
    """Set up the vision capability on an agent."""
    mgr = VisionManager(agent, vision_service=vision_service)
    agent.add_tool("vision", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    return mgr
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_vision_capability.py -v`
Note: `test_vision_not_intrinsic` will STILL FAIL because vision is still in the intrinsics registry. That's OK — we'll fix it in Task 3.

Run only the capability tests:
`python -m pytest tests/test_vision_capability.py -k "not not_intrinsic" -v`
Expected: PASS

- [ ] **Step 5: Smoke-test**

Run: `python -c "import stoai"`

- [ ] **Step 6: Commit**

```bash
git add src/stoai/capabilities/vision.py tests/test_vision_capability.py
git commit -m "feat: create vision capability module"
```

### Task 2: Create web_search capability

**Files:**
- Create: `src/stoai/capabilities/web_search.py`

- [ ] **Step 1: Write failing test**

Create `tests/test_web_search_capability.py`:

```python
"""Tests for web_search capability."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from stoai.agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_web_search_not_intrinsic(tmp_path):
    """web_search should NOT be an intrinsic anymore."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "web_search" not in agent._intrinsics


def test_web_search_added_by_capability(tmp_path):
    """add_capability('web_search') should register the web_search tool."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_capability("web_search")
    assert "web_search" in agent._mcp_handlers


def test_web_search_returns_results(tmp_path):
    """web_search capability should return search results."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    mock_response = MagicMock()
    mock_response.text = "Python is a programming language..."
    agent.service.web_search = MagicMock(return_value=mock_response)

    agent.add_capability("web_search")

    result = agent._mcp_handlers["web_search"]({"query": "what is python"})
    assert result["status"] == "ok"
    assert "Python" in result["results"]


def test_web_search_with_dedicated_service(tmp_path):
    """web_search capability should use SearchService if provided."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    mock_result = MagicMock()
    mock_result.title = "Python"
    mock_result.url = "https://python.org"
    mock_result.snippet = "Python programming language"
    mock_search_svc = MagicMock()
    mock_search_svc.search.return_value = [mock_result]

    agent.add_capability("web_search", search_service=mock_search_svc)

    result = agent._mcp_handlers["web_search"]({"query": "python"})
    assert result["status"] == "ok"
    assert "Python" in result["results"]
    mock_search_svc.search.assert_called_once()


def test_web_search_missing_query(tmp_path):
    """web_search should return error for missing query."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_capability("web_search")

    result = agent._mcp_handlers["web_search"]({"query": ""})
    assert "error" in result or result.get("status") == "error"
```

- [ ] **Step 2: Run test (skip not_intrinsic for now)**

Run: `python -m pytest tests/test_web_search_capability.py -k "not not_intrinsic" -v`
Expected: FAIL — capability module doesn't exist

- [ ] **Step 3: Create web_search capability module**

Create `src/stoai/capabilities/web_search.py`:

```python
"""Web search capability — web lookup via LLM or SearchService.

Adds the ability to search the web. Uses SearchService if provided,
otherwise falls back to the LLM's grounding/search endpoint.

Usage:
    agent.add_capability("web_search")  # uses LLM fallback
    agent.add_capability("web_search", search_service=my_svc)  # uses dedicated service
"""
from __future__ import annotations

from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "query": {"type": "string", "description": "Search query"},
    },
    "required": ["query"],
}

DESCRIPTION = (
    "Search the web for current information. "
    "Use for real-time data, recent events, documentation, "
    "or anything beyond your training knowledge. "
    "Returns ranked search results with titles, URLs, and snippets."
)


class WebSearchManager:
    """Handles web_search tool calls."""

    def __init__(self, agent: "BaseAgent", search_service: Any | None = None) -> None:
        self._agent = agent
        self._search_service = search_service

    def handle(self, args: dict) -> dict:
        query = args.get("query")
        if not query:
            return {"status": "error", "message": "Missing required parameter: query"}

        # Try SearchService first
        if self._search_service is not None:
            try:
                results = self._search_service.search(query)
                formatted = "\n\n".join(
                    f"**{r.title}**\n{r.url}\n{r.snippet}" for r in results
                )
                return {"status": "ok", "results": formatted or "No results found."}
            except NotImplementedError:
                pass  # Fall through to direct LLM call

        # Fall back to direct LLM grounding call
        resp = self._agent.service.web_search(query)
        if not resp.text:
            return {
                "status": "error",
                "message": "Web search returned no results. The web search provider may not be configured.",
            }
        return {"status": "ok", "results": resp.text}


def setup(agent: "BaseAgent", search_service: Any | None = None, **kwargs: Any) -> WebSearchManager:
    """Set up the web_search capability on an agent."""
    mgr = WebSearchManager(agent, search_service=search_service)
    agent.add_tool("web_search", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    return mgr
```

- [ ] **Step 4: Run tests**

Run: `python -m pytest tests/test_web_search_capability.py -k "not not_intrinsic" -v`
Expected: PASS

- [ ] **Step 5: Smoke-test**

Run: `python -c "import stoai"`

- [ ] **Step 6: Commit**

```bash
git add src/stoai/capabilities/web_search.py tests/test_web_search_capability.py
git commit -m "feat: create web_search capability module"
```

### Task 3: Remove vision and web_search from intrinsics

**Files:**
- Delete: `src/stoai/intrinsics/vision.py`
- Delete: `src/stoai/intrinsics/web_search.py`
- Modify: `src/stoai/intrinsics/__init__.py`
- Modify: `src/stoai/agent.py`
- Modify: `tests/test_agent.py`

- [ ] **Step 1: Add to capabilities registry**

In `src/stoai/capabilities/__init__.py`, add vision and web_search to `_BUILTIN`:

```python
_BUILTIN: dict[str, str] = {
    "bash": ".bash",
    "delegate": ".delegate",
    "email": ".email",
    "draw": ".draw",
    "compose": ".compose",
    "talk": ".talk",
    "listen": ".listen",
    "vision": ".vision",
    "web_search": ".web_search",
}
```

- [ ] **Step 2: Remove from intrinsics registry**

In `src/stoai/intrinsics/__init__.py`, remove `vision` and `web_search` from the import and `ALL_INTRINSICS`:

```python
from . import read, edit, write, glob, grep, mail, clock, status, memory

ALL_INTRINSICS = {
    "read": {"schema": read.SCHEMA, "description": read.DESCRIPTION, "handler": read.handle_read},
    "edit": {"schema": edit.SCHEMA, "description": edit.DESCRIPTION, "handler": edit.handle_edit},
    "write": {"schema": write.SCHEMA, "description": write.DESCRIPTION, "handler": write.handle_write},
    "glob": {"schema": glob.SCHEMA, "description": glob.DESCRIPTION, "handler": glob.handle_glob},
    "grep": {"schema": grep.SCHEMA, "description": grep.DESCRIPTION, "handler": grep.handle_grep},
    "mail": {"schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handler": None},
    "clock": {"schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handler": None},
    "status": {"schema": status.SCHEMA, "description": status.DESCRIPTION, "handler": None},
    "memory": {"schema": memory.SCHEMA, "description": memory.DESCRIPTION, "handler": None},
}
```

- [ ] **Step 3: Remove from agent.py**

In `src/stoai/agent.py`, make these removals:

**a) Remove `_MIME_BY_EXT` dict** (lines 149-157 — the MIME types constant at module level).

**b) Remove `vision=` and `search=` constructor params** (lines 200-201). Remove them from the `__init__` signature.

**c) Remove service wiring** in `__init__` (lines 267-276):
```python
        # VisionService: auto-create from LLM if not provided
        if vision is not None:
            self._vision_service = vision
        else:
            self._vision_service = None  # will fall back to direct LLM call

        # SearchService: auto-create from LLM if not provided
        if search is not None:
            self._search_service = search
        else:
            self._search_service = None  # will fall back to direct LLM call
```
Remove this entire block.

**d) Remove wiring in `_wire_intrinsics`** (lines 394-396):
```python
        # Vision and web_search always available (fall back to direct LLM calls)
        state_intrinsics["vision"] = self._handle_vision
        state_intrinsics["web_search"] = self._handle_web_search
```
Remove these lines.

**e) Remove handler methods** `_handle_vision` and `_handle_web_search` (approximately lines 613-672). Remove both methods entirely.

**f) Update class docstring** — remove references to vision and search services from the docstring.

- [ ] **Step 4: Delete intrinsic schema files**

```bash
rm src/stoai/intrinsics/vision.py
rm src/stoai/intrinsics/web_search.py
```

- [ ] **Step 5: Update test_agent.py**

In `tests/test_agent.py`:

**a) In `test_intrinsics_enabled_by_default`**: remove assertions for vision and web_search, update count:
```python
    # These were removed — no longer asserting vision/web_search
    assert len(agent._intrinsics) == 9  # read, edit, write, glob, grep, mail, clock, status, memory
```

**b) In `test_disabled_intrinsics`**: change the disabled set to not include vision:
```python
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        disabled_intrinsics={"mail", "clock"},
        base_dir=tmp_path,
    )
    assert "mail" not in agent._intrinsics
    assert "clock" not in agent._intrinsics
    assert "read" in agent._intrinsics
```

**c) In `test_enabled_intrinsics`**: vision is no longer valid, update:
```python
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        enabled_intrinsics={"read", "write"},
        base_dir=tmp_path,
    )
    assert "read" in agent._intrinsics
    assert "write" in agent._intrinsics
    assert "mail" not in agent._intrinsics
    assert "clock" not in agent._intrinsics
```

**d) Remove any tests that pass `vision=` to BaseAgent constructor.** Search for `vision=` in the test file.

- [ ] **Step 6: Update examples**

In `examples/three_agents.py` and `examples/two_agents.py`, find any agents that reference `web_search` or `vision` in system prompts and add `agent.add_capability("web_search")` / `agent.add_capability("vision")` calls where appropriate. Or remove the system prompt references if the example doesn't need those capabilities.

- [ ] **Step 7: Run all tests**

Run: `python -m pytest tests/ -v`
Expected: ALL PASS (including the `not_intrinsic` tests from Tasks 1 and 2)

- [ ] **Step 8: Smoke-test**

Run: `python -c "import stoai"`

- [ ] **Step 9: Commit**

```bash
git rm src/stoai/intrinsics/vision.py src/stoai/intrinsics/web_search.py
git add src/stoai/intrinsics/__init__.py src/stoai/capabilities/__init__.py src/stoai/agent.py tests/test_agent.py examples/
git commit -m "refactor: move vision and web_search from intrinsics to capabilities"
```

### Task 4: Update CLAUDE.md and memory

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update CLAUDE.md**

In `CLAUDE.md`, update the architecture sections:

**a) Six Services table**: Remove VisionService and SearchService rows, or mark them as "capability-level, not agent-level".

**b) Three-Tier Tool Model**: Update the intrinsics list to remove vision and web_search. Add vision and web_search to the capabilities list.

**c) Key Modules intrinsics section**: Update to reflect 9 intrinsics, not 11. Note vision and web_search moved to capabilities.

**d) Update service docstrings**: In `src/stoai/services/vision.py` and `src/stoai/services/search.py`, change references from "intrinsic" to "capability".

- [ ] **Step 2: Run full test suite one final time**

Run: `python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for vision/web_search capability migration"
```
