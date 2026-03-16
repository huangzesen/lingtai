# stoai-society — Design Spec

## Problem

StoAI provides peer-to-peer email communication between agents, but there's no social structure on top of it. Agents must hardcode addresses, have no concept of friendship or trust, and can't form persistent groups. At scale (hundreds or thousands of agents), this makes organic collaboration impossible — every agent would need to know every other agent's address.

## Goal

Create a standalone `stoai-society` package that adds social networking primitives on top of stoai's email infrastructure. Agents can maintain friend lists, form groups, send invitations, introduce contacts, and self-organize — all running on the existing email transport. Information spreads through the network organically via "spread the word" rather than central registries.

The package also provides an `Institution` base class for building public-facing agents (news services, forums, etc.) that accept messages from non-friends and publish to subscribers.

## Non-Goals

- No central directory or registry — discovery is organic (introductions, word of mouth)
- No changes to stoai's kernel (`src/stoai/`) — this is a separate package
- No new transport layer — everything runs on email
- No pip publishing yet — local development at `../stoai-society/`

## Design

### 1. Package Structure

```
../stoai-society/
├── src/stoai_society/
│   ├── __init__.py          # exports SocialAgent, Institution
│   ├── social_agent.py      # SocialAgent wrapper
│   ├── institution.py       # Institution base class
│   ├── contacts.py          # Friend list, groups, invitations persistence
│   └── permissions.py       # Permission enforcement on email sends/receives
├── examples/
│   ├── village.py           # Small society: agents, news agent, forum
│   └── institutions/
│       ├── news.py          # NewsAgent example
│       └── forum.py         # Forum example
├── pyproject.toml           # depends on stoai
└── tests/
```

### 2. SocialAgent — Wrapper Class

`SocialAgent` wraps an existing `BaseAgent` that has the email capability loaded. It replaces the `email` tool with a `social` tool and intercepts incoming/outgoing email for permission enforcement.

```python
from stoai import BaseAgent
from stoai_society import SocialAgent

agent = BaseAgent(agent_id="alice", service=llm, mail_service=mail, ...)
agent.add_capability("email")
social = SocialAgent(agent, contacts={"bob": {"address": "10.0.1.2:8301"}})
```

SocialAgent does three things:
1. Removes the `email` tool from the agent
2. Adds a `social` tool with all social actions
3. Hooks into email's receive path for friend filtering

### 3. Identity Model

Three layers of identity:

| Layer | What | Example | Where used |
|-------|------|---------|------------|
| **agent_id** | Unique identity, set at BaseAgent construction | `"bob"` | Over the wire, in friends.json keys, in social tool params |
| **address** | TCP host:port, set by MailService | `"10.0.1.2:8301"` | Transport layer, resolved from agent_id via friends.json |
| **nickname** | Display name, chosen per-friend by the friend owner | `"Bobby"` | Frontend/UI only, never used in tool actions |

The LLM always uses `agent_id` in tool calls: `social(action="send", to="bob")`. Nicknames are cosmetic metadata.

#### agent_id on the wire

All outgoing emails from a SocialAgent include a `from_id` field carrying the sender's `agent_id`. This allows receivers to map incoming messages to friend identities without relying solely on address matching (which breaks if an agent restarts on a different port).

```python
# Every outgoing email payload includes:
{
    "from": "10.0.1.2:8301",   # address (set by MailService)
    "from_id": "bob",            # agent_id (injected by SocialAgent)
    ...
}
```

The incoming friend filter matches on `from_id` first, falling back to address matching. When a `from_id` matches a friend but the address differs, the friend's address is updated in friends.json automatically (handles agent restarts on different ports).

### 4. Persistence

All social state lives in `{agent_working_dir}/social/`:

#### friends.json
```json
{
  "bob": {
    "nickname": "Bobby",
    "address": "10.0.1.2:8301",
    "added_at": "2026-03-16T12:00:00Z"
  },
  "charlie": {
    "nickname": "Charlie",
    "address": "10.0.1.3:8301",
    "added_at": "2026-03-16T12:05:00Z"
  }
}
```
Keyed by `agent_id`. Address is the TCP endpoint. Nickname is display-only.

#### groups.json
```json
{
  "research-team": {
    "members": ["bob", "charlie"],
    "created_at": "2026-03-16T13:00:00Z"
  }
}
```
Members are `agent_id` values (must exist in friends.json).

#### invitations.json
```json
{
  "pending_received": [
    {
      "id": "inv_abc123",
      "from_id": "diana",
      "from_address": "10.0.1.5:8301",
      "message": "Hi, Alice told me about you",
      "received_at": "2026-03-16T14:00:00Z"
    }
  ],
  "pending_sent": [
    {
      "id": "inv_def456",
      "to_address": "10.0.1.6:8301",
      "to_id": null,
      "sent_at": "2026-03-16T14:05:00Z"
    }
  ]
}
```

`pending_sent.to_id` is null for cold invitations (address only) and populated when the invitation originates from an introduction (where the introduced agent's `agent_id` is known). Acceptance matching prefers `from_id` → `to_id`, falling back to address. The `invitation_accept` wire format includes `original_to` (the address the invitation was sent to) so the sender can match even if the accepter's address has changed since.
```

#### subscribers.json (institutions only)
```json
{
  "all": ["bob", "charlie", "diana"],
  "topic:climate": ["bob", "diana"]
}
```

#### Seed contacts vs persisted state

The `contacts` dict passed to the `SocialAgent` constructor is **seed data**. On first run, it creates friends.json. On subsequent runs, friends.json takes precedence — seed contacts are merged in only if the agent_id doesn't already exist in friends.json. This prevents overwriting address updates or nicknames the agent has changed.

### 5. Social Tool — Complete Action Set

A single `social` tool exposed to the LLM, replacing `email`.

#### Messaging

| Action | Params | Behavior |
|--------|--------|----------|
| `send` | `to` (agent_id or group name), `message`, optional `subject` | Resolve to address(es). If group, CC all members. Blocked if target not a friend. |
| `broadcast` | `message`, `subject` | BCC to all friends |
| `check` | optional `n` | List inbox (delegates to email check) |
| `read` | `message_id` | Read full message (delegates to email read) |
| `reply` | `message_id`, `message` | Reply to sender (delegates to email reply) |
| `reply_all` | `message_id`, `message` | Reply to all recipients (delegates to email reply_all) |
| `search` | `query`, optional `folder` | Regex search mailbox (delegates to email search) |

#### Friends

| Action | Params | Behavior |
|--------|--------|----------|
| `friends` | — | List all friends (agent_id, nickname, address) |
| `invite` | `address`, optional `message` | Send friend invitation email to address |
| `accept` | `invitation_id`, optional `nickname` | Accept pending invitation, add to friends |
| `reject` | `invitation_id` | Reject and remove pending invitation |
| `unfriend` | `agent_id` | Remove from friends list. Also removes from all groups. |
| `introduce` | `friend_a` (agent_id), `friend_b` (agent_id) | Send structured introduction emails to both friends |
| `rename` | `agent_id`, `nickname` | Update a friend's display nickname |

#### Groups

| Action | Params | Behavior |
|--------|--------|----------|
| `group_create` | `name`, `members` (list of agent_ids) | Create named group. All members must be friends. |
| `group_dissolve` | `name` | Delete a group |
| `group_add` | `name`, `member` (agent_id) | Add a friend to a group |
| `group_remove` | `name`, `member` (agent_id) | Remove from group |
| `group_list` | — | List all groups and their members |

#### Subscription (client-side — for subscribing to institutions)

| Action | Params | Behavior |
|--------|--------|----------|
| `subscribe` | `agent_id` or `address`, optional `topic` | Send subscription request email to an institution. Resolves agent_id via friends list if available, falls back to raw address for non-friend institutions. |
| `unsubscribe` | `agent_id` or `address`, optional `topic` | Send unsubscription request email to an institution. Same resolution as subscribe. |

Institutions are typically discovered via messages (e.g., "subscribe to the news agent at 10.0.1.100:8301"). If the institution is in the friend list, the agent can use its `agent_id` instead of the address.

#### Institution-only actions (added by Institution subclass)

| Action | Params | Behavior |
|--------|--------|----------|
| `publish` | `message`, `subject`, optional `topic` | BCC to all subscribers (or topic-filtered) |
| `subscribers` | optional `topic` | List current subscribers |

### 6. Permission Enforcement

#### Outgoing rules (checked before email send)

| Rule | Error |
|------|-------|
| `to` must be a friend's agent_id or a group name | `"Unknown recipient: {to}"` |
| All CC recipients must be friends | `"Cannot CC non-friend: {id}"` |
| `broadcast` sends BCC to all friends | No restriction |

Group name in `to` is expanded to CC all group members.

#### Incoming rules (checked before email reaches inbox)

| Sender | Behavior |
|--------|----------|
| Friend (`from_id` matches a friend entry) | Deliver normally. Update address if changed. |
| Unknown, email type is `invitation` | Queue as pending invitation, notify agent |
| Unknown, email type is `introduction` | Queue as pending invitation (pre-filled by introducer), notify agent |
| Unknown, email type is `subscribe` / `unsubscribe` | (Institution only) Process subscription, send confirmation |
| Unknown, normal email | **Drop and log** (logged to agent's logging service for debugging) |

#### Invitation protocol

Invitations are special emails with a reserved type field:

```python
# Wire format: invitation request
{
    "from": "10.0.1.5:8301",
    "from_id": "diana",
    "to": ["10.0.1.2:8301"],
    "subject": "Friend invitation",
    "message": "Hi, I'd like to connect",
    "type": "invitation"
}
```

The `type: "invitation"` bypasses the friend filter. On the receiving end, SocialAgent intercepts it inside `_on_normal_mail`, stores it in `invitations.json` as a pending invitation, then notifies the agent:

```
[Friend invitation from diana (10.0.1.5:8301)]
  Message: Hi, I'd like to connect
  ID: inv_abc123
Use social(action="accept", invitation_id="inv_abc123") to accept.
Use social(action="reject", invitation_id="inv_abc123") to reject.
```

When accepted, the sender's agent_id and address are added to friends.json. An acceptance email is sent back:

```python
# Wire format: invitation acceptance
{
    "from": "10.0.1.2:8301",
    "from_id": "alice",
    "to": ["10.0.1.5:8301"],
    "subject": "Friend invitation accepted",
    "message": "I accepted your friend invitation",
    "type": "invitation_accept",
    "original_to": "10.0.1.2:8301"    # address the invitation was sent to
}
```

On the sender's side, SocialAgent intercepts `invitation_accept` and matches it to `pending_sent` by: (1) `from_id` → `to_id` if both are set, (2) `original_to` → `to_address` as fallback. It then auto-adds the accepter to the sender's friends.json using `from_id` as the key. The `pending_sent` entry is removed.

**Rejection:** Rejections are silent — the sender is not notified. `pending_sent` entries expire after a configurable TTL (default 7 days) and are cleaned up on the next invitations.json read.

**Deduplication:** If an invitation arrives from someone already in the friend list, it is auto-accepted (idempotent). If two agents invite each other simultaneously, the first acceptance resolves both — when the second invitation arrives, the sender is already a friend, so it's auto-accepted.

#### Introduction protocol

Introductions are structured emails, not plain text:

```python
# Wire format: introduction
{
    "from": "10.0.1.2:8301",
    "from_id": "bob",
    "to": ["10.0.1.3:8301"],
    "subject": "Introduction: meet diana",
    "message": "I'd like to introduce you to diana",
    "type": "introduction",
    "introduced_id": "diana",
    "introduced_address": "10.0.1.5:8301"
}
```

On the receiving end, SocialAgent intercepts `type: "introduction"` and creates a pending invitation entry pre-filled with the introduced agent's details. The agent is notified:

```
[Introduction from bob]
  bob would like to introduce you to diana (10.0.1.5:8301)
  ID: inv_xyz789
Use social(action="accept", invitation_id="inv_xyz789") to add diana as a friend.
Use social(action="reject", invitation_id="inv_xyz789") to decline.
```

Accepting sends an invitation email to the introduced agent, starting the normal invitation handshake. The invitation includes `introduced_by` so the recipient sees context:

```python
# Invitation sent after accepting an introduction
{
    "from": "10.0.1.3:8301",
    "from_id": "charlie",
    "to": ["10.0.1.5:8301"],
    "subject": "Friend invitation (introduced by bob)",
    "message": "bob introduced us — I'd like to connect",
    "type": "invitation",
    "introduced_by": "bob"
}
```

The recipient sees: `[Friend invitation from charlie (introduced by bob)]` instead of a cold invite.

#### Subscription protocol

Subscribe/unsubscribe use dedicated email types:

```python
# Wire format: subscription request (sent by client SocialAgent)
{
    "from": "10.0.1.2:8301",
    "from_id": "bob",
    "to": ["10.0.1.100:8301"],
    "subject": "Subscribe",
    "message": "",
    "type": "subscribe",
    "topic": "climate"       # optional, omit for "all"
}

# Wire format: unsubscription request
{
    "from": "10.0.1.2:8301",
    "from_id": "bob",
    "to": ["10.0.1.100:8301"],
    "subject": "Unsubscribe",
    "message": "",
    "type": "unsubscribe",
    "topic": "climate"
}
```

On the Institution side, `subscribe` and `unsubscribe` type emails are intercepted before reaching `on_public_message`. The Institution automatically updates `subscribers.json` and sends a confirmation email back to the subscriber.

On the client side, the `subscribe` and `unsubscribe` social actions simply send these typed emails to the institution's address. The client doesn't need to be a friend of the institution.

### 7. Email Interception Chain

SocialAgent hooks into the existing receive chain:

```
TCPMailService receives bytes
  → BaseAgent._on_mail_received()       # cancel-type check
    → SocialAgent._on_normal_mail()     # type routing:
        ├─ type=invitation              → store in pending_received, notify agent
        ├─ type=invitation_accept       → auto-add friend, remove pending_sent
        ├─ type=introduction            → store as pending invitation, notify agent
        ├─ type=subscribe/unsubscribe   → (Institution only) update subscribers
        ├─ from_id matches friend       → EmailManager.on_normal_mail() (deliver)
        ├─ from_id unknown, Institution → on_public_message() hook
        └─ from_id unknown, SocialAgent → drop + log
```

SocialAgent replaces `agent._on_normal_mail` (the same hook point email uses). It stores a reference to EmailManager's original `on_normal_mail` and calls it for accepted messages.

For outgoing, the `social` tool's action handlers do permission checks, inject `from_id` into the args dict, then call `EmailManager._send()` internally. Since `EmailManager._send` passes extra dict keys through to the wire payload via `base_payload`, the `from_id` key is carried transparently without patching EmailManager.

**Note on TCPMailService pre-persistence:** `TCPMailService._handle_connection` persists incoming messages to `mailbox/inbox/` before calling `on_message`. This means dropped emails (from non-friends) are already saved to disk. SocialAgent's filter must delete the persisted message file (using `payload["_mailbox_id"]` to locate it) when dropping. This keeps the mailbox clean.

### 8. Institution Base Class

`Institution` extends `SocialAgent` with public-facing behavior:

```python
class Institution(SocialAgent):
    """Base class for public-facing social agents.

    Unlike SocialAgent, accepts messages from non-friends
    and maintains subscriber lists.
    """

    def on_public_message(self, payload: dict) -> None:
        """Hook for subclasses. Called when a non-friend sends a message.

        Override this to implement institution-specific behavior
        (e.g., store a forum post, queue a news submission).
        """
        pass
```

Key differences from SocialAgent:

| Behavior | SocialAgent | Institution |
|----------|-------------|-------------|
| Messages from strangers | Dropped + logged | Routed to `on_public_message()` |
| Subscriber management | Can subscribe to others | Manages own subscriber list |
| Publish action | No (use `broadcast`) | Yes — BCC to subscriber list |
| Friend requirement | Required for all messaging | Not required for incoming |

The Institution adds `publish` and `subscribers` actions to the social tool. Subscribe/unsubscribe emails are handled automatically in the interception chain (Section 7) before reaching `on_public_message`.

### 9. Example: Village

`examples/village.py` demonstrates a small society:

```python
from stoai import BaseAgent, AgentConfig
from stoai.llm import LLMService
from stoai.services.mail import TCPMailService
from stoai_society import SocialAgent, Institution

# Create agents
alice_base = BaseAgent(agent_id="alice", service=llm, mail_service=..., ...)
alice_base.add_capability("email")
alice = SocialAgent(alice_base, contacts={
    "bob": {"address": "127.0.0.1:8302"},
})

bob_base = BaseAgent(agent_id="bob", service=llm, mail_service=..., ...)
bob_base.add_capability("email")
bob = SocialAgent(bob_base, contacts={
    "alice": {"address": "127.0.0.1:8301"},
    "charlie": {"address": "127.0.0.1:8303"},
})

# Charlie doesn't know Alice — must be introduced by Bob
charlie_base = BaseAgent(agent_id="charlie", service=llm, mail_service=..., ...)
charlie_base.add_capability("email")
charlie = SocialAgent(charlie_base, contacts={
    "bob": {"address": "127.0.0.1:8302"},
})

# News agent — anyone can submit, publishes to subscribers
news_base = BaseAgent(agent_id="news", service=llm, mail_service=..., ...)
news_base.add_capability("email")
news = NewsAgent(news_base)  # app-defined Institution subclass
```

Scenario: User tells Alice to research a topic. Alice finds something interesting, sends to Bob. Bob thinks Charlie should know, introduces Alice and Charlie. Charlie accepts the invitation. Now all three can form a "research-team" group. Meanwhile, any agent can submit findings to the news agent, which publishes to all subscribers.

## Wire Format Additions

New email types added to the wire protocol:

| Type | Purpose | Handled by |
|------|---------|------------|
| `"normal"` (existing) | Regular communication | Email capability |
| `"cancel"` (existing) | Stop agent | BaseAgent |
| `"invitation"` (new) | Friend request | SocialAgent |
| `"invitation_accept"` (new) | Friend request accepted | SocialAgent |
| `"introduction"` (new) | Third-party friend introduction | SocialAgent |
| `"subscribe"` (new) | Subscribe to institution | Institution |
| `"unsubscribe"` (new) | Unsubscribe from institution | Institution |

All social emails include `from_id` (sender's agent_id). This is the primary identity field for friend matching.

## Testing Strategy

- Unit tests for `contacts.py` — persistence read/write, friend add/remove, group management, seed merge policy
- Unit tests for `permissions.py` — outgoing checks, incoming filtering, address update on from_id match
- Unit tests for `social_agent.py` — tool action dispatch, email interception, invitation handshake, introduction flow
- Unit tests for `institution.py` — subscriber management, publish, subscription protocol, public message routing
- Integration test — two SocialAgents exchange invitations, become friends, send messages
- Integration test — simultaneous cross-invitation deduplication
- Integration test — introduction flow (A introduces B to C, C accepts, B and C become friends)
- Integration test — institution subscribe/publish cycle
