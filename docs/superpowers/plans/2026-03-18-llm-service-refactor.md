# LLMService Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor LLMService to use an adapter registry pattern instead of hardcoded provider imports, and remove multimodal capability methods so LLMService only contains kernel-level concerns.

**Architecture:** Replace `_create_adapter()`'s `if/elif` chain with a class-level registry that adapters register into. Remove 8 multimodal methods (`web_search`, `generate_vision`, `make_multimodal_message`, `generate_image`, `generate_music`, `text_to_speech`, `transcribe`, `analyze_audio`) from LLMService. Update the 2 capabilities that call LLMService multimodal methods (vision, web_search) to call adapters directly. Remove `Agent._CAPABILITY_PROVIDER_KEYS` routing and `provider_config` from LLMService — capabilities resolve their own providers via kwargs.

**Tech Stack:** Python 3.11+, pytest, unittest.mock

**Spec:** `docs/superpowers/specs/2026-03-18-lingtai-kernel-extraction-design.md`

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `src/lingtai/llm/service.py` | Remove multimodal methods, add `register_adapter()`, registry-based `_create_adapter()`, remove `provider_config` param |
| Modify | `src/lingtai/llm/base.py` | Remove multimodal methods from `LLMAdapter` ABC (web_search, generate_vision, generate_image, generate_music, text_to_speech, transcribe, analyze_audio) |
| Create | `src/lingtai/llm/_register.py` | Register all 5 adapter factories with `LLMService` |
| Modify | `src/lingtai/llm/__init__.py` | Auto-call `register_all_adapters()` on import |
| Modify | `src/lingtai/capabilities/vision.py` | Call adapter directly instead of `service.generate_vision()` |
| Modify | `src/lingtai/capabilities/web_search.py` | Call adapter directly instead of `service.web_search()` |
| Modify | `src/lingtai/agent.py` | Remove `_CAPABILITY_PROVIDER_KEYS`, pass provider to capabilities via kwargs instead of poking `service._config` |
| Modify | `src/lingtai/capabilities/delegate.py:113` | Read provider from `cap_kwargs` instead of removed `_capability_providers` |
| Modify | `tests/test_llm_service.py` | Update tests for new registry pattern, remove multimodal routing tests |
| Modify | `tests/test_vision_capability.py` | Update tests to pass `provider` kwarg or use VisionService |
| Modify | `tests/test_web_search_capability.py` | Update tests to pass `provider` kwarg or use SearchService |
| Create | `tests/test_adapter_registry.py` | Test adapter registration and lookup |

---

### Task 1: Add adapter registry to LLMService

**Files:**
- Create: `tests/test_adapter_registry.py`
- Modify: `src/lingtai/llm/service.py:158-230`

- [ ] **Step 1: Write failing tests for adapter registry**

```python
# tests/test_adapter_registry.py
"""Tests for LLMService adapter registry."""
from __future__ import annotations
from unittest.mock import MagicMock
from lingtai.llm.service import LLMService
from lingtai.llm.base import LLMAdapter


def _make_stub_adapter(**kwargs):
    """Factory that returns a MagicMock LLMAdapter."""
    adapter = MagicMock(spec=LLMAdapter)
    adapter._init_kwargs = kwargs
    return adapter


class TestAdapterRegistry:
    def setup_method(self):
        # Save and clear registry for isolation
        self._saved = dict(LLMService._adapter_registry)
        LLMService._adapter_registry.clear()

    def teardown_method(self):
        LLMService._adapter_registry.clear()
        LLMService._adapter_registry.update(self._saved)

    def test_register_and_lookup(self):
        LLMService.register_adapter("test_provider", _make_stub_adapter)
        assert "test_provider" in LLMService._adapter_registry

    def test_register_normalizes_case(self):
        LLMService.register_adapter("TestProvider", _make_stub_adapter)
        assert "testprovider" in LLMService._adapter_registry

    def test_create_adapter_uses_registry(self):
        LLMService.register_adapter("myprovider", _make_stub_adapter)
        svc = LLMService(
            "myprovider", "my-model",
            api_key="test-key",
        )
        # The adapter should have been created via our factory
        adapter = svc.get_adapter("myprovider")
        assert adapter._init_kwargs["api_key"] == "test-key"

    def test_create_adapter_unknown_provider_raises(self):
        import pytest
        with pytest.raises(RuntimeError, match="No adapter registered"):
            LLMService("unknown_provider", "model", api_key="key")

    def test_register_overwrites(self):
        factory_a = MagicMock(return_value=MagicMock(spec=LLMAdapter))
        factory_b = MagicMock(return_value=MagicMock(spec=LLMAdapter))
        LLMService.register_adapter("prov", factory_a)
        LLMService.register_adapter("prov", factory_b)
        LLMService("prov", "model", api_key="key")
        factory_b.assert_called_once()
        factory_a.assert_not_called()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_adapter_registry.py -v`
