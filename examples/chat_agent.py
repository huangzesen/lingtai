"""Launch an Agent on a TCP port and chat with it.

Usage:
    python examples/chat_agent.py

The agent listens on port 8301. Type messages to chat.
Press Ctrl+C to quit.
"""
from __future__ import annotations

import os
import sys
import time

# Load .env
from pathlib import Path
env_path = Path(__file__).parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

from stoai import Agent, AgentConfig, TCPMailService
from stoai.llm import LLMService

PORT = 8301

def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set. Check .env file.")
        sys.exit(1)

    print(f"Starting agent with MiniMax on port {PORT}...")

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_defaults={
            "minimax": {"model": "MiniMax-M2.5-highspeed"},
        },
    )

    mail_svc = TCPMailService(listen_port=PORT)

    agent = Agent(
        agent_name="assistant",
        service=llm,
        mail_service=mail_svc,
        config=AgentConfig(max_turns=20),
        base_dir=".",
        streaming=True,
        capabilities={
            "file": {},
            "bash": {"yolo": True},
            "email": {},
            "vision": {},
            "web_search": {},
            "psyche": {},
            "avatar": {},
        },
    )
    agent.start()

    print(f"Agent listening on 127.0.0.1:{PORT}")
    print("Type messages below. Press Ctrl+C to quit.\n")

    sender = TCPMailService()

    try:
        while True:
            try:
                user_input = input("You: ")
            except EOFError:
                break
            if not user_input.strip():
                continue

            # Send message to agent
            payload = {
                "from": "user",
                "to": f"127.0.0.1:{PORT}",
                "message": user_input,
            }
            err = sender.send(f"127.0.0.1:{PORT}", payload)
            if err is not None:
                print(f"  [Failed to send: {err}]")
                continue

            # Wait for agent to process (poll inbox for reply)
            # The agent processes asynchronously — we need to wait for it to finish
            print("Agent: ", end="", flush=True)

            # Wait for agent to go active then back to sleeping
            time.sleep(0.2)  # give it time to pick up the message
            timeout = 120.0
            start = time.monotonic()
            while not agent.is_idle and time.monotonic() - start < timeout:
                time.sleep(0.1)

            # Get the response from the chat session
            if agent._chat is not None:
                last = agent._chat.interface.last_assistant_entry()
                if last:
                    from stoai.llm.interface import TextBlock
                    text_parts = [b.text for b in last.content if isinstance(b, TextBlock)]
                    print("".join(text_parts))
                else:
                    print("[No response]")
            else:
                print("[No chat session]")

    except KeyboardInterrupt:
        print("\n\nShutting down...")
    finally:
        agent.stop(timeout=5.0)
        print("Done.")


if __name__ == "__main__":
    main()
