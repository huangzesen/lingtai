"""Telegram addon — Bot API client for customer service.

Adds a `telegram` tool with its own mailbox (working_dir/telegram/).
Supports multiple bot accounts, text + images + documents, inline keyboards.

Usage (single account):
    agent = Agent(
        capabilities=["email", "file"],
        addons={"telegram": {
            "bot_token": "123456:ABC-DEF...",
            "allowed_users": [111, 222],
        }},
    )

Usage (multi-account):
    agent = Agent(
        capabilities=["email", "file"],
        addons={"telegram": {
            "accounts": [
                {"alias": "support", "bot_token": "123:ABC", "allowed_users": [111]},
                {"alias": "sales", "bot_token": "789:DEF"},
            ],
        }},
    )
"""
from __future__ import annotations

import logging
from pathlib import Path
from typing import TYPE_CHECKING

from .manager import TelegramManager, SCHEMA, DESCRIPTION
from .service import TelegramService

if TYPE_CHECKING:
    from stoai_kernel.base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    accounts: list[dict] | None = None,
    bot_token: str | None = None,
    allowed_users: list[int] | None = None,
    poll_interval: float = 1.0,
    **kwargs,
) -> TelegramManager:
    """Set up Telegram addon — registers telegram tool, creates services.

    Listeners are NOT started here — they start in TelegramManager.start(),
    which is called by Agent.start() via the addon lifecycle.
    """
    # Normalize single-account shorthand to accounts list
    if accounts is None:
        if bot_token is None:
            raise ValueError("telegram addon requires 'bot_token' or 'accounts'")
        accounts = [{
            "alias": "default",
            "bot_token": bot_token,
            "allowed_users": allowed_users,
            "poll_interval": poll_interval,
        }]

    working_dir = Path(agent._working_dir)

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

    account_names = ", ".join(svc.list_accounts())
    agent.add_tool(
        "telegram", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt=(
            f"Telegram bot accounts: {account_names}\n"
            f"Use telegram(action=...) for Telegram conversations."
        ),
    )

    log.info("Telegram addon configured: %s", account_names)
    return mgr
