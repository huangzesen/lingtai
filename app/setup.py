"""Interactive setup wizard for lingtai — writes config.json and model.json."""
from __future__ import annotations

import imaplib
import json
import os
import smtplib
import sys
from pathlib import Path

# ---------------------------------------------------------------------------
# Colors (ANSI)
# ---------------------------------------------------------------------------

_RESET = "\033[0m"
_BOLD = "\033[1m"
_DIM = "\033[2m"
_CYAN = "\033[36m"
_GREEN = "\033[32m"
_YELLOW = "\033[33m"
_RED = "\033[31m"
_MAGENTA = "\033[35m"
_BLUE = "\033[34m"
_WHITE = "\033[97m"


def _header(text: str) -> None:
    print(f"\n{_BOLD}{_CYAN}{'═' * 50}{_RESET}")
    print(f"{_BOLD}{_CYAN}  {text}{_RESET}")
    print(f"{_BOLD}{_CYAN}{'═' * 50}{_RESET}\n")


def _section(text: str) -> None:
    print(f"\n{_BOLD}{_MAGENTA}── {text} ──{_RESET}\n")


def _info(text: str) -> None:
    print(f"  {_DIM}{text}{_RESET}")


def _success(text: str) -> None:
    print(f"  {_GREEN}✓ {text}{_RESET}")


def _error(text: str) -> None:
    print(f"  {_RED}✗ {text}{_RESET}")


def _warn(text: str) -> None:
    print(f"  {_YELLOW}! {text}{_RESET}")


def _prompt(label: str, default: str = "") -> str:
    """Prompt user for input with optional default."""
    if default:
        display = f"  {_WHITE}{label}{_RESET} {_DIM}[{default}]{_RESET}: "
    else:
        display = f"  {_WHITE}{label}{_RESET}: "
    try:
        val = input(display).strip()
    except (EOFError, KeyboardInterrupt):
        print()
        raise SystemExit(0)
    return val if val else default


def _prompt_yn(label: str, default: bool = True) -> bool:
    """Yes/no prompt."""
    hint = "Y/n" if default else "y/N"
    display = f"  {_WHITE}{label}{_RESET} {_DIM}[{hint}]{_RESET}: "
    try:
        val = input(display).strip().lower()
    except (EOFError, KeyboardInterrupt):
        print()
        raise SystemExit(0)
    if not val:
        return default
    return val in ("y", "yes")


def _prompt_secret(label: str) -> str:
    """Prompt for a secret value (still visible — no getpass for simplicity)."""
    display = f"  {_WHITE}{label}{_RESET}: "
    try:
        val = input(display).strip()
    except (EOFError, KeyboardInterrupt):
        print()
        raise SystemExit(0)
    return val


# ---------------------------------------------------------------------------
# Test helpers
# ---------------------------------------------------------------------------

def _test_imap(host: str, port: int, address: str, password: str) -> bool:
    """Test IMAP connection."""
    print(f"\n  {_BLUE}Testing IMAP connection...{_RESET}", end="", flush=True)
    try:
        imap = imaplib.IMAP4_SSL(host, port)
        imap.login(address, password)
        imap.logout()
        print(f"\r  {_GREEN}✓ IMAP connection successful{_RESET}       ")
        return True
    except Exception as e:
        print(f"\r  {_RED}✗ IMAP failed: {e}{_RESET}       ")
        return False


def _test_smtp(host: str, port: int, address: str, password: str) -> bool:
    """Test SMTP connection."""
    print(f"  {_BLUE}Testing SMTP connection...{_RESET}", end="", flush=True)
    try:
        with smtplib.SMTP(host, port) as server:
            server.starttls()
            server.login(address, password)
        print(f"\r  {_GREEN}✓ SMTP connection successful{_RESET}       ")
        return True
    except Exception as e:
        print(f"\r  {_RED}✗ SMTP failed: {e}{_RESET}       ")
        return False


