"""Launch a StoAI agent with Gmail — interact via real email.

Usage:
    python -m app.email

Configure via app/email/config.json (see config.example.json).
Requires a Gmail account with an App Password (2FA + myaccount.google.com → App Passwords).

Once launched, send an email to the configured Gmail address. The agent replies
back to your email. No CLI, no web UI needed.
"""
from __future__ import annotations

import json
import logging
import os
import signal
import sys
import threading
from pathlib import Path

# Load .env from project root
env_path = Path(__file__).parent.parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

from stoai import Agent, AgentConfig
from stoai.llm import LLMService
from stoai.services.mail import TCPMailService

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(name)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("app.email")

CONFIG_DIR = Path(__file__).parent
DEFAULT_PLAYGROUND = Path.home() / ".stoai" / "email"


def load_config() -> dict:
    """Load config.json from app/email/ directory."""
    config_file = CONFIG_DIR / "config.json"
    if not config_file.is_file():
        print(f"Error: No config.json found at {config_file}")
        print(f"Copy config.example.json and fill in your details:")
        print(f"  cp {CONFIG_DIR / 'config.example.json'} {config_file}")
        sys.exit(1)
    return json.loads(config_file.read_text())


def main():
    cfg = load_config()

    # Gmail settings
    gmail_address = cfg.get("gmail_address")
    gmail_password = cfg.get("gmail_password") or os.environ.get("GMAIL_APP_PASSWORD")
    if not gmail_address or not gmail_password:
        print("Error: gmail_address and gmail_password (or GMAIL_APP_PASSWORD env) required")
        sys.exit(1)

    allowed_senders = cfg.get("allowed_senders", [])

    # Agent settings
    agent_name = cfg.get("agent_name", "agent")
    agent_port = cfg.get("agent_port", 8301)
    bridge_port = cfg.get("bridge_port", 8399)
    playground = Path(cfg.get("playground", str(DEFAULT_PLAYGROUND)))
    playground.mkdir(parents=True, exist_ok=True)

    # LLM settings
    provider = cfg.get("provider", "minimax")
    model = cfg.get("model", "MiniMax-M2.5-highspeed")
    api_key_env = cfg.get("api_key_env", f"{provider.upper()}_API_KEY")
    api_key = os.environ.get(api_key_env)
    if not api_key:
        print(f"Error: {api_key_env} not set in environment or .env")
        sys.exit(1)

    # LLM
    provider_config = {}
    if cfg.get("web_search_provider"):
        provider_config["web_search_provider"] = cfg["web_search_provider"]
    if cfg.get("vision_provider"):
        provider_config["vision_provider"] = cfg["vision_provider"]

    llm = LLMService(
        provider=provider,
        model=model,
        api_key=api_key,
        base_url=cfg.get("base_url"),
        provider_config=provider_config,
    )

    # TCP mail service for inter-agent communication
    mail_svc = TCPMailService(
        listen_port=agent_port,
        working_dir=playground / agent_name,
    )

    # Character
    character = cfg.get("character", (
        "## Role\n"
        "You are a personal AI assistant reachable by email.\n"
        "You help with questions, research, writing, and analysis."
    ))
    char_dir = playground / agent_name / "system"
    char_dir.mkdir(parents=True, exist_ok=True)
    char_file = char_dir / "character.md"
    if not char_file.is_file():
        char_file.write_text(character)

    # Capabilities (inter-agent email + others)
    capabilities = cfg.get("capabilities", {
        "email": {},
        "file": {},
        "web_search": {},
        "anima": {},
    })

    # Gmail addon
    addons = {
        "gmail": {
            "gmail_address": gmail_address,
            "gmail_password": gmail_password,
            "allowed_senders": allowed_senders or None,
            "poll_interval": cfg.get("poll_interval", 30),
            "bridge_port": bridge_port,
        },
    }

    # Covenant
    covenant = cfg.get("covenant", (
        "## Communication\n"
        "- You have two mailboxes: `email` (inter-agent) and `gmail` (external).\n"
        "- When you receive a gmail, process it and reply via gmail.\n"
        "- When you receive an inter-agent email, reply via email.\n"
        "- Your text responses are your private diary.\n"
        "- Keep emails concise and helpful.\n"
        "- Never go back and forth with courtesy emails."
    ))

    # Create agent
    agent = Agent(
        agent_name=agent_name,
        service=llm,
        mail_service=mail_svc,
        config=AgentConfig(max_turns=cfg.get("max_turns", 20)),
        base_dir=playground,
        streaming=True,
        covenant=covenant,
        capabilities=capabilities,
        addons=addons,
    )

    agent.start()

    print()
    print(f"  Agent:      {agent_name}")
    print(f"  TCP:        127.0.0.1:{agent_port} (inter-agent)")
    print(f"  Gmail:      {gmail_address}")
    print(f"  Bridge:     127.0.0.1:{bridge_port} (TCP → Gmail)")
    if allowed_senders:
        print(f"  Accepts:    {', '.join(allowed_senders)}")
    else:
        print(f"  Accepts:    anyone")
    print(f"  Data:       {playground}")
    print()
    print("Send an email to interact. Press Ctrl+C to shut down.")
    print()

    # Block until signal
    stop_event = threading.Event()

    def on_signal(signum, frame):
        print("\nShutting down...")
        stop_event.set()

    signal.signal(signal.SIGINT, on_signal)
    signal.signal(signal.SIGTERM, on_signal)

    stop_event.wait()
    agent.stop(timeout=10.0)
    print("Done.")


if __name__ == "__main__":
    main()
