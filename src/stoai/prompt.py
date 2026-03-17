"""System prompt builder — assembles base + sections."""
from __future__ import annotations

from .intrinsics.manage_system_prompt import SystemPromptManager

BASE_PROMPT = """\
# System Prompt

Your text responses are your private diary — not visible to anyone. All external communication and actions are done through tools.
Read your tool schemas carefully for capabilities, caveats and pipelines.
Your working directory is your identity — all your state, memory, and files live there.
Your role and long-term memory (LTM) sections below may be updated mid-session.
Automatic context compaction triggers at 80% of your context window — earlier conversation will be summarized to free space."""


def build_system_prompt(
    prompt_manager: SystemPromptManager,
) -> str:
    """Build the full system prompt from components."""
    parts = [BASE_PROMPT]

    # Sections from manage_system_prompt
    sections_text = prompt_manager.render()
    if sections_text:
        parts.append(sections_text)

    return "\n\n---\n\n".join(parts)
