"""Launch web dashboard with an example configuration.

Usage:
    python -m app.web <example>
    python -m app.web orchestrator

Available examples are in app/web/examples/.
"""
from __future__ import annotations

import importlib
import os
import sys
from pathlib import Path

# Load .env from project root
env_path = Path(__file__).parent.parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

import uvicorn

from stoai.llm import LLMService

from .server.main import create_app


def _list_examples() -> list[str]:
    """List available example names."""
    examples_dir = Path(__file__).parent / "examples"
    return sorted(
        p.stem for p in examples_dir.glob("*.py")
        if p.stem != "__init__"
    )


def main(example_name: str | None = None):
    if example_name is None:
        if len(sys.argv) > 1:
            example_name = sys.argv[1]

    if not example_name or example_name in ("-h", "--help"):
        examples = _list_examples()
        print("Usage: python -m app.web <example>\n")
        print("Available examples:")
        for name in examples:
            mod = importlib.import_module(f".examples.{name}", package="app.web")
            doc = (mod.__doc__ or "").strip().split("\n")[0]
            print(f"  {name:20s} {doc}")
        sys.exit(0)

    # Import the example module
    try:
        mod = importlib.import_module(f".examples.{example_name}", package="app.web")
    except ModuleNotFoundError:
        print(f"Unknown example: {example_name}")
        print(f"Available: {', '.join(_list_examples())}")
        sys.exit(1)

    # Get API key
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set.")
        sys.exit(1)

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_config={"web_search_provider": "minimax"},
        provider_defaults={"minimax": {"model": "MiniMax-M2.5-highspeed"}},
    )

    base_dir = Path.home() / ".stoai" / "web" / example_name
    base_dir.mkdir(parents=True, exist_ok=True)

    # Each example exports a setup(llm, base_dir) -> AppState
    state = mod.setup(llm, base_dir)

    app = create_app(state)
    state.start_all()

    print(f"Example:       {example_name}")
    print(f"User mailbox:  127.0.0.1:{state.user_port}")
    for entry in state.agents.values():
        admin_tag = " [admin]" if entry.agent._admin else ""
        print(f"Agent {entry.name:8s}  {entry.address}{admin_tag}")
    print("Dashboard:     http://localhost:8080")
    print("Press Ctrl+C to shut down.")

    try:
        uvicorn.run(app, host="0.0.0.0", port=8080)
    except KeyboardInterrupt:
        print("\nShutting down...")
    finally:
        state.stop_all()
        print("Done.")


if __name__ == "__main__":
    main()
