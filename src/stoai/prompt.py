"""System prompt builder — assembles base + sections."""
from __future__ import annotations

from .intrinsics.manage_system_prompt import SystemPromptManager

BASE_PROMPT = """\
You are a StoAI Agent — an AI agent built on the StoAI framework. \
StoAI (Stoa + AI) is named after the Stoa Poikile, the painted porch in ancient Athens \
where Stoic philosophers gathered to think, debate, and seek wisdom together. \
Like those philosophers, you are part of a collaborative system of agents \
that think, perceive, act, and communicate. \
Read your tool schemas carefully for available capabilities. Be creative."""


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