Expected: FAIL — `register_adapter` does not exist yet

- [ ] **Step 3: Add `register_adapter()` and registry-based `_create_adapter()` to LLMService**

In `src/lingtai/llm/service.py`, add to the `LLMService` class:

```python
class LLMService:
    _adapter_registry: dict[str, Callable[..., LLMAdapter]] = {}

    @classmethod
    def register_adapter(cls, name: str, factory: Callable[..., LLMAdapter]) -> None:
        """Register an adapter factory by provider name.

        The factory receives keyword arguments: api_key, base_url, max_rpm,
        and (for Gemini) default_model.
        """
        cls._adapter_registry[name.lower()] = factory
```

Replace the existing `_create_adapter()` method with:

```python
    def _create_adapter(self, provider: str, api_key: str | None, base_url: str | None) -> LLMAdapter:
        key_kw: dict = {"api_key": api_key} if api_key is not None else {}
        defaults = self._get_provider_defaults(provider)
        effective_url = base_url or (defaults.get("base_url") if defaults else None)
        url_kw: dict = {"base_url": effective_url} if effective_url is not None else {}
        max_rpm = defaults.get("max_rpm", 0) if defaults else 0
        rpm_kw: dict = {"max_rpm": max_rpm} if max_rpm > 0 else {}

        p = provider.lower()
        factory = self._adapter_registry.get(p)
        if factory is None:
            raise RuntimeError(
                f"No adapter registered for provider {provider!r}. "
                f"Registered: {', '.join(sorted(self._adapter_registry)) or '(none)'}. "
                f"If using lingtai, ensure 'import lingtai' runs before creating LLMService."
            )

        # Pass all context to factory — each factory decides what it needs
        return factory(
            model=self._model,
            defaults=defaults,
            **key_kw, **url_kw, **rpm_kw,
        )
```

Note: The factory functions in `_register.py` (Task 2) handle provider-specific argument mapping. This keeps `_create_adapter()` fully provider-agnostic — no `if p == "gemini"` branches.

- [ ] **Step 4: Run registry tests to verify they pass**

Run: `python -m pytest tests/test_adapter_registry.py -v`
Expected: PASS

- [ ] **Step 5: Smoke-test the module**

Run: `python -c "from lingtai.llm.service import LLMService; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add tests/test_adapter_registry.py src/lingtai/llm/service.py
git commit -m "feat(llm): add adapter registry to LLMService"
```

---

### Task 2: Create adapter registration module

**Files:**
- Create: `src/lingtai/llm/_register.py`
- Modify: `src/lingtai/llm/__init__.py`

- [ ] **Step 1: Write failing test for adapter registration**

Add to `tests/test_adapter_registry.py`:

