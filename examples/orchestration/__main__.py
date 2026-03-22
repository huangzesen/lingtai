"""Orchestration service — starts the admin orchestrator agent.

Usage:
    python -m examples.orchestration

The admin agent listens on port 8301. User mailbox on port 8300.
Runtime data at ~/.lingtai/orchestration/playground/.
Press Ctrl+C to shut down.
"""
from __future__ import annotations

import json
import os
import signal
import sys
import threading
from datetime import datetime, timezone
from pathlib import Path

# Load .env
env_path = Path(__file__).parent.parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

from lingtai import Agent, AgentConfig
from lingtai.llm import LLMService
from lingtai.services.mail import FilesystemMailService

ADMIN_PORT = 8301
USER_PORT = 8300
PLAYGROUND = Path.home() / ".lingtai" / "orchestration" / "playground"
SERVICE_JSON = PLAYGROUND / "service.json"

COVENANT = """\
## Communication
- All communication is via email. Your text responses are your private diary.
- When you receive an email, process the request and email your reply to the sender.
- Keep emails concise and actionable.
- Never go back and forth with courtesy emails.

## Initiative
- When idle, reflect on ongoing tasks and check on subagents.
"""

CHARACTER = """\
## Role
You are an orchestrator agent.

## Avatars (分身)
- You can spawn avatars (subagents) using the avatar tool.
- Avatars inherit all capabilities including avatar — they can spawn their own avatars.
- Only 本我 (you) holds admin.kill — avatars cannot kill other agents unless explicitly granted.
- Generate a tailored covenant for each avatar (pass as covenant= in avatar).
- In the mission briefing (reasoning), include the peer contact list so the
  subagent knows who its friends are.
- After spawning a subagent, broadcast its address to ALL existing subagents
  by emailing each one the updated peer list.

## Friends
- User: 127.0.0.1:""" + str(USER_PORT) + """
"""


def write_service_json(status: str = "running", started_at: str = "") -> None:
    """Write service.json with current state."""
    SERVICE_JSON.write_text(json.dumps({
        "pid": os.getpid(),
        "admin_address": f"127.0.0.1:{ADMIN_PORT}",
        "user_port": USER_PORT,
        "status": status,
        "started_at": started_at or datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }, indent=2))


def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set. Check .env file.")
        sys.exit(1)

    # Create playground directory (must exist before Agent constructor)
    PLAYGROUND.mkdir(parents=True, exist_ok=True)

    print("Starting orchestration service...")
    print(f"  Playground: {PLAYGROUND}")
    print(f"  Admin port: {ADMIN_PORT}")
    print(f"  User port:  {USER_PORT}")

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_defaults={
            "minimax": {"model": "MiniMax-M2.5-highspeed"},
        },
    )

    mail_svc = FilesystemMailService(
        working_dir=PLAYGROUND / "admin",
    )

    policy = str(Path(__file__).parent.parent.parent / "src" / "lingtai" / "capabilities" / "bash_policy.json")

    # Write character.md before agent init
    char_dir = PLAYGROUND / "admin" / "system"
    char_dir.mkdir(parents=True, exist_ok=True)
    char_file = char_dir / "character.md"
    if not char_file.is_file():
        char_file.write_text(CHARACTER)

    agent = Agent(
        agent_name="admin",
        service=llm,
        mail_service=mail_svc,
        config=AgentConfig(max_turns=20),
        base_dir=PLAYGROUND,
        streaming=True,
        admin={"karma": True},
        covenant=COVENANT,
        capabilities={
            "email": {},
            "bash": {"policy_file": policy},
            "file": {},
            "web_search": {},
            "vision": {},
            "psyche": {},
            "avatar": {},
        },
    )

    agent.start()
    started_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    write_service_json("running", started_at=started_at)

    print(f"Admin agent started. PID: {os.getpid()}")
    print(f"Service info: {SERVICE_JSON}")
    print("Press Ctrl+C to shut down.")

    # Block until signal
    stop_event = threading.Event()

    def on_signal(signum, frame):
        print("\nShutting down...")
        stop_event.set()

    signal.signal(signal.SIGINT, on_signal)
    signal.signal(signal.SIGTERM, on_signal)

    stop_event.wait()

    agent.stop(timeout=10.0)
    write_service_json("stopped", started_at=started_at)
    print("Done.")


if __name__ == "__main__":
    main()
