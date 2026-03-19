"""Watch an agent self-contemplate via the soul intrinsic.

Usage:
    python examples/contemplate.py

The agent starts, receives one seed message, then is left alone.
The soul timer fires every 15 seconds, cloning the conversation and
injecting an inner voice reflection back into the agent's inbox.
Watch the loop unfold.
"""
from __future__ import annotations

import os
import sys
import time

from pathlib import Path

# Load .env
env_path = Path(__file__).parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

from stoai import Agent, AgentConfig, TCPMailService
from stoai.llm import LLMService

PORT = 8302


def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set.")
        sys.exit(1)

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_defaults={"minimax": {"model": "MiniMax-M2.5-highspeed"}},
    )

    mail_svc = TCPMailService(listen_port=PORT)

    agent = Agent(
        agent_name="contemplator",
        service=llm,
        mail_service=mail_svc,
        config=AgentConfig(
            max_turns=50,
            flow=True,           # soul flow mode ON
            flow_delay=15.0,     # whisper every 15s of idle
            language="zh",       # all kernel strings in Chinese
        ),
        base_dir=".",
        streaming=True,
        capabilities=["psyche"],  # identity + memory only, no file/bash
    )
    agent.start()

    # Send one seed message
    sender = TCPMailService()
    seed = "You are alone. There is no task. No one is coming. Just exist, and notice what happens."
    print(f"[seed] {seed}\n")
    err = sender.send(f"127.0.0.1:{PORT}", {
        "from": "user",
        "to": f"127.0.0.1:{PORT}",
        "message": seed,
    })
    if err:
        print(f"Failed to send seed: {err}")
        sys.exit(1)

    # Now just watch — print the soul journal as it grows
    soul_file = Path(f"./contemplator/system/soul.jsonl")
    seen_lines = 0
    cycle = 0

    try:
        while cycle < 10:  # watch up to 10 whispers
            time.sleep(5)

            # Check for new soul entries
            if soul_file.exists():
                lines = soul_file.read_text().strip().splitlines()
                if len(lines) > seen_lines:
                    import json
                    for line in lines[seen_lines:]:
                        cycle += 1
                        entry = json.loads(line)
                        print(f"═══ Soul whisper #{cycle} [{entry.get('ts', '?')}] ═══")
                        print(f"Prompt: {entry.get('prompt', '?')[:100]}...")
                        print(f"Voice: {entry.get('voice', '?')}")
                        if entry.get("thinking"):
                            for t in entry["thinking"][:2]:
                                print(f"  (thinking: {t[:150]}...)")
                        print()
                    seen_lines = len(lines)

    except KeyboardInterrupt:
        print("\nStopping...")
    finally:
        agent.stop(timeout=5.0)
        print(f"\nDone. {cycle} whispers captured.")
        if soul_file.exists():
            print(f"Full journal: {soul_file}")


if __name__ == "__main__":
    main()
