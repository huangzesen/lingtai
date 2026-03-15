from stoai.intrinsics.manage_system_prompt import SystemPromptManager


def test_system_prompt_manager():
    mgr = SystemPromptManager()
    mgr.write_section("role", "You are an orchestrator", protected=True)
    mgr.write_section("memory", "User likes concise")
    sections = mgr.list_sections()
    assert "role" in [s["name"] for s in sections]
    assert "memory" in [s["name"] for s in sections]
    assert mgr.read_section("role") == "You are an orchestrator"
    assert mgr.read_section("memory") == "User likes concise"


def test_system_prompt_manager_render():
    mgr = SystemPromptManager()
    mgr.write_section("role", "You are a test agent")
    mgr.write_section("memory", "Remember: user likes concise")
    rendered = mgr.render()
    assert "## role" in rendered
    assert "## memory" in rendered
    assert "You are a test agent" in rendered
    assert "Remember: user likes concise" in rendered


def test_system_prompt_manager_delete():
    mgr = SystemPromptManager()
    mgr.write_section("temp", "temporary content")
    assert mgr.read_section("temp") == "temporary content"
    assert mgr.delete_section("temp") is True
    assert mgr.read_section("temp") is None
    assert mgr.delete_section("temp") is False


def test_system_prompt_manager_read_nonexistent():
    mgr = SystemPromptManager()
    assert mgr.read_section("nonexistent") is None


def test_mail_send_passes_attachments(tmp_path):
    """Mail handler should pass attachments to mail service."""
    from stoai.agent import BaseAgent
    from unittest.mock import MagicMock
    from pathlib import Path

    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"

    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = True

    agent = BaseAgent(agent_id="test", service=svc, mail_service=mail_svc, working_dir=tmp_path)

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
