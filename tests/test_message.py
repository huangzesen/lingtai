from lingtai_kernel.message import Message, _make_message, MSG_REQUEST, MSG_USER_INPUT


def test_msg_constants():
    assert MSG_REQUEST == "request"
    assert MSG_USER_INPUT == "user_input"


def test_make_message():
    msg = _make_message(MSG_REQUEST, "user", "hello")
    assert msg.type == "request"
    assert msg.sender == "user"
    assert "hello" in msg.content
    assert msg.id.startswith("msg_")
