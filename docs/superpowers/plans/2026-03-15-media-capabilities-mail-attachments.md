# Media Capabilities & Mail Attachments Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add four media capabilities (draw, compose, talk, listen) and filesystem-based mail attachments with inline file transfer.

**Architecture:** Media capabilities are opt-in via `add_capability()`, routing through LLMService gateway methods to LLMAdapter methods. Mail attachments extend the existing FIFO with base64-encoded inline transfer over TCP and a filesystem-based mailbox (`mailbox/<uuid>/` per message).

**Tech Stack:** Python 3.11+, dataclasses, `uuid4`, `base64`, `pathlib`, `unittest.mock`

**Spec:** `docs/superpowers/specs/2026-03-15-media-capabilities-design.md`

---

## Chunk 1: LLM Layer Extensions + BaseAgent Property

### Task 1: Add `working_dir` property to BaseAgent

**Files:**
- Modify: `src/lingtai/agent.py:569-575` (properties section)
- Test: `tests/test_agent.py`

- [ ] **Step 1: Write the failing test**

In `tests/test_agent.py`, add:

```python
def test_working_dir_property(tmp_path):
    from lingtai.agent import BaseAgent
    from unittest.mock import MagicMock
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    agent = BaseAgent(agent_id="test", service=svc, working_dir=str(tmp_path))
    assert agent.working_dir == tmp_path

def test_working_dir_property_default():
    from lingtai.agent import BaseAgent
    from pathlib import Path
    from unittest.mock import MagicMock
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    agent = BaseAgent(agent_id="test", service=svc)
    assert agent.working_dir == Path.cwd()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_agent.py::test_working_dir_property tests/test_agent.py::test_working_dir_property_default -v`
Expected: FAIL with `AttributeError: 'BaseAgent' object has no attribute 'working_dir'`

- [ ] **Step 3: Add the property**

In `src/lingtai/agent.py`, after the `state` property (~line 575), add:

```python
@property
def working_dir(self) -> Path:
    """The agent's working directory."""
    return self._working_dir
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_agent.py::test_working_dir_property tests/test_agent.py::test_working_dir_property_default -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "import lingtai"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/agent.py tests/test_agent.py
git commit -m "feat: add public working_dir property to BaseAgent"
```

---

### Task 2: Add LLMAdapter media methods

**Files:**
- Modify: `src/lingtai/llm/base.py:335-357` (after `generate_vision` method)
- Test: `tests/test_llm_service.py`

- [ ] **Step 1: Write the failing test**

In `tests/test_llm_service.py`, add:

```python
def test_adapter_generate_image_raises_not_implemented():
    """Base LLMAdapter media methods raise NotImplementedError."""
    from lingtai.llm.base import LLMAdapter
    from unittest.mock import MagicMock
    # Create a minimal concrete subclass (all abstract methods must be overridden)
    class StubAdapter(LLMAdapter):
        def create_chat(self, *a, **kw): pass
        def generate(self, *a, **kw): return LLMResponse()
        def make_tool_result_message(self, *a, **kw): pass
        def make_multimodal_message(self, *a, **kw): pass
        def is_quota_error(self, *a, **kw): return False
    from lingtai.llm.base import LLMResponse
    adapter = StubAdapter()
    import pytest
    with pytest.raises(NotImplementedError):
        adapter.generate_image("a cat", model="test")
    with pytest.raises(NotImplementedError):
        adapter.generate_music("jazz", model="test")
    with pytest.raises(NotImplementedError):
        adapter.text_to_speech("hello", model="test")
    with pytest.raises(NotImplementedError):
        adapter.transcribe(b"audio", model="test")
    with pytest.raises(NotImplementedError):
        adapter.analyze_audio(b"audio", "what is this?", model="test")
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_llm_service.py::test_adapter_generate_image_raises_not_implemented -v`
Expected: FAIL with `AttributeError: 'StubAdapter' object has no attribute 'generate_image'`

- [ ] **Step 3: Add the five methods to LLMAdapter**

In `src/lingtai/llm/base.py`, after the `generate_vision` method (~line 357), add:

```python
def generate_image(self, prompt: str, model: str) -> bytes:
    """Text-to-image generation. Returns image bytes (PNG).

    Override in adapters that support image generation.
    Default: raises NotImplementedError.
    """
    raise NotImplementedError

def generate_music(
    self, prompt: str, model: str, duration_seconds: float | None = None,
) -> bytes:
    """Text-to-music generation. Returns audio bytes.

    Override in adapters that support music generation.
    Default: raises NotImplementedError.
    """
    raise NotImplementedError

def text_to_speech(self, text: str, model: str) -> bytes:
    """Text-to-speech synthesis. Returns audio bytes.

    Override in adapters that support TTS.
    Default: raises NotImplementedError.
    """
    raise NotImplementedError

def transcribe(self, audio_bytes: bytes, model: str) -> str:
    """Speech-to-text transcription. Returns transcription text.

    Override in adapters that support transcription.
    Default: raises NotImplementedError.
    """
    raise NotImplementedError

def analyze_audio(self, audio_bytes: bytes, prompt: str, model: str) -> str:
    """Audio analysis / description. Returns text analysis.

    Override in adapters that support audio understanding.
    Default: raises NotImplementedError.
    """
    raise NotImplementedError
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_llm_service.py::test_adapter_generate_image_raises_not_implemented -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from lingtai.llm.base import LLMAdapter"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/llm/base.py tests/test_llm_service.py
git commit -m "feat: add media generation/recognition methods to LLMAdapter"
```

---

### Task 3: Add LLMService gateway methods

**Files:**
- Modify: `src/lingtai/llm/service.py:353-357` (after `generate_vision` method)
- Test: `tests/test_llm_service.py`

