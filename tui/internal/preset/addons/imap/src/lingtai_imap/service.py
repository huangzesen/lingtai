"""IMAPMailService — multi-account IMAP/SMTP coordinator."""
from __future__ import annotations

import logging
from pathlib import Path
from typing import Callable

from lingtai_kernel.services.mail import MailService
from .account import IMAPAccount

logger = logging.getLogger(__name__)


class IMAPMailService(MailService):
    """Multi-account IMAP/SMTP coordinator."""

    def __init__(
        self,
        accounts: list[dict],
        *,
        working_dir: Path | str | None = None,
    ) -> None:
        self._working_dir = Path(working_dir) if working_dir else None
        self._accounts: list[IMAPAccount] = []
        for cfg in accounts:
            acct = IMAPAccount(
                email_address=cfg["email_address"],
                email_password=cfg["email_password"],
                imap_host=cfg.get("imap_host", "imap.gmail.com"),
                imap_port=cfg.get("imap_port", 993),
                smtp_host=cfg.get("smtp_host", "smtp.gmail.com"),
                smtp_port=cfg.get("smtp_port", 587),
                working_dir=self._working_dir,
                allowed_senders=cfg.get("allowed_senders"),
                poll_interval=cfg.get("poll_interval", 30),
            )
            self._accounts.append(acct)
        self._account_map: dict[str, IMAPAccount] = {
            a.address: a for a in self._accounts
        }

    @property
    def accounts(self) -> list[IMAPAccount]:
        return list(self._accounts)

    @property
    def default_account(self) -> IMAPAccount:
        return self._accounts[0]

    def get_account(self, address: str | None) -> IMAPAccount | None:
        if address is None:
            return self.default_account
        return self._account_map.get(address)

    # -- MailService interface --

    def send(self, address: str, message: dict) -> str | None:
        return self.default_account.send_email(
            to=[address],
            subject=message.get("subject", ""),
            body=message.get("message", ""),
        )

    def listen(self, on_message: Callable[[dict], None]) -> None:
        """Start listening on all accounts.

        Wraps the per-message callback so each header dict from the
        account's batch callback is dispatched individually.
        """
        def _dispatch_each(headers: list[dict]) -> None:
            for header in headers:
                on_message(header)

        for acct in self._accounts:
            acct.start_listening(_dispatch_each)

    def stop(self) -> None:
        for acct in self._accounts:
            acct.stop_listening()
            acct.disconnect()

    @property
    def address(self) -> str | None:
        return self.default_account.address if self._accounts else None
