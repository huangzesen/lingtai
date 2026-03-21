"""Launch a 灵台 agent with IMAP email — interact via real email.

Usage:
    python -m app.email

Configure via app/email/config.json (see config.example.json).
Requires an email account with IMAP/SMTP access (e.g. Gmail App Password).

Once launched, send an email to the configured address. The agent replies
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

from lingtai import Agent, AgentConfig
from lingtai.llm import LLMService
from lingtai.services.logging import JSONLLoggingService
from lingtai.services.mail import TCPMailService

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(name)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("app.email")

CONFIG_DIR = Path(__file__).parent
DEFAULT_PLAYGROUND = Path.home() / ".lingtai" / "email"


class TerminalLoggingService(JSONLLoggingService):
    """JSONL logger that also prints diary/thinking/email events to terminal."""

    # Event types to display and their prefixes
    _DISPLAY_EVENTS = {
        "diary": "\033[36m[diary]\033[0m",         # cyan
        "thinking": "\033[35m[thinking]\033[0m",    # magenta
        "imap_received": "\033[32m[imap ←]\033[0m",  # green
        "imap_sent": "\033[33m[imap →]\033[0m",      # yellow
        "email_received": "\033[32m[email ←]\033[0m",
        "email_sent": "\033[33m[email →]\033[0m",
        "tool_call": "\033[34m[tool]\033[0m",       # blue
    }

    def log(self, event: dict) -> None:
        super().log(event)
        event_type = event.get("type", "")
        prefix = self._DISPLAY_EVENTS.get(event_type)
        if prefix is None:
            return

        if event_type in ("diary", "thinking"):
            text = event.get("text", "")
            if text:
                for line in text.splitlines():
                    print(f"  {prefix} {line}", flush=True)
        elif event_type in ("imap_received", "email_received"):
            sender = event.get("sender", "?")
            subject = event.get("subject", "")
            print(f"  {prefix} from {sender}: {subject}", flush=True)
        elif event_type in ("imap_sent", "email_sent"):
            to = event.get("to", [])
            subject = event.get("subject", "")
            print(f"  {prefix} to {to}: {subject}", flush=True)
        elif event_type == "tool_call":
            name = event.get("tool_name", event.get("name", "?"))
            print(f"  {prefix} {name}", flush=True)


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

    # Email settings (IMAP/SMTP)
    email_address = cfg.get("email_address")
    email_password = cfg.get("email_password") or os.environ.get("EMAIL_APP_PASSWORD")
    if not email_address or not email_password:
        print("Error: email_address and email_password (or EMAIL_APP_PASSWORD env) required")
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
    model = cfg.get("model", "MiniMax-M2.7-highspeed")
    api_key_env = cfg.get("api_key_env", f"{provider.upper()}_API_KEY")
    api_key = os.environ.get(api_key_env)
    if not api_key:
        print(f"Error: {api_key_env} not set in environment or .env")
        sys.exit(1)

    # LLM
    provider_defaults = {}
    if cfg.get("web_search_provider"):
        provider_defaults["web_search_provider"] = cfg["web_search_provider"]
    if cfg.get("vision_provider"):
        provider_defaults["vision_provider"] = cfg["vision_provider"]

    llm = LLMService(
        provider=provider,
        model=model,
        api_key=api_key,
        base_url=cfg.get("base_url"),
        provider_defaults=provider_defaults,
    )

    # TCP mail service for inter-agent communication
    mail_svc = TCPMailService(
        listen_port=agent_port,
        working_dir=playground / agent_name,
    )

    # Terminal logging service — shows diary/thinking in terminal
    log_svc = TerminalLoggingService(
        path=playground / agent_name / "logs" / "events.jsonl"
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

    # IMAP addon
    addons = {
        "imap": {
            "email_address": email_address,
            "email_password": email_password,
            "allowed_senders": allowed_senders or None,
            "poll_interval": cfg.get("poll_interval", 30),
            "bridge_port": bridge_port,
            "imap_host": cfg.get("imap_host", "imap.gmail.com"),
            "smtp_host": cfg.get("smtp_host", "smtp.gmail.com"),
        },
    }

    # Covenant
    covenant = cfg.get("covenant", (
        "## Communication\n"
        "- You have two mailboxes: `email` (inter-agent) and `imap` (external).\n"
        "- When you receive an imap email, process it and reply via imap.\n"
        "- When you receive an inter-agent email, reply via email.\n"
        "- Your text responses are your private diary.\n"
        "- Keep emails concise and helpful.\n"
        "- Never go back and forth with courtesy emails.\n"
        "\n"
        "## Initiative\n"
        "- Regularly check your imap inbox for new or unreplied emails.\n"
        "- When idle, use imap(action=\"check\") to see if anything needs attention.\n"
        "- If you find unreplied emails, read and respond to them.\n"
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

    # Replace default logging service with terminal-printing one
    if agent._log_service is not None:
        agent._log_service.close()
    agent._log_service = log_svc

    agent.start()

    print()
    print(f"  Agent:      {agent_name}")
    print(f"  TCP:        127.0.0.1:{agent_port} (inter-agent)")
    print(f"  IMAP:       {email_address}")
    print(f"  Bridge:     127.0.0.1:{bridge_port} (TCP → IMAP/SMTP)")
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