- [ ] **Step 1: Write the failing tests**

In `tests/test_llm_service.py`, add:

```python
def test_generate_image_no_provider():
    """generate_image raises RuntimeError when image_provider not configured."""
    from lingtai.llm.service import LLMService
    from unittest.mock import MagicMock, patch
    import pytest
    # Patch _create_adapter to avoid real SDK imports
    with patch.object(LLMService, '_create_adapter', return_value=MagicMock()):
        svc = LLMService("gemini", "gemini-test", key_resolver=lambda p: "key", provider_defaults={})
    with pytest.raises(RuntimeError, match="image_provider"):
        svc.generate_image("a cat")

def test_generate_image_routes_to_adapter():
    """generate_image routes to the configured adapter."""
    from lingtai.llm.service import LLMService
    from unittest.mock import MagicMock, patch
    adapter = MagicMock()
    adapter.generate_image.return_value = b"PNG_BYTES"
    with patch.object(LLMService, '_create_adapter', return_value=MagicMock()):
        svc = LLMService(
            "gemini", "gemini-test",
            key_resolver=lambda p: "key",
            provider_defaults={"minimax": {"model": "mm-img"}},
        )
    svc._config["image_provider"] = "minimax"
    # _adapters is keyed by (provider, base_url) tuple
    svc._adapters[("minimax", None)] = adapter
    result = svc.generate_image("a cat")
    assert result == b"PNG_BYTES"
    adapter.generate_image.assert_called_once_with("a cat", model="mm-img")

def test_text_to_speech_no_provider():
    from lingtai.llm.service import LLMService
    from unittest.mock import MagicMock, patch
    import pytest
    with patch.object(LLMService, '_create_adapter', return_value=MagicMock()):
        svc = LLMService("gemini", "gemini-test", key_resolver=lambda p: "key", provider_defaults={})
    with pytest.raises(RuntimeError, match="tts_provider"):
        svc.text_to_speech("hello")

def test_transcribe_no_provider():
    from lingtai.llm.service import LLMService
    from unittest.mock import MagicMock, patch
    import pytest
    with patch.object(LLMService, '_create_adapter', return_value=MagicMock()):
        svc = LLMService("gemini", "gemini-test", key_resolver=lambda p: "key", provider_defaults={})
    with pytest.raises(RuntimeError, match="audio_provider"):
        svc.transcribe(b"audio")
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_llm_service.py::test_generate_image_no_provider tests/test_llm_service.py::test_generate_image_routes_to_adapter tests/test_llm_service.py::test_text_to_speech_no_provider tests/test_llm_service.py::test_transcribe_no_provider -v`
Expected: FAIL with `AttributeError`

- [ ] **Step 3: Add the five gateway methods to LLMService**

In `src/lingtai/llm/service.py`, after the `generate_vision` method, add:

```python
def generate_image(self, prompt: str) -> bytes:
    """Text-to-image — routed to configured image_provider."""
    provider_name = self._config.get("image_provider")
    if provider_name is None:
        raise RuntimeError("No image_provider configured")
    adapter = self.get_adapter(provider_name)
    defaults = self._get_provider_defaults(provider_name)
    model = defaults.get("model", "") if defaults else ""
    return adapter.generate_image(prompt, model=model)

def generate_music(self, prompt: str, duration_seconds: float | None = None) -> bytes:
    """Text-to-music — routed to configured music_provider."""
    provider_name = self._config.get("music_provider")
    if provider_name is None:
        raise RuntimeError("No music_provider configured")
    adapter = self.get_adapter(provider_name)
    defaults = self._get_provider_defaults(provider_name)
    model = defaults.get("model", "") if defaults else ""
    return adapter.generate_music(prompt, model=model, duration_seconds=duration_seconds)

def text_to_speech(self, text: str) -> bytes:
    """TTS — routed to configured tts_provider."""
    provider_name = self._config.get("tts_provider")
    if provider_name is None:
        raise RuntimeError("No tts_provider configured")
    adapter = self.get_adapter(provider_name)
    defaults = self._get_provider_defaults(provider_name)
    model = defaults.get("model", "") if defaults else ""
    return adapter.text_to_speech(text, model=model)

def transcribe(self, audio_bytes: bytes) -> str:
    """Speech-to-text — routed to configured audio_provider."""
    provider_name = self._config.get("audio_provider")
    if provider_name is None:
        raise RuntimeError("No audio_provider configured")
    adapter = self.get_adapter(provider_name)
    defaults = self._get_provider_defaults(provider_name)
    model = defaults.get("model", "") if defaults else ""
    return adapter.transcribe(audio_bytes, model=model)

def analyze_audio(self, audio_bytes: bytes, prompt: str) -> str:
    """Audio analysis — routed to configured audio_provider."""
    provider_name = self._config.get("audio_provider")
    if provider_name is None:
        raise RuntimeError("No audio_provider configured")
    adapter = self.get_adapter(provider_name)
    defaults = self._get_provider_defaults(provider_name)
    model = defaults.get("model", "") if defaults else ""
    return adapter.analyze_audio(audio_bytes, prompt, model=model)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_llm_service.py::test_generate_image_no_provider tests/test_llm_service.py::test_generate_image_routes_to_adapter tests/test_llm_service.py::test_text_to_speech_no_provider tests/test_llm_service.py::test_transcribe_no_provider -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from lingtai.llm.service import LLMService"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/llm/service.py tests/test_llm_service.py
git commit -m "feat: add media gateway methods to LLMService"
```

---

## Chunk 2: Four Media Capabilities

### Task 4: Create `draw` capability