def _test_telegram(token: str) -> bool:
    """Test Telegram bot token via getMe."""
    print(f"  {_BLUE}Testing Telegram bot...{_RESET}", end="", flush=True)
    try:
        import urllib.request
        import json as _json
        url = f"https://api.telegram.org/bot{token}/getMe"
        req = urllib.request.Request(url, method="GET")
        with urllib.request.urlopen(req, timeout=10) as resp:
            data = _json.loads(resp.read())
        if data.get("ok"):
            bot_name = data["result"].get("username", "unknown")
            print(f"\r  {_GREEN}✓ Telegram bot: @{bot_name}{_RESET}       ")
            return True
        else:
            print(f"\r  {_RED}✗ Telegram API error: {data}{_RESET}       ")
            return False
    except Exception as e:
        print(f"\r  {_RED}✗ Telegram failed: {e}{_RESET}       ")
        return False


def _test_api_key(provider: str, env_var: str) -> bool:
    """Check if the env var is set."""
    val = os.environ.get(env_var, "")
    if val:
        _success(f"{env_var} is set ({len(val)} chars)")
        return True
    else:
        _error(f"{env_var} is not set")
        return False


# ---------------------------------------------------------------------------
# Setup sections
# ---------------------------------------------------------------------------

def _setup_model() -> dict:
    """Set up model.json configuration."""
    _section("LLM Provider")
    _info("Configure your primary language model.")
    _info("Supported: minimax, anthropic, openai, gemini, deepseek, grok, qwen")
    print()

    provider = _prompt("Provider", "minimax")
    model = _prompt("Model name", "MiniMax-M2.7-highspeed")
    api_key_env = _prompt("API key env var", f"{provider.upper()}_API_KEY")

    _test_api_key(provider, api_key_env)

    model_cfg: dict = {
        "provider": provider,
        "model": model,
        "api_key_env": api_key_env,
    }

    base_url = _prompt("Custom base URL (leave empty for default)", "")
    if base_url:
        model_cfg["base_url"] = base_url

    # Vision
    if _prompt_yn("Configure a dedicated vision provider?", default=False):
        _section("Vision Provider")
        v_provider = _prompt("Vision provider", "openai")
        v_model = _prompt("Vision model", "gpt-4o")
        v_key_env = _prompt("Vision API key env var", f"{v_provider.upper()}_API_KEY")
        _test_api_key(v_provider, v_key_env)
        model_cfg["vision"] = {
            "provider": v_provider,
            "model": v_model,
            "api_key_env": v_key_env,
        }

    # Web search
    if _prompt_yn("Configure a dedicated web search provider?", default=False):
        _section("Web Search Provider")
        ws_provider = _prompt("Web search provider", "gemini")
        ws_model = _prompt("Web search model", "gemini-2.0-flash")
        ws_key_env = _prompt("Web search API key env var", f"{ws_provider.upper()}_API_KEY")
        _test_api_key(ws_provider, ws_key_env)
        model_cfg["web_search"] = {
            "provider": ws_provider,
            "model": ws_model,
            "api_key_env": ws_key_env,
        }

    return model_cfg


def _setup_imap(env_vars: dict) -> dict | None:
    """Set up IMAP email channel. Secrets go into env_vars dict."""
    _section("IMAP Email")
    if not _prompt_yn("Enable IMAP email channel?", default=True):
        return None

    _info("Requires an email account with IMAP/SMTP access.")
    _info("For Gmail: enable 2FA, then create an App Password.")
    _info("Password will be stored in .env, not in config.json.")
    print()

    address = _prompt("Email address")
    if not address:
        _warn("Skipped — no address provided.")
        return None

    password = _prompt_secret("App password")
    imap_host = _prompt("IMAP host", "imap.gmail.com")
    imap_port = int(_prompt("IMAP port", "993"))
    smtp_host = _prompt("SMTP host", "smtp.gmail.com")
    smtp_port = int(_prompt("SMTP port", "587"))

    # Test connections
    imap_ok = _test_imap(imap_host, imap_port, address, password)
    smtp_ok = _test_smtp(smtp_host, smtp_port, address, password)

    if not imap_ok or not smtp_ok:
        if not _prompt_yn("Connection test failed. Save anyway?", default=False):
            return None

    allowed = _prompt("Allowed senders (comma-separated, empty = accept all)", "")
    allowed_list = [s.strip() for s in allowed.split(",") if s.strip()] if allowed else []

    # Store password in .env
    env_vars["IMAP_PASSWORD"] = password

    cfg: dict = {
        "email_address": address,
        "email_password_env": "IMAP_PASSWORD",
        "imap_host": imap_host,
        "imap_port": imap_port,
        "smtp_host": smtp_host,
        "smtp_port": smtp_port,
    }
    if allowed_list:
        cfg["allowed_senders"] = allowed_list

    _success("IMAP configured.")
    return cfg


