"""Launch two agents with email-based web UI.

Agent A (Alice/researcher): TCP 8301
Agent B (Bob/assistant):    TCP 8302
User mailbox:               TCP 8300
Web UI:                     http://localhost:8080

Communication is all email. User messages are emails to agents.
Agent replies are emails to the user. Agent text responses are diary entries.

Usage:
    python examples/two_agents.py

Press Ctrl+C to shut down.
"""
from __future__ import annotations

import http.server
import json
import os
import sys
import threading
import time
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4

# Load .env
env_path = Path(__file__).parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

from stoai import Agent, AgentConfig
from stoai.llm import LLMService
from stoai.services.mail import TCPMailService


# ---------------------------------------------------------------------------
# User mailbox — stores emails received from agents
# ---------------------------------------------------------------------------

user_mailbox: list[dict] = []
user_mailbox_lock = threading.Lock()


def on_user_mail(payload: dict) -> None:
    """Callback when user's TCPMailService receives an email."""
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    entry = {
        "id": f"mail_{uuid4().hex[:8]}",
        "from": payload.get("from", "unknown"),
        "subject": payload.get("subject", "(no subject)"),
        "message": payload.get("message", ""),
        "time": ts,
    }
    with user_mailbox_lock:
        user_mailbox.append(entry)


# ---------------------------------------------------------------------------
# HTML
# ---------------------------------------------------------------------------


USER_PORT = 8300

