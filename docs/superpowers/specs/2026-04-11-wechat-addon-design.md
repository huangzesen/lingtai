# WeChat Addon (`lingtai.addons.wechat`) Design

## Summary

A LingTai addon that connects agents to WeChat via Tencent's official iLink Bot API — the same HTTP protocol used by `@tencent-weixin/openclaw-weixin`. Reimplements the 5 iLink endpoints directly in Python, with no OpenClaw or Node.js dependency.

**Goal:** Let a LingTai agent send and receive WeChat messages (text, images, voice, video, files) as a first-class communication channel alongside IMAP, Telegram, and Feishu.

## Protocol

The iLink Bot API is an HTTP JSON protocol at `https://ilinkai.weixin.qq.com`. Authentication is via bearer token obtained through QR code login. Five endpoints:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `ilink/bot/get_bot_qrcode?bot_type=3` | GET | Generate QR code for login |
| `ilink/bot/get_qrcode_status?qrcode=<qr>` | GET | Poll login status → returns `bot_token` on success |
| `ilink/bot/getupdates` | POST | Long-poll for incoming messages (35s server timeout) |
| `ilink/bot/sendmessage` | POST | Send text/media messages |
| `ilink/bot/getconfig` | POST | Get bot config (typing ticket) |

All authenticated requests carry:
- `Authorization: Bearer <bot_token>`
- `AuthorizationType: ilink_bot_token`
- `Content-Type: application/json`

## Config Schema

### `config.json` — Human-editable settings

**Path:** `.lingtai/.addons/wechat/config.json` (project-level, shared by all agents)

```json
{
  "base_url": "https://ilinkai.weixin.qq.com",
  "cdn_base_url": "https://novac2c.cdn.weixin.qq.com/c2c",
  "poll_interval": 1.0,
  "allowed_users": []
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `base_url` | string | `https://ilinkai.weixin.qq.com` | iLink API base URL |
| `cdn_base_url` | string | `https://novac2c.cdn.weixin.qq.com/c2c` | CDN base URL for media |
| `poll_interval` | float | `1.0` | Seconds between long-poll retries |
| `allowed_users` | string[] | `[]` | WeChat user IDs to accept messages from. Empty = accept all. |

### `credentials.json` — Machine-managed, written by QR login

**Path:** `.lingtai/.addons/wechat/credentials.json` (mode 0600)

```json
{
  "bot_token": "...",
  "user_id": "b0f5860fdecb@im.bot",
  "base_url": "https://ilinkai.weixin.qq.com",
  "saved_at": "2026-04-11T12:00:00Z"
}
```

No `*_env` fields — unlike Feishu/Telegram, there are no static secrets. The `bot_token` is obtained through QR code login and managed entirely by the addon.

## QR Login Flow

Login is handled via the setup skill. The agent runs a CLI entry point:

```bash
python -c "from lingtai.addons.wechat.login import cli_login; cli_login('.lingtai/.addons/wechat')"
```

### Sequence

1. `cli_login` creates `config.json` with defaults if it doesn't exist.
2. Calls `GET ilink/bot/get_bot_qrcode?bot_type=3` → receives QR code data.
3. Renders QR code in terminal using the `qrcode` Python package (text mode).
4. Prints "Scan this QR code with WeChat on your phone."
5. Polls `GET ilink/bot/get_qrcode_status?qrcode=<qr>` with 35s long-poll timeout.
6. Status transitions: `wait` → `scaned` → `confirmed` / `expired`.
7. On `confirmed`: saves `bot_token`, `user_id`, `base_url` to `credentials.json`. Prints "Connected as \<user_id\>."
8. On `expired`: prints error, exits non-zero.
9. On `scaned_but_redirect`: updates `base_url` to redirect host, continues polling.

### Re-login on Session Expiry

When the addon's long-poll loop receives `errcode: -14` (session timeout):

1. Pauses the long-poll loop.
2. Sends an internal mail to the human: "WeChat session expired. Please ask me to re-login to WeChat."
3. When the human requests re-login, the agent re-invokes the setup skill, which runs `cli_login` again.
4. After successful login, human runs `/refresh` to reload the addon with new credentials.

## Python Module Structure

**Package:** `lingtai.addons.wechat` (in `lingtai-kernel` repo at `src/lingtai/addons/wechat/`)

```
wechat/
  __init__.py       — exports WechatAddon class (entry point for kernel addon loader)
  api.py            — HTTP wrappers for the 5 iLink endpoints
  types.py          — dataclasses mirroring iLink protocol types
  login.py          — QR login flow + cli_login() entry point
  media.py          — download/upload helpers for images, voice, video, files
  bridge.py         — translates iLink messages ↔ LingTai internal mail
```

### Module Responsibilities

**`__init__.py` (WechatAddon)**
- Entry point for the kernel's addon loader.
- Reads `config.json` + `credentials.json` on init.
- Starts `getUpdates` long-poll loop as an async task on start.
- Handles graceful shutdown (cancel poll task).

**`api.py`**
- Thin HTTP wrappers using `aiohttp` or `httpx` (whichever the kernel uses).
- `get_qrcode(base_url)` → QR data
- `poll_qr_status(base_url, qrcode)` → login status
- `get_updates(base_url, token, buf)` → incoming messages
- `send_message(base_url, token, msg)` → send outgoing
- `get_config(base_url, token)` → typing ticket
- All functions handle HTTP errors, timeouts, and retries.

