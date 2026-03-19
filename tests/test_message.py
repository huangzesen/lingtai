from stoai.message import Message, _make_message, MSG_REQUEST, MSG_USER_INPUT


def test_msg_constants():
    assert MSG_REQUEST == "request"
    assert MSG_USER_INPUT == "user_input"


def test_make_message():
    msg = _make_message(MSG_REQUEST, "user", "hello")
    assert msg.type == "request"
    assert msg.sender == "user"
    assert "hello" in msg.content
    assert msg.id.startswith("msg_")
    assert msg._reply_event is None


def test_message_reply_event():
    import threading
    evt = threading.Event()
    msg = _make_message(MSG_REQUEST, "user", "test", reply_event=evt)
    assert msg._reply_event is evt