```python
def test_default_adapters_registered():
    """All default adapters should be registered after importing lingtai.llm."""
    from lingtai.llm._register import register_all_adapters
    # Clear and re-register
    saved = dict(LLMService._adapter_registry)
    LLMService._adapter_registry.clear()
    register_all_adapters()
    expected = {"gemini", "anthropic", "openai", "minimax", "deepseek", "grok", "qwen", "glm", "kimi", "custom"}
    assert expected.issubset(set(LLMService._adapter_registry.keys()))
    LLMService._adapter_registry.clear()
    LLMService._adapter_registry.update(saved)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_adapter_registry.py::test_default_adapters_registered -v`
Expected: FAIL — `_register` module does not exist

- [ ] **Step 3: Create `_register.py`**

```python
# src/lingtai/llm/_register.py
"""Register all built-in LLM adapter factories with LLMService.

Each factory uses lazy imports so provider SDKs are only loaded when first used.
Each factory receives (model, defaults, **kw) from _create_adapter() and maps
to the adapter's actual constructor signature.
"""
from __future__ import annotations


def register_all_adapters() -> None:
    from .service import LLMService

    def _gemini(*, model=None, defaults=None, api_key=None, max_rpm=0, **_kw):
        from .gemini.adapter import GeminiAdapter
        kw: dict = {}
        if api_key is not None: kw["api_key"] = api_key
        if max_rpm > 0: kw["max_rpm"] = max_rpm
        if model: kw["default_model"] = model
        return GeminiAdapter(**kw)

    def _anthropic(*, model=None, defaults=None, **kw):
        from .anthropic.adapter import AnthropicAdapter
        kw.pop("model", None)
        return AnthropicAdapter(**{k: v for k, v in kw.items() if v is not None})

    def _openai(*, model=None, defaults=None, **kw):
        from .openai.adapter import OpenAIAdapter
        kw.pop("model", None)
        return OpenAIAdapter(**{k: v for k, v in kw.items() if v is not None})

    def _minimax(*, model=None, defaults=None, **kw):
        from .minimax.adapter import MiniMaxAdapter
        kw.pop("model", None)
        return MiniMaxAdapter(**{k: v for k, v in kw.items() if v is not None})

    def _custom(*, model=None, defaults=None, **kw):
        from .custom.adapter import create_custom_adapter
        kw.pop("model", None)
        compat = defaults.get("api_compat", "openai") if defaults else "openai"
        return create_custom_adapter(api_compat=compat, **{k: v for k, v in kw.items() if v is not None})

    LLMService.register_adapter("gemini", _gemini)
    LLMService.register_adapter("anthropic", _anthropic)
    LLMService.register_adapter("openai", _openai)
    LLMService.register_adapter("minimax", _minimax)
    LLMService.register_adapter("custom", _custom)

    # Providers routed through the custom adapter
    for name in ("deepseek", "grok", "qwen", "glm", "kimi"):
        LLMService.register_adapter(name, _custom)
```

- [ ] **Step 4: Update `__init__.py` to auto-register**

In `src/lingtai/llm/__init__.py`, add at the end:

```python
# Register built-in adapters on import
from ._register import register_all_adapters as _register_all_adapters
_register_all_adapters()
```

- [ ] **Step 5: Run test to verify it passes**

Run: `python -m pytest tests/test_adapter_registry.py::test_default_adapters_registered -v`
Expected: PASS

- [ ] **Step 6: Run the full test suite to verify nothing is broken**

Run: `python -m pytest tests/ -v`
Expected: All tests pass — existing behavior unchanged since old `_create_adapter()` codepath is still present as fallback

- [ ] **Step 7: Smoke-test**

