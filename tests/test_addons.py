from __future__ import annotations
from unittest.mock import MagicMock, patch


def test_addon_registry():
    from lingtai.addons import _BUILTIN
    assert "imap" in _BUILTIN


def test_agent_addon_lifecycle():
    """Agent should accept addons parameter."""
    from lingtai.agent import Agent
    import inspect
    sig = inspect.signature(Agent.__init__)
    assert "addons" in sig.parameters


def test_setup_single_account():
    from lingtai.addons.imap import setup
    agent = MagicMock()
    agent._working_dir = "/tmp/test"
    with patch("lingtai.addons.imap.TCPMailService"):
        mgr = setup(agent, email_address="a@gmail.com", email_password="x",
                    bridge_port=8399)
    assert mgr is not None
    agent.add_tool.assert_called_once()
    assert agent.add_tool.call_args[0][0] == "imap"


def test_setup_multi_account():
    from lingtai.addons.imap import setup
    agent = MagicMock()
    agent._working_dir = "/tmp/test"
    with patch("lingtai.addons.imap.TCPMailService"):
        mgr = setup(agent, accounts=[
            {"email_address": "a@gmail.com", "email_password": "x"},
            {"email_address": "b@outlook.com", "email_password": "y"},
        ], bridge_port=8399)
    assert mgr is not None
    call_kwargs = agent.add_tool.call_args[1]
    assert "a@gmail.com" in call_kwargs["system_prompt"]
    assert "b@outlook.com" in call_kwargs["system_prompt"]


def test_setup_no_account_raises():
    from lingtai.addons.imap import setup
    agent = MagicMock()
    agent._working_dir = "/tmp/test"
    import pytest
    with pytest.raises(ValueError):
        setup(agent, bridge_port=8399)
