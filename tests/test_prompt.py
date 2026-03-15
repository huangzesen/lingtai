from stoai.prompt import build_system_prompt
from stoai.intrinsics.manage_system_prompt import SystemPromptManager


def test_build_system_prompt_minimal():
    mgr = SystemPromptManager()
    prompt = build_system_prompt(mgr)
    assert "tool schemas" in prompt


def test_build_system_prompt_with_sections():
    mgr = SystemPromptManager()
    mgr.write_section("role", "You are a test agent")
    mgr.write_section("memory", "Remember: user likes concise")
    prompt = build_system_prompt(mgr)
    assert "You are a test agent" in prompt
    assert "Remember: user likes concise" in prompt
