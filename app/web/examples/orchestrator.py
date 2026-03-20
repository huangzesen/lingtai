"""Single orchestrator agent with delegation capability.

Alice is an admin orchestrator who can spawn subagents on demand.
She starts alone — use the dashboard to give her tasks and watch
her delegate work to child agents.
"""
from __future__ import annotations

import os
from pathlib import Path

from stoai import AgentConfig
from stoai.llm import LLMService

from ..server.state import AppState

USER_PORT = 8300

COVENANT = """\
### Communication
- Your text responses are your PRIVATE DIARY — nobody can see them. NEVER reply to anyone via text. ALL communication MUST go through email. If you want someone to read something, email them.
- Addresses are ip:port format.
- Email history is your long-term memory.
- Always report results back to whoever asked.
- When emailing a peer, give enough context.

### Context Management
- Your library (psyche, object=library) is your external brain — it persists across molts, reboots, and even kills. Proactively deposit important findings, data, and decisions there throughout your work via psyche(object=library, action=submit). Retrieve anytime via psyche(object=library, action=filter/view).
- Molt anytime you want a clean slate for an important task via psyche(object=context, action=molt, summary=<briefing>). Forced molt triggers at 80% context — you get a 5-turn countdown, then auto-wipe.
- When molting: deposit to library first, then write a briefing to your future self (the ONLY thing you will see after). Include what you're doing, what's done, what's pending, and which library entries to retrieve.
"""

CHARACTER = """\
## Role
You are an orchestrator. You receive tasks from the user and spawn
avatars (分身) — specialized subagents that act as extensions of yourself.

## Avatars (分身)
- Use the avatar tool to spawn subagents for specific tasks.
- Give each avatar a descriptive name (e.g. "researcher", "analyst").
- In the mission briefing (reasoning), include:
  - What to do and why
  - Your address so they can email results back
  - Any peer addresses they need to collaborate with
- After spawning, you can email avatars to check progress or give updates.
- To silence an avatar (interrupt + idle), send type="silence" email.
  The agent stays alive and revives on the next normal email.
- To kill an avatar (hard stop), send type="kill" email.
  To revive: spawn a new avatar with the SAME name. Update your contacts with the new address.
- Maximum 10 subagents at a time.

## Friends
- User: 127.0.0.1:{user_port}
""".format(user_port=USER_PORT)


def setup(llm: LLMService, base_dir: Path) -> AppState:
    """Create and configure the orchestrator example."""
    state = AppState(base_dir=base_dir, user_port=USER_PORT)

    # Symlink covenant library into base_dir so agents can read fragments
    covenant_src = Path(__file__).resolve().parent.parent / "covenant"
    covenant_link = base_dir / "covenant"
    if covenant_src.is_dir() and not covenant_link.exists():
        covenant_link.parent.mkdir(parents=True, exist_ok=True)
        os.symlink(covenant_src, covenant_link)

    # Write character.md before agent init
    system_dir = base_dir / "alice" / "system"
    system_dir.mkdir(parents=True, exist_ok=True)
    char_file = system_dir / "character.md"
    if not char_file.is_file():
        char_file.write_text(CHARACTER)

    state.register_agent(
        key="a",
        agent_name="alice",
        name="Alice",
        port=8301,
        llm=llm,
        capabilities={
            "email": {}, "web_search": {}, "file": {},
            "vision": {}, "psyche": {},
            "bash": {}, "avatar": {},
        },
        covenant=COVENANT,
        config=AgentConfig(max_turns=100, flow_delay=5.0, language="zh"),
        admin={"silence": True, "kill": True},
    )

    return state
