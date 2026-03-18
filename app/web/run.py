"""Launch web dashboard with agents.

Usage:
    python -m app.web

Edit this file to add/remove agents, change models, or configure capabilities.
Frontend dev server: cd app/web/frontend && npm run dev
"""
from __future__ import annotations

import os
import sys
from pathlib import Path

# Load .env from project root
env_path = Path(__file__).parent.parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

import uvicorn

from stoai import AgentConfig
from stoai.llm import LLMService

from .server.main import create_app
from .server.state import AppState


COVENANT = """\
### Communication
- All communication — including with the user — is done via email.
- Addresses are ip:port format.
- Your text responses are your private diary — no one sees them.
- Email history is your long-term memory.
- Always report results back to whoever asked. Don't just do work silently.
- When emailing a peer, give enough context. Don't send one-word emails.
"""


def write_character(agent_dir: Path, contacts: dict[str, str]) -> None:
    """Write initial character.md with contacts (friends)."""
    contact_lines = "\n".join(f"- {n}: {a}" for n, a in contacts.items())
    system_dir = agent_dir / "system"
    system_dir.mkdir(parents=True, exist_ok=True)
    char_file = system_dir / "character.md"
    if not char_file.is_file():
        char_file.write_text(f"### Friends\n{contact_lines}\n")


USER_PORT = 8300
AGENTS = [
    {"key": "a", "id": "alice", "name": "Alice", "port": 8301},
    {"key": "b", "id": "bob", "name": "Bob", "port": 8302},
    {"key": "c", "id": "charlie", "name": "Charlie", "port": 8303},
]


def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set.")
        sys.exit(1)

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_config={"web_search_provider": "minimax"},
        provider_defaults={"minimax": {"model": "MiniMax-M2.5-highspeed"}},
    )

    base_dir = Path.home() / ".stoai" / "web" / "playground"
    state = AppState(base_dir=base_dir, user_port=USER_PORT)

    # Build contact list for covenants
    all_contacts = {a["name"]: f"127.0.0.1:{a['port']}" for a in AGENTS}
    all_contacts["User"] = f"127.0.0.1:{USER_PORT}"

    for a in AGENTS:
        # Each agent's contacts = all others + user (excluding self)
        contacts = {k: v for k, v in all_contacts.items() if k != a["name"]}
        # Write character.md with friends before agent init
        write_character(base_dir / a["id"], contacts)
        state.register_agent(
            key=a["key"],
            agent_name=a["id"],
            name=a["name"],
            port=a["port"],
            llm=llm,
            capabilities={
                "email": {}, "web_search": {}, "file": {},
                "vision": {}, "anima": {}, "conscience": {"interval": 10},
                "bash": {},
            },
            covenant=COVENANT,
        )

    app = create_app(state)
    state.start_all()

    print(f"User mailbox:  127.0.0.1:{USER_PORT}")
    for a in AGENTS:
        print(f"Agent {a['name']:8s}  127.0.0.1:{a['port']}")
    print("API server:    http://localhost:8080")
    print("Frontend dev:  cd app/web/frontend && npm run dev")
    print("Press Ctrl+C to shut down.")

    try:
        uvicorn.run(app, host="0.0.0.0", port=8080)
    except KeyboardInterrupt:
        print("\nShutting down...")
    finally:
        state.stop_all()
        print("Done.")


if __name__ == "__main__":
    main()
