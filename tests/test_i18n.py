"""Tests for lingtai capability i18n."""
from lingtai.i18n import t


def test_en_simple_key():
    assert "text file" in t("en", "read.description")


def test_unknown_lang_falls_back_to_en():
    assert "text file" in t("xx", "read.description")


def test_unknown_key_returns_key():
    assert t("en", "nonexistent.key") == "nonexistent.key"


def test_zh_simple_key():
    result = t("zh", "read.description")
    assert result != "read.description"  # not the fallback key
    assert "文件" in result  # Chinese text present


# --- File I/O capability get_schema / get_description tests ---


def test_capability_get_schema_en():
    from lingtai.capabilities.read import get_schema, get_description
    schema = get_schema("en")
    assert "file_path" in schema["properties"]
    desc = get_description("en")
    assert "text file" in desc.lower()


def test_capability_get_schema_zh():
    from lingtai.capabilities.read import get_schema, get_description
    schema = get_schema("zh")
    assert schema["properties"]["file_path"]["description"] != "read.file_path"
    desc = get_description("zh")
    assert desc != "read.description"


def test_write_get_schema_en():
    from lingtai.capabilities.write import get_schema, get_description
    schema = get_schema("en")
    assert "file_path" in schema["properties"]
    assert "content" in schema["properties"]
    desc = get_description("en")
    assert len(desc) > 0


def test_write_get_schema_zh():
    from lingtai.capabilities.write import get_schema, get_description
    schema = get_schema("zh")
    assert schema["properties"]["file_path"]["description"] != "write.file_path"
    desc = get_description("zh")
    assert desc != "write.description"


def test_edit_get_schema_en():
    from lingtai.capabilities.edit import get_schema, get_description
    schema = get_schema("en")
    assert "old_string" in schema["properties"]
    assert "new_string" in schema["properties"]
    assert "replace_all" in schema["properties"]
    desc = get_description("en")
    assert len(desc) > 0


def test_edit_get_schema_zh():
    from lingtai.capabilities.edit import get_schema, get_description
    schema = get_schema("zh")
    assert schema["properties"]["old_string"]["description"] != "edit.old_string"
    desc = get_description("zh")
    assert desc != "edit.description"


def test_glob_get_schema_en():
    from lingtai.capabilities.glob import get_schema, get_description
    schema = get_schema("en")
    assert "pattern" in schema["properties"]
    desc = get_description("en")
    assert "glob" in desc.lower()


def test_glob_get_schema_zh():
    from lingtai.capabilities.glob import get_schema, get_description
    schema = get_schema("zh")
    assert schema["properties"]["pattern"]["description"] != "glob.pattern"
    desc = get_description("zh")
    assert desc != "glob.description"


def test_grep_get_schema_en():
    from lingtai.capabilities.grep import get_schema, get_description
    schema = get_schema("en")
    assert "pattern" in schema["properties"]
    assert "max_matches" in schema["properties"]
    desc = get_description("en")
    assert "regex" in desc.lower()


def test_grep_get_schema_zh():
    from lingtai.capabilities.grep import get_schema, get_description
    schema = get_schema("zh")
    assert schema["properties"]["pattern"]["description"] != "grep.pattern"
    desc = get_description("zh")
    assert desc != "grep.description"


def test_backward_compat_constants():
    """SCHEMA and DESCRIPTION module-level constants still work."""
    from lingtai.capabilities.read import SCHEMA as R_S, DESCRIPTION as R_D
    from lingtai.capabilities.write import SCHEMA as W_S, DESCRIPTION as W_D
    from lingtai.capabilities.edit import SCHEMA as E_S, DESCRIPTION as E_D
    from lingtai.capabilities.glob import SCHEMA as G_S, DESCRIPTION as G_D
    from lingtai.capabilities.grep import SCHEMA as GR_S, DESCRIPTION as GR_D

    for schema in (R_S, W_S, E_S, G_S, GR_S):
        assert schema["type"] == "object"
        assert "properties" in schema
    for desc in (R_D, W_D, E_D, G_D, GR_D):
        assert isinstance(desc, str)
        assert len(desc) > 10


# --- All capabilities: no key typos ---

import importlib
import pytest

_ALL_CAPABILITIES = [
    "read", "write", "edit", "glob", "grep",
    "bash", "psyche", "avatar", "email",
    "vision", "web_search", "talk", "compose", "draw", "listen",
]


def _get_all_descriptions(schema: dict) -> list[str]:
    """Recursively extract all 'description' values from a schema dict."""
    descs = []
    if isinstance(schema, dict):
        if "description" in schema:
            descs.append(schema["description"])
        for v in schema.values():
            descs.extend(_get_all_descriptions(v))
    elif isinstance(schema, list):
        for item in schema:
            descs.extend(_get_all_descriptions(item))
    return descs


def _looks_like_i18n_key(s: str) -> bool:
    """Check if a string looks like a raw i18n key (e.g. 'read.file_path')."""
    import re
    return bool(re.fullmatch(r"[a-z_]+\.[a-z_]+", s))


@pytest.mark.parametrize("cap_name", _ALL_CAPABILITIES)
def test_all_capabilities_en_no_key_fallback(cap_name):
    """English schema descriptions should never be a raw i18n key."""
    mod = importlib.import_module(f"lingtai.capabilities.{cap_name}")
    desc = mod.get_description("en")
    assert not _looks_like_i18n_key(desc), f"{cap_name} description is a fallback key: {desc}"
    for d in _get_all_descriptions(mod.get_schema("en").get("properties", {})):
        assert not _looks_like_i18n_key(d), f"{cap_name} schema has key-like description: {d}"


@pytest.mark.parametrize("cap_name", _ALL_CAPABILITIES)
def test_all_capabilities_zh_no_key_fallback(cap_name):
    """Chinese schema descriptions should not fall back to raw i18n keys."""
    mod = importlib.import_module(f"lingtai.capabilities.{cap_name}")
    desc = mod.get_description("zh")
    assert not _looks_like_i18n_key(desc), f"{cap_name} zh description is a fallback key: {desc}"
    for d in _get_all_descriptions(mod.get_schema("zh").get("properties", {})):
        assert not _looks_like_i18n_key(d), f"{cap_name} zh schema has key-like description: {d}"
