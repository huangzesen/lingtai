"""Feishu (Lark) addon — enterprise messaging bot.

Adds a `feishu` tool with its own mailbox (working_dir/feishu/).
Supports multiple app accounts, text messages, and group chats.
Uses lark-oapi WebSocket long connection — no public IP required.

Usage (config file — recommended):
    agent = Agent(
        addons={"feishu": {"config": "feishu.json"}},
    )

Usage (inline, single account):
    agent = Agent(
        addons={"feishu": {
            "app_id": "cli_xxxxxxxxxx",
            "app_secret": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
            "allowed_users": ["ou_xxxxxxxxxxxxxxxx"],
        }},
    )

Usage (inline, multi-account):
    agent = Agent(
        addons={"feishu": {
            "accounts": [
                {
                    "alias": "support",
                    "app_id": "cli_aaa",
                    "app_secret": "xxx",
                    "allowed_users": ["ou_yyy"],
                },
            ],
        }},
    )

Prerequisites (Feishu Open Platform):
    1. Create an enterprise self-built app at https://open.feishu.cn/app
    2. Enable permissions: im:message
    3. In "Event Subscriptions", enable "Use long connection to receive events"
       and subscribe to im.message.receive_v1
    4. Publish the app version
"""
from __future__ import annotations

import json
import logging
from pathlib import Path
from typing import TYPE_CHECKING

from lingtai_feishu.manager import FeishuManager, SCHEMA, DESCRIPTION
from lingtai_feishu.service import FeishuService

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    config: str | Path | None = None,
    accounts: list[dict] | None = None,
    app_id: str | None = None,
    app_secret: str | None = None,
    allowed_users: list[str] | None = None,
    **kwargs,
) -> FeishuManager:
    """Set up Feishu addon — registers feishu tool, creates services.

    Args:
        config: Path to a JSON config file.  Keys match the kwargs above.
                ``*_env`` suffix keys are resolved from environment variables
                (e.g. ``app_id_env: "FEISHU_APP_ID"``).
                Inline kwargs override config file values.

    Listeners are NOT started here — they start in FeishuManager.start(),
    which is called by Agent.start() via the addon lifecycle.
    """
    if config is not None:
        config_path = Path(config)
        if not config_path.is_file():
            raise FileNotFoundError(f"Feishu config not found: {config_path}")
        file_cfg = json.loads(config_path.read_text(encoding="utf-8"))
        from lingtai.config_resolve import resolve_env_fields
        file_cfg = resolve_env_fields(file_cfg)
        if "accounts" in file_cfg:
            file_cfg["accounts"] = [
                resolve_env_fields(acct) for acct in file_cfg["accounts"]
            ]
        if accounts is None:
            accounts = file_cfg.get("accounts")
        if app_id is None:
            app_id = file_cfg.get("app_id")
        if app_secret is None:
            app_secret = file_cfg.get("app_secret")
        if allowed_users is None:
            allowed_users = file_cfg.get("allowed_users")

    # Normalize single-account shorthand to accounts list
    if accounts is None:
        if app_id is None or app_secret is None:
            raise ValueError(
                "feishu addon requires 'config', 'app_id'+'app_secret', or 'accounts'"
            )
        alias = kwargs.get("alias", "default")
        accounts = [{
            "alias": alias,
            "app_id": app_id,
            "app_secret": app_secret,
            "allowed_users": allowed_users,
        }]

    working_dir = agent.working_dir

    # mgr_ref trick: FeishuService needs the on_message callback pointing at
    # the manager before the manager is created.
    mgr_ref: list[FeishuManager | None] = [None]

    svc = FeishuService(
        working_dir=working_dir,
        accounts_config=accounts,
        on_message=lambda alias, event: mgr_ref[0].on_incoming(alias, event),
    )

    mgr = FeishuManager(agent=agent, service=svc, working_dir=working_dir)
    mgr_ref[0] = mgr

    agent.add_tool(
        "feishu", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )

    log.info("Feishu addon configured: %s", ", ".join(svc.list_accounts()))
    return mgr
