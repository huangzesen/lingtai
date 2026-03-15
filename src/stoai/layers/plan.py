"""Plan layer — file-based planning."""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {"type": "string", "enum": ["create", "read", "update", "check_off"]},
        "content": {"type": "string", "description": "Plan content (for create/update)"},
        "step": {"type": "string", "description": "Step description to check off"},
    },
    "required": ["action"],
}

DESCRIPTION = "Manage a file-based plan. Create, read, update, or check off steps."


class PlanManager:
    """Manages a file-based plan."""

    def __init__(self, working_dir: Path | str | None = None):
        self._working_dir = Path(working_dir) if working_dir else Path(".")
        self._plan_path = self._working_dir / "plan.md"

    def handle(self, args: dict) -> dict:
        action = args["action"]
        if action == "create":
            return self._create(args.get("content", ""))
        elif action == "read":
            return self._read()
        elif action == "update":
            return self._update(args.get("content", ""))
        elif action == "check_off":
            return self._check_off(args.get("step", ""))
        return {"error": f"Unknown action: {action}"}

    def _create(self, content: str) -> dict:
        if not content:
            return {"error": "content is required for create"}
        self._plan_path.parent.mkdir(parents=True, exist_ok=True)
        self._plan_path.write_text(content, encoding="utf-8")
        return {"status": "ok", "path": str(self._plan_path)}

    def _read(self) -> dict:
        if not self._plan_path.exists():
            return {"error": "No plan found. Use action='create' first."}
        return {"content": self._plan_path.read_text(encoding="utf-8")}

    def _update(self, content: str) -> dict:
        if not content:
            return {"error": "content is required for update"}
        if not self._plan_path.exists():
            return {"error": "No plan found. Use action='create' first."}
        self._plan_path.write_text(content, encoding="utf-8")
        return {"status": "ok"}

    def _check_off(self, step: str) -> dict:
        if not step:
            return {"error": "step is required for check_off"}
        if not self._plan_path.exists():
            return {"error": "No plan found"}
        content = self._plan_path.read_text(encoding="utf-8")
        # Find unchecked step matching description
        target = f"- [ ] {step}"
        if target not in content:
            # Try partial match
            for line in content.splitlines():
                if "- [ ]" in line and step.lower() in line.lower():
                    target = line.strip()
                    break
            else:
                return {"error": f"Step not found: {step}"}
        checked = target.replace("- [ ]", "- [x]", 1)
        content = content.replace(target, checked, 1)
        self._plan_path.write_text(content, encoding="utf-8")
        return {"status": "ok", "checked": checked.strip()}


def add_plan_layer(agent: "BaseAgent", working_dir: Path | str | None = None) -> PlanManager:
    """Add planning capability to an agent.

    Returns the PlanManager instance for programmatic access.
    """
    mgr = PlanManager(working_dir=working_dir)
    agent.add_tool("plan", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt("plan_instructions",
        "You have a plan tool. Use it to create and track implementation plans. "
        "Create a plan before starting complex work. Check off steps as you complete them.")
    return mgr