HTML_PAGE = """<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>StoAI — Two Agents</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #1a1a2e; color: #e0e0e0; height: 100vh; display: flex; flex-direction: column; }
#header { padding: 10px 20px; background: #16213e; border-bottom: 1px solid #0f3460; display: flex; align-items: center; gap: 12px; }
#header h1 { font-size: 16px; color: #e94560; }
.tab { padding: 6px 14px; border-radius: 6px; cursor: pointer; font-size: 13px; border: 1px solid #0f3460; background: #1a1a2e; color: #888; }
.tab.active { background: #0f3460; color: #e0e0e0; border-color: #e94560; }
#main { flex: 1; display: flex; overflow: hidden; }
#inbox-panel { flex: 2; display: flex; flex-direction: column; border-right: 1px solid #0f3460; }
#diary-panel { flex: 1; display: flex; flex-direction: column; background: #12122a; }
.panel-header { padding: 8px 16px; font-size: 12px; color: #e94560; text-transform: uppercase; letter-spacing: 1px; border-bottom: 1px solid #0f3460; }
#inbox { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 8px; }
#diary { flex: 1; overflow-y: auto; padding: 12px; font-size: 12px; color: #888; }
.email { padding: 10px 14px; border-radius: 8px; line-height: 1.5; white-space: pre-wrap; word-wrap: break-word; font-size: 14px; }
.email.from-user { align-self: flex-end; background: #0f3460; max-width: 80%%; }
.email.from-agent { align-self: flex-start; background: #16213e; border: 1px solid #0f3460; max-width: 80%%; }
.email .meta { font-size: 11px; color: #666; margin-bottom: 4px; }
.diary-entry { padding: 4px 0; border-bottom: 1px solid #1a1a2e; line-height: 1.4; }
.diary-entry .ts { color: #555; }
.diary-entry .agent-tag { font-weight: bold; }
.diary-entry .agent-tag.alice { color: #e94560; }
.diary-entry .agent-tag.bob { color: #4ecdc4; }
.diary-entry .tag { font-size: 10px; padding: 1px 4px; border-radius: 3px; margin-right: 4px; }
.tag-diary { background: #1a3a1a; color: #6bcb77; }
.tag-thinking { background: #3a3a1a; color: #cbc76b; }
.tag-tool { background: #1a1a3a; color: #6b9bcb; }
.tag-reasoning { background: #2a1a3a; color: #b06bcb; }
.tag-result { background: #1a2a2a; color: #6bcbbb; }
.tag-email-out { background: #1a2a3a; color: #6bb5cb; }
.tag-email-in { background: #2a1a2a; color: #cb6bb5; }
.email-body { margin-top: 4px; padding: 6px 8px; background: rgba(255,255,255,0.03); border-radius: 4px; white-space: pre-wrap; font-size: 11px; color: #aaa; max-height: 200px; overflow-y: auto; }
#input-bar { padding: 10px 16px; background: #16213e; border-top: 1px solid #0f3460; display: flex; gap: 8px; }
#target { padding: 8px; border: 1px solid #0f3460; border-radius: 6px; background: #1a1a2e; color: #e0e0e0; font-size: 13px; }
#input { flex: 1; padding: 8px 12px; border: 1px solid #0f3460; border-radius: 6px; background: #1a1a2e; color: #e0e0e0; font-size: 14px; outline: none; }
#input:focus { border-color: #e94560; }
#send-btn { padding: 8px 16px; background: #e94560; color: white; border: none; border-radius: 6px; cursor: pointer; font-size: 13px; }
#send-btn:hover { background: #c73e54; }
</style>
</head>
<body>
<div id="header">
  <h1>StoAI</h1>
  <span style="color:#666;font-size:12px">User mailbox: :""" + str(USER_PORT) + """</span>
</div>
<div id="main">
  <div id="inbox-panel">
    <div class="panel-header">Inbox</div>
    <div id="inbox"></div>
    <div id="input-bar">
      <select id="target">
        <option value="a">To: Alice (:8301)</option>
        <option value="b">To: Bob (:8302)</option>
      </select>
      <input id="input" placeholder="Type a message..." autofocus>
      <button id="send-btn" onclick="sendEmail()">Send</button>
    </div>
  </div>
  <div id="diary-panel">
    <div class="panel-header">Agent Diary</div>
    <div id="diary"></div>
  </div>
</div>
<script>
const inbox = document.getElementById('inbox');
const diary = document.getElementById('diary');
const input = document.getElementById('input');
const target = document.getElementById('target');

const agentPorts = { a: '8301', b: '8302' };
const agentNames = { '127.0.0.1:8301': 'Alice', '127.0.0.1:8302': 'Bob' };
let sentMessages = [];

input.addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); sendEmail(); } });

async function sendEmail() {
  const text = input.value.trim();
  if (!text) return;
  input.value = '';
  const agentKey = target.value;
  const port = agentPorts[agentKey];

  // Show sent message in inbox
  sentMessages.push({ to: agentKey, text, time: new Date().toISOString() });
  renderInbox();

  await fetch('/send', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({ agent: agentKey, message: text }),
  });
  input.focus();
}

function renderInbox() {
  inbox.innerHTML = '';

  // Merge sent + received, sort by time
  const all = [];
  for (const s of sentMessages) {
    all.push({ type: 'sent', to: s.to, text: s.text, time: s.time });
  }
  for (const e of receivedEmails) {
    all.push({ type: 'received', from: e.from, subject: e.subject, text: e.message, time: e.time });
  }
  all.sort((a, b) => a.time.localeCompare(b.time));

  for (const m of all) {
    const div = document.createElement('div');
    if (m.type === 'sent') {
      div.className = 'email from-user';
      const name = m.to === 'a' ? 'Alice' : 'Bob';
      div.innerHTML = '<div class="meta">To: ' + name + '</div>' + escapeHtml(m.text);
    } else {
      div.className = 'email from-agent';
      const name = agentNames[m.from] || m.from;
      const subj = m.subject && m.subject !== '(no subject)' ? ' — ' + m.subject : '';
      div.innerHTML = '<div class="meta">From: ' + name + subj + '</div>' + escapeHtml(m.text);
    }
    inbox.appendChild(div);
  }
  inbox.scrollTop = inbox.scrollHeight;
}

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

// Poll for new emails and diary entries
let receivedEmails = [];
let lastDiaryLen = {};

async function poll() {
  try {
    // Poll user inbox
    const inboxResp = await fetch('/inbox');
    const inboxData = await inboxResp.json();
    if (inboxData.emails.length > receivedEmails.length) {
      receivedEmails = inboxData.emails;
      renderInbox();
    }
    // Poll diary
    const diaryResp = await fetch('/diary');
    const diaryData = await diaryResp.json();
    let newDiary = false;
    for (const k of Object.keys(diaryData)) {
      const len = (diaryData[k]||[]).length;
      if (len > (lastDiaryLen[k]||0)) { lastDiaryLen[k] = len; newDiary = true; }
    }
    if (newDiary) {
      diary.innerHTML = '';
      const allDiary = [];
      for (const e of (diaryData.a||[])) allDiary.push({...e, agent: 'Alice', agentCls: 'alice'});
      for (const e of (diaryData.b||[])) allDiary.push({...e, agent: 'Bob', agentCls: 'bob'});
      allDiary.sort((a, b) => (a.time||0) - (b.time||0));
      for (const e of allDiary) {
        const div = document.createElement('div');
        div.className = 'diary-entry';
        const ts = new Date((e.time||0)*1000).toLocaleTimeString();
        const agentTag = '<span class="agent-tag ' + e.agentCls + '">[' + e.agent + ']</span> ';
        let content = '';
        if (e.type === 'diary') {
          content = '<span class="tag tag-diary">diary</span>' + escapeHtml(e.text||'');
        } else if (e.type === 'thinking') {
          content = '<span class="tag tag-thinking">thinking</span>' + escapeHtml(e.text||'');
        } else if (e.type === 'tool_call') {
          const args = JSON.stringify(e.args||{}).slice(0,80);
          content = '<span class="tag tag-tool">tool</span>' + escapeHtml(e.tool) + '(' + escapeHtml(args) + ')';
        } else if (e.type === 'reasoning') {
          content = '<span class="tag tag-reasoning">why</span>' + escapeHtml(e.tool) + ': ' + escapeHtml(e.text||'');
        } else if (e.type === 'tool_result') {
          content = '<span class="tag tag-result">result</span>' + escapeHtml(e.tool) + ' → ' + escapeHtml(e.status||'');
        } else if (e.type === 'email_out') {
          const toName = agentNames[e.to] || e.to || '';
          const subj = e.subject ? ' — ' + e.subject : '';
          content = '<span class="tag tag-email-out">sent</span>to ' + escapeHtml(toName) + escapeHtml(subj) +
            '<div class="email-body">' + escapeHtml(e.message||'') + '</div>';
        } else if (e.type === 'email_in') {
          const fromName = agentNames[e.from] || e.from || '';
          const subj = e.subject ? ' — ' + e.subject : '';
          content = '<span class="tag tag-email-in">received</span>from ' + escapeHtml(fromName) + escapeHtml(subj) +
            '<div class="email-body">' + escapeHtml(e.message||'') + '</div>';
        } else {
          content = escapeHtml(JSON.stringify(e));
        }
        div.innerHTML = '<span class="ts">' + ts + '</span> ' + agentTag + content;
        diary.appendChild(div);
      }
      diary.scrollTop = diary.scrollHeight;
    }
  } catch(e) {}
}

setInterval(poll, 1500);
</script>
</body>
</html>"""


