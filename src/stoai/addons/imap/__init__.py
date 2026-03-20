"""IMAP addon — real email via IMAP/SMTP.

Adds an `imap` tool with its own mailbox (working_dir/imap/).
An internal TCP bridge port lets other agents relay messages outward.

Usage:
    agent = Agent(
        capabilities=["email", "file"],
        addons={"imap": {
            "email_address": "agent@example.com",
            "email_password": "xxxx xxxx xxxx xxxx",
            "imap_host": "imap.gmail.com",
            "smtp_host": "smtp.gmail.com",
        }},
    )
"""
from __future__ import annotations

import logging
from pathlib import Path
from typing import TYPE_CHECKING

from stoai_kernel.services.mail import TCPMailService
from .manager import IMAPMailManager, SCHEMA, DESCRIPTION
from .service import IMAPMailService

if TYPE_CHECKING:
    from stoai_kernel.base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    email_address: str,
    email_password: str,
    allowed_senders: list[str] | None = None,
    poll_interval: int = 30,
    bridge_port: int = 8399,
    imap_host: str = "imap.gmail.com",
    imap_port: int = 993,
    smtp_host: str = "smtp.gmail.com",
    smtp_port: int = 587,
) -> IMAPMailManager:
    """Set up IMAP addon — registers imap tool, creates services.

    Listeners are NOT started here — they start in IMAPMailManager.start(),
    which is called by Agent.start() via the addon lifecycle.
    """
    working_dir = Path(agent._working_dir)
    tcp_alias = f"127.0.0.1:{bridge_port}"

    imap_svc = IMAPMailService(
        email_address=email_address,
        email_password=email_password,
        allowed_senders=allowed_senders,
        poll_interval=poll_interval,
        working_dir=working_dir,
        imap_host=imap_host,
        imap_port=imap_port,
        smtp_host=smtp_host,
        smtp_port=smtp_port,
    )

    bridge = TCPMailService(listen_port=bridge_port)

    mgr = IMAPMailManager(agent, service=imap_svc, tcp_alias=tcp_alias)
    mgr._bridge = bridge

    agent.add_tool(
        "imap", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt=(
            f"IMAP email account: {email_address}\n"
            f"Internal TCP alias: {tcp_alias} "
            f"(other agents can send to this address to relay via IMAP/SMTP)\n"
            f"Use imap(action=...) for external email. "
            f"Use email(action=...) for inter-agent communication."
        ),
    )

    log.info("IMAP addon configured: %s (bridge: %s)", email_address, tcp_alias)
    return mgr
