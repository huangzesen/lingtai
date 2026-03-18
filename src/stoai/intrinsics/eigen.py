"""Eigen intrinsic — bare essentials of agent self.

Objects:
    memory — edit/load system/memory.md (agent's working notes)
    context — molt (shed context, keep a briefing)

Internal:
    context_forget — forced molt with system message (after ignored warnings)
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "object": {
            "type": "string",
            "enum": ["memory", "context"],
            "description": (
                "memory: your working notes (system/memory.md). "
                "context: manage conversation context."
            ),
        },
        "action": {
            "type": "string",
            "enum": ["edit", "load", "molt"],
            "description": (
                "memory: edit | load.\n"
                "context: molt."
            ),
        },
        "content": {
            "type": "string",
            "description": "Text content for memory edit.",
        },
        "summary": {
            "type": "string",
            "description": (
                "For context molt: a briefing to your future self — "
                "the ONLY thing you will see after molt. "
                "Write what you are doing, what you have found, "
                "what remains to be done, and who you are working with. "
                "~10000 tokens max."
            ),
        },
    },
    "required": ["object", "action"],
}

DESCRIPTION = (
    "Core self-management — working notes and context control.\n"
    "memory: edit to write your working notes (system/memory.md), "
    "load to inject them into your active prompt.\n"
    "context: molt to molt — write a briefing to your future self, "
    "your conversation history is wiped and your summary becomes the new starting context. "
    "Before molting, save important data elsewhere first."
)


def handle(agent, args: dict) -> dict:
    """Handle eigen tool — memory and context management."""
    obj = args.get("object", "")
    action = args.get("action", "")

    if obj == "memory":
        if action == "edit":
            return _memory_edit(agent, args)
        elif action == "load":
            return _memory_load(agent, args)
        else:
            return {"error": f"Unknown memory action: {action}. Use edit or load."}
    elif obj == "context":
        if action == "molt":
            return _context_molt(agent, args)
        else:
            return {"error": f"Unknown context action: {action}. Use molt."}
    else:
        return {"error": f"Unknown object: {obj}. Use memory or context."}


def _memory_edit(agent, args: dict) -> dict:
    """Write content to system/memory.md."""
    content = args.get("content", "")

    system_dir = agent._working_dir / "system"
    system_dir.mkdir(exist_ok=True)
    mem_path = system_dir / "memory.md"
    mem_path.write_text(content)

    agent._log("eigen_memory_edit", length=len(content))
    return {"status": "ok", "path": str(mem_path), "size_bytes": len(content.encode("utf-8"))}


def _memory_load(agent, args: dict) -> dict:
    """Load system/memory.md into the system prompt."""
    system_dir = agent._working_dir / "system"
    system_dir.mkdir(exist_ok=True)
    mem_path = system_dir / "memory.md"
    if not mem_path.is_file():
        mem_path.write_text("")

    content = mem_path.read_text()
    size_bytes = len(content.encode("utf-8"))

    if content.strip():
        agent._prompt_manager.write_section("memory", content)
    else:
        agent._prompt_manager.delete_section("memory")
    agent._token_decomp_dirty = True

    if agent._chat is not None:
        agent._chat.update_system_prompt(agent._build_system_prompt())

    rel_path = "system/memory.md"
    git_diff, commit_hash = agent._workdir.diff_and_commit(rel_path, "memory")

    agent._log("eigen_memory_load", size_bytes=size_bytes, changed=commit_hash is not None)

    return {
        "status": "ok",
        "path": str(mem_path),
        "size_bytes": size_bytes,
        "content_preview": content[:200],
        "diff": {
            "changed": commit_hash is not None,
            "git_diff": git_diff or "",
            "commit": commit_hash,
        },
    }


def _context_molt(agent, args: dict) -> dict:
    """Agent molt: summary IS the briefing, wipe + re-inject."""
    summary = args.get("summary")
    if summary is None:
        return {"error": "summary is required — write a briefing to your future self."}
    if not summary.strip():
        return {"error": "summary cannot be empty — write what you need to remember."}

    if agent._chat is None:
        return {"error": "No active chat session to molt."}

    before_tokens = agent._chat.interface.estimate_context_tokens()

    # Wipe context and start fresh session
    agent._session._chat = None
    agent._session._interaction_id = None
    agent._session.ensure_session()

    # Inject the agent's summary as the opening context
    from ..llm.interface import TextBlock
    iface = agent._session._chat.interface
    iface.add_user_message(f"[Previous conversation summary]\n{summary}")
    iface.add_assistant_message(
        [TextBlock(text="Understood. I have my previous context restored.")],
    )

    after_tokens = iface.estimate_context_tokens()

    # Reset molt warnings since agent just molted
    if hasattr(agent._session, "_compaction_warnings"):
        agent._session._compaction_warnings = 0

    agent._log(
        "eigen_molt",
        before_tokens=before_tokens,
        after_tokens=after_tokens,
    )

    return {
        "status": "ok",
        "before_tokens": before_tokens,
        "after_tokens": after_tokens,
    }


def context_forget(agent) -> dict:
    """Forced molt with system message. Internal only — not exposed in SCHEMA.

    Called by base_agent auto-forget after ignored molt warnings.
    Same mechanism as molt, just with a system-authored summary.
    """
    return _context_molt(agent, {
        "summary": (
            "[System-initiated molt — you ignored 5 warnings.]\n"
            "Context wiped by system. Check persistent knowledge "
            "(mail, email, library) to recover context."
        ),
    })
