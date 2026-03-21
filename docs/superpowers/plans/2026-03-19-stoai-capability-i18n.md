# lingtai Capability i18n Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mirror the kernel's i18n pattern in the lingtai wrapper so capability descriptions and schemas are language-aware, controlled by `AgentConfig.language`.

**Architecture:** Create `src/lingtai/i18n/` with `t(lang, key)` (identical pattern to `lingtai_kernel.i18n`), JSON string tables for en/zh, and convert each capability from static `SCHEMA`/`DESCRIPTION` constants to `get_schema(lang)`/`get_description(lang)` functions. A Chinese `base_prompt_zh.md` is added alongside the existing `base_prompt.md`. The `Agent._build_system_prompt()` already passes `lang` — it just needs to select the right base prompt file.

**Tech Stack:** Python 3.11+, JSON string tables, no new dependencies.

---

## Design Decisions

1. **Separate i18n module** — lingtai has its own `src/lingtai/i18n/` (not extending kernel's). Dependency stays one-directional.
2. **Same API** — `t(lang, key, **kwargs)` with identical fallback behavior (requested lang → en → key itself).
3. **Backward compat** — Module-level `SCHEMA` and `DESCRIPTION` constants stay as English defaults. New `get_schema(lang)` and `get_description(lang)` functions added alongside.
4. **Language source** — Capabilities read `agent._config.language` inside `setup()`. No new parameter needed on `setup()` signatures.
5. **Key naming** — `<capability>.<description|property_name>` (e.g., `read.description`, `read.file_path`).
6. **Delegate special case** — `_build_schema(agent)` already deep-copies SCHEMA. It will call `get_schema(lang)` instead of using the static `SCHEMA`.
7. **Base prompt** — Language-aware loading with `base_prompt_zh.md` file, same pattern as kernel's `manifesto_zh.md`.
8. **Out of scope** — Error messages inside handlers (e.g., "file_path is required") stay English — they're debugging aids, not agent-facing prose.

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `src/lingtai/i18n/__init__.py` | `t(lang, key)` translation function |
| Create | `src/lingtai/i18n/en.json` | English string table (~100 keys) |
| Create | `src/lingtai/i18n/zh.json` | Chinese string table (~100 keys) |
| Create | `src/lingtai/base_prompt_zh.md` | Chinese base prompt |
| Modify | `src/lingtai/agent.py` | Language-aware base prompt loading |
| Modify | `src/lingtai/capabilities/read.py` | `get_schema(lang)` + `get_description(lang)` |
| Modify | `src/lingtai/capabilities/write.py` | same |
| Modify | `src/lingtai/capabilities/edit.py` | same |
| Modify | `src/lingtai/capabilities/glob.py` | same |
| Modify | `src/lingtai/capabilities/grep.py` | same |
| Modify | `src/lingtai/capabilities/bash.py` | same |
| Modify | `src/lingtai/capabilities/psyche.py` | same |
| Modify | `src/lingtai/capabilities/delegate.py` | same + update `_build_schema` |
| Modify | `src/lingtai/capabilities/email.py` | same |
| Modify | `src/lingtai/capabilities/vision.py` | same |
| Modify | `src/lingtai/capabilities/web_search.py` | same |
| Modify | `src/lingtai/capabilities/talk.py` | same |
| Modify | `src/lingtai/capabilities/compose.py` | same |
| Modify | `src/lingtai/capabilities/draw.py` | same |
| Modify | `src/lingtai/capabilities/listen.py` | same |
| Create | `tests/test_i18n.py` | Tests for `t()` and capability `get_schema`/`get_description` |

---

### Task 1: Create the i18n module and English string table

**Files:**
- Create: `src/lingtai/i18n/__init__.py`
- Create: `src/lingtai/i18n/en.json`
- Test: `tests/test_i18n.py`

- [ ] **Step 1: Write the i18n module**

`src/lingtai/i18n/__init__.py` — identical pattern to `lingtai_kernel/i18n/__init__.py`:

```python
"""Capability i18n — language-aware string tables for lingtai capabilities.

Usage: t(lang, key, **kwargs)
  lang: language code ("en", "zh")
  key: dotted string ID ("read.description")
  kwargs: template substitutions

Mirrors lingtai_kernel.i18n for the capability layer.
"""
from __future__ import annotations

import json
from pathlib import Path

_DIR = Path(__file__).parent
_CACHE: dict[str, dict[str, str]] = {}


def _load(lang: str) -> dict[str, str]:
    if lang not in _CACHE:
        path = _DIR / f"{lang}.json"
        if path.is_file():
            _CACHE[lang] = json.loads(path.read_text(encoding="utf-8"))
        else:
            _CACHE[lang] = {}
    return _CACHE[lang]


def t(lang: str, key: str, **kwargs) -> str:
    from collections import defaultdict
    table = _load(lang)
    value = table.get(key)
    if value is None and lang != "en":
        value = _load("en").get(key)
    if value is None:
        return key
    if kwargs:
        return value.format_map(defaultdict(str, kwargs))
    return value
```

- [ ] **Step 2: Create the English string table**

`src/lingtai/i18n/en.json` — extract all DESCRIPTION and SCHEMA property descriptions from all 15 capabilities. Key naming: `<capability>.description` for the tool description, `<capability>.<property_name>` for schema property descriptions.

Keys to extract (organized by capability):

**File I/O (read, write, edit, glob, grep):**
- `read.description`, `read.file_path`, `read.offset`, `read.limit`
- `write.description`, `write.file_path`, `write.content`
- `edit.description`, `edit.file_path`, `edit.old_string`, `edit.new_string`, `edit.replace_all`
- `glob.description`, `glob.pattern`, `glob.path`
- `grep.description`, `grep.pattern`, `grep.path`, `grep.glob`, `grep.max_matches`

**Core capabilities:**
- `bash.description`, `bash.command`, `bash.timeout`, `bash.working_dir`
- `psyche.description`, `psyche.object`, `psyche.action`, `psyche.title`, `psyche.summary`, `psyche.content`, `psyche.supplementary`, `psyche.ids`, `psyche.notes`, `psyche.pattern`, `psyche.limit`, `psyche.depth`
- `delegate.description`, `delegate.name`, `delegate.covenant`, `delegate.memory`, `delegate.capabilities`, `delegate.admin`, `delegate.provider`, `delegate.model`, `delegate.provider_dynamic`, `delegate.model_dynamic`
- `email.description`, `email.action`, `email.address`, `email.cc`, `email.bcc`, `email.attachments`, `email.subject`, `email.message`, `email.email_id`, `email.n`, `email.query`, `email.folder`, `email.type`, `email.name`, `email.note`, `email.schedule_action`, `email.schedule_interval`, `email.schedule_count`, `email.schedule_id`

**Media/external:**
- `vision.description`, `vision.image_path`, `vision.question`
- `web_search.description`, `web_search.query`
- `talk.description`, `talk.text`, `talk.voice_id`, `talk.emotion`, `talk.speed`
- `compose.description`, `compose.prompt`, `compose.lyrics`
- `draw.description`, `draw.prompt`, `draw.aspect_ratio`
- `listen.description`, `listen.audio_path`, `listen.action`

Total: ~95 keys. Copy exact English text from current module-level constants.

- [ ] **Step 3: Write the failing test**

`tests/test_i18n.py`:

```python
"""Tests for lingtai capability i18n."""
from lingtai.i18n import t


def test_en_simple_key():
    assert "text file" in t("en", "read.description")


def test_unknown_lang_falls_back_to_en():
    assert "text file" in t("xx", "read.description")


def test_unknown_key_returns_key():
    assert t("en", "nonexistent.key") == "nonexistent.key"
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_i18n.py -v`
Expected: 3 PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/i18n/ tests/test_i18n.py
git commit -m "feat(i18n): add lingtai capability i18n module with English string table"
```

---

### Task 2: Create Chinese string table and base prompt

**Files:**
- Create: `src/lingtai/i18n/zh.json`
- Create: `src/lingtai/base_prompt_zh.md`
- Test: `tests/test_i18n.py`

- [ ] **Step 1: Create the Chinese string table**

`src/lingtai/i18n/zh.json` — translate all ~95 keys from `en.json` to Chinese. Same keys, Chinese values. Template variables (`{variable}`) stay as-is.

Guidelines:
- Tool behavior descriptions should be clear and natural in Chinese
- Technical terms (e.g., "glob pattern", "regex", "TCP port") can stay in English within Chinese text
- Action names in enum values (e.g., "send", "check") stay English — they're code, not prose

- [ ] **Step 2: Create Chinese base prompt**

`src/lingtai/base_prompt_zh.md`:
```markdown
# 系统提示

仔细阅读你的工具 schema 以了解能力、注意事项和流程。
你的工作目录就是你的身份——你的所有状态、记忆和文件都在那里。
下面的记忆区域可能会在会话中更新。
当上下文使用达到 80% 时，你会收到 5 轮倒计时提醒进行蜕变。将重要数据保存到知识库，然后写一份简报给未来的自己进行蜕变。如果忽略全部 5 次警告，你的对话将被自动清除。
```

- [ ] **Step 3: Add Chinese test**

Append to `tests/test_i18n.py`:

```python
def test_zh_simple_key():
    result = t("zh", "read.description")
    assert result != "read.description"  # not the fallback key
    assert "read" not in result.lower() or "文件" in result  # Chinese text present
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_i18n.py -v`
Expected: 4 PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/i18n/zh.json src/lingtai/base_prompt_zh.md tests/test_i18n.py
git commit -m "feat(i18n): add Chinese string table and base prompt"
```

---

### Task 3: Wire language into Agent base prompt loading

**Files:**
- Modify: `src/lingtai/agent.py:17-25` (base prompt loading)
- Modify: `src/lingtai/agent.py:103-123` (`_build_system_prompt`)

- [ ] **Step 1: Update base prompt loader to be language-aware**

Replace the single-file `_load_base_prompt()` with a language-aware version:

```python
_BASE_PROMPTS: dict[str, str] = {}


def _load_base_prompt(lang: str = "en") -> str:
    """Load base_prompt[_lang].md shipped with the package."""
    if lang not in _BASE_PROMPTS:
        base = Path(__file__).parent
        if lang != "en":
            path = base / f"base_prompt_{lang}.md"
            if path.is_file():
                _BASE_PROMPTS[lang] = path.read_text().strip()
                return _BASE_PROMPTS[lang]
        _BASE_PROMPTS[lang] = (base / "base_prompt.md").read_text().strip()
    return _BASE_PROMPTS[lang]
```

- [ ] **Step 2: Update `_build_system_prompt` to pass language to base prompt loader**

In `_build_system_prompt`, change:
```python
base_prompt=_load_base_prompt(),
```
to:
```python
base_prompt=_load_base_prompt(lang),
```

- [ ] **Step 3: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/agent.py
git commit -m "feat(i18n): language-aware base prompt loading in Agent"
```

---

### Task 4: Convert file I/O capabilities (read, write, edit, glob, grep)

**Files:**
- Modify: `src/lingtai/capabilities/read.py`
- Modify: `src/lingtai/capabilities/write.py`
- Modify: `src/lingtai/capabilities/edit.py`
- Modify: `src/lingtai/capabilities/glob.py`
- Modify: `src/lingtai/capabilities/grep.py`
- Test: `tests/test_i18n.py`

The pattern for every capability is the same. Using `read.py` as the example:

- [ ] **Step 1: Add `get_description` and `get_schema` functions**

For each of the 5 file I/O capabilities, add two functions and update `setup()`:

```python
# read.py example — same pattern for write, edit, glob, grep

def get_description(lang: str = "en") -> str:
    from ..i18n import t
    return t(lang, "read.description")

def get_schema(lang: str = "en") -> dict:
    from ..i18n import t
    return {
        "type": "object",
        "properties": {
            "file_path": {"type": "string", "description": t(lang, "read.file_path")},
            "offset": {"type": "integer", "description": t(lang, "read.offset"), "default": 1},
            "limit": {"type": "integer", "description": t(lang, "read.limit"), "default": 2000},
        },
        "required": ["file_path"],
    }

# Keep backward compat
SCHEMA = get_schema("en")
DESCRIPTION = get_description("en")
```

Update `setup()` to use language:

```python
def setup(agent: "BaseAgent") -> None:
    lang = agent._config.language
    # ... handler unchanged ...
    agent.add_tool("read", schema=get_schema(lang), handler=handle_read, description=get_description(lang))
```

- [ ] **Step 2: Apply same pattern to write.py, edit.py, glob.py, grep.py**

Each file gets `get_description(lang)`, `get_schema(lang)`, backward-compat constants, and updated `setup()`.

- [ ] **Step 3: Add test**

Append to `tests/test_i18n.py`:

```python
def test_capability_get_schema_en():
    from lingtai.capabilities.read import get_schema, get_description
    schema = get_schema("en")
    assert "file_path" in schema["properties"]
    desc = get_description("en")
    assert "text file" in desc.lower()


def test_capability_get_schema_zh():
    from lingtai.capabilities.read import get_schema, get_description
    schema = get_schema("zh")
    # Chinese description in schema property
    assert schema["properties"]["file_path"]["description"] != "read.file_path"
    desc = get_description("zh")
    assert desc != "read.description"
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_i18n.py -v`
Expected: all PASS

- [ ] **Step 5: Smoke test all 5 modules**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "from lingtai.capabilities.read import get_schema; from lingtai.capabilities.write import get_schema; from lingtai.capabilities.edit import get_schema; from lingtai.capabilities.glob import get_schema; from lingtai.capabilities.grep import get_schema; print('ok')"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/read.py src/lingtai/capabilities/write.py src/lingtai/capabilities/edit.py src/lingtai/capabilities/glob.py src/lingtai/capabilities/grep.py tests/test_i18n.py
git commit -m "feat(i18n): language-aware file I/O capabilities (read/write/edit/glob/grep)"
```

---

### Task 5: Convert core capabilities (bash, psyche, delegate, email)

**Files:**
- Modify: `src/lingtai/capabilities/bash.py`
- Modify: `src/lingtai/capabilities/psyche.py`
- Modify: `src/lingtai/capabilities/delegate.py`
- Modify: `src/lingtai/capabilities/email.py`

Same pattern as Task 4. Special notes:

- [ ] **Step 1: Convert bash.py**

Same `get_description`/`get_schema` pattern. Note: `setup()` has a dynamic `desc` that appends policy summary. The language-aware version:

```python
def setup(agent, policy_file=None, yolo=False):
    lang = agent._config.language
    # ... manager setup unchanged ...
    desc = get_description(lang)
    if policy_summary:
        desc = f"{desc}\n\n{policy_summary}"
    agent.add_tool("bash", schema=get_schema(lang), handler=mgr.handle, description=desc)
```

- [ ] **Step 2: Convert psyche.py**

Large schema with 11 properties. Same pattern. Note the `DESCRIPTION` is very long — the Chinese translation should preserve all workflow guidance.

- [ ] **Step 3: Convert delegate.py**

Special: has `_build_schema(agent)` that deep-copies and extends the base schema. Change it to call `get_schema(lang)` instead of using the static `SCHEMA`.

**Important:** `_build_schema` (lines 301-311) overwrites `provider` and `model` descriptions with hardcoded English strings containing dynamic runtime data (`Available: ...`, `Known: ...`). These dynamic descriptions must also use `t()` with template keys:

- Add keys `delegate.provider_dynamic` and `delegate.model_dynamic` to the string tables:
  - en: `"LLM provider for the delegatee (optional, default = same as delegator). Available: {available}."`
  - en: `"LLM model for the delegatee (optional, default = same as delegator). Known: {known}."`
  - zh: `"代理的 LLM 提供方（可选，默认与委托方相同）。可用: {available}。"`
  - zh: `"代理的 LLM 模型（可选，默认与委托方相同）。已知: {known}。"`

```python
def _build_schema(agent: "Agent") -> dict:
    import copy
    from ..i18n import t
    lang = agent._config.language
    schema = copy.deepcopy(get_schema(lang))

    # ... available/provider_models detection unchanged ...

    schema["properties"]["provider"]["description"] = t(
        lang, "delegate.provider_dynamic", available=", ".join(available)
    )
    schema["properties"]["provider"]["enum"] = available

    if provider_models:
        schema["properties"]["model"]["description"] = t(
            lang, "delegate.model_dynamic", known="; ".join(provider_models)
        )

    return schema
```

And update `setup()`:

```python
def setup(agent: "Agent") -> DelegateManager:
    lang = agent._config.language
    mgr = DelegateManager(agent)
    schema = _build_schema(agent)
    agent.add_tool("delegate", schema=schema, handler=mgr.handle, description=get_description(lang))
    return mgr
```

- [ ] **Step 3b: Fix delegate language propagation**

In `_delegate()` (line 218), `AgentConfig` is constructed for peer agents but does **not** pass `language`. Add it:

```python
peer_config = AgentConfig(
    max_turns=parent._config.max_turns,
    provider=peer_provider,
    model=peer_model,
    retry_timeout=parent._config.retry_timeout,
    thinking_budget=parent._config.thinking_budget,
    language=parent._config.language,
)
```

Without this, delegated agents revert to English even when the parent is Chinese.

- [ ] **Step 4: Convert email.py**

Large schema with 17 properties + nested schedule object (4 more). Same pattern. Note: `email.schedule_action`, `email.schedule_interval`, etc. for the nested properties.

- [ ] **Step 5: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "from lingtai.capabilities.bash import get_schema; from lingtai.capabilities.psyche import get_schema; from lingtai.capabilities.delegate import get_schema; from lingtai.capabilities.email import get_schema; print('ok')"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/bash.py src/lingtai/capabilities/psyche.py src/lingtai/capabilities/delegate.py src/lingtai/capabilities/email.py
git commit -m "feat(i18n): language-aware core capabilities (bash/psyche/delegate/email)"
```

---

### Task 6: Convert media/external capabilities (vision, web_search, talk, compose, draw, listen)

**Files:**
- Modify: `src/lingtai/capabilities/vision.py`
- Modify: `src/lingtai/capabilities/web_search.py`
- Modify: `src/lingtai/capabilities/talk.py`
- Modify: `src/lingtai/capabilities/compose.py`
- Modify: `src/lingtai/capabilities/draw.py`
- Modify: `src/lingtai/capabilities/listen.py`

- [ ] **Step 1: Convert all 6 capabilities**

Same pattern as Tasks 4-5. These are the simpler capabilities (2-4 schema properties each).

- [ ] **Step 2: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "from lingtai.capabilities.vision import get_schema; from lingtai.capabilities.web_search import get_schema; from lingtai.capabilities.talk import get_schema; from lingtai.capabilities.compose import get_schema; from lingtai.capabilities.draw import get_schema; from lingtai.capabilities.listen import get_schema; print('ok')"`

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/capabilities/vision.py src/lingtai/capabilities/web_search.py src/lingtai/capabilities/talk.py src/lingtai/capabilities/compose.py src/lingtai/capabilities/draw.py src/lingtai/capabilities/listen.py
git commit -m "feat(i18n): language-aware media capabilities (vision/web_search/talk/compose/draw/listen)"
```

---

### Task 7: Final verification

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -v`

- [ ] **Step 2: Smoke test with Chinese language**

```python
cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "
from lingtai.i18n import t
# Spot-check a few keys
print(t('zh', 'read.description'))
print(t('zh', 'email.description'))
print(t('zh', 'bash.command'))
print(t('zh', 'delegate.description'))
print('--- all ok ---')
"
```

- [ ] **Step 3: Verify backward compat**

```python
cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "
from lingtai.capabilities.read import SCHEMA, DESCRIPTION
from lingtai.capabilities.email import SCHEMA, DESCRIPTION
from lingtai.capabilities.bash import SCHEMA, DESCRIPTION
assert isinstance(SCHEMA, dict)
assert isinstance(DESCRIPTION, str)
print('backward compat ok')
"
```

- [ ] **Step 4: Verify key count matches between en.json and zh.json**

```python
cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "
import json
en = json.load(open('src/lingtai/i18n/en.json'))
zh = json.load(open('src/lingtai/i18n/zh.json'))
missing = set(en) - set(zh)
extra = set(zh) - set(en)
assert not missing, f'Missing in zh: {missing}'
assert not extra, f'Extra in zh: {extra}'
print(f'{len(en)} keys, en/zh match ✓')
"
```
