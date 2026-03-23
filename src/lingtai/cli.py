"""lingtai run <working_dir> — boot an agent from init.json."""
from __future__ import annotations

import argparse
import json
import os
import signal
import sys
from pathlib import Path

from lingtai.init_schema import validate_init
from lingtai.llm.service import LLMService
from lingtai.agent import Agent
from lingtai_kernel.services.mail import FilesystemMailService
from lingtai_kernel.config import AgentConfig


def load_init(working_dir: Path) -> dict:
    """Read and validate init.json from working_dir. Exits on error."""
    init_path = working_dir / "init.json"
    if not init_path.is_file():
        print(f"error: {init_path} not found", file=sys.stderr)
        sys.exit(1)

    try:
        data = json.loads(init_path.read_text())
    except (json.JSONDecodeError, OSError) as e:
        print(f"error: failed to read {init_path}: {e}", file=sys.stderr)
        sys.exit(1)

    try:
        validate_init(data)
    except ValueError as e:
        print(f"error: invalid init.json: {e}", file=sys.stderr)
        sys.exit(1)

    return data


def build_agent(data: dict, working_dir: Path) -> Agent:
    """Construct LLMService, MailService, and Agent from validated init data."""
    m = data["manifest"]
    llm = m["llm"]

    service = LLMService(
        provider=llm["provider"],
        model=llm["model"],
        api_key=llm["api_key"],
        base_url=llm["base_url"],
    )

    mail_service = FilesystemMailService(working_dir=working_dir)

    soul = m["soul"]

    config = AgentConfig(
        vigil=m["vigil"],
        soul_delay=soul["delay"],
        max_turns=m["max_turns"],
        language=m["language"],
    )

    agent = Agent(
        service,
        agent_name=m["agent_name"],
        working_dir=working_dir,
        mail_service=mail_service,
        config=config,
        admin=m["admin"],
        streaming=m["streaming"],
        covenant=data["covenant"],
        memory=data["memory"],
        capabilities=m["capabilities"],
    )

    # Restore molt count from previous run (if resuming)
    prev_manifest = working_dir / ".agent.json"
    if prev_manifest.is_file():
        try:
            prev = json.loads(prev_manifest.read_text())
            agent._molt_count = prev.get("molt_count", 0)
        except (json.JSONDecodeError, OSError):
            pass

    return agent


def write_pid(working_dir: Path) -> None:
    (working_dir / ".agent.pid").write_text(str(os.getpid()))


def remove_pid(working_dir: Path) -> None:
    pid_file = working_dir / ".agent.pid"
    if pid_file.is_file():
        pid_file.unlink()


def run(working_dir: Path) -> None:
    """Full boot sequence: load, build, start, block, stop."""
    data = load_init(working_dir)
    agent = build_agent(data, working_dir)

    write_pid(working_dir)

    # Signal handlers: SIGTERM/SIGINT → touch .quell and unblock main thread
    quell_file = working_dir / ".quell"

    def _signal_handler(signum, frame):
        quell_file.touch()
        agent._shutdown.set()

    signal.signal(signal.SIGTERM, _signal_handler)
    signal.signal(signal.SIGINT, _signal_handler)

    try:
        agent.start()

        # Inject starting prompt if provided
        prompt = data.get("prompt", "")
        if prompt:
            from lingtai_kernel.message import _make_message, MSG_REQUEST
            agent.inbox.put(_make_message(MSG_REQUEST, "system", prompt))

        # Block until the agent shuts down (vigil, .quell, or external stop)
        agent._shutdown.wait()
    finally:
        try:
            agent.stop(timeout=10.0)
        except Exception:
            pass
        remove_pid(working_dir)


def main() -> None:
    parser = argparse.ArgumentParser(
        prog="lingtai",
        description="lingtai agent runtime",
    )
    sub = parser.add_subparsers(dest="command")

    run_parser = sub.add_parser("run", help="Boot an agent from init.json in working_dir")
    run_parser.add_argument("working_dir", type=Path, help="Agent working directory containing init.json")

    args = parser.parse_args()

    if args.command == "run":
        working_dir = args.working_dir.resolve()
        if not working_dir.is_dir():
            print(f"error: {working_dir} is not a directory", file=sys.stderr)
            sys.exit(1)
        run(working_dir)
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
