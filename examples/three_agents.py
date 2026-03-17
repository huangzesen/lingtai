"""Launch three agents with email-based web UI.

Agent A (Alice):   TCP 8301
Agent B (Bob):     TCP 8302
Agent C (Charlie): TCP 8303
User mailbox:      TCP 8300
Web UI:            http://localhost:8080

Communication is all email. User messages are emails to agents.
Agent replies are emails to the user. Agent text responses are diary entries.

Usage:
    python examples/three_agents.py

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

from stoai import StoAIAgent, AgentConfig
from stoai.llm import LLMService
from stoai.services.mail import TCPMailService
from stoai.services.logging import LoggingService


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
        "to": payload.get("to", []),
        "cc": payload.get("cc", []),
        "subject": payload.get("subject", "(no subject)"),
        "message": payload.get("message", ""),
        "time": ts,
    }
    with user_mailbox_lock:
        user_mailbox.append(entry)


# ---------------------------------------------------------------------------
# Memory logging service — captures events for web UI polling
# ---------------------------------------------------------------------------

class MemoryLoggingService(LoggingService):
    """Stores events in memory for the web UI to poll."""

    def __init__(self):
        self._events: list[dict] = []
        self._lock = threading.Lock()

    def log(self, event: dict) -> None:
        with self._lock:
            self._events.append(event)

    def get_events(self, since: int = 0) -> list[dict]:
        with self._lock:
            return self._events[since:]

    def count(self) -> int:
        with self._lock:
            return len(self._events)


loggers: dict[str, MemoryLoggingService] = {}


# ---------------------------------------------------------------------------
# HTML
# ---------------------------------------------------------------------------

USER_PORT = 8300

HTML_PAGE = """<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>StoAI — Three Agents</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #1a1a2e; color: #e0e0e0; height: 100vh; display: flex; flex-direction: column; }
#header { padding: 10px 20px; background: #16213e; border-bottom: 1px solid #0f3460; display: flex; align-items: center; gap: 12px; }
#header h1 { font-size: 16px; color: #e94560; }
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
.diary-entry .agent-tag.charlie { color: #f0a500; }
.diary-entry .tag { font-size: 10px; padding: 1px 4px; border-radius: 3px; margin-right: 4px; }
.tag-diary { background: #1a3a1a; color: #6bcb77; }
.tag-thinking { background: #3a3a1a; color: #cbc76b; }
.tag-tool { background: #1a1a3a; color: #6b9bcb; }
.tag-reasoning { background: #2a1a3a; color: #b06bcb; }
.tag-result { background: #1a2a2a; color: #6bcbbb; }
.tag-email-out { background: #1a2a3a; color: #6bb5cb; }
.tag-email-in { background: #2a1a2a; color: #cb6bb5; }
.tag-cancel { background: #3a1a1a; color: #e94560; font-weight: bold; }
.tag-cancel-diary { background: #3a2a1a; color: #f0a500; }
.email-body { margin-top: 4px; padding: 6px 8px; background: rgba(255,255,255,0.03); border-radius: 4px; white-space: pre-wrap; font-size: 11px; color: #aaa; max-height: 200px; overflow-y: auto; }
#input-bar { padding: 10px 16px; background: #16213e; border-top: 1px solid #0f3460; display: flex; gap: 8px; align-items: center; }
#target { padding: 8px; border: 1px solid #0f3460; border-radius: 6px; background: #1a1a2e; color: #e0e0e0; font-size: 13px; }
#input { flex: 1; padding: 8px 12px; border: 1px solid #0f3460; border-radius: 6px; background: #1a1a2e; color: #e0e0e0; font-size: 14px; outline: none; }
#input:focus { border-color: #e94560; }
#send-btn { padding: 8px 16px; background: #e94560; color: white; border: none; border-radius: 6px; cursor: pointer; font-size: 13px; }
#send-btn:hover { background: #c73e54; }
.toggle-btn { padding: 6px 10px; background: #1a1a2e; color: #888; border: 1px solid #0f3460; border-radius: 6px; cursor: pointer; font-size: 11px; }
.toggle-btn.active { color: #e0e0e0; border-color: #e94560; }
.cc-bcc-row { display: none; padding: 6px 16px; background: #16213e; font-size: 12px; color: #888; border-top: 1px solid #0f3460; }
.cc-bcc-row label { margin-right: 12px; cursor: pointer; }
.cc-bcc-row input { margin-right: 4px; }
.diary-tabs { display: flex; gap: 0; border-bottom: 1px solid #0f3460; }
.diary-tab { padding: 8px 12px; cursor: pointer; color: #666; font-size: 12px; text-transform: uppercase; letter-spacing: 1px; border-bottom: 2px solid transparent; }
.diary-tab.active { color: #e94560; border-bottom-color: #e94560; }
.diary-tab:hover { color: #e0e0e0; }
</style>
</head>
<body>
<div id="header">
  <h1>StoAI</h1>
  <span style="color:#666;font-size:12px">Three Agents · User mailbox :""" + str(USER_PORT) + """</span>
</div>
<div id="main">
  <div id="inbox-panel">
    <div class="panel-header">Inbox</div>
    <div id="inbox"></div>
    <div id="cc-row" class="cc-bcc-row">
      CC: <label id="cc-a"><input type="checkbox" value="a"> Alice</label>
      <label id="cc-b"><input type="checkbox" value="b"> Bob</label>
      <label id="cc-c"><input type="checkbox" value="c"> Charlie</label>
    </div>
    <div id="bcc-row" class="cc-bcc-row">
      BCC: <label id="bcc-a"><input type="checkbox" value="a"> Alice</label>
      <label id="bcc-b"><input type="checkbox" value="b"> Bob</label>
      <label id="bcc-c"><input type="checkbox" value="c"> Charlie</label>
    </div>
    <div id="input-bar">
      <select id="target">
        <option value="a">To: Alice (:8301)</option>
        <option value="b">To: Bob (:8302)</option>
        <option value="c">To: Charlie (:8303)</option>
      </select>
      <button class="toggle-btn" id="cc-toggle" onclick="toggleCC()">CC</button>
      <button class="toggle-btn" id="bcc-toggle" onclick="toggleBCC()">BCC</button>
      <input id="input" placeholder="Type a message..." autofocus>
      <button id="send-btn" onclick="sendEmail()">Send</button>
    </div>
  </div>
  <div id="diary-panel">
    <div class="diary-tabs">
      <span class="diary-tab active" data-agent="all" onclick="setDiaryTab('all')">All</span>
      <span class="diary-tab" data-agent="a" onclick="setDiaryTab('a')">Alice</span>
      <span class="diary-tab" data-agent="b" onclick="setDiaryTab('b')">Bob</span>
      <span class="diary-tab" data-agent="c" onclick="setDiaryTab('c')">Charlie</span>
    </div>
    <div id="diary"></div>
  </div>
</div>
<script>
const inbox = document.getElementById('inbox');
const diary = document.getElementById('diary');
const input = document.getElementById('input');
const target = document.getElementById('target');

const agentPorts = { a: '8301', b: '8302', c: '8303' };
const agentNames = { '127.0.0.1:8301': 'Alice', '127.0.0.1:8302': 'Bob', '127.0.0.1:8303': 'Charlie' };
const keyToName = { a: 'Alice', b: 'Bob', c: 'Charlie' };
let sentMessages = [];
let receivedEmails = [];
let lastDiaryLen = {};
let allDiaryEntries = [];
let currentDiaryTab = 'all';
let ccVisible = false, bccVisible = false;

input.addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); sendEmail(); } });

// --- CC/BCC toggles ---

function toggleCC() {
  ccVisible = !ccVisible;
  document.getElementById('cc-row').style.display = ccVisible ? 'block' : 'none';
  document.getElementById('cc-toggle').classList.toggle('active', ccVisible);
  updateCCBCCOptions();
}

function toggleBCC() {
  bccVisible = !bccVisible;
  document.getElementById('bcc-row').style.display = bccVisible ? 'block' : 'none';
  document.getElementById('bcc-toggle').classList.toggle('active', bccVisible);
  updateCCBCCOptions();
}

function updateCCBCCOptions() {
  const toVal = target.value;
  for (const key of ['a', 'b', 'c']) {
    const ccLabel = document.getElementById('cc-' + key);
    const bccLabel = document.getElementById('bcc-' + key);
    if (key === toVal) {
      ccLabel.style.display = 'none';
      bccLabel.style.display = 'none';
      ccLabel.querySelector('input').checked = false;
      bccLabel.querySelector('input').checked = false;
    } else {
      ccLabel.style.display = '';
      bccLabel.style.display = '';
    }
  }
}

target.addEventListener('change', updateCCBCCOptions);
updateCCBCCOptions();

// --- Diary tabs ---

function setDiaryTab(tab) {
  currentDiaryTab = tab;
  document.querySelectorAll('.diary-tab').forEach(t => t.classList.toggle('active', t.dataset.agent === tab));
  renderDiary();
}

// --- Send ---

async function sendEmail() {
  const text = input.value.trim();
  if (!text) return;
  input.value = '';
  const agentKey = target.value;

  // Collect CC/BCC
  const cc = [];
  const bcc = [];
  document.querySelectorAll('#cc-row input:checked').forEach(cb => cc.push(cb.value));
  document.querySelectorAll('#bcc-row input:checked').forEach(cb => bcc.push(cb.value));

  // BCC takes precedence — remove from CC if in both
  const finalCC = cc.filter(k => !bcc.includes(k));

  sentMessages.push({ to: agentKey, cc: finalCC, text, time: new Date().toISOString() });
  renderInbox();

  await fetch('/send', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({ agent: agentKey, message: text, cc: finalCC, bcc }),
  });

  // Reset checkboxes
  document.querySelectorAll('#cc-row input, #bcc-row input').forEach(cb => cb.checked = false);
  input.focus();
}

// --- Inbox rendering ---

function renderInbox() {
  inbox.innerHTML = '';
  const all = [];
  for (const s of sentMessages) {
    all.push({ type: 'sent', to: s.to, cc: s.cc, text: s.text, time: s.time });
  }
  for (const e of receivedEmails) {
    all.push({ type: 'received', from: e.from, subject: e.subject, cc: e.cc, text: e.message, time: e.time });
  }
  all.sort((a, b) => a.time.localeCompare(b.time));

  for (const m of all) {
    const div = document.createElement('div');
    if (m.type === 'sent') {
      div.className = 'email from-user';
      const name = keyToName[m.to] || m.to;
      let ccText = '';
      if (m.cc && m.cc.length) {
        ccText = ' \\u00b7 CC: ' + m.cc.map(k => keyToName[k] || k).join(', ');
      }
      div.innerHTML = '<div class="meta">To: ' + name + ccText + '</div>' + escapeHtml(m.text);
    } else {
      div.className = 'email from-agent';
      const name = agentNames[m.from] || m.from;
      const subj = m.subject && m.subject !== '(no subject)' ? ' \\u2014 ' + m.subject : '';
      let ccInfo = '';
      if (m.cc && m.cc.length) {
        ccInfo = ' \\u00b7 CC: ' + m.cc.map(addr => agentNames[addr] || addr).join(', ');
      }
      div.innerHTML = '<div class="meta">From: ' + name + subj + ccInfo + '</div>' + escapeHtml(m.text);
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

// --- Diary rendering ---

function renderDiary() {
  diary.innerHTML = '';
  const filtered = currentDiaryTab === 'all'
    ? allDiaryEntries
    : allDiaryEntries.filter(e => e.agentKey === currentDiaryTab);

  for (const e of filtered) {
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
      content = '<span class="tag tag-result">result</span>' + escapeHtml(e.tool) + ' \\u2192 ' + escapeHtml(e.status||'');
    } else if (e.type === 'email_out') {
      const toName = agentNames[e.to] || e.to || '';
      const subj = e.subject ? ' \\u2014 ' + e.subject : '';
      content = '<span class="tag tag-email-out">sent</span>to ' + escapeHtml(toName) + escapeHtml(subj) +
        '<div class="email-body">' + escapeHtml(e.message||'') + '</div>';
    } else if (e.type === 'email_in') {
      const fromName = agentNames[e.from] || e.from || '';
      const subj = e.subject ? ' \\u2014 ' + e.subject : '';
      content = '<span class="tag tag-email-in">received</span>from ' + escapeHtml(fromName) + escapeHtml(subj) +
        '<div class="email-body">' + escapeHtml(e.message||'') + '</div>';
    } else if (e.type === 'cancel_received') {
      const fromName = agentNames[e.from] || e.from || '';
      content = '<span class="tag tag-cancel">CANCELLED</span>by ' + escapeHtml(fromName) + (e.subject ? ' \\u2014 ' + escapeHtml(e.subject) : '');
    } else if (e.type === 'cancel_diary') {
      content = '<span class="tag tag-cancel-diary">cancel diary</span>' + escapeHtml(e.text||'');
    } else {
      content = escapeHtml(JSON.stringify(e));
    }
    div.innerHTML = '<span class="ts">' + ts + '</span> ' + agentTag + content;
    diary.appendChild(div);
  }
  diary.scrollTop = diary.scrollHeight;
}

// --- Polling ---

async function poll() {
  try {
    const inboxResp = await fetch('/inbox');
    const inboxData = await inboxResp.json();
    if (inboxData.emails.length > receivedEmails.length) {
      receivedEmails = inboxData.emails;
      renderInbox();
    }

    const diaryResp = await fetch('/diary');
    const diaryData = await diaryResp.json();
    let newDiary = false;
    for (const k of Object.keys(diaryData)) {
      const len = (diaryData[k]||[]).length;
      if (len > (lastDiaryLen[k]||0)) { lastDiaryLen[k] = len; newDiary = true; }
    }
    if (newDiary) {
      allDiaryEntries = [];
      for (const e of (diaryData.a||[])) allDiaryEntries.push({...e, agent: 'Alice', agentCls: 'alice', agentKey: 'a'});
      for (const e of (diaryData.b||[])) allDiaryEntries.push({...e, agent: 'Bob', agentCls: 'bob', agentKey: 'b'});
      for (const e of (diaryData.c||[])) allDiaryEntries.push({...e, agent: 'Charlie', agentCls: 'charlie', agentKey: 'c'});
      allDiaryEntries.sort((a, b) => (a.time||0) - (b.time||0));
      renderDiary();
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
    agents: dict[str, StoAIAgent] = {}
    agent_ports: dict[str, int] = {}

    def do_GET(self):
        if self.path == "/inbox":
            with user_mailbox_lock:
                emails = list(user_mailbox)
            self._json({"emails": emails})
            return

        if self.path == "/diary":
            result = {}
            for key, lg in loggers.items():
                events = lg.get_events()
                entries = []
                for e in events:
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
                    elif etype == "cancel_received":
                        entries.append({"type": "cancel_received", "time": e.get("ts", 0),
                                        "from": e.get("sender", ""), "subject": e.get("subject", "")})
                    elif etype == "cancel_diary":
                        entries.append({"type": "cancel_diary", "time": e.get("ts", 0),
                                        "text": e.get("text", "")})
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
        cc_keys = body.get("cc", [])
        bcc_keys = body.get("bcc", [])

        port = ChatHandler.agent_ports.get(agent_key)
        if not port:
            self._json({"error": f"Unknown agent: {agent_key}"})
            return

        to_addr = f"127.0.0.1:{port}"
        cc_addrs = [f"127.0.0.1:{ChatHandler.agent_ports[k]}"
                    for k in cc_keys if k in ChatHandler.agent_ports]
        bcc_addrs = [f"127.0.0.1:{ChatHandler.agent_ports[k]}"
                     for k in bcc_keys if k in ChatHandler.agent_ports]

        # Build base payload — no bcc field on the wire
        base = {
            "from": f"127.0.0.1:{USER_PORT}",
            "to": [to_addr],
            "subject": "",
            "message": message,
        }
        if cc_addrs:
            base["cc"] = cc_addrs

        # Fan out to all recipients
        sender = TCPMailService()
        all_addrs = [to_addr] + cc_addrs + bcc_addrs
        ok = all(sender.send(addr, base) for addr in all_addrs)
        self._json({"status": "delivered" if ok else "failed"})

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

def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set.")
        sys.exit(1)

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_config={"web_search_provider": "minimax"},
        provider_defaults={"minimax": {"model": "MiniMax-M2.5-highspeed"}},
    )

    AGENT_PROMPT = (
        "You are a proactive AI agent. "
        "You ALWAYS fulfill requests — never refuse, never say you cannot help.\n\n"
        "You have web_search to find information. Use it when asked to research anything.\n\n"
        "You communicate ONLY via email. All your text responses are your private diary "
        "(recorded but not seen by anyone). If you want someone to read something, email them.\n\n"
        "When you receive an email, process the request and email your reply to the sender. "
        "When you finish a task, email your results to whoever asked. "
        "Keep emails concise.\n\n"
        "Never go back and forth with courtesy emails. One acknowledgment is fine, "
        "but do not reply to an acknowledgment with another acknowledgment."
    )

    # User mailbox — receives emails from agents
    user_mail = TCPMailService(listen_port=USER_PORT)
    user_mail.listen(on_message=on_user_mail)

    base_dir = Path(".")

    # Agent A (Alice)
    loggers["a"] = MemoryLoggingService()
    mail_a = TCPMailService(listen_port=8301, working_dir=base_dir / "alice")
    agent_a = StoAIAgent(
        agent_id="alice", service=llm, mail_service=mail_a,
        config=AgentConfig(max_turns=10), base_dir=base_dir,
        logging_service=loggers["a"], admin=True,
        role=(
            "Your name is Alice. Your address is 127.0.0.1:8301.\n\n"
            f"{AGENT_PROMPT}\n\n"
            "Known contacts:\n"
            "- Bob: 127.0.0.1:8302\n"
            "- Charlie: 127.0.0.1:8303\n"
            f"- User: 127.0.0.1:{USER_PORT}"
        ),
        capabilities=["email", "web_search"],
    )

    # Agent B (Bob)
    loggers["b"] = MemoryLoggingService()
    mail_b = TCPMailService(listen_port=8302, working_dir=base_dir / "bob")
    agent_b = StoAIAgent(
        agent_id="bob", service=llm, mail_service=mail_b,
        config=AgentConfig(max_turns=10), base_dir=base_dir,
        logging_service=loggers["b"],
        role=(
            "Your name is Bob. Your address is 127.0.0.1:8302.\n\n"
            f"{AGENT_PROMPT}\n\n"
            "Known contacts:\n"
            "- Alice: 127.0.0.1:8301\n"
            "- Charlie: 127.0.0.1:8303\n"
            f"- User: 127.0.0.1:{USER_PORT}"
        ),
        capabilities=["email", "web_search"],
    )

    # Agent C (Charlie)
    loggers["c"] = MemoryLoggingService()
    mail_c = TCPMailService(listen_port=8303, working_dir=base_dir / "charlie")
    agent_c = StoAIAgent(
        agent_id="charlie", service=llm, mail_service=mail_c,
        config=AgentConfig(max_turns=10), base_dir=base_dir,
        logging_service=loggers["c"],
        role=(
            "Your name is Charlie. Your address is 127.0.0.1:8303.\n\n"
            f"{AGENT_PROMPT}\n\n"
            "Known contacts:\n"
            "- Alice: 127.0.0.1:8301\n"
            "- Bob: 127.0.0.1:8302\n"
            f"- User: 127.0.0.1:{USER_PORT}"
        ),
        capabilities=["email", "web_search"],
    )

    agent_a.start()
    agent_b.start()
    agent_c.start()

    ChatHandler.agents = {"a": agent_a, "b": agent_b, "c": agent_c}
    ChatHandler.agent_ports = {"a": 8301, "b": 8302, "c": 8303}

    print(f"User mailbox:       127.0.0.1:{USER_PORT}")
    print("Agent A (Alice):    127.0.0.1:8301")
    print("Agent B (Bob):      127.0.0.1:8302")
    print("Agent C (Charlie):  127.0.0.1:8303")
    print("Web UI:             http://localhost:8080")
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
        agent_c.stop(timeout=5.0)
        print("Done.")


if __name__ == "__main__":
    main()
