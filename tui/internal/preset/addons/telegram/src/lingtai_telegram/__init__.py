"""Telegram addon — Bot API client for customer service.

Adds a `telegram` tool with its own mailbox (working_dir/telegram/).
Supports multiple bot accounts, text + images + documents, inline keyboards.

Usage (config file — recommended):
    agent = Agent(
        addons={"telegram": {"config": "telegram.json"}},
    )

Usage (inline, single account):
    agent = Agent(
        addons={"telegram": {
            "bot_token": "123456:ABC-DEF...",
            "allowed_users": [111, 222],
        }},
    )

Usage (inline, multi-account):
    agent = Agent(
        addons={"telegram": {
            "accounts": [
                {"alias": "support", "bot_token": "123:ABC", "allowed_users": [111]},
                {"alias": "sales", "bot_token": "789:DEF"},
            ],
        }},
    )
"""
from __future__ import annotations

import json
import logging
from pathlib import Path
from typing import TYPE_CHECKING

from lingtai_telegram.manager import TelegramManager, SCHEMA, DESCRIPTION
from lingtai_telegram.service import TelegramService

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    config: str | Path | None = None,
    accounts: list[dict] | None = None,
    bot_token: str | None = None,
    allowed_users: list[int] | None = None,
    poll_interval: float = 1.0,
    **kwargs,
) -> TelegramManager:
    """Set up Telegram addon — registers telegram tool, creates services.

    Args:
        config: Path to a JSON config file. Keys are the same as the kwargs
                (bot_token, allowed_users, poll_interval, accounts).
                Inline kwargs override config file values.

    Listeners are NOT started here — they start in TelegramManager.start(),
    which is called by Agent.start() via the addon lifecycle.
    """
    # Load config file if provided — inline kwargs override file values
    if config is not None:
        config_path = Path(config)
        if not config_path.is_file():
            raise FileNotFoundError(f"Telegram config not found: {config_path}")
        file_cfg = json.loads(config_path.read_text(encoding="utf-8"))
        # Resolve *_env keys (e.g. bot_token_env → bot_token)
        from lingtai.config_resolve import resolve_env_fields
        file_cfg = resolve_env_fields(file_cfg)
        # Also resolve *_env in each account dict for multi-account configs
        if "accounts" in file_cfg:
            file_cfg["accounts"] = [
                resolve_env_fields(acct) for acct in file_cfg["accounts"]
            ]
        if accounts is None:
            accounts = file_cfg.get("accounts")
        if bot_token is None:
            bot_token = file_cfg.get("bot_token")
        if allowed_users is None:
            allowed_users = file_cfg.get("allowed_users")
        if poll_interval == 1.0 and "poll_interval" in file_cfg:
            poll_interval = file_cfg["poll_interval"]

    # Normalize single-account shorthand to accounts list
    if accounts is None:
        if bot_token is None:
            raise ValueError(
                "telegram addon requires 'config', 'bot_token', or 'accounts'"
            )
        accounts = [{
            "alias": "default",
            "bot_token": bot_token,
            "allowed_users": allowed_users,
            "poll_interval": poll_interval,
        }]

    working_dir = agent.working_dir

    # Use a list to hold the manager reference so the lambda can capture it
    # before the manager is created (resolved on first call, after start()).
    mgr_ref: list[TelegramManager | None] = [None]

    svc = TelegramService(
        working_dir=working_dir,
        accounts_config=accounts,
        on_message=lambda alias, update: mgr_ref[0].on_incoming(alias, update),
    )

    mgr = TelegramManager(agent=agent, service=svc, working_dir=working_dir)
    mgr_ref[0] = mgr

    agent.add_tool(
        "telegram", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )

    log.info("Telegram addon configured: %s", ", ".join(svc.list_accounts()))
    return mgr