**Files:**
- Create: `src/lingtai/capabilities/draw.py`
- Test: `tests/test_layers_draw.py`

- [ ] **Step 1: Write the failing test**

Create `tests/test_layers_draw.py`:

```python
"""Tests for the draw capability."""
import hashlib
import time
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai.capabilities.draw import DrawManager, setup as setup_draw


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    agent.service = MagicMock()
    return agent


class TestDrawManager:
    def test_generate_image_success(self, tmp_path):
        svc = MagicMock()
        svc.generate_image.return_value = b"\x89PNG_FAKE_BYTES"
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "ok"
        assert "file_path" in result
        path = Path(result["file_path"])
        assert path.exists()
        assert path.read_bytes() == b"\x89PNG_FAKE_BYTES"
        assert path.parent == tmp_path / "media" / "images"

    def test_generate_image_no_provider(self, tmp_path):
        svc = MagicMock()
        svc.generate_image.side_effect = RuntimeError("No image_provider configured")
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "error"
        assert "image_provider" in result["message"]

    def test_generate_image_not_implemented(self, tmp_path):
        svc = MagicMock()
        svc.generate_image.side_effect = NotImplementedError
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "error"

    def test_missing_prompt(self, tmp_path):
        svc = MagicMock()
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({})
        assert result["status"] == "error"

    def test_empty_bytes_is_error(self, tmp_path):
        svc = MagicMock()
        svc.generate_image.return_value = b""
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "error"


class TestSetupDraw:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mgr = setup_draw(agent)
        assert isinstance(mgr, DrawManager)
        agent.add_tool.assert_called_once()
        agent.update_system_prompt.assert_called_once()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_layers_draw.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'lingtai.capabilities.draw'`

- [ ] **Step 3: Implement `draw.py`**

Create `src/lingtai/capabilities/draw.py`:

```python
"""Draw capability — text-to-image generation via LLM adapter."""
from __future__ import annotations

import hashlib
import time
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..agent import BaseAgent
    from ..llm.service import LLMService

SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "prompt": {
            "type": "string",
            "description": "Description of the image to generate",
        },
    },
    "required": ["prompt"],
}

DESCRIPTION = "Generate an image from a text description."


class DrawManager:
    def __init__(self, *, working_dir: Path, llm_service: "LLMService") -> None:
        self._working_dir = working_dir
        self._service = llm_service

    def handle(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"status": "error", "message": "Missing required parameter: prompt"}

        try:
            image_bytes = self._service.generate_image(prompt)
        except RuntimeError as exc:
            return {"status": "error", "message": str(exc)}
        except NotImplementedError:
            return {"status": "error", "message": "Provider does not support image generation"}

        if not image_bytes:
            return {"status": "error", "message": "Image generation returned empty result"}

        out_dir = self._working_dir / "media" / "images"
        out_dir.mkdir(parents=True, exist_ok=True)
        ts = int(time.time())
        short_hash = hashlib.md5(prompt.encode()).hexdigest()[:4]
        filename = f"draw_{ts}_{short_hash}.png"
        out_path = out_dir / filename
        out_path.write_bytes(image_bytes)
        return {"status": "ok", "file_path": str(out_path)}


def setup(agent: "BaseAgent", **kwargs: Any) -> DrawManager:
    """Set up the draw capability on an agent."""
    mgr = DrawManager(working_dir=agent.working_dir, llm_service=agent.service)
    agent.add_tool("draw", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt(
        "draw_instructions",
        "You can generate images via the draw tool. "
        "Provide a text prompt describing the image you want. "
        f"Generated images are saved to media/images/ in your working directory.",
    )
    return mgr
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_draw.py -v`
Expected: PASS (all 6 tests)

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from lingtai.capabilities.draw import DrawManager, setup"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/draw.py tests/test_layers_draw.py
git commit -m "feat: add draw capability (text-to-image)"
```

---

### Task 5: Create `compose` capability

**Files:**
- Create: `src/lingtai/capabilities/compose.py`
- Test: `tests/test_layers_compose.py`

- [ ] **Step 1: Write the failing test**

Create `tests/test_layers_compose.py`:

```python
"""Tests for the compose capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai.capabilities.compose import ComposeManager, setup as setup_compose


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    agent.service = MagicMock()
    return agent


class TestComposeManager:
    def test_generate_music_success(self, tmp_path):
        svc = MagicMock()
        svc.generate_music.return_value = b"FAKE_MP3_BYTES"
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "jazz piano"})
        assert result["status"] == "ok"
        path = Path(result["file_path"])
        assert path.exists()
        assert path.read_bytes() == b"FAKE_MP3_BYTES"
        assert path.parent == tmp_path / "media" / "music"

    def test_generate_music_with_duration(self, tmp_path):
        svc = MagicMock()
        svc.generate_music.return_value = b"AUDIO"
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "jazz", "duration_seconds": 30.0})
        assert result["status"] == "ok"
        svc.generate_music.assert_called_once_with("jazz", duration_seconds=30.0)

    def test_no_provider(self, tmp_path):
        svc = MagicMock()
        svc.generate_music.side_effect = RuntimeError("No music_provider configured")
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "jazz"})
        assert result["status"] == "error"

    def test_missing_prompt(self, tmp_path):
        svc = MagicMock()
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({})
        assert result["status"] == "error"

    def test_empty_bytes_is_error(self, tmp_path):
        svc = MagicMock()
        svc.generate_music.return_value = b""
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "jazz"})
        assert result["status"] == "error"


class TestSetupCompose:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mgr = setup_compose(agent)
        assert isinstance(mgr, ComposeManager)
        agent.add_tool.assert_called_once()
        agent.update_system_prompt.assert_called_once()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_layers_compose.py -v`
Expected: FAIL with `ModuleNotFoundError`

- [ ] **Step 3: Implement `compose.py`**

Create `src/lingtai/capabilities/compose.py`:

```python
"""Compose capability — music generation via LLM adapter."""
from __future__ import annotations

import hashlib
import time
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..agent import BaseAgent
    from ..llm.service import LLMService

SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "prompt": {
            "type": "string",
            "description": "Description of the music to generate",
        },
        "duration_seconds": {
            "type": "number",
            "description": "Desired duration in seconds",
        },
    },
    "required": ["prompt"],
}

DESCRIPTION = "Generate music from a text description."


class ComposeManager:
    def __init__(self, *, working_dir: Path, llm_service: "LLMService") -> None:
        self._working_dir = working_dir
        self._service = llm_service

    def handle(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"status": "error", "message": "Missing required parameter: prompt"}

        duration = args.get("duration_seconds")

        try:
            audio_bytes = self._service.generate_music(prompt, duration_seconds=duration)
        except RuntimeError as exc:
            return {"status": "error", "message": str(exc)}
        except NotImplementedError:
            return {"status": "error", "message": "Provider does not support music generation"}

        if not audio_bytes:
            return {"status": "error", "message": "Music generation returned empty result"}

        out_dir = self._working_dir / "media" / "music"
        out_dir.mkdir(parents=True, exist_ok=True)
        ts = int(time.time())
        short_hash = hashlib.md5(prompt.encode()).hexdigest()[:4]
        filename = f"compose_{ts}_{short_hash}.mp3"
        out_path = out_dir / filename
        out_path.write_bytes(audio_bytes)
        return {"status": "ok", "file_path": str(out_path)}


def setup(agent: "BaseAgent", **kwargs: Any) -> ComposeManager:
    """Set up the compose capability on an agent."""
    mgr = ComposeManager(working_dir=agent.working_dir, llm_service=agent.service)
    agent.add_tool("compose", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt(
        "compose_instructions",
        "You can generate music via the compose tool. "
        "Provide a text prompt describing the music you want. "
        "Optionally specify duration_seconds. "
        f"Generated music is saved to media/music/ in your working directory.",
    )
    return mgr
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_compose.py -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from lingtai.capabilities.compose import ComposeManager, setup"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/compose.py tests/test_layers_compose.py
git commit -m "feat: add compose capability (text-to-music)"
```

---

### Task 6: Create `talk` capability

**Files:**
- Create: `src/lingtai/capabilities/talk.py`
- Test: `tests/test_layers_talk.py`

- [ ] **Step 1: Write the failing test**

Create `tests/test_layers_talk.py`:

```python
"""Tests for the talk capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai.capabilities.talk import TalkManager, setup as setup_talk


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    agent.service = MagicMock()
    return agent


