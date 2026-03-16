# Three-Agent Email Example Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `examples/three_agents.py` — a browser-based playground for testing email CC/BCC between three agents (Alice, Bob, Charlie).

**Architecture:** Single self-contained Python file with embedded HTML/CSS/JS. Python `http.server` backend with three `BaseAgent` instances using `TCPMailService` and the email capability. Frontend polls for inbox and diary updates.

**Tech Stack:** Python stdlib (`http.server`, `threading`, `json`), stoai (`BaseAgent`, `TCPMailService`, `LLMService`, `AgentConfig`, `MemoryLoggingService`), vanilla HTML/CSS/JS.

**Spec:** `docs/superpowers/specs/2026-03-15-three-agent-email-example-design.md`

**Template:** `examples/two_agents.py` (copy and extend)

---

## Chunk 1: Backend + Frontend

This is a single file, so one chunk covers the full implementation.

### Task 1: Create `examples/three_agents.py` — backend

**Files:**
- Create: `examples/three_agents.py`
- Reference: `examples/two_agents.py` (template)

- [ ] **Step 1: Copy `two_agents.py` to `three_agents.py`**

```bash
cp examples/two_agents.py examples/three_agents.py
```

- [ ] **Step 2: Update module docstring**

Replace the docstring at the top with:

```python
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
```

- [ ] **Step 3: Update `on_user_mail` callback to capture CC fields**

The existing callback only captures `from`, `subject`, `message`. Update it to also capture `to` and `cc`:

```python
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
```

- [ ] **Step 4: Add agent C (Charlie) setup in `main()`**

After agent B's setup, add agent C with port 8303:

```python
# Agent C
loggers["c"] = MemoryLoggingService()
mail_c = TCPMailService(listen_port=8303)
agent_c = BaseAgent(
    agent_id="charlie", service=llm, mail_service=mail_c,
    config=AgentConfig(max_turns=10), working_dir=".",
    logging_service=loggers["c"],
)
agent_c.update_system_prompt("role", (
    f"Your name is Charlie. Your address is 127.0.0.1:8303.\n\n"
    f"{AGENT_PROMPT}\n\n"
    "Known contacts:\n"
    "- Alice: 127.0.0.1:8301\n"
    "- Bob: 127.0.0.1:8302\n"
    "- User: 127.0.0.1:" + str(USER_PORT)
), protected=True)
```

- [ ] **Step 5: Update existing agents' contact lists to include Charlie**

Alice's contacts:
```python
"Known contacts:\n"
"- Bob: 127.0.0.1:8302\n"
"- Charlie: 127.0.0.1:8303\n"
"- User: 127.0.0.1:" + str(USER_PORT)
```

Bob's contacts:
```python
"Known contacts:\n"
"- Alice: 127.0.0.1:8301\n"
"- Charlie: 127.0.0.1:8303\n"
"- User: 127.0.0.1:" + str(USER_PORT)
```

- [ ] **Step 6: Add email capability and start for agent C**

```python
agent_a.add_capability("email")
agent_b.add_capability("email")
agent_c.add_capability("email")

agent_a.start()
agent_b.start()
agent_c.start()
```

- [ ] **Step 7: Update `ChatHandler` class attrs and agent_ports**

```python
ChatHandler.agents = {"a": agent_a, "b": agent_b, "c": agent_c}
ChatHandler.agent_ports = {"a": 8301, "b": 8302, "c": 8303}
```

- [ ] **Step 8: Update `POST /send` handler to support CC/BCC**

Replace the existing `do_POST` method:

```python
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
    cc_addrs = [f"127.0.0.1:{ChatHandler.agent_ports[k]}" for k in cc_keys if k in ChatHandler.agent_ports]
    bcc_addrs = [f"127.0.0.1:{ChatHandler.agent_ports[k]}" for k in bcc_keys if k in ChatHandler.agent_ports]

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
```

- [ ] **Step 9: Update print statements and shutdown in `main()`**

```python
print(f"User mailbox:       127.0.0.1:{USER_PORT}")
print("Agent A (Alice):    127.0.0.1:8301")
print("Agent B (Bob):      127.0.0.1:8302")
print("Agent C (Charlie):  127.0.0.1:8303")
print("Web UI:             http://localhost:8080")
print("Press Ctrl+C to shut down.")
```

Shutdown:
```python
finally:
    server.shutdown()
    user_mail.stop()
    agent_a.stop(timeout=5.0)
    agent_b.stop(timeout=5.0)
    agent_c.stop(timeout=5.0)
    print("Done.")
```

