"""lingtai run <working_dir> — boot agent into ASLEEP, wake on external messages."""
from __future__ import annotations

import argparse
import json
import signal
import sys
from pathlib import Path

from lingtai.config_resolve import (
    resolve_env,
    load_env_file,
)
from lingtai.init_schema import validate_init
from lingtai.llm.service import LLMService
from lingtai.agent import Agent
from lingtai_kernel.services.mail import FilesystemMailService


def load_init(working_dir: Path) -> dict:
    """Read and validate init.json from working_dir. Exits on error."""
    init_path = working_dir / "init.json"
    if not init_path.is_file():
        print(f"error: {init_path} not found", file=sys.stderr)
        sys.exit(1)

    try:
        from lingtai.config_resolve import load_jsonc
        data = load_jsonc(init_path)
    except (json.JSONDecodeError, OSError, ValueError) as e:
        print(f"error: failed to read {init_path}: {e}", file=sys.stderr)
        sys.exit(1)

    try:
        validate_init(data)
    except ValueError as e:
        print(f"error: invalid init.json: {e}", file=sys.stderr)
        sys.exit(1)

    return data


def build_agent(data: dict, working_dir: Path) -> Agent:
    """Construct Agent from validated init data.

    Creates a minimal Agent (LLMService + working_dir + mail_service),
    then delegates all setup to _perform_refresh() which reads init.json.
    This ensures boot and live refresh share one code path.
    """
    # Load env file if specified (needed for LLM API key resolution)
    env_file = data.get("env_file")
    if env_file:
        load_env_file(env_file)

    m = data["manifest"]
    llm = m["llm"]

    api_key = resolve_env(llm.get("api_key"), llm.get("api_key_env"))

    service = LLMService(
        provider=llm["provider"],
        model=llm["model"],
        api_key=api_key,
        base_url=llm.get("base_url"),
    )

    mail_service = FilesystemMailService(working_dir=working_dir)

    # Minimal construction — _perform_refresh reads init.json for everything else
    agent = Agent(
        service,
        agent_name=m.get("agent_name"),
        working_dir=working_dir,
        mail_service=mail_service,
        streaming=m.get("streaming", True),
    )

    # Full setup from init.json (capabilities, addons, config, covenant, etc.)
    agent._perform_refresh()

    # Restore molt count from previous run (if resuming)
    prev_manifest = working_dir / ".agent.json"
    if prev_manifest.is_file():
        try:
            prev = json.loads(prev_manifest.read_text())
            agent._molt_count = prev.get("molt_count", 0)
        except (json.JSONDecodeError, OSError):
            pass

    return agent


def _clean_signal_files(working_dir: Path) -> None:
    """Remove stale .suspend / .sleep files left over from a previous run."""
    for name in (".suspend", ".sleep"):
        f = working_dir / name
        if f.is_file():
            try:
                f.unlink()
            except OSError:
                pass


def _install_signal_handlers(working_dir: Path, agent: Agent) -> None:
    """SIGTERM/SIGINT → touch .suspend and unblock main thread."""
    suspend_file = working_dir / ".suspend"

    def _handler(signum, frame):
        suspend_file.touch()
        agent._shutdown.set()

    signal.signal(signal.SIGTERM, _handler)
    signal.signal(signal.SIGINT, _handler)


def run(working_dir: Path) -> None:
    """Boot agent into ASLEEP — wakes on external messages (mail/imap/telegram)."""
    _clean_signal_files(working_dir)
    data = load_init(working_dir)
    agent = build_agent(data, working_dir)
    _install_signal_handlers(working_dir, agent)

    from lingtai_kernel.state import AgentState
    agent._asleep.set()
    agent._state = AgentState.ASLEEP

    try:
        agent.start()
        agent._shutdown.wait()
    finally:
        try:
            agent.stop(timeout=10.0)
        except Exception:
            pass


def main() -> None:
    parser = argparse.ArgumentParser(
        prog="lingtai",
        description="lingtai agent runtime",
    )
    sub = parser.add_subparsers(dest="command")

    run_parser = sub.add_parser("run", help="Boot agent into sleep — wakes on external messages")
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
