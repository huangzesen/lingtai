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
    from lingtai.config_resolve import resolve_paths

    init_path = working_dir / "init.json"
    if not init_path.is_file():
        print(f"error: {init_path} not found", file=sys.stderr)
        sys.exit(1)

    try:
        data = json.loads(init_path.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError, ValueError) as e:
        print(f"error: failed to read {init_path}: {e}", file=sys.stderr)
        sys.exit(1)

    try:
        warnings = validate_init(data)
    except ValueError as e:
        print(f"error: invalid init.json: {e}", file=sys.stderr)
        sys.exit(1)
    for w in warnings:
        print(f"warning: init.json: {w}", file=sys.stderr)

    resolve_paths(data, working_dir)
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
        context_window=m.get("context_limit", 200_000),
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
    agent._setup_from_init()

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

    # Resolve venv and store on agent for CPR/avatar to use
    from lingtai.venv_resolve import resolve_venv
    venv_dir = resolve_venv(data)
    # Write back to init.json if not already set (self-sufficient)
    if not data.get("venv_path") or data["venv_path"] != str(venv_dir):
        data["venv_path"] = str(venv_dir)
        init_path = working_dir / "init.json"
        init_path.write_text(json.dumps(data, indent=2, ensure_ascii=False))

    agent = build_agent(data, working_dir)
    agent._venv_path = str(venv_dir)
    _install_signal_handlers(working_dir, agent)

    from lingtai_kernel.state import AgentState
    agent._asleep.set()
    agent._state = AgentState.ASLEEP

    # Detect refresh boot — old process wrote .refresh before spawning us
    refresh_file = working_dir / ".refresh"
    is_refresh = refresh_file.is_file()
    if is_refresh:
        refresh_file.unlink()

    try:
        agent.start()

        # Kick-start after refresh — wake agent with a system message
        if is_refresh:
            from lingtai_kernel.i18n import t
            lang = agent._config.language
            agent.send(t(lang, "system.refresh_successful"), sender="system")

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

    sub.add_parser("check-caps", help="Output capability provider metadata as JSON")

    args = parser.parse_args()

    if args.command == "run":
        working_dir = args.working_dir.resolve()
        if not working_dir.is_dir():
            print(f"error: {working_dir} is not a directory", file=sys.stderr)
            sys.exit(1)
        run(working_dir)
    elif args.command == "check-caps":
        from lingtai.capabilities import get_all_providers
        print(json.dumps(get_all_providers()))
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