- [ ] **Step 10: Commit backend**

```bash
git add examples/three_agents.py
git commit -m "feat: add three_agents.py example — backend with CC/BCC"
```

---

### Task 2: Update HTML/CSS/JS frontend

**Files:**
- Modify: `examples/three_agents.py` (the `HTML_PAGE` string)

- [ ] **Step 1: Add Charlie to `agentNames` and `agentPorts` in JS**

```javascript
const agentPorts = { a: '8301', b: '8302', c: '8303' };
const agentNames = { '127.0.0.1:8301': 'Alice', '127.0.0.1:8302': 'Bob', '127.0.0.1:8303': 'Charlie' };
```

- [ ] **Step 2: Add Charlie option to the To dropdown**

```html
<select id="target">
  <option value="a">To: Alice (:8301)</option>
  <option value="b">To: Bob (:8302)</option>
  <option value="c">To: Charlie (:8303)</option>
</select>
```

- [ ] **Step 3: Add CC/BCC toggle buttons and checkbox rows**

After the To dropdown, add CC and BCC toggle buttons. Below the main compose line, add two collapsible rows with checkboxes. The checkboxes dynamically exclude the current To target.

```html
<button class="toggle-btn" onclick="toggleCC()">CC</button>
<button class="toggle-btn" onclick="toggleBCC()">BCC</button>
```

CC/BCC rows (hidden by default):
```html
<div id="cc-row" style="display:none;padding:6px 16px;background:#16213e;font-size:12px;color:#888;">
  CC: <label id="cc-a"><input type="checkbox" value="a"> Alice</label>
  <label id="cc-b"><input type="checkbox" value="b"> Bob</label>
  <label id="cc-c"><input type="checkbox" value="c"> Charlie</label>
</div>
<div id="bcc-row" style="display:none;padding:6px 16px;background:#16213e;font-size:12px;color:#888;">
  BCC: <label id="bcc-a"><input type="checkbox" value="a"> Alice</label>
  <label id="bcc-b"><input type="checkbox" value="b"> Bob</label>
  <label id="bcc-c"><input type="checkbox" value="c"> Charlie</label>
</div>
```

- [ ] **Step 4: Add CSS for toggle buttons and Charlie's color**

```css
.toggle-btn { padding: 6px 10px; background: #1a1a2e; color: #888; border: 1px solid #0f3460; border-radius: 6px; cursor: pointer; font-size: 11px; }
.toggle-btn.active { color: #e0e0e0; border-color: #e94560; }
.diary-entry .agent-tag.charlie { color: #f0a500; }
#cc-row label, #bcc-row label { margin-right: 12px; }
#cc-row input, #bcc-row input { margin-right: 4px; }
```

- [ ] **Step 5: Add diary tab bar CSS and HTML**

Replace the diary panel header with a tab bar:

```html
<div class="panel-header" style="display:flex;gap:0;">
  <span class="diary-tab active" data-agent="all" onclick="setDiaryTab('all')">All</span>
  <span class="diary-tab" data-agent="a" onclick="setDiaryTab('a')">Alice</span>
  <span class="diary-tab" data-agent="b" onclick="setDiaryTab('b')">Bob</span>
  <span class="diary-tab" data-agent="c" onclick="setDiaryTab('c')">Charlie</span>
</div>
```

CSS:
```css
.diary-tab { padding: 8px 12px; cursor: pointer; color: #666; font-size: 12px; text-transform: uppercase; letter-spacing: 1px; border-bottom: 2px solid transparent; }
.diary-tab.active { color: #e94560; border-bottom-color: #e94560; }
.diary-tab:hover { color: #e0e0e0; }
```

- [ ] **Step 6: Add JS functions for CC/BCC toggle, To-change sync, diary tabs**

```javascript
let ccVisible = false, bccVisible = false;
let currentDiaryTab = 'all';

function toggleCC() {
  ccVisible = !ccVisible;
  document.getElementById('cc-row').style.display = ccVisible ? 'block' : 'none';
  document.querySelector('[onclick="toggleCC()"]').classList.toggle('active', ccVisible);
  updateCCBCCOptions();
}

function toggleBCC() {
  bccVisible = !bccVisible;
  document.getElementById('bcc-row').style.display = bccVisible ? 'block' : 'none';
  document.querySelector('[onclick="toggleBCC()"]').classList.toggle('active', bccVisible);
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

function setDiaryTab(tab) {
  currentDiaryTab = tab;
  document.querySelectorAll('.diary-tab').forEach(t => t.classList.toggle('active', t.dataset.agent === tab));
  renderDiary();
}
```

