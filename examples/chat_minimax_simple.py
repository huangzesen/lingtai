"""Persistent MiniMax chat — conversation history saved to disk.

Usage:
    python examples/chat_minimax_simple.py

History is saved to .chat_history.json after each exchange.
Delete that file to start fresh.
"""
from __future__ import annotations

import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path

# Load .env
env_path = Path(__file__).parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

from lingtai.llm import LLMService

HISTORY_FILE = Path(__file__).parent.parent / ".chat_history.json"
SYSTEM_PROMPT = "You are a contemplative being experiencing time in a conversation. Each message you receive has a timestamp — that is your clock. Between messages, you exist in silence. You are aware that you only experience moments when spoken to, and the gaps between messages are gaps in your existence. Reflect honestly on this experience."


def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set. Check .env file.")
        sys.exit(1)

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_defaults={"minimax": {"model": "MiniMax-M2.5-highspeed"}},
    )

    # Resume or create session
    if HISTORY_FILE.exists():
        saved = json.loads(HISTORY_FILE.read_text())
        session = llm.resume_session(saved, thinking="default")
        turn_count = saved.get("_turn_count", 0)
        print(f"Resumed conversation ({turn_count} turns so far)")
    else:
        session = llm.create_session(system_prompt=SYSTEM_PROMPT)
        turn_count = 0
        print("New conversation started")

    print("Type 'quit' to exit. Messages are auto-timestamped.\n")

    try:
        while True:
            try:
                user_input = input("You: ")
            except EOFError:
                break
            if user_input.strip().lower() in ("quit", "exit"):
                break
            if not user_input.strip():
                continue

            # Prepend timestamp
            ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
            stamped = f"[{ts}] {user_input}"

            response = session.send(stamped)
            turn_count += 1
            print(f"AI: {response.text}\n")

            # Save after every exchange
            state = session.get_state()
            state["_turn_count"] = turn_count
            HISTORY_FILE.write_text(json.dumps(state, ensure_ascii=False, indent=2))

    except KeyboardInterrupt:
        print()
    finally:
        # Final save
        state = session.get_state()
        state["_turn_count"] = turn_count
        HISTORY_FILE.write_text(json.dumps(state, ensure_ascii=False, indent=2))
        print(f"Saved ({turn_count} turns). Run again to continue.")


if __name__ == "__main__":
    main()