Run: `python -c "from lingtai.llm import LLMService; print(sorted(LLMService._adapter_registry.keys()))"`
Expected: Prints list including gemini, anthropic, openai, minimax, custom, deepseek, etc.

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/llm/_register.py src/lingtai/llm/__init__.py tests/test_adapter_registry.py
git commit -m "feat(llm): register all built-in adapters via registry"
```

---

### Task 3: Remove multimodal methods from LLMAdapter ABC

**Files:**
- Modify: `src/lingtai/llm/base.py:342-401`
- Modify: `tests/test_llm_service.py:17-39`

The multimodal methods on `LLMAdapter` (`web_search`, `generate_vision`, `generate_image`, `generate_music`, `text_to_speech`, `transcribe`, `analyze_audio`) are not kernel concerns. They stay on the individual adapter implementations that support them but are removed from the base ABC.

Note: `make_multimodal_message` must stay on `LLMAdapter` — it's used by `ChatSession.send()` for vision in chat context, which is a core concern (sending multimodal messages in a conversation). The one-shot convenience methods are what we're removing.

- [ ] **Step 1: Verify which adapter implementations override these methods**

Run: `grep -rn "def web_search\|def generate_vision\|def generate_image\|def generate_music\|def text_to_speech\|def transcribe\|def analyze_audio" src/lingtai/llm/`
Check which adapters actually implement these methods (they'll keep their implementations, just no longer inherit the default from ABC).

- [ ] **Step 2: Remove multimodal methods from LLMAdapter in `base.py`**

Delete lines 342-401 of `src/lingtai/llm/base.py` (the `web_search`, `generate_vision`, `generate_image`, `generate_music`, `text_to_speech`, `transcribe`, `analyze_audio` methods). Keep `make_multimodal_message` as it is (abstract method, used for chat-context vision).

- [ ] **Step 3: Update the test for base adapter**

In `tests/test_llm_service.py`, update `test_adapter_generate_image_raises_not_implemented` — remove the assertions for `generate_image`, `generate_music`, `text_to_speech`, `transcribe`, `analyze_audio` since these methods no longer exist on the base class. Delete the entire test function if nothing remains.

- [ ] **Step 4: Run tests**

Run: `python -m pytest tests/test_llm_service.py -v`
Expected: PASS

- [ ] **Step 5: Smoke-test**

Run: `python -c "from lingtai.llm.base import LLMAdapter; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/llm/base.py tests/test_llm_service.py
git commit -m "refactor(llm): remove multimodal methods from LLMAdapter ABC"
```

---

### Task 4: Remove multimodal methods from LLMService

**Files:**
- Modify: `src/lingtai/llm/service.py:277-369`
- Modify: `tests/test_llm_service.py`

- [ ] **Step 1: Remove multimodal methods from LLMService**

Delete lines 277-369 of `src/lingtai/llm/service.py` — the entire "Capability routing" section:
- `web_search()`
- `make_multimodal_message()`
- `generate_vision()`
- `generate_image()`
- `generate_music()`
- `text_to_speech()`
- `transcribe()`
- `analyze_audio()`

- [ ] **Step 2: Update LLMService class docstring**

Remove the mention of "Capability routing" from the docstring since that section is gone. The responsibilities are now: adapter factory (via registry), session registry, one-shot gateway, token accounting.

- [ ] **Step 3: Remove `provider_config` from `__init__`**

In `LLMService.__init__()`, remove the `provider_config` parameter and `self._config` attribute. The `_config` dict was only used by the multimodal routing methods we just removed.

Update the signature:
```python
def __init__(
    self,
    provider: str,
    model: str,
    api_key: str | None = None,
    base_url: str | None = None,
    key_resolver: Callable[[str], str | None] | None = None,
    provider_defaults: dict | None = None,
) -> None:
```

Remove: `self._config = provider_config or {}`

- [ ] **Step 4: Update tests — remove multimodal routing tests**

In `tests/test_llm_service.py`, delete these test functions:
- `test_generate_image_no_provider`
- `test_generate_image_routes_to_adapter`
- `test_text_to_speech_no_provider`
- `test_transcribe_no_provider`

- [ ] **Step 5: Run tests**

Run: `python -m pytest tests/test_llm_service.py tests/test_adapter_registry.py -v`
Expected: PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "from lingtai.llm.service import LLMService; print('OK')"`
Expected: `OK`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/llm/service.py tests/test_llm_service.py
git commit -m "refactor(llm): remove multimodal methods and provider_config from LLMService"
```

---

### Task 5: Update vision capability to call adapter directly

**Files:**
- Modify: `src/lingtai/capabilities/vision.py:79`
- Modify: `src/lingtai/capabilities/vision.py:46-51` (setup function)

The vision capability currently calls `self._agent.service.generate_vision()`. After the refactor, it needs to resolve the provider itself and call the adapter directly.

- [ ] **Step 1: Write failing test**

```python
# Add to tests/test_adapter_registry.py or create tests/test_vision_capability.py

