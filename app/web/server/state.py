"""Agent registry and user mailbox state."""
from __future__ import annotations

import threading
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4

from stoai import Agent, AgentConfig
from stoai.llm import LLMService
from stoai.services.mail import TCPMailService


@dataclass
class AgentEntry:
    """One registered agent."""
    agent_name: str
    name: str
    key: str
    address: str
    port: int
    agent: Agent
    mail_service: TCPMailService
    working_dir: Path


class AppState:
    """Shared application state for the FastAPI server."""

    def __init__(self, base_dir: Path, user_port: int = 8300):
        self.base_dir = base_dir
        self.base_dir.mkdir(parents=True, exist_ok=True)
        self.user_port = user_port
        self.agents: dict[str, AgentEntry] = {}
        self.user_mailbox: list[dict] = []
        self._mailbox_lock = threading.Lock()
        self.user_mail: TCPMailService | None = None

    def _on_user_mail(self, payload: dict) -> None:
        """Callback when user's TCPMailService receives an email."""
        ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        entry = {
            "id": f"mail_{uuid4().hex[:8]}",
            "from": payload.get("from", "unknown"),
            "to": payload.get("to", []),
            "cc": payload.get("cc", []),
            "subject": payload.get("subject", "(no subject)"),
            "message": payload.get("message", ""),
            "time": ts,
        }
        with self._mailbox_lock:
            self.user_mailbox.append(entry)

    def get_inbox(self) -> list[dict]:
        """Return a snapshot of the user mailbox."""
        with self._mailbox_lock:
            return list(self.user_mailbox)

    def register_agent(
        self,
        key: str,
        agent_name: str,
        name: str,
        port: int,
        llm: LLMService,
        capabilities: list | dict | None = None,
        covenant: str = "",
        config: AgentConfig | None = None,
        admin: bool = False,
    ) -> AgentEntry:
        """Create Agent + TCPMailService, wire together, store in registry."""
        working_dir = self.base_dir / agent_name
        mail_svc = TCPMailService(listen_port=port, working_dir=working_dir)
        agent = Agent(
            agent_name=agent_name,
            service=llm,
            mail_service=mail_svc,
            config=config or AgentConfig(max_turns=10),
            base_dir=self.base_dir,
            covenant=covenant,
            capabilities=capabilities or ["email", "web_search"],
            admin=admin,
        )
        entry = AgentEntry(
            agent_name=agent_name,
            name=name,
            key=key,
            address=f"127.0.0.1:{port}",
            port=port,
            agent=agent,
            mail_service=mail_svc,
            working_dir=working_dir,
        )
        self.agents[key] = entry
        return entry

    def start_all(self) -> None:
        """Start user mailbox listener + all agents."""
        self.user_mail = TCPMailService(listen_port=self.user_port)
        self.user_mail.listen(on_message=self._on_user_mail)
        for entry in self.agents.values():
            entry.agent.start()

    def stop_all(self) -> None:
        """Stop all agents + user mailbox."""
        for entry in self.agents.values():
            entry.agent.stop(timeout=5.0)
        if self.user_mail:
            self.user_mail.stop()
