"""Web chat UI for an Agent.

Usage:
    python examples/chat_web.py

Opens a browser at http://localhost:8080 to chat with the agent.
Agent runs on TCP port 8301 (internal), web UI on port 8080.
"""
from __future__ import annotations

import http.server
import json
import os
import socket
import struct
import sys
import threading
import time
from pathlib import Path

# Load .env
env_path = Path(__file__).parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

from lingtai import Agent, AgentConfig
from lingtai.llm import LLMService
from lingtai.services.mail import TCPMailService

AGENT_PORT = 8301
WEB_PORT = 8080

HTML_PAGE = """<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>灵台 Chat</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #1a1a2e; color: #e0e0e0; height: 100vh; display: flex; flex-direction: column; }
#header { padding: 16px 24px; background: #16213e; border-bottom: 1px solid #0f3460; }
#header h1 { font-size: 18px; color: #e94560; }
#header span { font-size: 12px; color: #888; }
#messages { flex: 1; overflow-y: auto; padding: 24px; display: flex; flex-direction: column; gap: 12px; }
.msg { max-width: 80%; padding: 12px 16px; border-radius: 12px; line-height: 1.5; white-space: pre-wrap; word-wrap: break-word; }
.msg.user { align-self: flex-end; background: #0f3460; color: #e0e0e0; }
.msg.agent { align-self: flex-start; background: #16213e; border: 1px solid #0f3460; }
.msg.error { align-self: center; background: #4a0000; color: #ff6b6b; font-size: 13px; }
.msg.thinking { align-self: flex-start; color: #666; font-style: italic; }
#input-bar { padding: 16px 24px; background: #16213e; border-top: 1px solid #0f3460; display: flex; gap: 12px; }
#input { flex: 1; padding: 12px 16px; border: 1px solid #0f3460; border-radius: 8px; background: #1a1a2e; color: #e0e0e0; font-size: 15px; outline: none; }
#input:focus { border-color: #e94560; }
#send { padding: 12px 24px; background: #e94560; color: white; border: none; border-radius: 8px; cursor: pointer; font-size: 15px; }
#send:hover { background: #c73e54; }
#send:disabled { background: #555; cursor: not-allowed; }
</style>
</head>
<body>
<div id="header"><h1>灵台 Chat</h1><span>MiniMax agent on port """ + str(AGENT_PORT) + """</span></div>
<div id="messages"></div>
<div id="input-bar">
  <input id="input" placeholder="Type a message..." autofocus>
  <button id="send" onclick="sendMsg()">Send</button>
</div>
<script>
const msgs = document.getElementById('messages');
const input = document.getElementById('input');
const sendBtn = document.getElementById('send');

input.addEventListener('keydown', e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMsg(); } });

function addMsg(text, cls) {
  const div = document.createElement('div');
  div.className = 'msg ' + cls;
  div.textContent = text;
  msgs.appendChild(div);
  msgs.scrollTop = msgs.scrollHeight;
  return div;
}

async function sendMsg() {
  const text = input.value.trim();
  if (!text) return;
  input.value = '';
  addMsg(text, 'user');
  sendBtn.disabled = true;
  const thinking = addMsg('Thinking...', 'thinking');

  try {
    const resp = await fetch('/chat', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({message: text}),
    });
    const data = await resp.json();
    thinking.remove();
    if (data.error) {
      addMsg('Error: ' + data.error, 'error');
    } else {
      addMsg(data.reply, 'agent');
    }
  } catch (e) {
    thinking.remove();
    addMsg('Network error: ' + e.message, 'error');
  }
  sendBtn.disabled = false;
  input.focus();
}
</script>
</body>
</html>"""


class ChatHandler(http.server.BaseHTTPRequestHandler):
    """HTTP handler: serves chat page and proxies messages to agent."""

    agent: Agent = None  # set by main()

    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.end_headers()
        self.wfile.write(HTML_PAGE.encode("utf-8"))

    def do_POST(self):
        if self.path != "/chat":
            self.send_error(404)
            return

        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length))
        user_msg = body.get("message", "")

        if not user_msg:
            self._json_response({"error": "Empty message"})
            return

        # Send to agent via its public send() API (blocks until response)
        agent = ChatHandler.agent
        result = agent.send(user_msg, sender="web_user", wait=True, timeout=120.0)

        if result is None:
            self._json_response({"error": "No response (timeout)"})
        elif result.get("failed"):
            self._json_response({"error": result.get("errors", ["Unknown error"])[0]})
        else:
            self._json_response({"reply": result.get("text", "")})

    def _json_response(self, data: dict):
        payload = json.dumps(data, ensure_ascii=False).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format, *args):
        pass  # silence HTTP request logs


def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set. Check .env file.")
        sys.exit(1)

    print("Starting agent...")

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_defaults={
            "minimax": {"model": "MiniMax-M2.5-highspeed"},
        },
    )

    mail_svc = TCPMailService(listen_port=AGENT_PORT)

    policy = str(Path(__file__).parent / "bash_policy.json")
    agent = Agent(
        agent_name="assistant",
        service=llm,
        mail_service=mail_svc,
        config=AgentConfig(max_turns=20),
        base_dir=".",
        role="You are a helpful AI assistant.",
        capabilities={"email": {}, "bash": {"policy_file": policy}, "file": {}},
    )
    agent.start()

    ChatHandler.agent = agent

    print(f"Agent running on TCP port {AGENT_PORT}")
    print(f"Web UI at http://localhost:{WEB_PORT}")
    print("Press Ctrl+C to quit.\n")

    server = http.server.HTTPServer(("127.0.0.1", WEB_PORT), ChatHandler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down...")
    finally:
        server.shutdown()
        agent.stop(timeout=5.0)
        print("Done.")


if __name__ == "__main__":
    main()