class TestTalkManager:
    def test_tts_success(self, tmp_path):
        svc = MagicMock()
        svc.text_to_speech.return_value = b"FAKE_AUDIO"
        mgr = TalkManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"text": "Hello world"})
        assert result["status"] == "ok"
        path = Path(result["file_path"])
        assert path.exists()
        assert path.read_bytes() == b"FAKE_AUDIO"
        assert path.parent == tmp_path / "media" / "audio"

    def test_no_provider(self, tmp_path):
        svc = MagicMock()
        svc.text_to_speech.side_effect = RuntimeError("No tts_provider configured")
        mgr = TalkManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"text": "hello"})
        assert result["status"] == "error"

    def test_missing_text(self, tmp_path):
        svc = MagicMock()
        mgr = TalkManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({})
        assert result["status"] == "error"

    def test_empty_bytes_is_error(self, tmp_path):
        svc = MagicMock()
        svc.text_to_speech.return_value = b""
        mgr = TalkManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"text": "hello"})
        assert result["status"] == "error"


class TestSetupTalk:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mgr = setup_talk(agent)
        assert isinstance(mgr, TalkManager)
        agent.add_tool.assert_called_once()
        agent.update_system_prompt.assert_called_once()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_layers_talk.py -v`
Expected: FAIL with `ModuleNotFoundError`

- [ ] **Step 3: Implement `talk.py`**

Create `src/lingtai/capabilities/talk.py`:

```python
"""Talk capability — text-to-speech via LLM adapter."""
from __future__ import annotations

import hashlib
import time
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..agent import BaseAgent
    from ..llm.service import LLMService

SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "text": {
            "type": "string",
            "description": "Text to convert to speech",
        },
    },
    "required": ["text"],
}

DESCRIPTION = "Convert text to speech audio."


class TalkManager:
    def __init__(self, *, working_dir: Path, llm_service: "LLMService") -> None:
        self._working_dir = working_dir
        self._service = llm_service

    def handle(self, args: dict) -> dict:
        text = args.get("text")
        if not text:
            return {"status": "error", "message": "Missing required parameter: text"}

        try:
            audio_bytes = self._service.text_to_speech(text)
        except RuntimeError as exc:
            return {"status": "error", "message": str(exc)}
        except NotImplementedError:
            return {"status": "error", "message": "Provider does not support text-to-speech"}

        if not audio_bytes:
            return {"status": "error", "message": "Text-to-speech returned empty result"}

        out_dir = self._working_dir / "media" / "audio"
        out_dir.mkdir(parents=True, exist_ok=True)
        ts = int(time.time())
        short_hash = hashlib.md5(text.encode()).hexdigest()[:4]
        filename = f"talk_{ts}_{short_hash}.mp3"
        out_path = out_dir / filename
        out_path.write_bytes(audio_bytes)
        return {"status": "ok", "file_path": str(out_path)}


