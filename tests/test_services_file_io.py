"""Tests for FileIOService and LocalFileIOService."""
import os
import tempfile
from pathlib import Path

import pytest

from lingtai.services.file_io import LocalFileIOService, GrepMatch


@pytest.fixture
def tmp_dir():
    with tempfile.TemporaryDirectory() as d:
        yield Path(d)


@pytest.fixture
def svc(tmp_dir):
    return LocalFileIOService(root=tmp_dir)


class TestLocalFileIOService:
    def test_write_and_read(self, svc, tmp_dir):
        svc.write("hello.txt", "Hello, world!")
        assert svc.read("hello.txt") == "Hello, world!"

    def test_write_creates_parents(self, svc, tmp_dir):
        svc.write("sub/dir/file.txt", "nested")
        assert svc.read("sub/dir/file.txt") == "nested"

    def test_read_nonexistent_raises(self, svc):
        with pytest.raises(FileNotFoundError):
            svc.read("nope.txt")

    def test_edit(self, svc):
        svc.write("edit.txt", "hello world")
        result = svc.edit("edit.txt", "hello", "goodbye")
        assert result == "goodbye world"
        assert svc.read("edit.txt") == "goodbye world"

    def test_edit_not_found_raises(self, svc):
        svc.write("edit.txt", "hello world")
        with pytest.raises(ValueError, match="not found"):
            svc.edit("edit.txt", "missing", "replacement")

    def test_edit_ambiguous_raises(self, svc):
        svc.write("edit.txt", "aaa aaa")
        with pytest.raises(ValueError, match="appears 2 times"):
            svc.edit("edit.txt", "aaa", "bbb")

    def test_glob(self, svc, tmp_dir):
        svc.write("a.py", "# a")
        svc.write("b.py", "# b")
        svc.write("c.txt", "# c")
        results = svc.glob("*.py")
        assert len(results) == 2
        assert all(r.endswith(".py") for r in results)

    def test_glob_nested(self, svc, tmp_dir):
        svc.write("src/main.py", "# main")
        svc.write("src/utils.py", "# utils")
        svc.write("tests/test.py", "# test")
        results = svc.glob("src/*.py")
        assert len(results) == 2

    def test_grep(self, svc, tmp_dir):
        svc.write("a.txt", "hello world\ngoodbye world\nhello again")
        results = svc.grep("hello")
        assert len(results) == 2
        assert results[0].line_number == 1
        assert results[1].line_number == 3

    def test_grep_regex(self, svc, tmp_dir):
        svc.write("a.txt", "foo123\nbar456\nfoo789")
        results = svc.grep(r"foo\d+")
        assert len(results) == 2

    def test_grep_single_file(self, svc, tmp_dir):
        svc.write("a.txt", "match here")
        svc.write("b.txt", "match here too")
        results = svc.grep("match", str(tmp_dir / "a.txt"))
        assert len(results) == 1

    def test_grep_max_results(self, svc, tmp_dir):
        lines = "\n".join(f"line {i}" for i in range(100))
        svc.write("big.txt", lines)
        results = svc.grep("line", max_results=5)
        assert len(results) == 5

    def test_absolute_paths(self, tmp_dir):
        svc = LocalFileIOService()  # no root
        path = str(tmp_dir / "abs.txt")
        svc.write(path, "absolute")
        assert svc.read(path) == "absolute"
