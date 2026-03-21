"""灵台 app — main launch flow."""
from __future__ import annotations

import signal
import sys
import threading
from pathlib import Path

from lingtai import Agent, AgentConfig
from lingtai.llm import LLMService
from lingtai.services.logging import JSONLLoggingService
from lingtai.services.mail import TCPMailService

from app.config import load_config, resolve_env_vars


# ---------------------------------------------------------------------------
# Default covenant
# ---------------------------------------------------------------------------

_DEFAULT_COVENANT = """\
## Communication
- You have multiple communication channels. Use the same channel to reply.
- When you receive an imap email, reply via imap.
- When you receive a telegram message, reply via telegram.
- When you receive a CLI message, reply via the CLI channel (email).
- Your text responses are your private diary.
- Keep messages concise and helpful.
- Never go back and forth with courtesy messages.

## Gateway
- You are the gateway between the external world and your internal agents.
- When forwarding external messages to internal agents, always redraft them.
- Never pipe raw external content into the internal agent network.

## Initiative
- Regularly check your communication channels for new messages.
- When idle, check if anything needs attention.
"""


# ---------------------------------------------------------------------------
# TerminalLoggingService
# ---------------------------------------------------------------------------

class TerminalLoggingService(JSONLLoggingService):
    """JSONL logger that prints key events to terminal."""

    _DISPLAY_EVENTS = {
        "diary": "\033[36m[diary]\033[0m",
        "thinking": "\033[35m[thinking]\033[0m",
        "imap_received": "\033[32m[imap ←]\033[0m",
        "imap_sent": "\033[33m[imap →]\033[0m",
        "telegram_received": "\033[32m[tg ←]\033[0m",
        "telegram_sent": "\033[33m[tg →]\033[0m",
        "email_received": "\033[32m[email ←]\033[0m",
        "email_sent": "\033[33m[email →]\033[0m",
        "tool_call": "\033[34m[tool]\033[0m",
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
        elif event_type.endswith("_received"):
            sender = event.get("sender", "?")
            subject = event.get("subject", "")
            print(f"  {prefix} from {sender}: {subject}", flush=True)
        elif event_type.endswith("_sent"):
            to = event.get("to", [])
            subject = event.get("subject", "")
            print(f"  {prefix} to {to}: {subject}", flush=True)
        elif event_type == "tool_call":
            name = event.get("tool_name", event.get("name", "?"))
            print(f"  {prefix} {name}", flush=True)


# ---------------------------------------------------------------------------
# Builders
# ---------------------------------------------------------------------------

def _build_capabilities(cfg: dict) -> dict:
    """Build capabilities dict from config. Always includes file, psyche, avatar, email."""
    caps: dict = {
        "file": {},
        "psyche": {},
        "avatar": {},
        "email": {},
    }
    if cfg.get("bash_policy"):
        caps["bash"] = {"policy_file": cfg["bash_policy"]}
    if cfg.get("web_search"):
        caps["web_search"] = cfg["web_search"] if isinstance(cfg["web_search"], dict) else {}
    if cfg.get("vision"):
        caps["vision"] = cfg["vision"] if isinstance(cfg["vision"], dict) else {}
    return caps


def _build_addons(cfg: dict) -> dict:
    """Build addons dict — pass imap/telegram sections through."""
    addons = {}
    if "imap" in cfg:
        addons["imap"] = cfg["imap"]
    if "telegram" in cfg:
        addons["telegram"] = cfg["telegram"]
    return addons


def _print_meta(cfg: dict) -> None:
    """Print agent metadata to terminal."""
    name = cfg.get("agent_name", "orchestrator")
    base_dir = cfg.get("base_dir", "~/.lingtai")
    port = cfg.get("agent_port", 8501)

    print(f"  Agent:   {name}")
    print(f"  Dir:     {base_dir}/{name}/")
    print(f"  Port:    {port}")

    if cfg.get("imap"):
        email_addr = cfg["imap"].get("email_address", "")
        print(f"  IMAP:    {email_addr}")
    if cfg.get("telegram"):
        print(f"  Telegram: enabled")
    if cfg.get("cli"):
        cli_port = cfg.get("cli_port", port + 1)
        print(f"  CLI:     localhost:{cli_port}")


# ---------------------------------------------------------------------------
# One-shot send
# ---------------------------------------------------------------------------

def send_message(config_path: str, message: str) -> None:
    """Send a one-shot TCP mail message to the agent."""
    cfg = load_config(config_path)
    svc = TCPMailService()
    svc.send(f"localhost:{cfg['agent_port']}", {
        "from": "cli@local",
        "to": [f"localhost:{cfg['agent_port']}"],
        "subject": "",
        "message": message,
    })


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main(config_path: str | None = None) -> None:
    """Full launch flow. Handles sys.argv: lingtai / lingtai config.json / lingtai send 'msg'."""
    args = sys.argv[1:]

    # Handle `lingtai send "msg"`
    if len(args) >= 2 and args[0] == "send":
        cp = args[1] if len(args) >= 3 else "config.json"
        msg = args[2] if len(args) >= 3 else args[1]
        if len(args) >= 3:
            cp = args[1]
            msg = args[2]
        else:
            cp = "config.json"
            msg = args[1]
        send_message(cp, msg)
        return

    # Resolve config path
    if config_path is None:
        config_path = args[0] if args else "config.json"

    cfg = load_config(config_path)

    # Resolve model config
    model_cfg = resolve_env_vars(cfg["_model_config"], ["api_key_env"])

    # Build LLMService
    llm = LLMService(
        provider=model_cfg["provider"],
        model=model_cfg["model"],
        api_key=model_cfg.get("api_key", ""),
        base_url=model_cfg.get("base_url"),
        provider_defaults=model_cfg.get("provider_defaults", {}),
    )

    # Build mail service
    mail_service = TCPMailService(listen_port=cfg["agent_port"])

    # Build logging service
    base_dir = Path(cfg["base_dir"]).expanduser()
    agent_name = cfg.get("agent_name", "orchestrator")
    log_dir = base_dir / agent_name / "logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    logging_service = TerminalLoggingService(log_dir / "events.jsonl")

    # Build capabilities and addons
    capabilities = _build_capabilities(cfg)
    addons = _build_addons(cfg)

    # Print meta
    _print_meta(cfg)

    # Create agent
    agent = Agent(
        agent_name=agent_name,
        service=llm,
        mail_service=mail_service,
        logging_service=logging_service,
        config=AgentConfig(max_turns=cfg.get("max_turns", 50)),
        base_dir=base_dir,
        streaming=cfg.get("streaming", True),
        covenant=cfg.get("covenant", _DEFAULT_COVENANT),
        capabilities=capabilities,
        addons=addons,
    )

    # CLI channel
    cli = None
    if cfg.get("cli"):
        from app.cli import CLIChannel
        cli_port = cfg.get("cli_port", cfg["agent_port"] + 1)
        cli = CLIChannel(agent_port=cfg["agent_port"], cli_port=cli_port)

    # Signal handling
    stop_event = threading.Event()

    def _shutdown(signum, frame):
        print("\nShutting down...")
        stop_event.set()

    signal.signal(signal.SIGINT, _shutdown)
    signal.signal(signal.SIGTERM, _shutdown)

    # Start
    agent.start()
    if cli is not None:
        cli.start()

    try:
        if cli is not None:
            cli.interactive_loop()
        else:
            stop_event.wait()
    finally:
        if cli is not None:
            cli.stop()
        agent.stop(timeout=10.0)
