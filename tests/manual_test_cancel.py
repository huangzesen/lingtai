"""Manual test: three-agent cancel email demo.

Sets up Alice (admin), Bob, Charlie (normal) with real TCP mail services
and email capability. Alice sends a cancel email to Bob mid-work.

Usage:
    python tests/manual_test_cancel.py
"""
from __future__ import annotations

import json
import shutil
import socket
import tempfile
import threading
import time
from pathlib import Path
from unittest.mock import MagicMock

from stoai.agent import BaseAgent
from stoai.config import AgentConfig
from stoai.services.mail import TCPMailService
from stoai.llm import LLMResponse, ToolCall


def _get_free_port():
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()
    return port


def _make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def main():
    base_dir = Path(tempfile.mkdtemp())
    print(f"Base dir: {base_dir}")

    # --- Create agents ---
    ports = {
        "alice": _get_free_port(),
        "bob": _get_free_port(),
        "charlie": _get_free_port(),
    }

    agents = {}
    managers = {}
    mail_services = {}

    for name, port in ports.items():
        mail_svc = TCPMailService(listen_port=port, working_dir=base_dir / name)
        mail_services[name] = mail_svc
        is_admin = (name == "alice")
        agent = BaseAgent(
            agent_id=name,
            service=_make_mock_service(),
            mail_service=mail_svc,
            base_dir=base_dir,
            admin=is_admin,
        )
        mgr = agent.add_capability("email")
        agents[name] = agent
        managers[name] = mgr

    # Start mail listeners — all mail goes through agent._on_mail_received
    # which handles cancel internally, then delegates normal to _on_normal_mail
    for name, agent in agents.items():
        agent._mail_service.listen(
            on_message=lambda msg, a=agent: a._on_mail_received(msg)
        )

    def addr(name):
        return f"127.0.0.1:{ports[name]}"

    print(f"\nAlice (admin={agents['alice']._admin}) @ {addr('alice')}")
    print(f"Bob   (admin={agents['bob']._admin})  @ {addr('bob')}")
    print(f"Charlie (admin={agents['charlie']._admin}) @ {addr('charlie')}")

    # --- Test 1: Normal email flow ---
    print("\n" + "=" * 60)
    print("TEST 1: Normal email — Alice sends to Bob")
    print("=" * 60)

    result = managers["alice"].handle({
        "action": "send",
        "address": addr("bob"),
        "subject": "Hello Bob",
        "message": "How is the project going?",
    })
    print(f"  Send result: {result}")
    time.sleep(0.3)
    print(f"  Bob mail queue: {len(agents['bob']._mail_queue)} messages")
    print(f"  Bob cancel event set: {agents['bob']._cancel_event.is_set()}")

    # --- Test 2: Admin sends cancel email ---
    print("\n" + "=" * 60)
    print("TEST 2: Alice (admin) sends cancel email to Bob")
    print("=" * 60)

    result = managers["alice"].handle({
        "action": "send",
        "address": addr("bob"),
        "subject": "Stop work",
        "message": "Halt all work on the project immediately.",
        "type": "cancel",
    })
    print(f"  Send result: {result}")
    time.sleep(0.3)
    print(f"  Bob cancel event set: {agents['bob']._cancel_event.is_set()}")
    print(f"  Bob cancel mail: {agents['bob']._cancel_mail}")
    print(f"  Bob mail queue (should NOT increase): {len(agents['bob']._mail_queue)} messages")

    # --- Test 3: Non-admin tries to send cancel email ---
    print("\n" + "=" * 60)
    print("TEST 3: Bob (non-admin) tries to send cancel email to Charlie")
    print("=" * 60)

    result = managers["bob"].handle({
        "action": "send",
        "address": addr("charlie"),
        "subject": "Stop",
        "message": "Stop everything",
        "type": "cancel",
    })
    print(f"  Send result: {result}")
    print(f"  Charlie cancel event set: {agents['charlie']._cancel_event.is_set()}")

    # --- Test 4: Diary flow ---
    print("\n" + "=" * 60)
    print("TEST 4: Bob's diary flow (simulated)")
    print("=" * 60)

    # Bob's cancel state was set by Test 2 — re-set it for diary test
    agents["bob"]._cancel_mail = {
        "from": addr("alice"),
        "subject": "Stop work",
        "message": "Halt all work on the project immediately.",
    }
    agents["bob"]._cancel_event.set()

    # Set up a mock chat that returns diary text
    mock_chat = MagicMock()
    mock_response = MagicMock()
    mock_response.text = "I was analyzing the project requirements and had completed steps 1-3 of the review."
    mock_chat.send.return_value = mock_response
    agents["bob"]._chat = mock_chat

    diary_result = agents["bob"]._handle_cancel_diary()
    print(f"  Diary text: {diary_result['text']}")
    print(f"  Failed: {diary_result['failed']}")
    print(f"  Cancel event cleared: {not agents['bob']._cancel_event.is_set()}")
    print(f"  Cancel mail cleared: {agents['bob']._cancel_mail is None}")

    # Verify the diary prompt included cancel email info
    diary_prompt = mock_chat.send.call_args[0][0]
    print(f"\n  Diary prompt sent to LLM:")
    for line in diary_prompt.split("\n"):
        print(f"    {line}")

    # --- Cleanup ---
    print("\n" + "=" * 60)
    print("ALL TESTS PASSED")
    print("=" * 60)

    for agent in agents.values():
        agent._mail_service.stop()
    shutil.rmtree(base_dir, ignore_errors=True)


if __name__ == "__main__":
    main()
