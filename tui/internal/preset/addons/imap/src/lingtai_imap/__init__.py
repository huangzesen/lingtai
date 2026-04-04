"""IMAP addon — real email via IMAP/SMTP.

Adds an `imap` tool with its own mailbox (working_dir/imap/).
An internal TCP bridge port lets other agents relay messages outward.

Usage (config file — recommended):
    agent = Agent(
        addons={"imap": {"config": "imap.json"}},
    )

Usage (inline, single account):
    agent = Agent(
        addons={"imap": {
            "email_address": "agent@example.com",
            "email_password": "xxxx xxxx xxxx xxxx",
            "imap_host": "imap.gmail.com",
            "smtp_host": "smtp.gmail.com",
        }},
    )

Usage (inline, multi-account):
    agent = Agent(
        addons={"imap": {
            "accounts": [
                {"email_address": "a@gmail.com", "email_password": "xxxx"},
                {"email_address": "b@outlook.com", "email_password": "yyyy",
                 "imap_host": "imap.outlook.com", "smtp_host": "smtp.outlook.com"},
            ],
        }},
    )
"""
from __future__ import annotations

import json
import logging
from pathlib import Path
from typing import TYPE_CHECKING

from lingtai_kernel.services.mail import FilesystemMailService
from lingtai_imap.manager import IMAPMailManager, SCHEMA, DESCRIPTION
from lingtai_imap.service import IMAPMailService

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    # Config file
    config: str | Path | None = None,
    # Single-account shorthand
    email_address: str | None = None,
    email_password: str | None = None,
    imap_host: str = "imap.gmail.com",
    imap_port: int = 993,
    smtp_host: str = "smtp.gmail.com",
    smtp_port: int = 587,
    allowed_senders: list[str] | None = None,
    poll_interval: int = 30,
    # Multi-account
    accounts: list[dict] | None = None,
    # Addon-level
    bridge_port: int = 8399,
) -> IMAPMailManager:
    """Set up IMAP addon — registers imap tool, creates services.

    Args:
        config: Path to a JSON config file. Keys are the same as the kwargs.
                Inline kwargs override config file values.

    Accepts either a flat single-account config or a list of account dicts.

    Listeners are NOT started here — they start in IMAPMailManager.start(),
    which is called by Agent.start() via the addon lifecycle.
    """
    # Load config file if provided — inline kwargs override file values
    if config is not None:
        config_path = Path(config)
        if not config_path.is_file():
            raise FileNotFoundError(f"IMAP config not found: {config_path}")
        file_cfg = json.loads(config_path.read_text(encoding="utf-8"))
        # Resolve *_env keys (e.g. email_password_env → email_password)
        from lingtai.config_resolve import resolve_env_fields
        file_cfg = resolve_env_fields(file_cfg)
        # Also resolve *_env in each account dict for multi-account configs
        if "accounts" in file_cfg:
            file_cfg["accounts"] = [
                resolve_env_fields(acct) for acct in file_cfg["accounts"]
            ]
        if accounts is None:
            accounts = file_cfg.get("accounts")
        if email_address is None:
            email_address = file_cfg.get("email_address")
        if email_password is None:
            email_password = file_cfg.get("email_password")
        if imap_host == "imap.gmail.com" and "imap_host" in file_cfg:
            imap_host = file_cfg["imap_host"]
        if imap_port == 993 and "imap_port" in file_cfg:
            imap_port = file_cfg["imap_port"]
        if smtp_host == "smtp.gmail.com" and "smtp_host" in file_cfg:
            smtp_host = file_cfg["smtp_host"]
        if smtp_port == 587 and "smtp_port" in file_cfg:
            smtp_port = file_cfg["smtp_port"]
        if allowed_senders is None:
            allowed_senders = file_cfg.get("allowed_senders")
        if poll_interval == 30 and "poll_interval" in file_cfg:
            poll_interval = file_cfg["poll_interval"]
        if bridge_port == 8399 and "bridge_port" in file_cfg:
            bridge_port = file_cfg["bridge_port"]

    if accounts is not None:
        account_list = accounts
    elif email_address is not None:
        account_list = [{
            "email_address": email_address,
            "email_password": email_password,
            "imap_host": imap_host,
            "imap_port": imap_port,
            "smtp_host": smtp_host,
            "smtp_port": smtp_port,
            "allowed_senders": allowed_senders,
            "poll_interval": poll_interval,
        }]
    else:
        raise ValueError(
            "imap addon requires 'config', 'accounts', or 'email_address'"
        )

    working_dir = agent.working_dir
    bridge_dir = working_dir / "imap_bridge"
    bridge_dir.mkdir(parents=True, exist_ok=True)

    imap_svc = IMAPMailService(
        accounts=account_list,
        working_dir=working_dir,
    )

    bridge = FilesystemMailService(working_dir=bridge_dir)

    mgr = IMAPMailManager(agent, service=imap_svc, tcp_alias=str(bridge_dir))
    mgr._bridge = bridge

    agent.add_tool(
        "imap", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )

    log.info(
        "IMAP addon configured: %d account(s) (bridge: %s)",
        len(account_list), bridge_dir,
    )
    return mgr