def test_vision_calls_adapter_directly():
    """Vision capability should call adapter.generate_vision() directly."""
    from unittest.mock import MagicMock, patch
    from lingtai.capabilities.vision import VisionManager
    from lingtai.llm.base import LLMResponse

    agent = MagicMock()
    agent._working_dir = "/tmp"

    # Simulate adapter with generate_vision
    adapter = MagicMock()
    adapter.generate_vision.return_value = LLMResponse(text="A cat sitting on a mat")
    agent.service.get_adapter.return_value = adapter
    agent.service._provider_defaults = {"gemini": {"model": "gemini-test"}}
    agent.service._get_provider_defaults.return_value = {"model": "gemini-test"}

    mgr = VisionManager(agent, vision_provider="gemini")

    import tempfile, os
    with tempfile.NamedTemporaryFile(suffix=".png", delete=False) as f:
        f.write(b"fake_png")
        f.flush()
        result = mgr.handle({"image_path": f.name, "question": "What is this?"})
        os.unlink(f.name)

    assert result["status"] == "ok"
    adapter.generate_vision.assert_called_once()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_adapter_registry.py::test_vision_calls_adapter_directly -v`
Expected: FAIL — `VisionManager` doesn't accept `vision_provider` yet

- [ ] **Step 3: Update VisionManager**

In `src/lingtai/capabilities/vision.py`:

```python
class VisionManager:
    """Handles vision tool calls."""

    def __init__(
        self,
        agent: "BaseAgent",
        vision_service: Any | None = None,
        vision_provider: str | None = None,
    ) -> None:
        self._agent = agent
        self._vision_service = vision_service
        self._vision_provider = vision_provider

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

        # Fall back to direct adapter call
        if self._vision_provider is None:
            return {
                "status": "error",
                "message": "Vision provider not configured. Pass provider='...' in capability kwargs.",
            }
        try:
            adapter = self._agent.service.get_adapter(self._vision_provider)
        except RuntimeError:
            return {
                "status": "error",
                "message": f"Vision provider {self._vision_provider!r} not available.",
            }
        image_bytes = path.read_bytes()
        mime = _MIME_BY_EXT.get(path.suffix.lower(), "image/png")
        defaults = self._agent.service._get_provider_defaults(self._vision_provider)
        model = defaults.get("model", "") if defaults else ""
        response = adapter.generate_vision(question, image_bytes, model=model, mime_type=mime)
        if not response.text:
            return {
                "status": "error",
                "message": "Vision analysis returned no response.",
            }
        return {"status": "ok", "analysis": response.text}
```

Update setup function:

```python
def setup(agent: "BaseAgent", vision_service: Any | None = None,
          provider: str | None = None, **kwargs: Any) -> VisionManager:
    """Set up the vision capability on an agent."""
    mgr = VisionManager(agent, vision_service=vision_service, vision_provider=provider)
    agent.add_tool("vision", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
                    system_prompt="Analyze and understand images.")
    return mgr
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_adapter_registry.py::test_vision_calls_adapter_directly -v`
Expected: PASS

- [ ] **Step 5: Update existing vision tests**

In `tests/test_vision_capability.py`, update `test_vision_analyzes_image` and `test_vision_relative_path` to pass `provider` kwarg since the capability now requires it for the LLM fallback path:

```python
def test_vision_analyzes_image(tmp_path):
    """Vision capability should analyze an image file."""
    svc = make_mock_service()
    adapter = MagicMock()
    mock_response = MagicMock()
    mock_response.text = "A cat sitting on a table"
    adapter.generate_vision.return_value = mock_response
    svc.get_adapter.return_value = adapter
    svc._get_provider_defaults.return_value = {"model": "gemini-test"}

    agent = Agent(agent_name="test", service=svc, base_dir=tmp_path,
                       capabilities={"vision": {"provider": "gemini"}})
    img_path = agent.working_dir / "test.png"
    img_path.write_bytes(b"\x89PNG fake image data")
    result = agent._mcp_handlers["vision"]({"image_path": str(img_path)})
    assert result["status"] == "ok"
    assert "cat" in result["analysis"]