def setup(agent: "BaseAgent", **kwargs: Any) -> TalkManager:
    """Set up the talk capability on an agent."""
    mgr = TalkManager(working_dir=agent.working_dir, llm_service=agent.service)
    agent.add_tool("talk", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt(
        "talk_instructions",
        "You can convert text to speech via the talk tool. "
        "Provide the text you want spoken. "
        f"Generated audio is saved to media/audio/ in your working directory.",
    )
    return mgr
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_talk.py -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from lingtai.capabilities.talk import TalkManager, setup"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/talk.py tests/test_layers_talk.py
git commit -m "feat: add talk capability (text-to-speech)"
```

---

### Task 7: Create `listen` capability

**Files:**
- Create: `src/lingtai/capabilities/listen.py`
- Test: `tests/test_layers_listen.py`

- [ ] **Step 1: Write the failing test**

Create `tests/test_layers_listen.py`:

```python
"""Tests for the listen capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai.capabilities.listen import ListenManager, setup as setup_listen


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    agent.service = MagicMock()
    return agent


class TestListenManager:
    def test_transcribe_success(self, tmp_path):
        svc = MagicMock()
        svc.transcribe.return_value = "Hello world"
        audio_file = tmp_path / "test.mp3"
        audio_file.write_bytes(b"FAKE_AUDIO")
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"audio_path": str(audio_file)})
        assert result["status"] == "ok"
        assert result["text"] == "Hello world"

    def test_transcribe_relative_path(self, tmp_path):
        svc = MagicMock()
        svc.transcribe.return_value = "Hi"
        audio_file = tmp_path / "test.mp3"
        audio_file.write_bytes(b"AUDIO")
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"audio_path": "test.mp3"})
        assert result["status"] == "ok"
        assert result["text"] == "Hi"

    def test_analyze_mode(self, tmp_path):
        svc = MagicMock()
        svc.analyze_audio.return_value = "This is jazz music"
        audio_file = tmp_path / "song.mp3"
        audio_file.write_bytes(b"AUDIO")
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({
            "audio_path": str(audio_file),
            "mode": "analyze",
            "prompt": "What genre is this?",
        })
        assert result["status"] == "ok"
        assert result["text"] == "This is jazz music"
        svc.analyze_audio.assert_called_once()

    def test_file_not_found(self, tmp_path):
        svc = MagicMock()
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"audio_path": "/nonexistent/file.mp3"})
        assert result["status"] == "error"
        assert "not found" in result["message"]

    def test_missing_audio_path(self, tmp_path):
        svc = MagicMock()
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({})
        assert result["status"] == "error"

    def test_no_provider(self, tmp_path):
        svc = MagicMock()
        svc.transcribe.side_effect = RuntimeError("No audio_provider configured")
        audio_file = tmp_path / "test.mp3"
        audio_file.write_bytes(b"AUDIO")
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"audio_path": str(audio_file)})
        assert result["status"] == "error"


class TestSetupListen:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mgr = setup_listen(agent)
        assert isinstance(mgr, ListenManager)
        agent.add_tool.assert_called_once()
        agent.update_system_prompt.assert_called_once()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_layers_listen.py -v`
Expected: FAIL with `ModuleNotFoundError`

- [ ] **Step 3: Implement `listen.py`**

Create `src/lingtai/capabilities/listen.py`:

```python
"""Listen capability — speech transcription and audio analysis via LLM adapter."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..agent import BaseAgent
    from ..llm.service import LLMService

SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "audio_path": {
            "type": "string",
            "description": "Path to the audio file",
        },
        "mode": {
            "type": "string",
            "enum": ["transcribe", "analyze"],
            "description": "Transcribe speech or analyze audio content",
            "default": "transcribe",
        },
        "prompt": {
            "type": "string",
            "description": "Question about the audio (for analyze mode)",
        },
    },
    "required": ["audio_path"],
}

DESCRIPTION = "Transcribe speech or analyze audio content from a file."


class ListenManager:
    def __init__(self, *, working_dir: Path, llm_service: "LLMService") -> None:
        self._working_dir = working_dir
        self._service = llm_service

    def handle(self, args: dict) -> dict:
        audio_path = args.get("audio_path")
        if not audio_path:
            return {"status": "error", "message": "Missing required parameter: audio_path"}

        path = Path(audio_path)
        if not path.is_absolute():
            path = self._working_dir / path

        if not path.is_file():
            return {"status": "error", "message": f"Audio file not found: {path}"}

        audio_bytes = path.read_bytes()
        mode = args.get("mode", "transcribe")

        try:
            if mode == "analyze":
                prompt = args.get("prompt", "Describe this audio.")
                text = self._service.analyze_audio(audio_bytes, prompt)
            else:
                text = self._service.transcribe(audio_bytes)
        except RuntimeError as exc:
            return {"status": "error", "message": str(exc)}
        except NotImplementedError:
            return {"status": "error", "message": f"Provider does not support audio {mode}"}

        return {"status": "ok", "text": text}