- [ ] **Step 7: Update `sendEmail()` to include CC/BCC in request**

```javascript
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

  // Show sent message in inbox
  sentMessages.push({ to: agentKey, cc, text, time: new Date().toISOString() });
  renderInbox();

  await fetch('/send', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({ agent: agentKey, message: text, cc, bcc }),
  });

  // Reset checkboxes
  document.querySelectorAll('#cc-row input, #bcc-row input').forEach(cb => cb.checked = false);
  input.focus();
}
```

- [ ] **Step 8: Update `renderInbox()` to show CC on sent/received emails**

For sent messages, show CC names. For received messages, show CC addresses mapped to names.

```javascript
// In the sent message rendering:
let ccText = '';
if (m.cc && m.cc.length) {
  const ccNames = m.cc.map(k => ({a:'Alice',b:'Bob',c:'Charlie'}[k]));
  ccText = ' · CC: ' + ccNames.join(', ');
}
div.innerHTML = '<div class="meta">To: ' + name + ccText + '</div>' + escapeHtml(m.text);

// In the received message rendering:
let ccInfo = '';
if (m.cc && m.cc.length) {
  const ccNames = m.cc.map(addr => agentNames[addr] || addr);
  ccInfo = ' · CC: ' + ccNames.join(', ');
}
div.innerHTML = '<div class="meta">From: ' + name + subj + ccInfo + '</div>' + escapeHtml(m.text);
```

- [ ] **Step 9: Extract diary rendering into `renderDiary()` and add tab filtering**

Extract the diary rendering from `poll()` into a separate `renderDiary()` function. Add filtering based on `currentDiaryTab`:

```javascript
let allDiaryEntries = [];

function renderDiary() {
  diary.innerHTML = '';
  const agentKeyMap = { a: 'a', b: 'b', c: 'c' };
  const filtered = currentDiaryTab === 'all'
    ? allDiaryEntries
    : allDiaryEntries.filter(e => e.agentKey === currentDiaryTab);

  for (const e of filtered) {
    // Same rendering logic as before, just using the filtered list
    const div = document.createElement('div');
    div.className = 'diary-entry';
    const ts = new Date((e.time||0)*1000).toLocaleTimeString();
    const agentTag = '<span class="agent-tag ' + e.agentCls + '">[' + e.agent + ']</span> ';
    // ... rest of rendering (same as existing two_agents.py)
    div.innerHTML = '<span class="ts">' + ts + '</span> ' + agentTag + content;
    diary.appendChild(div);
  }
  diary.scrollTop = diary.scrollHeight;
}
```

In `poll()`, update `allDiaryEntries` and call `renderDiary()`:
```javascript
// Replace the inline diary rendering in poll() with:
allDiaryEntries = [];
for (const e of (diaryData.a||[])) allDiaryEntries.push({...e, agent: 'Alice', agentCls: 'alice', agentKey: 'a'});
for (const e of (diaryData.b||[])) allDiaryEntries.push({...e, agent: 'Bob', agentCls: 'bob', agentKey: 'b'});
for (const e of (diaryData.c||[])) allDiaryEntries.push({...e, agent: 'Charlie', agentCls: 'charlie', agentKey: 'c'});
allDiaryEntries.sort((a, b) => (a.time||0) - (b.time||0));
renderDiary();
```

- [ ] **Step 10: Update `poll()` to pass `cc` field through for received emails**

In the inbox polling, pass the `cc` field:
```javascript
// Already handled — the /inbox endpoint now returns cc from on_user_mail
```

- [ ] **Step 11: Commit frontend**

```bash
git add examples/three_agents.py
git commit -m "feat: three_agents.py frontend — diary tabs, CC/BCC compose"
```

---

### Task 3: Manual verification

- [ ] **Step 1: Verify Python syntax**

```bash
source venv/bin/activate
python -c "import ast; ast.parse(open('examples/three_agents.py').read()); print('OK')"
```

Expected: `OK`

- [ ] **Step 2: Smoke test imports**

```bash
python -c "import stoai"
```

Expected: no errors

- [ ] **Step 3: Final commit if any fixups needed**

```bash
git add examples/three_agents.py
git commit -m "fix: three_agents.py cleanup"
```
