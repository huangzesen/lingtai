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


def load_env_file(path: str | Path) -> None:
    """Load a .env file into os.environ. Existing vars are not overwritten."""
    env_path = Path(path).expanduser()
    if not env_path.is_file():
        return
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, _, val = line.partition("=")
        if not _:
            continue
        key = key.strip()
        val = val.strip().strip("'\"")
        if key not in os.environ:
            os.environ[key] = val


def resolve_env(value: str | None, env_name: str | None) -> str | None:
    """Resolve a value from env var name, falling back to raw value.

    If env_name is provided and the env var is set, use it.
    Otherwise return the raw value as-is.
    """
    if env_name:
        env_val = os.environ.get(env_name)
        if env_val:
            return env_val
    return value


def build_agent(data: dict, working_dir: Path) -> Agent:
    """Construct LLMService, MailService, and Agent from validated init data."""
    # Load env file if specified
    env_file = data.get("env_file")
    if env_file:
        load_env_file(env_file)

    m = data["manifest"]
    llm = m["llm"]

    api_key = resolve_env(llm["api_key"], llm.get("api_key_env"))

    service = LLMService(
        provider=llm["provider"],
        model=llm["model"],
        api_key=api_key,
        base_url=llm["base_url"],
    )

    mail_service = FilesystemMailService(working_dir=working_dir)

    soul = m["soul"]

    config = AgentConfig(
        stamina=m["stamina"],
        soul_delay=soul["delay"],
        max_turns=m["max_turns"],
        language=m["language"],
        context_limit=m["context_limit"],
        molt_pressure=m["molt_pressure"],
        molt_prompt=m["molt_prompt"],
    )

    # Build addons dict — resolve *_env fields
    addons = _resolve_addons(data.get("addons"))

    # Resolve *_env fields in capability kwargs
    capabilities = _resolve_capabilities(m["capabilities"])

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
        capabilities=capabilities,
        addons=addons,
    )

    # Inject principle (raw text before all sections)
    principle = data.get("principle", "")
    if principle:
        agent._prompt_manager.write_section("principle", principle, protected=True)

    # Restore molt count from previous run (if resuming)
    prev_manifest = working_dir / ".agent.json"
    if prev_manifest.is_file():
        try:
            prev = json.loads(prev_manifest.read_text())
            agent._molt_count = prev.get("molt_count", 0)
        except (json.JSONDecodeError, OSError):
            pass

    return agent


def _resolve_env_fields(d: dict) -> dict:
    """Resolve ``*_env`` keys in a dict using ``resolve_env``.

    For each key ending with ``_env``, resolve the env var and store the value
    under the base key (without the ``_env`` suffix).  The ``_env`` key is
    removed from the result.  Non-env keys are kept as-is.

    Example::

        {"api_key": null, "api_key_env": "MY_KEY"}
        → {"api_key": "<value of $MY_KEY>"}
    """
    result = dict(d)
    env_keys = [k for k in result if k.endswith("_env")]
    for env_key in env_keys:
        base_key = env_key[: -len("_env")]
        result[base_key] = resolve_env(result.get(base_key), result.pop(env_key))
    return result


def _resolve_capabilities(capabilities: dict) -> dict:
    """Resolve ``*_env`` fields in each capability's kwargs."""
    resolved = {}
    for name, kwargs in capabilities.items():
        if isinstance(kwargs, dict) and kwargs:
            resolved[name] = _resolve_env_fields(kwargs)
        else:
            resolved[name] = kwargs
    return resolved


def _resolve_addons(addons: dict | None) -> dict | None:
    """Resolve *_env fields in addon configs to actual values."""
    if not addons:
        return addons

    resolved = {}
    for name, cfg in addons.items():
        if isinstance(cfg, dict):
            resolved[name] = _resolve_env_fields(cfg)

    return resolved or None


def run(working_dir: Path) -> None:
    """Full boot sequence: load, build, start, block, stop."""
    data = load_init(working_dir)
    agent = build_agent(data, working_dir)

    # Signal handlers: SIGTERM/SIGINT → touch .suspend and unblock main thread
    suspend_file = working_dir / ".suspend"

    def _signal_handler(signum, frame):
        suspend_file.touch()
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

        # Block until the agent shuts down (SUSPENDED via .suspend or external stop)
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
