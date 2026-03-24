"""Interactive CLI channel — stdin/stdout via inter-agent email."""
from __future__ import annotations

import sys
import threading
from lingtai.services.mail import TCPMailService


class CLIChannel:
    """CLI channel that exchanges messages with the agent via TCP mail.

    Starts a TCPMailService listener on cli_port to receive replies.
    Sends messages to the agent on agent_port.
    """

    def __init__(self, agent_port: int, cli_port: int) -> None:
        self._agent_port = agent_port
        self._cli_port = cli_port
        self._listener: TCPMailService | None = None
        self._sender: TCPMailService | None = None

    @property
    def address(self) -> str:
        return f"cli@localhost:{self._cli_port}"

    def start(self) -> None:
        """Start the TCP listener for incoming replies."""
        self._listener = TCPMailService(listen_port=self._cli_port)
        self._listener.listen(on_message=self._on_message)
        self._sender = TCPMailService()

    def stop(self) -> None:
        """Stop the TCP listener."""
        if self._listener is not None:
            self._listener.stop()

    def send(self, text: str) -> None:
        """Send a message to the agent."""
        if self._sender is None:
            self._sender = TCPMailService()
        self._sender.send(f"localhost:{self._agent_port}", {
            "from": self.address,
            "to": [f"localhost:{self._agent_port}"],
            "subject": "",
            "message": text,
        })

    def _on_message(self, payload: dict) -> None:
        """Handle incoming message — print to stdout."""
        sender = payload.get("from", "agent")
        message = payload.get("message", "")
        if message:
            name = sender.split("@")[0] if "@" in sender else sender
            print(f"[{name}] {message}", flush=True)

    def interactive_loop(self) -> None:
        """Run the interactive stdin/stdout loop. Blocks until EOF or Ctrl+C."""
        try:
            while True:
                try:
                    line = input("> ")
                except EOFError:
                    break
                line = line.strip()
                if not line:
                    continue
                self.send(line)
        except KeyboardInterrupt:
            pass
