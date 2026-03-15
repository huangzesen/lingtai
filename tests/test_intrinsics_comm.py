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
