"""Tests for stoai capability i18n."""
from stoai.i18n import t


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
