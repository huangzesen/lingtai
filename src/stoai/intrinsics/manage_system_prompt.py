"""System prompt manager — dynamic system prompt section management.

Python-only API used by BaseAgent.update_system_prompt(). Not exposed as an LLM tool.
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
        """Render all sections into a single system prompt string."""
        parts = []
        for name, entry in self._sections.items():
            parts.append(f"## {name}\n{entry['content']}")
        return "\n\n".join(parts)
