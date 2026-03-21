from lingtai_kernel.types import UnknownToolError


def test_unknown_tool_error():
    err = UnknownToolError("bad_tool")
    assert "bad_tool" in str(err)