def _setup_telegram() -> dict | None:
    """Set up Telegram channel."""
    _section("Telegram Bot")
    if not _prompt_yn("Enable Telegram channel?", default=False):
        return None

    _info("Requires a Telegram bot token from @BotFather.")
    print()

    token = _prompt_secret("Bot token")
    if not token:
        _warn("Skipped — no token provided.")
        return None

    _test_telegram(token)

    allowed = _prompt("Allowed user IDs (comma-separated, empty = accept all)", "")
    allowed_list = [int(s.strip()) for s in allowed.split(",") if s.strip()] if allowed else []

    cfg: dict = {"bot_token": token}
    if allowed_list:
        cfg["allowed_users"] = allowed_list

    _success("Telegram configured.")
    return cfg


def _setup_general() -> dict:
    """Set up general agent settings."""
    _section("General Settings")

    agent_name = _prompt("Agent name", "orchestrator")
    base_dir = _prompt("Base directory", "~/.lingtai")
    cli = _prompt_yn("Enable interactive CLI?", default=True)
    agent_port = int(_prompt("Agent TCP port", "8501"))

    return {
        "agent_name": agent_name,
        "base_dir": base_dir,
        "cli": cli,
        "agent_port": agent_port,
    }


# ---------------------------------------------------------------------------
# Main setup flow
# ---------------------------------------------------------------------------

def setup(output_dir: str = ".") -> None:
    """Run the interactive setup wizard."""
    out = Path(output_dir)

    _header("灵台 Setup Wizard")
    _info("This wizard will help you create config.json and model.json.")
    _info("Press Ctrl+C at any time to exit without saving.")
    print()

    # Step 1: Model
    model_cfg = _setup_model()

    # Step 2: IMAP
    imap_cfg = _setup_imap()

    # Step 3: Telegram
    telegram_cfg = _setup_telegram()

    # Step 4: General
    general_cfg = _setup_general()

    # Build config.json
    config: dict = {
        "model": "model.json",
        **general_cfg,
    }
    if imap_cfg is not None:
        config["imap"] = imap_cfg
    if telegram_cfg is not None:
        config["telegram"] = telegram_cfg

    # Preview
    _section("Review")
    print(f"  {_DIM}config.json:{_RESET}")
    for line in json.dumps(config, indent=2).splitlines():
        print(f"    {_WHITE}{line}{_RESET}")
    print()
    print(f"  {_DIM}model.json:{_RESET}")
    for line in json.dumps(model_cfg, indent=2).splitlines():
        print(f"    {_WHITE}{line}{_RESET}")
    print()

    if not _prompt_yn("Write these files?", default=True):
        _warn("Aborted — no files written.")
        return

    # Write files
    config_path = out / "config.json"
    model_path = out / "model.json"

    config_path.write_text(json.dumps(config, indent=2) + "\n")
    model_path.write_text(json.dumps(model_cfg, indent=2) + "\n")

    _success(f"Wrote {config_path}")
    _success(f"Wrote {model_path}")

    print()
    _header("Setup Complete")
    _info(f"Run your agent:  {_BOLD}{_GREEN}lingtai{_RESET}")
    _info(f"Or with config:  {_BOLD}{_GREEN}lingtai {config_path}{_RESET}")
    print()