def setup(agent: "BaseAgent", **kwargs: Any) -> ListenManager:
    """Set up the listen capability on an agent."""
    mgr = ListenManager(working_dir=agent.working_dir, llm_service=agent.service)
    agent.add_tool("listen", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt(
        "listen_instructions",
        "You can transcribe speech or analyze audio via the listen tool. "
        "Provide the audio file path. Use mode='transcribe' for speech-to-text "
        "or mode='analyze' with a prompt to ask questions about the audio.",
    )
    return mgr
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_listen.py -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from lingtai.capabilities.listen import ListenManager, setup"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/listen.py tests/test_layers_listen.py
git commit -m "feat: add listen capability (speech transcription + audio analysis)"
```

---

### Task 8: Register capabilities in `__init__.py`

**Files:**
- Modify: `src/lingtai/capabilities/__init__.py:11-15`

- [ ] **Step 1: Write the failing test**

In `tests/test_layers_draw.py`, add at the bottom:

```python
class TestAddCapabilityIntegration:
    def test_add_capability_draw(self, tmp_path):
        from lingtai.agent import BaseAgent
        from unittest.mock import MagicMock
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = BaseAgent(agent_id="test", service=svc, working_dir=str(tmp_path))
        mgr = agent.add_capability("draw")
        assert "draw" in agent._mcp_handlers
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_layers_draw.py::TestAddCapabilityIntegration -v`
Expected: FAIL with `ValueError: Unknown capability: 'draw'`

- [ ] **Step 3: Add to `_BUILTIN`**

In `src/lingtai/capabilities/__init__.py`, update the `_BUILTIN` dict:

```python
_BUILTIN: dict[str, str] = {
    "bash": ".bash",
    "delegate": ".delegate",
    "email": ".email",
    "draw": ".draw",
    "compose": ".compose",
    "talk": ".talk",
    "listen": ".listen",
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_draw.py::TestAddCapabilityIntegration -v`
Expected: PASS

- [ ] **Step 5: Run all media capability tests together**

Run: `python -m pytest tests/test_layers_draw.py tests/test_layers_compose.py tests/test_layers_talk.py tests/test_layers_listen.py -v`
Expected: ALL PASS

- [ ] **Step 6: Smoke-test import**

Run: `python -c "import lingtai"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/capabilities/__init__.py tests/test_layers_draw.py
git commit -m "feat: register draw/compose/talk/listen in capability registry"
```

---

## Chunk 3: Mail Attachments

### Task 9: Add `attachments` to mail message model and intrinsic schema

**Files:**
- Modify: `src/lingtai/intrinsics/mail.py`
- Modify: `src/lingtai/services/mail.py` (MailMessage if it exists, or the dict-based message format)
- Test: `tests/test_intrinsics_comm.py`

Note: The current mail system uses plain dicts, not a MailMessage dataclass. The intrinsic schema needs an `attachments` field. The TCPMailService just passes dicts through.

- [ ] **Step 1: Write the failing test**

In `tests/test_intrinsics_comm.py`, add:

```python
def test_mail_schema_has_attachments():
    from lingtai.intrinsics.mail import SCHEMA
    assert "attachments" in SCHEMA["properties"]
    assert SCHEMA["properties"]["attachments"]["type"] == "array"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_intrinsics_comm.py::test_mail_schema_has_attachments -v`
Expected: FAIL with `KeyError: 'attachments'`

- [ ] **Step 3: Add `attachments` to mail SCHEMA**

In `src/lingtai/intrinsics/mail.py`, add to the `properties` dict inside `SCHEMA`:

```python
"attachments": {
    "type": "array",
    "items": {"type": "string"},
    "description": "List of file paths to attach to the message",
},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_intrinsics_comm.py::test_mail_schema_has_attachments -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/intrinsics/mail.py tests/test_intrinsics_comm.py
git commit -m "feat: add attachments field to mail intrinsic schema"
```

---

### Task 10: Implement filesystem-based mailbox and attachment transfer in TCPMailService

**Files:**
- Modify: `src/lingtai/services/mail.py`
- Test: `tests/test_services_mail.py`

This is the core of mail attachments. The TCPMailService needs to:
1. On send: read attachment files, base64-encode, include in wire message
2. On receive: decode, save to `mailbox/<uuid>/attachments/`, rewrite attachment paths
3. Increase the 10MB limit to 100MB
4. Save every received message to the filesystem mailbox

- [ ] **Step 1: Write the failing tests**

Add to `tests/test_services_mail.py`:

```python
import base64
import json
import uuid
from pathlib import Path


class TestMailAttachments:
    def test_send_with_attachment(self, tmp_path):
        """Sender encodes attachment files into the wire message."""
        import socket
        import threading

        received = []
        event = threading.Event()

        def on_message(msg):
            received.append(msg)
            event.set()

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        # Create a file to attach
        attachment = tmp_path / "image.png"
        attachment.write_bytes(b"\x89PNG_TEST_DATA")

        receiver_dir = tmp_path / "receiver"
        receiver_dir.mkdir()
        listener = TCPMailService(listen_port=port, working_dir=receiver_dir)
        listener.listen(on_message)

        try:
            sender = TCPMailService(working_dir=tmp_path)
            result = sender.send(
                f"127.0.0.1:{port}",
                {
                    "from": "sender",
                    "to": f"127.0.0.1:{port}",
                    "message": "here is an image",
                    "attachments": [str(attachment)],
                },
            )
            assert result is True
            assert event.wait(timeout=5.0)
            msg = received[0]
            # Receiver should get local file paths, not base64
            assert "attachments" in msg
            assert len(msg["attachments"]) == 1
            recv_path = Path(msg["attachments"][0])
            assert recv_path.exists()
            assert recv_path.read_bytes() == b"\x89PNG_TEST_DATA"
            assert "mailbox" in str(recv_path)
        finally:
            listener.stop()

    def test_send_without_attachment(self, tmp_path):
        """Messages without attachments still work normally."""
        import socket
        import threading

        received = []
        event = threading.Event()

        def on_message(msg):
            received.append(msg)
            event.set()

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        listener = TCPMailService(listen_port=port, working_dir=tmp_path)
        listener.listen(on_message)

        try:
            sender = TCPMailService()
            result = sender.send(
                f"127.0.0.1:{port}",
                {"from": "sender", "to": f"127.0.0.1:{port}", "message": "no attachments"},
            )
            assert result is True
            assert event.wait(timeout=5.0)
            assert received[0]["message"] == "no attachments"
        finally:
            listener.stop()

    def test_attachment_file_not_found(self, tmp_path):
        """send() returns False when attachment file does not exist."""
        sender = TCPMailService(working_dir=tmp_path)
        result = sender.send(
            "127.0.0.1:1",
            {"from": "s", "to": "r", "message": "hi", "attachments": ["/nonexistent/file.png"]},
        )
        assert result is False

    def test_mailbox_directory_structure(self, tmp_path):
        """Received messages are saved in mailbox/<uuid>/message.json + attachments/."""
        import socket
        import threading

        received = []
        event = threading.Event()

        def on_message(msg):
            received.append(msg)
            event.set()

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        attachment = tmp_path / "song.mp3"
        attachment.write_bytes(b"MP3_DATA")

        receiver_dir = tmp_path / "receiver"
        receiver_dir.mkdir()
        listener = TCPMailService(listen_port=port, working_dir=receiver_dir)
        listener.listen(on_message)

        try:
            sender = TCPMailService(working_dir=tmp_path)
            sender.send(
                f"127.0.0.1:{port}",
                {"from": "s", "to": "r", "message": "music", "attachments": [str(attachment)]},
            )
            assert event.wait(timeout=5.0)

            # Check mailbox structure
            mailbox = receiver_dir / "mailbox"
            assert mailbox.is_dir()
            msg_dirs = list(mailbox.iterdir())
            assert len(msg_dirs) == 1
            msg_dir = msg_dirs[0]
            assert (msg_dir / "message.json").is_file()
            assert (msg_dir / "attachments" / "song.mp3").is_file()
            assert (msg_dir / "attachments" / "song.mp3").read_bytes() == b"MP3_DATA"
        finally:
            listener.stop()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_services_mail.py::TestMailAttachments -v`
Expected: FAIL (TCPMailService doesn't accept `working_dir`, doesn't handle attachments)

- [ ] **Step 3: Implement attachment support in TCPMailService**

Modify `src/lingtai/services/mail.py`:

1. Add `working_dir` parameter to `TCPMailService.__init__()` (default `None`)
2. In `send()`: if message has `attachments` list, read each file, base64-encode, replace the `attachments` field with `[{filename, data}]` in the wire message. Return `False` if any file not found.
3. In `_handle_connection()`: if received message has encoded attachments, create `mailbox/<uuid>/attachments/` dir, decode and save files, replace attachment data with local paths, save `message.json`. Increase size limit to 100MB.
4. All received messages (even without attachments) get persisted to the mailbox.

The key changes to `src/lingtai/services/mail.py`:

Add imports at top:
```python
import base64
import uuid
from pathlib import Path
```

Update `__init__`:
```python
def __init__(
    self,
    listen_port: int | None = None,
    listen_host: str = "127.0.0.1",
    working_dir: Path | str | None = None,
) -> None:
    # ... existing init code ...
    self._working_dir = Path(working_dir) if working_dir else None
```

Update `send()` to encode attachments (IMPORTANT: create a new dict, never mutate the caller's):
```python
# Before JSON serialization, encode any file attachments
if "attachments" in message and message["attachments"]:
    encoded = []
    for fpath in message["attachments"]:
        p = Path(fpath)
        if not p.is_file():
            return False
        encoded.append({
            "filename": p.name,
            "data": base64.b64encode(p.read_bytes()).decode("ascii"),
        })
    # Create a NEW dict — do not mutate the original
    message = {k: v for k, v in message.items() if k != "attachments"}
    message["_encoded_attachments"] = encoded
```

Update `_handle_connection()` to decode and save:
```python
# Change size limit from 10MB to 100MB
if length > 100_000_000:
    return

# After JSON parsing:
payload = json.loads(data.decode("utf-8"))

# Persist to mailbox and decode attachments
if self._working_dir is not None:
    msg_id = str(uuid.uuid4())
    msg_dir = self._working_dir / "mailbox" / msg_id
    att_dir = msg_dir / "attachments"

    if "_encoded_attachments" in payload:
        att_dir.mkdir(parents=True, exist_ok=True)
        local_paths = []
        for att in payload["_encoded_attachments"]:
            out = att_dir / att["filename"]
            out.write_bytes(base64.b64decode(att["data"]))
            local_paths.append(str(out))
        del payload["_encoded_attachments"]
        payload["attachments"] = local_paths
    else:
        msg_dir.mkdir(parents=True, exist_ok=True)

    # Save message.json (without binary data)
    (msg_dir / "message.json").write_text(
        json.dumps({k: v for k, v in payload.items()}, indent=2, default=str)
    )

on_message(payload)  # on_message is the callback passed to _handle_connection
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_services_mail.py -v`
Expected: ALL PASS (both old and new tests)

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from lingtai.services.mail import TCPMailService"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/services/mail.py tests/test_services_mail.py
git commit -m "feat: add attachment support to TCPMailService with filesystem mailbox"
```

---

### Task 11: Wire mail attachments through BaseAgent handler

**Files:**
- Modify: `src/lingtai/agent.py` (the `_handle_mail` or mail dispatch section)
- Test: `tests/test_intrinsics_comm.py`

The `_handle_mail` in agent.py needs to:
1. Pass `attachments` from tool args through to the mail message
2. Resolve relative attachment paths against `working_dir`
3. Pass `working_dir` to TCPMailService (or handle it at the agent level)

- [ ] **Step 1: Read `_handle_mail` in agent.py to understand current implementation**

Read the mail handling section of `agent.py` to identify exact lines. The handler should already be dispatching `action=send` with the message dict to `self._mail_service.send()`.

- [ ] **Step 2: Write the failing test**

Add to `tests/test_intrinsics_comm.py`:

```python
def test_mail_send_passes_attachments():
    """Mail handler should pass attachments to mail service."""
    from lingtai.agent import BaseAgent
    from unittest.mock import MagicMock, patch
    from pathlib import Path

    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"

    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = True

    agent = BaseAgent(agent_id="test", service=svc, mail_service=mail_svc)

    # Call the mail handler directly
    result = agent._intrinsics["mail"]({
        "action": "send",
        "address": "127.0.0.1:8888",
        "message": "here is a file",
        "attachments": ["/path/to/file.png"],
    })
    assert result["status"] == "delivered"
    # Verify attachments were passed through
    call_args = mail_svc.send.call_args
    sent_message = call_args[0][1]  # second positional arg is the message dict
    assert "attachments" in sent_message
    assert sent_message["attachments"] == ["/path/to/file.png"]
```

- [ ] **Step 3: Run test to verify it fails**

Run: `python -m pytest tests/test_intrinsics_comm.py::test_mail_send_passes_attachments -v`
Expected: FAIL (attachments not included in sent message)

- [ ] **Step 4: Update `_handle_mail` to pass attachments**

In the mail send handler section of `agent.py`, when constructing the message dict to pass to `self._mail_service.send()`, include:

```python
attachments = args.get("attachments", [])
if attachments:
    # Resolve relative paths against working_dir
    resolved = []
    for p in attachments:
        path = Path(p)
        if not path.is_absolute():
            path = self._working_dir / path
        resolved.append(str(path))
    msg["attachments"] = resolved
```

- [ ] **Step 5: Run test to verify it passes**

Run: `python -m pytest tests/test_intrinsics_comm.py::test_mail_send_passes_attachments -v`
Expected: PASS

- [ ] **Step 6: Run all mail tests**

Run: `python -m pytest tests/test_intrinsics_comm.py tests/test_services_mail.py -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/agent.py tests/test_intrinsics_comm.py
git commit -m "feat: wire mail attachments through BaseAgent handler"
```

---

### Task 12: Update email capability for attachments

**Files:**
- Modify: `src/lingtai/capabilities/email.py`
- Test: `tests/test_layers_email.py`

The email capability needs to:
1. Add `attachments` to the email SCHEMA
2. Pass attachments through `_send()`
3. Show attachments in `_read()` and `_check()` output
4. Support attachments in `_reply()` / `_reply_all()`

Note: The spec mentions `forward` carrying attachments, but `EmailManager` does not have a forward action yet. Forward is out of scope for this plan — it can be added as a follow-up.

- [ ] **Step 1: Write the failing tests**

Add to `tests/test_layers_email.py`:

```python
def test_email_send_with_attachments():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    result = mgr.handle({
        "action": "send",
        "address": "127.0.0.1:8888",
        "subject": "file for you",
        "message": "see attached",
        "attachments": ["/path/to/file.png"],
    })
    assert result["status"] == "delivered"
    sent = mail_svc.send.call_args[0][1]
    assert sent.get("attachments") == ["/path/to/file.png"]


def test_email_read_shows_attachments():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = agent.add_capability("email")
    mgr.on_mail_received({
        "from": "sender",
        "to": ["test"],
        "subject": "photo",
        "message": "look at this",
        "attachments": ["/receiver/mailbox/abc/attachments/photo.png"],
    })
    result = mgr.handle({"action": "read", "email_id": mgr._mailbox[0]["id"]})
    assert result["status"] == "ok"
    assert "photo.png" in result["message"]
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_layers_email.py::test_email_send_with_attachments tests/test_layers_email.py::test_email_read_shows_attachments -v`
Expected: FAIL

- [ ] **Step 3: Update email capability**

In `src/lingtai/capabilities/email.py`:

1. Add `attachments` to SCHEMA properties:
   ```python
   "attachments": {
       "type": "array",
       "items": {"type": "string"},
       "description": "File paths to attach",
   },
   ```

2. In `_send()`, pass attachments through to the mail message dict.

3. In `_read()`, include attachment paths in the formatted output.

4. Update system prompt to mention attachment support.

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL PASS

- [ ] **Step 5: Update system prompt with symlink convention**

In the email capability's `setup()`, update the system prompt to include:

```
"Attachments received via email are stored in the mailbox. "
"To use an attachment elsewhere, create a symlink to it — "
"do not move the original file. Example: "
"os.symlink('mailbox/<id>/attachments/image.png', 'media/images/image.png')"
```

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/email.py tests/test_layers_email.py
git commit -m "feat: add attachment support to email capability"
```

---

### Task 13: Final integration test and full test suite

**Files:**
- Test: all test files

- [ ] **Step 1: Run the full test suite**

Run: `python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 2: Smoke-test full import**

Run: `python -c "import lingtai; from lingtai.capabilities.draw import DrawManager; from lingtai.capabilities.compose import ComposeManager; from lingtai.capabilities.talk import TalkManager; from lingtai.capabilities.listen import ListenManager"`

- [ ] **Step 3: Verify all capabilities can be added together**

Run:
```python
python -c "
from unittest.mock import MagicMock
from lingtai.agent import BaseAgent
svc = MagicMock()
svc.get_adapter.return_value = MagicMock()
svc.provider = 'gemini'
svc.model = 'gemini-test'
agent = BaseAgent(agent_id='test', service=svc)
agent.add_capability('draw', 'compose', 'talk', 'listen')
print('All capabilities registered:', list(agent._mcp_handlers.keys()))
"
```

- [ ] **Step 4: Commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: integration fixes from full test suite run"
```
