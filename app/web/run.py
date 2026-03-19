"""Launch web dashboard with an example configuration.

Usage:
    python -m app.web <example>
    python -m app.web orchestrator

Available examples are in app/web/examples/.
"""
from __future__ import annotations

import importlib
import json
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

# Maps addon name → LLMService provider_config key
_ADDON_TO_PROVIDER_KEY = {
    "web_search": "web_search_provider",
    "vision": "vision_provider",
    "draw": "image_provider",
    "compose": "music_provider",
    "talk": "tts_provider",
}


def _list_examples() -> list[str]:
    """List available example names."""
    examples_dir = Path(__file__).parent / "examples"
    return sorted(
        p.stem for p in examples_dir.glob("*.py")
        if p.stem != "__init__"
    )


def _build_key_resolver(cfg: dict) -> callable:
    """Build a key resolver from providers, addons, and main config."""
    provider_env: dict[str, str] = {}

    # From providers section
    for name, pcfg in cfg.get("providers", {}).items():
        if isinstance(pcfg, dict) and pcfg.get("api_key_env"):
            provider_env[name.lower()] = pcfg["api_key_env"]

    # From addons (may override)
    for addon_cfg in cfg.get("addons", {}).values():
        if isinstance(addon_cfg, dict) and addon_cfg.get("provider"):
            env_var = addon_cfg.get("api_key_env")
            if env_var:
                provider_env[addon_cfg["provider"].lower()] = env_var

    # Main provider
    main_env = cfg.get("api_key_env")
    if main_env and cfg.get("provider"):
        provider_env[cfg["provider"].lower()] = main_env

    def resolver(provider: str) -> str | None:
        p = provider.lower()
        env_var = provider_env.get(p, f"{p.upper()}_API_KEY")
        return os.environ.get(env_var)

    return resolver


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

    # Load config
    config_path = Path(__file__).parent / "config.json"
    if config_path.exists():
        cfg = json.loads(config_path.read_text())
    else:
        cfg = {}

    provider = cfg.get("provider", "minimax")
    model = cfg.get("model", "MiniMax-M2.7-highspeed")
    base_url = cfg.get("base_url")
    max_rpm = cfg.get("max_rpm", 0)
    dashboard_port = cfg.get("dashboard_port", 8080)

    # Build key resolver from config
    key_resolver = _build_key_resolver(cfg)

    # Get main API key
    api_key = key_resolver(provider)
    if not api_key:
        env_var = cfg.get("api_key_env", f"{provider.upper()}_API_KEY")
        print(f"Error: {env_var} not set.")
        sys.exit(1)

    # Build provider_defaults from providers section + main config
    provider_defaults: dict = {}
    for pname, pcfg in cfg.get("providers", {}).items():
        if isinstance(pcfg, dict):
            provider_defaults[pname] = {k: v for k, v in pcfg.items()
                                        if k != "api_key_env"}
    # Main provider overrides
    provider_defaults.setdefault(provider, {})["model"] = model
    if max_rpm > 0:
        provider_defaults[provider]["max_rpm"] = max_rpm
    if base_url:
        provider_defaults[provider]["base_url"] = base_url

    # Build provider_config from addons, merge addon base_urls into provider_defaults
    provider_config: dict = {}
    addons = cfg.get("addons", {})
    for addon_name, addon_cfg in addons.items():
        if not isinstance(addon_cfg, dict):
            continue
        addon_provider = addon_cfg.get("provider")
        if not addon_provider:
            continue  # local-only (e.g. listen)
        config_key = _ADDON_TO_PROVIDER_KEY.get(addon_name)
        if config_key:
            provider_config[config_key] = addon_provider
        # Merge addon base_url into provider_defaults
        addon_url = addon_cfg.get("base_url")
        if addon_url:
            provider_defaults.setdefault(addon_provider, {})["base_url"] = addon_url

    # Pass all addon API keys to MiniMax MCP subprocess
    if provider == "minimax":
        mcp_env: dict[str, str] = {}
        for addon_cfg in addons.values():
            if not isinstance(addon_cfg, dict):
                continue
            key_env = addon_cfg.get("api_key_env")
            if key_env:
                key_val = os.environ.get(key_env)
                if key_val:
                    mcp_env[key_env] = key_val
        if mcp_env:
            from stoai.llm.minimax.mcp_client import set_extra_env
            set_extra_env(mcp_env)

    llm = LLMService(
        provider=provider,
        model=model,
        api_key=api_key,
        base_url=base_url,
        provider_defaults=provider_defaults,
        key_resolver=key_resolver,
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
    print(f"Dashboard:     http://localhost:{dashboard_port}")
    print("Press Ctrl+C to shut down.")

    try:
        uvicorn.run(app, host="0.0.0.0", port=dashboard_port)
    except KeyboardInterrupt:
        print("\nShutting down...")
    finally:
        state.stop_all()
        print("Done.")


if __name__ == "__main__":
    main()