# ---------------------------------------------------------------------------
# HTTP handler
# ---------------------------------------------------------------------------

class ChatHandler(http.server.BaseHTTPRequestHandler):
    agents: dict[str, Agent] = {}
    agent_ports: dict[str, int] = {}
    base_dir: Path = Path(".")

    def do_GET(self):
        if self.path == "/inbox":
            with user_mailbox_lock:
                emails = list(user_mailbox)
            self._json({"emails": emails})
            return

        if self.path == "/diary":
            result = {}
            # Read from JSONL files in the working directories
            # base_dir / agent_name / logs / events.jsonl
            agent_names = {"a": "alice", "b": "bob"}
            for key, agent_name in agent_names.items():
                log_file = ChatHandler.base_dir / agent_name / "logs" / "events.jsonl"
                entries = []
                if log_file.exists():
                    with open(log_file, "r") as f:
                        for line in f:
                            line = line.strip()
                            if not line: continue
                            try:
                                e = json.loads(line)
                            except: continue
                            
                            etype = e.get("type", "")
                            if etype == "diary":
                                entries.append({"type": "diary", "time": e.get("ts", 0), "text": e.get("text", "")})
                            elif etype == "thinking":
                                entries.append({"type": "thinking", "time": e.get("ts", 0), "text": e.get("text", "")})
                            elif etype == "tool_call":
                                entries.append({"type": "tool_call", "time": e.get("ts", 0),
                                                "tool": e.get("tool_name", ""), "args": e.get("tool_args", {})})
                            elif etype == "tool_reasoning":
                                entries.append({"type": "reasoning", "time": e.get("ts", 0),
                                                "tool": e.get("tool", ""), "text": e.get("reasoning", "")})
                            elif etype == "tool_result":
                                entries.append({"type": "tool_result", "time": e.get("ts", 0),
                                                "tool": e.get("tool_name", ""), "status": e.get("status", "")})
                            elif etype == "email_sent":
                                to = e.get("to") or e.get("address", "")
                                if isinstance(to, list):
                                    to = ", ".join(to)
                                entries.append({"type": "email_out", "time": e.get("ts", 0),
                                                "to": to, "subject": e.get("subject", ""),
                                                "message": e.get("message", ""), "status": e.get("status", "")})
                            elif etype == "email_received":
                                entries.append({"type": "email_in", "time": e.get("ts", 0),
                                                "from": e.get("sender", ""), "subject": e.get("subject", ""),
                                                "message": e.get("message", "")})
                result[key] = entries
            self._json(result)
            return


        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.end_headers()
        self.wfile.write(HTML_PAGE.encode("utf-8"))

    def do_POST(self):
        if self.path != "/send":
            self.send_error(404)
            return
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length))
        agent_key = body.get("agent", "a")
        message = body.get("message", "")

        port = ChatHandler.agent_ports.get(agent_key)
        if not port:
            self._json({"error": f"Unknown agent: {agent_key}"})
            return

        # Send as email from user to agent
        sender = TCPMailService()
        err = sender.send(f"127.0.0.1:{port}", {
            "from": f"127.0.0.1:{USER_PORT}",
            "to": [f"127.0.0.1:{port}"],
            "subject": "",
            "message": message,
        })
        self._json({"status": "delivered"} if err is None else {"status": "failed", "error": err})

    def _json(self, data):
        payload = json.dumps(data, ensure_ascii=False).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, *a):
        pass


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def load_covenant() -> str:
    """Load covenant template (behavioral rules only, no identity or contacts)."""
    template_path = Path(__file__).parent.parent / "prompt" / "covenant" / "covenant.example.md"
    if template_path.exists():
        return template_path.read_text()
    return ""