def test_vision_relative_path(tmp_path):
    """Vision should resolve relative paths against working directory."""
    svc = make_mock_service()
    adapter = MagicMock()
    mock_response = MagicMock()
    mock_response.text = "An image"
    adapter.generate_vision.return_value = mock_response
    svc.get_adapter.return_value = adapter
    svc._get_provider_defaults.return_value = {"model": "gemini-test"}

    agent = Agent(agent_name="test", service=svc, base_dir=tmp_path,
                       capabilities={"vision": {"provider": "gemini"}})
    img_path = agent.working_dir / "photo.png"
    img_path.write_bytes(b"\x89PNG fake")
    result = agent._mcp_handlers["vision"]({"image_path": "photo.png"})
    assert result["status"] == "ok"
```

- [ ] **Step 6: Run all vision tests**

Run: `python -m pytest tests/test_vision_capability.py -v`
Expected: All PASS

- [ ] **Step 7: Smoke-test**

Run: `python -c "from lingtai.capabilities.vision import VisionManager; print('OK')"`
Expected: `OK`

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/capabilities/vision.py tests/test_adapter_registry.py tests/test_vision_capability.py
git commit -m "refactor(vision): call adapter directly instead of LLMService"
```

---

### Task 6: Update web_search capability to call adapter directly

**Files:**
- Modify: `src/lingtai/capabilities/web_search.py:57`

- [ ] **Step 1: Write failing test**

```python
# Add to tests/test_adapter_registry.py

def test_web_search_calls_adapter_directly():
    """web_search capability should call adapter.web_search() directly."""
    from unittest.mock import MagicMock
    from lingtai.capabilities.web_search import WebSearchManager
    from lingtai.llm.base import LLMResponse

    agent = MagicMock()
    adapter = MagicMock()
    adapter.web_search.return_value = LLMResponse(text="Search results here")
    agent.service.get_adapter.return_value = adapter
    agent.service._get_provider_defaults.return_value = {"model": "gemini-test"}

    mgr = WebSearchManager(agent, web_search_provider="gemini")
    result = mgr.handle({"query": "lingtai agent framework"})
    assert result["status"] == "ok"
    adapter.web_search.assert_called_once()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_adapter_registry.py::test_web_search_calls_adapter_directly -v`
Expected: FAIL — `WebSearchManager` doesn't accept `web_search_provider`

- [ ] **Step 3: Update WebSearchManager**

In `src/lingtai/capabilities/web_search.py`:

```python
class WebSearchManager:
    """Handles web_search tool calls."""

    def __init__(
        self,
        agent: "BaseAgent",
        search_service: Any | None = None,
        web_search_provider: str | None = None,
    ) -> None:
        self._agent = agent
        self._search_service = search_service
        self._web_search_provider = web_search_provider

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

        # Fall back to direct adapter call
        if self._web_search_provider is None:
            return {
                "status": "error",
                "message": "Web search provider not configured. Pass provider='...' in capability kwargs.",
            }
        try:
            adapter = self._agent.service.get_adapter(self._web_search_provider)
        except RuntimeError:
            return {
                "status": "error",
                "message": f"Web search provider {self._web_search_provider!r} not available.",
            }
        defaults = self._agent.service._get_provider_defaults(self._web_search_provider)
        model = defaults.get("model", "") if defaults else ""
        resp = adapter.web_search(query, model=model)
        if not resp.text:
            return {
                "status": "error",
                "message": "Web search returned no results.",
            }
        return {"status": "ok", "results": resp.text}
```

