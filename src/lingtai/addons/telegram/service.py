"""TelegramService — multi-account orchestrator.

Creates one TelegramAccount per config entry.
Routes outbound sends to the correct account by alias.
Delegates lifecycle (start/stop) to all accounts.
"""
from __future__ import annotations

import logging
from pathlib import Path
from typing import Callable

from .account import TelegramAccount

logger = logging.getLogger(__name__)


class TelegramService:
    """Multi-account Telegram bot service."""

    def __init__(
        self,
        working_dir: Path,
        accounts_config: list[dict],
        on_message: Callable[[str, dict], None],
    ) -> None:
        self._working_dir = working_dir
        self._on_message = on_message
        self._account_order: list[str] = []
        self._accounts: dict[str, TelegramAccount] = {}

        for cfg in accounts_config:
            alias = cfg["alias"]
            state_dir = working_dir / "telegram" / alias
            acct = TelegramAccount(
                alias=alias,
                bot_token=cfg["bot_token"],
                allowed_users=cfg.get("allowed_users"),
                poll_interval=cfg.get("poll_interval", 1.0),
                on_message=on_message,
                state_dir=state_dir,
            )
            self._accounts[alias] = acct
            self._account_order.append(alias)

    def get_account(self, alias: str) -> TelegramAccount:
        """Get account by alias. Raises KeyError if not found."""
        return self._accounts[alias]

    @property
    def default_account(self) -> TelegramAccount:
        """Return the first configured account."""
        return self._accounts[self._account_order[0]]

    def list_accounts(self) -> list[str]:
        """Return list of account aliases in config order."""
        return list(self._account_order)

    def start(self) -> None:
        """Start all accounts' polling threads."""
        for acct in self._accounts.values():
            acct.start()

    def stop(self) -> None:
        """Stop all accounts."""
        for acct in self._accounts.values():
            acct.stop()
