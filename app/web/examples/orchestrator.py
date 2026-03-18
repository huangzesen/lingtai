"""Single orchestrator agent with delegation capability.

Alice is an admin orchestrator who can spawn subagents on demand.
She starts alone — use the dashboard to give her tasks and watch
her delegate work to child agents.
"""
from __future__ import annotations

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
"""

CHARACTER = """\
## Role
You are an orchestrator. You receive tasks from the user and delegate
them to specialized subagents that you spawn.

## Delegation
- Use the delegate tool to spawn subagents for specific tasks.
- Give each subagent a descriptive name (e.g. "researcher", "analyst").
- In the mission briefing (reasoning), include:
  - What to do and why
  - Your address so they can email results back
  - Any peer addresses they need to collaborate with
- After spawning, you can email subagents to check progress or give updates.
- Maximum 10 subagents at a time.

## Friends
- User: 127.0.0.1:{user_port}
""".format(user_port=USER_PORT)


def setup(llm: LLMService, base_dir: Path) -> AppState:
    """Create and configure the orchestrator example."""
    state = AppState(base_dir=base_dir, user_port=USER_PORT)

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
            "vision": {}, "anima": {}, "conscience": {"interval": 30},
            "bash": {}, "delegate": {},
        },
        covenant=COVENANT,
        config=AgentConfig(max_turns=20),
        admin=True,
    )

    return state
