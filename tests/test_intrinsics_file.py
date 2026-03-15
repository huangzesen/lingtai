import os
import tempfile
from pathlib import Path
from stoai.intrinsics.read import handle_read, SCHEMA as READ_SCHEMA
from stoai.intrinsics.write import handle_write, SCHEMA as WRITE_SCHEMA
from stoai.intrinsics.edit import handle_edit, SCHEMA as EDIT_SCHEMA
from stoai.intrinsics.glob import handle_glob, SCHEMA as GLOB_SCHEMA
from stoai.intrinsics.grep import handle_grep, SCHEMA as GREP_SCHEMA

def test_write_and_read(tmp_path):
    path = str(tmp_path / "test.txt")
    result = handle_write({"file_path": path, "content": "hello world"})
    assert result["status"] == "ok"
    result = handle_read({"file_path": path})
    assert "hello world" in result["content"]

def test_edit(tmp_path):
    path = str(tmp_path / "test.txt")
    handle_write({"file_path": path, "content": "foo bar baz"})
    result = handle_edit({"file_path": path, "old_string": "bar", "new_string": "qux"})
    assert result["status"] == "ok"
    result = handle_read({"file_path": path})
    assert "foo qux baz" in result["content"]

def test_glob(tmp_path):
    (tmp_path / "a.py").write_text("pass")
    (tmp_path / "b.txt").write_text("text")
    result = handle_glob({"pattern": "*.py", "path": str(tmp_path)})
    assert len(result["matches"]) == 1
    assert "a.py" in result["matches"][0]

def test_grep(tmp_path):
    (tmp_path / "file.py").write_text("def hello():\n    return 42\n")
    result = handle_grep({"pattern": "def hello", "path": str(tmp_path)})
    assert len(result["matches"]) > 0

def test_schemas_are_dicts():
    for schema in [READ_SCHEMA, WRITE_SCHEMA, EDIT_SCHEMA, GLOB_SCHEMA, GREP_SCHEMA]:
        assert isinstance(schema, dict)
        assert "properties" in schema or "type" in schema