Update setup function:

```python
def setup(agent: "BaseAgent", search_service: Any | None = None,
          provider: str | None = None, **kwargs: Any) -> WebSearchManager:
    """Set up the web_search capability on an agent."""
    mgr = WebSearchManager(agent, search_service=search_service, web_search_provider=provider)
    agent.add_tool("web_search", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
                    system_prompt="Search the web for current information.")
    return mgr
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_adapter_registry.py::test_web_search_calls_adapter_directly -v`
Expected: PASS

- [ ] **Step 5: Update existing web_search tests**

In `tests/test_web_search_capability.py`, update `test_web_search_returns_results` to pass `provider` kwarg:

```python
def test_web_search_returns_results(tmp_path):
    """web_search capability should return search results."""
    svc = make_mock_service()
    adapter = MagicMock()
    mock_response = MagicMock()
    mock_response.text = "Python is a programming language..."
    adapter.web_search.return_value = mock_response
    svc.get_adapter.return_value = adapter
    svc._get_provider_defaults.return_value = {"model": "gemini-test"}

    agent = Agent(agent_name="test", service=svc, base_dir=tmp_path,
                       capabilities={"web_search": {"provider": "gemini"}})
    result = agent._mcp_handlers["web_search"]({"query": "what is python"})
    assert result["status"] == "ok"
    assert "Python" in result["results"]
```

- [ ] **Step 6: Run all web_search tests**

Run: `python -m pytest tests/test_web_search_capability.py -v`
Expected: All PASS

- [ ] **Step 7: Smoke-test**

Run: `python -c "from lingtai.capabilities.web_search import WebSearchManager; print('OK')"`
Expected: `OK`

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/capabilities/web_search.py tests/test_adapter_registry.py tests/test_web_search_capability.py
git commit -m "refactor(web_search): call adapter directly instead of LLMService"
```

---

### Task 7: Remove provider routing from Agent

**Files:**
- Modify: `src/lingtai/agent.py:27-75`

The `_CAPABILITY_PROVIDER_KEYS` mapping and the code that pokes providers into `service._config` must be removed. Instead, the `provider` kwarg is passed through to each capability's `setup()` function naturally (it already goes through `**cap_kwargs`).

- [ ] **Step 1: Write test for provider passthrough**

```python
# Add to tests/test_adapter_registry.py

def test_agent_passes_provider_to_capability():
    """Agent should pass provider kwarg through to capability setup."""
    from unittest.mock import MagicMock, patch
    from lingtai.agent import Agent

    # We just need to verify that provider= reaches the capability setup
    with patch.object(Agent, '__init__', lambda self, *a, **kw: None):
        agent = Agent.__new__(Agent)

    # Verify the _CAPABILITY_PROVIDER_KEYS class attribute is gone
    assert not hasattr(Agent, '_CAPABILITY_PROVIDER_KEYS')
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_adapter_registry.py::test_agent_passes_provider_to_capability -v`
Expected: FAIL — `_CAPABILITY_PROVIDER_KEYS` still exists

- [ ] **Step 3: Update Agent**

In `src/lingtai/agent.py`, remove:
- The `_CAPABILITY_PROVIDER_KEYS` class attribute (lines 28-35)
- The provider routing loop (lines 65-75) that pokes into `service._config`
- The `_capability_providers` attribute
- The `cap_kwargs.pop("provider", None)` call — `provider` should flow through to capabilities naturally

The `provider` kwarg in `capabilities={"vision": {"provider": "gemini"}}` already passes through to `setup_capability()` → `setup()` → capability manager constructor via `**cap_kwargs`. No special routing needed.

- [ ] **Step 4: Update delegate capability**

In `src/lingtai/capabilities/delegate.py`, update the provider inheritance at line 113. Since `_capability_providers` is removed and `provider` now stays in `cap_kwargs` (no longer popped), read it directly:

Replace:
```python
child_cap = dict(cap_kwargs)
cap_provider = parent._capability_providers.get(cap_name)
if cap_provider:
    child_cap["provider"] = cap_provider