def write_character(agent_dir: Path, contacts: dict[str, str]) -> None:
    """Write initial character.md with friends."""
    contact_lines = "\n".join(f"- {n}: {a}" for n, a in contacts.items())
    system_dir = agent_dir / "system"
    system_dir.mkdir(parents=True, exist_ok=True)
    char_file = system_dir / "character.md"
    if not char_file.is_file():
        char_file.write_text(f"### Friends\n{contact_lines}\n")


def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set.")
        sys.exit(1)

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_config={"web_search_provider": "minimax", "vision_provider": "minimax"},
        provider_defaults={"minimax": {"model": "MiniMax-M2.5-highspeed"}},
    )

    # User mailbox — receives emails from agents
    user_mail = TCPMailService(listen_port=USER_PORT)
    user_mail.listen(on_message=on_user_mail)

    base_dir = Path.home() / ".stoai" / "two-agent" / "playground"
    base_dir.mkdir(parents=True, exist_ok=True)

    # Symlink into the project for easy access
    project_link = Path(__file__).parent.parent / "playground"
    if not project_link.exists():
        project_link.symlink_to(base_dir)

    covenant = load_covenant()

    # Agent A
    write_character(base_dir / "alice", {
        "Bob": "127.0.0.1:8302",
        "User": f"127.0.0.1:{USER_PORT}",
    })
    mail_a = TCPMailService(listen_port=8301, working_dir=base_dir / "alice")
    agent_a = Agent(
        agent_name="alice", service=llm, mail_service=mail_a,
        config=AgentConfig(max_turns=10), base_dir=base_dir,
        covenant=covenant,
        capabilities={
            "email": {}, "web_search": {}, "file": {},
            "vision": {}, "anima": {}, "conscience": {"interval": 10},
            "bash": {},
        },
    )

    # Agent B
    write_character(base_dir / "bob", {
        "Alice": "127.0.0.1:8301",
        "User": f"127.0.0.1:{USER_PORT}",
    })
    mail_b = TCPMailService(listen_port=8302, working_dir=base_dir / "bob")
    agent_b = Agent(
        agent_name="bob", service=llm, mail_service=mail_b,
        config=AgentConfig(max_turns=10), base_dir=base_dir,
        covenant=covenant,
        capabilities={
            "email": {}, "web_search": {}, "file": {},
            "vision": {}, "anima": {}, "conscience": {"interval": 10},
            "bash": {},
        },
    )


    agent_a.start()
    agent_b.start()

    ChatHandler.agents = {"a": agent_a, "b": agent_b}
    ChatHandler.agent_ports = {"a": 8301, "b": 8302}
    ChatHandler.base_dir = base_dir

    print(f"User mailbox:    127.0.0.1:{USER_PORT}")
    print("Agent A (Alice): 127.0.0.1:8301")
    print("Agent B (Bob):   127.0.0.1:8302")
    print("Web UI:          http://localhost:8080")
    print("Press Ctrl+C to shut down.")

    server = http.server.HTTPServer(("0.0.0.0", 8080), ChatHandler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down...")
    finally:
        server.shutdown()
        user_mail.stop()
        agent_a.stop(timeout=5.0)
        agent_b.stop(timeout=5.0)
        print("Done.")


if __name__ == "__main__":
    main()
