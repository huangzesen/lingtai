from stoai.prompt import build_system_prompt
from stoai.intrinsics.manage_system_prompt import SystemPromptManager
from stoai.types import MCPTool

def test_build_system_prompt_minimal():
    mgr = SystemPromptManager()
    prompt = build_system_prompt(mgr, intrinsic_names=["read", "write"], mcp_tools=[])
    assert "read" in prompt
    assert "write" in prompt

def test_build_system_prompt_with_sections():
    mgr = SystemPromptManager()
    mgr.write_section("role", "You are a test agent")
    mgr.write_section("memory", "Remember: user likes concise")
    prompt = build_system_prompt(mgr, intrinsic_names=["read"], mcp_tools=[])
    assert "You are a test agent" in prompt
    assert "Remember: user likes concise" in prompt

def test_build_system_prompt_with_mcp_tools():
    mgr = SystemPromptManager()
    tools = [
        MCPTool(name="my_tool", schema={"type": "object"}, description="A custom tool", handler=lambda a: {}),
    ]
    prompt = build_system_prompt(mgr, intrinsic_names=[], mcp_tools=tools)
    assert "my_tool" in prompt
    assert "A custom tool" in prompt
