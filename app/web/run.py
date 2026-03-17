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


def make_covenant(name: str, address: str, contacts: dict[str, str]) -> str:
    """Build a structured covenant for an agent."""
    contact_lines = "\n".join(f"- {n}: {a}" for n, a in contacts.items())
    return (
        f"### Identity\n"
        f"Name: {name}\n"
        f"Address: {address}\n"
        f"\n"
        f"### Communication\n"
        f"- All communication — including with the user — is done via email.\n"
        f"- Addresses are ip:port format.\n"
        f"- Your text responses are your private diary — no one sees them.\n"
        f"- Email history is your long-term memory.\n"
        f"- Always report results back to whoever asked. Don't just do work silently.\n"
        f"- When emailing a peer, give enough context. Don't send one-word emails.\n"
        f"\n"
        f"### Contacts\n"
        f"{contact_lines}"
    )


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
        state.register_agent(
            key=a["key"],
            agent_id=a["id"],
            name=a["name"],
            port=a["port"],
            llm=llm,
            capabilities={"email": {}, "web_search": {}, "file": {}, "bash": {}},
            covenant=make_covenant(a["name"], f"127.0.0.1:{a['port']}", contacts),
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
