"""System intrinsic — agent memory management.

Actions:
    diff   — show uncommitted git diff for memory.md
    load   — read the file, inject into live system prompt, git add+commit

Objects:
    memory — system/memory.md (the agent's long-term memory)
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["diff", "load"],
            "description": (
                "diff: show uncommitted git diff (what changed since last commit).\n"
                "load: read the file, inject into the live system prompt, "
                "and git commit. This updates the agent's live memory."
            ),
        },
        "object": {
            "type": "string",
            "enum": ["memory"],
            "description": "memory: the agent's long-term memory (system/memory.md).",
        },
    },
    "required": ["action", "object"],
}

DESCRIPTION = (
    "Agent memory management. Long-term memory lives in system/memory.md. "
    "Use 'diff' to see uncommitted changes, "
    "and 'load' to apply changes into the live system prompt (with git commit)."
)


def handle(agent, args: dict) -> dict:
    """Handle system tool — agent memory management."""
    action = args.get("action", "")
    obj = args.get("object", "")
    if obj != "memory":
        return {"error": f"Unknown object: {obj!r}. Must be 'memory'."}

    system_dir = agent._working_dir / "system"
    system_dir.mkdir(exist_ok=True)
    file_path = system_dir / "memory.md"
    if not file_path.is_file():
        file_path.write_text("")

    if action == "diff":
        return _diff(agent, file_path, "memory")
    elif action == "load":
        return _load(agent, file_path, "memory")
    else:
        return {"error": f"Unknown action: {action!r}. Must be 'diff' or 'load'."}


def _diff(agent, file_path, obj: str) -> dict:
    rel_path = f"system/{obj}.md"
    diff_text = agent._workdir.diff(rel_path)
    return {"status": "ok", "path": str(file_path), "git_diff": diff_text}


def _load(agent, file_path, obj: str) -> dict:
    content = file_path.read_text()
    size_bytes = len(content.encode("utf-8"))

    if content.strip():
        agent._prompt_manager.write_section(obj, content)
    else:
        agent._prompt_manager.delete_section(obj)
    agent._token_decomp_dirty = True

    if agent._chat is not None:
        agent._chat.update_system_prompt(agent._build_system_prompt())

    rel_path = f"system/{obj}.md"
    git_diff, commit_hash = agent._workdir.diff_and_commit(rel_path, obj)

    agent._log(f"system_load_{obj}", size_bytes=size_bytes, changed=commit_hash is not None)

    return {
        "status": "ok",
        "path": str(file_path),
        "size_bytes": size_bytes,
        "content_preview": content[:200],
        "diff": {
            "changed": commit_hash is not None,
            "git_diff": git_diff or "",
            "commit": commit_hash,
        },
    }