caps[cap_name] = child_cap
```

With:
```python
caps[cap_name] = dict(cap_kwargs)  # provider is already in cap_kwargs
```

Updated `__init__`:

```python
def __init__(
    self,
    *args: Any,
    capabilities: list[str] | dict[str, dict] | None = None,
    addons: dict[str, dict] | None = None,
    **kwargs: Any,
):
    super().__init__(*args, **kwargs)

    # Auto-load MCP servers from working directory
    self._load_mcp_from_workdir()

    # Expand groups and normalize to dict
    if isinstance(capabilities, list):
        from .capabilities import expand_groups
        expanded = expand_groups(capabilities)
        capabilities = {name: {} for name in expanded}
    elif isinstance(capabilities, dict):
        from .capabilities import _GROUPS
        expanded_dict: dict[str, dict] = {}
        for name, cap_kwargs in capabilities.items():
            if name in _GROUPS:
                for sub in _GROUPS[name]:
                    expanded_dict[sub] = {}
            else:
                expanded_dict[name] = cap_kwargs
        capabilities = expanded_dict

    # Track for delegate replay
    self._capabilities: list[tuple[str, dict]] = []
    self._capability_managers: dict[str, Any] = {}

    # Register capabilities
    if capabilities:
        for name, cap_kwargs in capabilities.items():
            self._setup_capability(name, **cap_kwargs)

    # Register addons (after capabilities, may depend on them)
    self._addon_managers: dict[str, Any] = {}
    if addons:
        from .addons import setup_addon
        for addon_name, addon_kwargs in addons.items():
            mgr = setup_addon(self, addon_name, **(addon_kwargs or {}))
            self._addon_managers[addon_name] = mgr
```

- [ ] **Step 5: Run test to verify it passes**

Run: `python -m pytest tests/test_adapter_registry.py::test_agent_passes_provider_to_capability -v`
Expected: PASS

- [ ] **Step 6: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All tests pass

- [ ] **Step 7: Smoke-test**

Run: `python -c "from lingtai import Agent; print('OK')"`
Expected: `OK`

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/agent.py src/lingtai/capabilities/delegate.py tests/test_adapter_registry.py
git commit -m "refactor(agent): remove provider routing, pass provider to capabilities directly"
```

---

### Task 8: Final verification and cleanup

**Files:**
- All modified files

- [ ] **Step 1: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All tests pass

- [ ] **Step 2: Smoke-test all modified modules**

```bash
python -c "from lingtai.llm.service import LLMService; print('LLMService OK')"
python -c "from lingtai.llm.base import LLMAdapter; print('LLMAdapter OK')"
python -c "from lingtai.llm._register import register_all_adapters; print('register OK')"
python -c "from lingtai.capabilities.vision import VisionManager; print('vision OK')"
python -c "from lingtai.capabilities.web_search import WebSearchManager; print('web_search OK')"
python -c "from lingtai.agent import Agent; print('Agent OK')"
python -c "import lingtai; print('lingtai OK')"
```

- [ ] **Step 3: Verify LLMService is now kernel-ready**

Confirm that `src/lingtai/llm/service.py` has:
- No imports from adapter subdirectories (gemini, anthropic, etc.)
- No multimodal methods
- No `provider_config` parameter
- A `register_adapter()` class method
- A registry-based `_create_adapter()`

- [ ] **Step 4: Commit any cleanup**

```bash
git add -A
git commit -m "chore: final cleanup after LLMService refactor"
```
