"""System prompt builder — assembles base + sections + MCP tool descriptions."""
from __future__ import annotations

from .intrinsics import ALL_INTRINSICS
from .intrinsics.manage_system_prompt import SystemPromptManager
from .types import MCPTool

BASE_PROMPT = """You are an AI research agent with intrinsic capabilities.

You have the following built-in tools that are always available:
{intrinsic_list}

You may also have domain-specific tools provided by connected services."""


def build_system_prompt(
    prompt_manager: SystemPromptManager,
    intrinsic_names: list[str],
    mcp_tools: list[MCPTool],
) -> str:
    """Build the full system prompt from components."""
    parts = []

    # Base prompt with intrinsic tool list
    intrinsic_descs = []
    for name in intrinsic_names:
        info = ALL_INTRINSICS.get(name)
        if info:
            intrinsic_descs.append(f"- **{name}**: {info['description']}")
    intrinsic_list = "\n".join(intrinsic_descs) if intrinsic_descs else "(none)"
    parts.append(BASE_PROMPT.format(intrinsic_list=intrinsic_list))

    # Sections from manage_system_prompt
    sections_text = prompt_manager.render()
    if sections_text:
        parts.append(sections_text)

    # MCP tool descriptions
    if mcp_tools:
        tool_descs = []
        for tool in mcp_tools:
            tool_descs.append(f"- **{tool.name}**: {tool.description}")
        parts.append("## Available Domain Tools\n\n" + "\n".join(tool_descs))

    return "\n\n---\n\n".join(parts)
