"""Gmail addon — real email via Gmail IMAP/SMTP.

Adds a `gmail` tool with its own mailbox (working_dir/gmail/).
An internal TCP bridge port lets other agents relay messages outward.

Usage:
    agent = Agent(
        capabilities=["email", "file"],
        addons={"gmail": {
            "gmail_address": "agent@gmail.com",
            "gmail_password": "xxxx xxxx xxxx xxxx",
            "allowed_senders": ["you@gmail.com"],
        }},
    )
"""
from __future__ import annotations

import logging
from pathlib import Path
from typing import TYPE_CHECKING

from ...services.mail import TCPMailService
from .manager import GmailManager, SCHEMA, DESCRIPTION
from .service import GoogleMailService

if TYPE_CHECKING:
    from ...base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    gmail_address: str,
    gmail_password: str,
    allowed_senders: list[str] | None = None,
    poll_interval: int = 30,
    bridge_port: int = 8399,
    imap_host: str = "imap.gmail.com",
    imap_port: int = 993,
    smtp_host: str = "smtp.gmail.com",
    smtp_port: int = 587,
) -> GmailManager:
    """Set up gmail addon — registers gmail tool, creates services.

    Listeners are NOT started here — they start in GmailManager.start(),
    which is called by Agent.start() via the addon lifecycle.
    """
    working_dir = Path(agent._working_dir)
    tcp_alias = f"127.0.0.1:{bridge_port}"

    gmail_svc = GoogleMailService(
        gmail_address=gmail_address,
        gmail_password=gmail_password,
        allowed_senders=allowed_senders,
        poll_interval=poll_interval,
        working_dir=working_dir,
        imap_host=imap_host,
        imap_port=imap_port,
        smtp_host=smtp_host,
        smtp_port=smtp_port,
    )

    bridge = TCPMailService(listen_port=bridge_port)

    mgr = GmailManager(agent, gmail_service=gmail_svc, tcp_alias=tcp_alias)
    mgr._bridge = bridge

    agent.add_tool(
        "gmail", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt=(
            f"Gmail account: {gmail_address}\n"
            f"Internal TCP alias: {tcp_alias} "
            f"(other agents can send to this address to relay via Gmail)\n"
            f"Use gmail(action=...) for external email. "
            f"Use email(action=...) for inter-agent communication."
        ),
    )

    log.info("Gmail addon configured: %s (bridge: %s)", gmail_address, tcp_alias)
    return mgr
