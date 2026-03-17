"""System prompt — section manager + builder.

SystemPromptManager manages named sections (covenant, memory, etc.) of an agent's
system prompt. build_system_prompt() assembles the base prompt + rendered
sections into the final string sent to the LLM.
"""
from __future__ import annotations

from typing import Optional


class SystemPromptManager:
    """Manages named sections of an agent's system prompt.

    Sections can be marked as protected (host-written, not overwritable by the LLM)
    or unprotected (LLM-writable at runtime).
    """

    def __init__(self) -> None:
        # {name: {"content": str, "protected": bool}}
        self._sections: dict[str, dict] = {}

    def write_section(self, name: str, content: str, protected: bool = False) -> None:
        """Write a section (host API — bypasses protection checks)."""
        self._sections[name] = {"content": content, "protected": protected}

    def read_section(self, name: str) -> Optional[str]:
        """Read a section's content, or None if not found."""
        entry = self._sections.get(name)
        return entry["content"] if entry else None

    def delete_section(self, name: str) -> bool:
        """Delete a section. Returns True if it existed."""
        return self._sections.pop(name, None) is not None

    def list_sections(self) -> list[dict]:
        """Return a list of section metadata dicts."""
        return [
            {"name": name, "protected": entry["protected"], "length": len(entry["content"])}
            for name, entry in self._sections.items()
        ]

    def render(self) -> str:
        """Render all sections into a single system prompt string.

        Ordering: tools → covenant → rest → memory (always last).
        """
        ordered: list[str] = []
        priority = ["tools", "covenant"]

        for key in priority:
            entry = self._sections.get(key)
            if entry:
                ordered.append(f"## {key}\n{entry['content']}")

        for name, entry in self._sections.items():
            if name in priority or name == "memory":
                continue
            ordered.append(f"## {name}\n{entry['content']}")

        # Memory always last
        mem = self._sections.get("memory")
        if mem:
            ordered.append(f"## memory\n{mem['content']}")

        return "\n\n".join(ordered)


BASE_PROMPT = """\
# System Prompt

Read your tool schemas carefully for capabilities, caveats and pipelines.
Your working directory is your identity — all your state, memory, and files live there.
Your memory section below may be updated mid-session.
Automatic context compaction triggers at 80% of your context window — earlier conversation will be summarized to free space."""


def build_system_prompt(
    prompt_manager: SystemPromptManager,
) -> str:
    """Build the full system prompt from components."""
    parts = [BASE_PROMPT]

    sections_text = prompt_manager.render()
    if sections_text:
        parts.append(sections_text)

    return "\n\n---\n\n".join(parts)