**`types.py`**
- Python dataclasses mirroring the iLink types: `WeixinMessage`, `MessageItem`, `TextItem`, `ImageItem`, `VoiceItem`, `FileItem`, `VideoItem`, `CDNMedia`, `GetUpdatesReq`, `GetUpdatesResp`, `SendMessageReq`.
- Enum-like constants: `MessageItemType` (TEXT=1, IMAGE=2, VOICE=3, FILE=4, VIDEO=5), `UploadMediaType`, `MessageState`.

**`login.py`**
- `cli_login(addon_dir: str)` — synchronous CLI entry point. Handles the full QR flow, writes credentials.json.
- Uses `qrcode` package for terminal QR rendering.

**`media.py`**
- `download_media(cdn_media: CDNMedia, cdn_base_url: str, dest_dir: str) → str` — downloads and returns local path.
- `decode_voice(silk_path: str, out_path: str)` — decodes Silk audio to WAV using `pilk`.
- `upload_media(file_path: str, base_url: str, token: str, to_user_id: str) → CDNMedia` — gets upload URL, uploads to CDN, returns media reference.
- File type detection by extension for correct `UploadMediaType`.

**`bridge.py`**
- `incoming_to_mail(msg: WeixinMessage, media_dir: str) → Mail` — converts iLink message to LingTai internal mail.
- `mail_to_outgoing(mail: Mail) → list[WeixinMessage]` — converts agent's outgoing mail to iLink messages. Handles text chunking (4000 char limit) and file path detection for media upload.
- `context_tokens: dict[str, str]` — caches per-user context tokens for conversation threading.

### Addon Lifecycle

1. **Init** — kernel reads `config.json` + `credentials.json`, instantiates `WechatAddon`.
2. **Start** — begins `getUpdates` long-poll loop as async background task.
3. **Poll loop** — calls `getUpdates` with cached `get_updates_buf`. On response, processes each message through `bridge.incoming_to_mail()`, deposits in agent inbox. Updates `get_updates_buf` cursor.
4. **Outgoing** — agent sends mail to `wechat:<user_id>`. Bridge converts and calls `send_message`.
5. **Error -14** — session expired. Pauses poll, sends notification mail.
6. **Stop** — cancels poll task on agent shutdown.

## Message Bridge

### Addressing

`wechat:<wechat_user_id>` — e.g., `wechat:wxid_abc123@im.wechat`

### Incoming (WeChat → Agent)

Each `WeixinMessage` from `getUpdates` maps to an internal mail:

| iLink field | Mail field |
|-------------|-----------|
| `from_user_id` | `from: wechat:<id>` |
| agent address | `to: <agent_address>` |
| `create_time_ms` | timestamp |
| `item_list` | body (see below) |

**Body construction from `item_list`:**

| Item type | Body format |
|-----------|-------------|
| `TEXT` | Plain text content |
| `IMAGE` | `[Image: /path/to/img.jpg]` (downloaded via CDN) |
| `VOICE` | `[Voice: "transcribed text" (audio: /path/to/voice.wav)]` (Silk decoded + WeChat transcription from `VoiceItem.text`) |
| `FILE` | `[File: filename.pdf (/path/to/file)]` (downloaded) |
| `VIDEO` | `[Video: /path/to/video.mp4]` (downloaded) |

Multiple items in one message are concatenated with newlines.

### Outgoing (Agent → WeChat)

Agent sends mail to `wechat:<user_id>`. The bridge:

1. Scans body for file paths (absolute paths pointing to existing files).
2. Plain text → `sendMessage` with `TEXT` item. Chunked at 4000 chars if needed.
3. Detected file paths → uploaded via `getUploadUrl` + CDN PUT, sent as media with type determined by extension (`.jpg/.png` → IMAGE, `.mp4` → VIDEO, `.wav/.mp3` → VOICE, else → FILE).
4. Attaches cached `context_token` for the target user to maintain conversation threading.

## TUI Side (This Repo)

### Files to Create/Modify

**Modify:**
- `tui/internal/tui/presets.go` — add `"wechat"` to `AllAddons`
- `tui/i18n/en.json`, `zh.json`, `wen.json` — add `addon.wechat` key

**Create:**
- `tui/internal/preset/skills/lingtai-wechat-setup/SKILL.md` — setup skill
- `tui/internal/preset/skills/lingtai-wechat-setup/assets/config.json` — example config
- `tui/internal/preset/templates/wechat.jsonc` — config template with comments

### No Changes Needed

These files work generically off `AllAddons` and addon name conventions:
- `tui/internal/tui/addon.go` — auto-discovers any addon in `AllAddons`
- `tui/internal/preset/preset.go` — auto-wires addon config paths
- `tui/internal/config/venv.go` — auto-checks `import lingtai.addons.<name>`
- `tui/internal/tui/firstrun.go` — auto-lists addons from `AllAddons`

## Dependencies (Kernel Side)

| Package | Purpose | Notes |
|---------|---------|-------|
| `qrcode` | Terminal QR rendering for login | Pure Python, no C deps |
| `pilk` | Silk audio → WAV decoding | Wraps Silk codec via ctypes |
| `httpx` or `aiohttp` | HTTP client | Use whichever the kernel already uses |

## Scope Boundary

This spec covers the WeChat addon only. Out of scope:
- `/addon` page redesign (mentioned as future work)
- Multi-account support (single account per project, can be added later)
- Typing indicators (`sendTyping` endpoint — cosmetic, defer)
- Group chat support (1:1 messaging only in v1)
