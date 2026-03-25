#!/usr/bin/env python3
"""Quick interactive agent with Telegram addon.

Usage:
    source venv/bin/activate
    python scripts/telegram_chat.py

Reads MINIMAX_API_KEY, TELEGRAM_BOT_TOKEN, and TELEGRAM_CHAT_ID from .env.
"""
from __future__ import annotations

import logging
import os
import signal
import sys
import time
from pathlib import Path

# Load .env
env_path = Path(__file__).resolve().parent.parent / ".env"
if env_path.is_file():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, _, val = line.partition("=")
        val = val.strip().strip("'\"")
        os.environ.setdefault(key.strip(), val)

# Verbose logging so we can watch what happens
logging.basicConfig(
    level=logging.DEBUG,
    format="%(asctime)s %(levelname)-5s %(name)s: %(message)s",
    datefmt="%H:%M:%S",
    stream=sys.stdout,
)
# Quiet down noisy loggers
for noisy in ("httpx", "httpcore", "hpack", "urllib3"):
    logging.getLogger(noisy).setLevel(logging.WARNING)

# Ensure adapters are registered
import lingtai  # noqa: F401

from lingtai.llm.service import LLMService
from lingtai.agent import Agent

BOT_TOKEN = os.environ["TELEGRAM_BOT_TOKEN"]
CHAT_ID = int(os.environ["TELEGRAM_CHAT_ID"])
WORKING_DIR = Path(__file__).resolve().parent.parent / "_telegram_agent"


def main():
    svc = LLMService(
        provider="minimax",
        model="MiniMax-M2.7-highspeed",
        api_key=os.environ["MINIMAX_API_KEY"],
        context_window=1_000_000,
    )

    agent = Agent(
        svc,
        agent_name="lingtai-telegram-test",
        working_dir=str(WORKING_DIR),
        capabilities={
            "file": {},        # read, write, edit, glob, grep
            "bash": {},        # shell commands (default deny-list policy)
            "vision": {        # image understanding via Gemini
                "provider": "gemini",
                "api_key": os.environ["GEMINI_API_KEY"],
            },
        },
        addons={
            "telegram": {
                "bot_token": BOT_TOKEN,
                "allowed_users": [CHAT_ID],
            },
        },
    )

    # Graceful shutdown on Ctrl+C
    def on_sigint(sig, frame):
        print("\nShutting down...")
        agent.stop()
        sys.exit(0)

    signal.signal(signal.SIGINT, on_sigint)

    print(f"Starting agent (working_dir: {WORKING_DIR})")
    print(f"Telegram bot: @lingtai_bot")
    print(f"Send messages in Telegram — the agent will respond.")
    print(f"Press Ctrl+C to stop.\n")

    agent.start()

    # Keep the main thread alive
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nShutting down...")
        agent.stop()


if __name__ == "__main__":
    main()
