# WeChat Addon (Kernel Side) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `lingtai.addons.wechat` — a Python addon that connects LingTai agents to WeChat via the iLink Bot API, with full media support (text, images, voice, video, files).

**Architecture:** Follows the existing addon pattern (feishu/telegram/imap): a `setup()` entry point creates a Manager that registers a `wechat` tool with the agent. The manager handles incoming messages via long-polling and outgoing messages via HTTP API calls. No external SDK — we implement the 5 iLink HTTP endpoints directly.

**Tech Stack:** Python 3.10+, httpx (async HTTP), qrcode (terminal QR rendering), pilk (Silk audio decoding)

**Spec:** `docs/superpowers/specs/2026-04-11-wechat-addon-design.md`

**Working directory:** `/Users/huangzesen/Documents/GitHub/lingtai-kernel`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai/addons/__init__.py` | Modify | Add `"wechat": ".wechat"` to `_BUILTIN` |
| `src/lingtai/addons/wechat/__init__.py` | Create | `setup()` entry point, config loading |
| `src/lingtai/addons/wechat/types.py` | Create | Dataclasses for iLink protocol types |
| `src/lingtai/addons/wechat/api.py` | Create | HTTP wrappers for 5 iLink endpoints |
| `src/lingtai/addons/wechat/login.py` | Create | QR login flow + `cli_login()` CLI entry point |
| `src/lingtai/addons/wechat/media.py` | Create | Media download/upload + Silk voice decoding |
| `src/lingtai/addons/wechat/manager.py` | Create | Tool schema, handle() dispatch, message persistence, incoming/outgoing bridge |
| `tests/test_wechat_types.py` | Create | Tests for protocol types |
| `tests/test_wechat_api.py` | Create | Tests for API wrappers (mocked HTTP) |
| `tests/test_wechat_manager.py` | Create | Tests for manager dispatch and message persistence |

---

### Task 1: Protocol Types

**Files:**
- Create: `src/lingtai/addons/wechat/__init__.py` (empty init for now)
- Create: `src/lingtai/addons/wechat/types.py`
- Test: `tests/test_wechat_types.py`

- [ ] **Step 1: Create package directory and empty init**

```bash
mkdir -p src/lingtai/addons/wechat
```

Create `src/lingtai/addons/wechat/__init__.py`:

```python
"""WeChat addon — iLink Bot API integration."""
```

- [ ] **Step 2: Write the types module**

Create `src/lingtai/addons/wechat/types.py`:

```python
"""iLink Bot protocol types — mirrors openclaw-weixin's type definitions."""
from __future__ import annotations

from dataclasses import dataclass, field
from enum import IntEnum


class MessageItemType(IntEnum):
    NONE = 0
    TEXT = 1
    IMAGE = 2
    VOICE = 3
    FILE = 4
    VIDEO = 5


class UploadMediaType(IntEnum):
    IMAGE = 1
    VIDEO = 2
    FILE = 3
    VOICE = 4


class MessageState(IntEnum):
    NEW = 0
    GENERATING = 1
    FINISH = 2


@dataclass
class CDNMedia:
    encrypt_query_param: str | None = None
    aes_key: str | None = None
    encrypt_type: int | None = None
    full_url: str | None = None


@dataclass
class TextItem:
    text: str | None = None


@dataclass
class ImageItem:
    media: CDNMedia | None = None
    thumb_media: CDNMedia | None = None
    aeskey: str | None = None
    url: str | None = None


@dataclass
class VoiceItem:
    media: CDNMedia | None = None
    encode_type: int | None = None
    playtime: int | None = None
    text: str | None = None  # server-side transcription


@dataclass
class FileItem:
    media: CDNMedia | None = None
    file_name: str | None = None
    md5: str | None = None
    len: str | None = None


@dataclass
class VideoItem:
    media: CDNMedia | None = None
    video_size: int | None = None
    play_length: int | None = None
    thumb_media: CDNMedia | None = None


@dataclass
class MessageItem:
    type: int | None = None
    create_time_ms: int | None = None
    update_time_ms: int | None = None
    is_completed: bool | None = None
    msg_id: str | None = None
    text_item: TextItem | None = None
    image_item: ImageItem | None = None
    voice_item: VoiceItem | None = None
    file_item: FileItem | None = None
    video_item: VideoItem | None = None


@dataclass
class WeixinMessage:
    seq: int | None = None
    message_id: int | None = None
    from_user_id: str | None = None
    to_user_id: str | None = None
    client_id: str | None = None
    create_time_ms: int | None = None
    update_time_ms: int | None = None
    session_id: str | None = None
    group_id: str | None = None
    message_type: int | None = None
    message_state: int | None = None
    item_list: list[MessageItem] = field(default_factory=list)
    context_token: str | None = None


@dataclass
class BaseInfo:
    channel_version: str | None = None


@dataclass
class GetUpdatesReq:
    get_updates_buf: str = ""
    base_info: BaseInfo | None = None


@dataclass
class GetUpdatesResp:
    ret: int | None = None
    errcode: int | None = None
    errmsg: str | None = None
    msgs: list[WeixinMessage] = field(default_factory=list)
    get_updates_buf: str | None = None
    longpolling_timeout_ms: int | None = None


@dataclass
class SendMessageReq:
    msg: WeixinMessage | None = None


@dataclass
class GetUploadUrlReq:
    filekey: str | None = None
    media_type: int | None = None
    to_user_id: str | None = None
    rawsize: int | None = None
    rawfilemd5: str | None = None
    filesize: int | None = None
    aeskey: str | None = None


@dataclass
class GetUploadUrlResp:
    upload_param: str | None = None
    upload_full_url: str | None = None


@dataclass
class GetConfigResp:
    ret: int | None = None
    errmsg: str | None = None
    typing_ticket: str | None = None


def msg_from_dict(d: dict) -> WeixinMessage:
    """Parse a WeixinMessage from a JSON dict (getUpdates response)."""
    items = []
    for raw_item in d.get("item_list", []):
        item = MessageItem(
            type=raw_item.get("type"),
            create_time_ms=raw_item.get("create_time_ms"),
            update_time_ms=raw_item.get("update_time_ms"),
            is_completed=raw_item.get("is_completed"),
            msg_id=raw_item.get("msg_id"),
        )
        if "text_item" in raw_item:
            item.text_item = TextItem(**raw_item["text_item"])
        if "image_item" in raw_item:
            img = raw_item["image_item"]
            item.image_item = ImageItem(
                media=CDNMedia(**img["media"]) if "media" in img else None,
                thumb_media=CDNMedia(**img["thumb_media"]) if "thumb_media" in img else None,
                aeskey=img.get("aeskey"),
                url=img.get("url"),
            )
        if "voice_item" in raw_item:
            v = raw_item["voice_item"]
            item.voice_item = VoiceItem(
                media=CDNMedia(**v["media"]) if "media" in v else None,
                encode_type=v.get("encode_type"),
                playtime=v.get("playtime"),
                text=v.get("text"),
            )
        if "file_item" in raw_item:
            f = raw_item["file_item"]
            item.file_item = FileItem(
                media=CDNMedia(**f["media"]) if "media" in f else None,
                file_name=f.get("file_name"),
                md5=f.get("md5"),
                len=f.get("len"),
            )
        if "video_item" in raw_item:
            vid = raw_item["video_item"]
            item.video_item = VideoItem(
                media=CDNMedia(**vid["media"]) if "media" in vid else None,
                video_size=vid.get("video_size"),
                play_length=vid.get("play_length"),
                thumb_media=CDNMedia(**vid["thumb_media"]) if "thumb_media" in vid else None,
            )
        items.append(item)

    return WeixinMessage(
        seq=d.get("seq"),
        message_id=d.get("message_id"),
        from_user_id=d.get("from_user_id"),
        to_user_id=d.get("to_user_id"),
        client_id=d.get("client_id"),
        create_time_ms=d.get("create_time_ms"),
        update_time_ms=d.get("update_time_ms"),
        session_id=d.get("session_id"),
        group_id=d.get("group_id"),
        message_type=d.get("message_type"),
        message_state=d.get("message_state"),
        item_list=items,
        context_token=d.get("context_token"),
    )


def msg_to_dict(msg: WeixinMessage) -> dict:
    """Serialize a WeixinMessage to a JSON-compatible dict for sendMessage."""
    d: dict = {}
    for fld in ("seq", "message_id", "from_user_id", "to_user_id",
                "client_id", "create_time_ms", "update_time_ms",
                "session_id", "group_id", "message_type",
                "message_state", "context_token"):
        val = getattr(msg, fld, None)
        if val is not None:
            d[fld] = val

    if msg.item_list:
        items = []
        for item in msg.item_list:
            raw: dict = {}
            if item.type is not None:
                raw["type"] = item.type
            if item.text_item and item.text_item.text is not None:
                raw["text_item"] = {"text": item.text_item.text}
            # Media items are serialized minimally for sends
            if item.image_item and item.image_item.media:
                raw["image_item"] = {"media": _cdn_to_dict(item.image_item.media)}
            if item.voice_item and item.voice_item.media:
                raw["voice_item"] = {"media": _cdn_to_dict(item.voice_item.media)}
            if item.file_item and item.file_item.media:
                raw["file_item"] = {
                    "media": _cdn_to_dict(item.file_item.media),
                    "file_name": item.file_item.file_name,
                }
            if item.video_item and item.video_item.media:
                raw["video_item"] = {"media": _cdn_to_dict(item.video_item.media)}
            items.append(raw)
        d["item_list"] = items

    return d


def _cdn_to_dict(cdn: CDNMedia) -> dict:
    d: dict = {}
    for fld in ("encrypt_query_param", "aes_key", "encrypt_type", "full_url"):
        val = getattr(cdn, fld, None)
        if val is not None:
            d[fld] = val
    return d
```

- [ ] **Step 3: Write tests for types**

Create `tests/test_wechat_types.py`:

```python
from lingtai.addons.wechat.types import (
    MessageItemType, WeixinMessage, MessageItem, TextItem,
    ImageItem, CDNMedia, VoiceItem, msg_from_dict, msg_to_dict,
)


def test_message_item_type_values():
    assert MessageItemType.TEXT == 1
    assert MessageItemType.IMAGE == 2
    assert MessageItemType.VOICE == 3
    assert MessageItemType.FILE == 4
    assert MessageItemType.VIDEO == 5


def test_msg_from_dict_text():
    raw = {
        "from_user_id": "wxid_abc@im.wechat",
        "to_user_id": "bot123@im.bot",
        "create_time_ms": 1700000000000,
        "context_token": "tok123",
        "item_list": [
            {"type": 1, "text_item": {"text": "hello"}}
        ],
    }
    msg = msg_from_dict(raw)
    assert msg.from_user_id == "wxid_abc@im.wechat"
    assert msg.context_token == "tok123"
    assert len(msg.item_list) == 1
    assert msg.item_list[0].type == MessageItemType.TEXT
    assert msg.item_list[0].text_item.text == "hello"


def test_msg_from_dict_image():
    raw = {
        "from_user_id": "wxid_abc@im.wechat",
        "item_list": [
            {
                "type": 2,
                "image_item": {
                    "media": {"full_url": "https://cdn.example.com/img.jpg"},
                    "aeskey": "abc123",
                },
            }
        ],
    }
    msg = msg_from_dict(raw)
    assert msg.item_list[0].type == MessageItemType.IMAGE
    assert msg.item_list[0].image_item.media.full_url == "https://cdn.example.com/img.jpg"
    assert msg.item_list[0].image_item.aeskey == "abc123"


def test_msg_from_dict_voice_with_transcription():
    raw = {
        "from_user_id": "wxid_abc@im.wechat",
        "item_list": [
            {
                "type": 3,
                "voice_item": {
                    "media": {"full_url": "https://cdn.example.com/voice.silk"},
                    "text": "transcribed text",
                    "playtime": 5000,
                },
            }
        ],
    }
    msg = msg_from_dict(raw)
    assert msg.item_list[0].voice_item.text == "transcribed text"
    assert msg.item_list[0].voice_item.playtime == 5000


def test_msg_to_dict_text():
    msg = WeixinMessage(
        from_user_id="bot@im.bot",
        to_user_id="wxid_abc@im.wechat",
        context_token="tok",
        item_list=[
            MessageItem(type=1, text_item=TextItem(text="hi")),
        ],
    )
    d = msg_to_dict(msg)
    assert d["from_user_id"] == "bot@im.bot"
    assert d["context_token"] == "tok"
    assert d["item_list"][0]["text_item"]["text"] == "hi"


def test_msg_from_dict_empty():
    msg = msg_from_dict({})
    assert msg.from_user_id is None
    assert msg.item_list == []


def test_roundtrip_text():
    raw = {
        "from_user_id": "wxid@im.wechat",
        "to_user_id": "bot@im.bot",
        "context_token": "ctx",
        "item_list": [{"type": 1, "text_item": {"text": "roundtrip"}}],
    }
    msg = msg_from_dict(raw)
    d = msg_to_dict(msg)
    assert d["from_user_id"] == "wxid@im.wechat"
    assert d["item_list"][0]["text_item"]["text"] == "roundtrip"
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_wechat_types.py -v`
Expected: All 7 tests pass.

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai/addons/wechat/__init__.py src/lingtai/addons/wechat/types.py tests/test_wechat_types.py
git commit -m "feat(addon): add wechat protocol types"
```

---

### Task 2: API Wrappers

**Files:**
- Create: `src/lingtai/addons/wechat/api.py`
- Test: `tests/test_wechat_api.py`

- [ ] **Step 1: Write the API module**

Create `src/lingtai/addons/wechat/api.py`:

```python
"""HTTP wrappers for the 5 iLink Bot API endpoints."""
from __future__ import annotations

import json
import logging
import struct
from importlib.metadata import version as pkg_version
from pathlib import Path
from typing import Any

import httpx

from .types import (
    GetUpdatesResp, GetUploadUrlResp, GetConfigResp,
    WeixinMessage, msg_from_dict, msg_to_dict,
)

log = logging.getLogger(__name__)

DEFAULT_BASE_URL = "https://ilinkai.weixin.qq.com"
CDN_BASE_URL = "https://novac2c.cdn.weixin.qq.com/c2c"
DEFAULT_LONG_POLL_TIMEOUT = 35.0
DEFAULT_SEND_TIMEOUT = 15.0

# Package version for channel_version header
_PKG_VERSION = "1.0.0"


def _ensure_trailing_slash(url: str) -> str:
    return url if url.endswith("/") else url + "/"


def _auth_headers(token: str | None) -> dict[str, str]:
    headers: dict[str, str] = {
        "Content-Type": "application/json",
        "AuthorizationType": "ilink_bot_token",
    }
    if token:
        headers["Authorization"] = f"Bearer {token.strip()}"
    return headers


def _base_info() -> dict:
    return {"channel_version": _PKG_VERSION}


async def get_qrcode(base_url: str = DEFAULT_BASE_URL) -> dict:
    """Fetch a QR code for WeChat login.

    Returns dict with 'qrcode' (str) and 'qrcode_img_content' (str) keys.
    """
    url = _ensure_trailing_slash(base_url) + "ilink/bot/get_bot_qrcode?bot_type=3"
    async with httpx.AsyncClient() as client:
        resp = await client.get(url, timeout=15.0)
        resp.raise_for_status()
        return resp.json()


async def poll_qr_status(base_url: str, qrcode: str) -> dict:
    """Poll QR code login status. Returns dict with 'status' key.

    Status values: 'wait', 'scaned', 'confirmed', 'expired', 'scaned_but_redirect'.
    On 'confirmed': also has 'bot_token', 'ilink_bot_id', 'baseurl', 'ilink_user_id'.
    """
    url = (
        _ensure_trailing_slash(base_url)
        + f"ilink/bot/get_qrcode_status?qrcode={qrcode}"
    )
    async with httpx.AsyncClient() as client:
        resp = await client.get(url, timeout=DEFAULT_LONG_POLL_TIMEOUT + 5)
        resp.raise_for_status()
        return resp.json()


async def get_updates(
    base_url: str,
    token: str,
    get_updates_buf: str = "",
    timeout: float = DEFAULT_LONG_POLL_TIMEOUT,
) -> GetUpdatesResp:
    """Long-poll for incoming messages.

    Returns GetUpdatesResp with msgs list and updated get_updates_buf cursor.
    On client-side timeout, returns empty response to allow retry.
    """
    url = _ensure_trailing_slash(base_url) + "ilink/bot/getupdates"
    body = {
        "get_updates_buf": get_updates_buf,
        "base_info": _base_info(),
    }
    try:
        async with httpx.AsyncClient() as client:
            resp = await client.post(
                url,
                json=body,
                headers=_auth_headers(token),
                timeout=timeout + 5,
            )
            resp.raise_for_status()
            data = resp.json()
    except httpx.TimeoutException:
        # Server didn't respond in time — return empty to retry
        return GetUpdatesResp(
            ret=0, msgs=[], get_updates_buf=get_updates_buf,
        )

    msgs = [msg_from_dict(m) for m in data.get("msgs", [])]
    return GetUpdatesResp(
        ret=data.get("ret"),
        errcode=data.get("errcode"),
        errmsg=data.get("errmsg"),
        msgs=msgs,
        get_updates_buf=data.get("get_updates_buf", get_updates_buf),
        longpolling_timeout_ms=data.get("longpolling_timeout_ms"),
    )


async def send_message(
    base_url: str,
    token: str,
    msg: WeixinMessage,
) -> None:
    """Send a message (text or media)."""
    url = _ensure_trailing_slash(base_url) + "ilink/bot/sendmessage"
    body = {
        "msg": msg_to_dict(msg),
        "base_info": _base_info(),
    }
    async with httpx.AsyncClient() as client:
        resp = await client.post(
            url,
            json=body,
            headers=_auth_headers(token),
            timeout=DEFAULT_SEND_TIMEOUT,
        )
        resp.raise_for_status()


async def get_upload_url(
    base_url: str,
    token: str,
    *,
    media_type: int,
    to_user_id: str,
    rawsize: int,
    rawfilemd5: str,
    filesize: int,
    aeskey: str | None = None,
) -> GetUploadUrlResp:
    """Get a pre-signed CDN upload URL."""
    url = _ensure_trailing_slash(base_url) + "ilink/bot/getuploadurl"
    body: dict[str, Any] = {
        "media_type": media_type,
        "to_user_id": to_user_id,
        "rawsize": rawsize,
        "rawfilemd5": rawfilemd5,
        "filesize": filesize,
        "base_info": _base_info(),
    }
    if aeskey:
        body["aeskey"] = aeskey
    async with httpx.AsyncClient() as client:
        resp = await client.post(
            url,
            json=body,
            headers=_auth_headers(token),
            timeout=DEFAULT_SEND_TIMEOUT,
        )
        resp.raise_for_status()
        data = resp.json()
    return GetUploadUrlResp(
        upload_param=data.get("upload_param"),
        upload_full_url=data.get("upload_full_url"),
    )


async def get_config(base_url: str, token: str) -> GetConfigResp:
    """Get bot config (typing ticket etc.)."""
    url = _ensure_trailing_slash(base_url) + "ilink/bot/getconfig"
    async with httpx.AsyncClient() as client:
        resp = await client.post(
            url,
            json={"base_info": _base_info()},
            headers=_auth_headers(token),
            timeout=DEFAULT_SEND_TIMEOUT,
        )
        resp.raise_for_status()
        data = resp.json()
    return GetConfigResp(
        ret=data.get("ret"),
        errmsg=data.get("errmsg"),
        typing_ticket=data.get("typing_ticket"),
    )
```

- [ ] **Step 2: Write API tests (mocked HTTP)**

Create `tests/test_wechat_api.py`:

```python
import json
import pytest
import httpx

from lingtai.addons.wechat import api
from lingtai.addons.wechat.types import WeixinMessage, MessageItem, TextItem


@pytest.mark.anyio
async def test_get_qrcode(httpx_mock):
    httpx_mock.add_response(
        url="https://ilinkai.weixin.qq.com/ilink/bot/get_bot_qrcode?bot_type=3",
        json={"qrcode": "qr123", "qrcode_img_content": "data:image/png;base64,..."},
    )
    result = await api.get_qrcode()
    assert result["qrcode"] == "qr123"


@pytest.mark.anyio
async def test_poll_qr_status_confirmed(httpx_mock):
    httpx_mock.add_response(
        json={
            "status": "confirmed",
            "bot_token": "tok123",
            "ilink_bot_id": "bot@im.bot",
            "ilink_user_id": "wxid@im.wechat",
        },
    )
    result = await api.poll_qr_status("https://ilinkai.weixin.qq.com", "qr123")
    assert result["status"] == "confirmed"
    assert result["bot_token"] == "tok123"


@pytest.mark.anyio
async def test_get_updates_with_messages(httpx_mock):
    httpx_mock.add_response(
        json={
            "ret": 0,
            "msgs": [
                {
                    "from_user_id": "wxid@im.wechat",
                    "item_list": [{"type": 1, "text_item": {"text": "hello"}}],
                }
            ],
            "get_updates_buf": "buf2",
        },
    )
    resp = await api.get_updates("https://ilinkai.weixin.qq.com", "tok123")
    assert len(resp.msgs) == 1
    assert resp.msgs[0].from_user_id == "wxid@im.wechat"
    assert resp.get_updates_buf == "buf2"


@pytest.mark.anyio
async def test_get_updates_timeout():
    """On timeout, returns empty response with same buf."""
    # Use a very short timeout against a non-routable address
    resp = await api.get_updates(
        "http://192.0.2.1",  # RFC 5737 TEST-NET
        "tok", get_updates_buf="old_buf", timeout=0.1,
    )
    assert resp.msgs == []
    assert resp.get_updates_buf == "old_buf"


@pytest.mark.anyio
async def test_send_message(httpx_mock):
    httpx_mock.add_response(json={})
    msg = WeixinMessage(
        to_user_id="wxid@im.wechat",
        item_list=[MessageItem(type=1, text_item=TextItem(text="hi"))],
    )
    await api.send_message("https://ilinkai.weixin.qq.com", "tok123", msg)
    req = httpx_mock.get_request()
    body = json.loads(req.content)
    assert body["msg"]["to_user_id"] == "wxid@im.wechat"


@pytest.mark.anyio
async def test_get_config(httpx_mock):
    httpx_mock.add_response(
        json={"ret": 0, "typing_ticket": "ticket123"},
    )
    resp = await api.get_config("https://ilinkai.weixin.qq.com", "tok123")
    assert resp.typing_ticket == "ticket123"
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_wechat_api.py -v`

Note: Tests require `pytest-httpx` and `anyio` for async mocking. If not installed:
```bash
pip install pytest-httpx anyio
```

Expected: All 6 tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/addons/wechat/api.py tests/test_wechat_api.py
git commit -m "feat(addon): add wechat iLink API wrappers"
```

---

### Task 3: QR Login

**Files:**
- Create: `src/lingtai/addons/wechat/login.py`

- [ ] **Step 1: Write the login module**

Create `src/lingtai/addons/wechat/login.py`:

```python
"""QR code login flow for WeChat iLink Bot API.

Provides cli_login() as a synchronous entry point for the setup skill.
"""
from __future__ import annotations

import asyncio
import json
import os
import sys
import time
from pathlib import Path

from . import api

LOGIN_TIMEOUT = 300  # 5 minutes
POLL_INTERVAL = 2.0


def cli_login(addon_dir: str) -> None:
    """CLI entry point for WeChat QR login.

    Called by the setup skill via:
        python -c "from lingtai.addons.wechat.login import cli_login; cli_login('.lingtai/.addons/wechat')"

    Creates config.json with defaults if missing, runs QR login,
    saves credentials.json on success.
    """
    addon_path = Path(addon_dir)
    addon_path.mkdir(parents=True, exist_ok=True)

    # Create default config if not present
    config_path = addon_path / "config.json"
    if not config_path.is_file():
        config_path.write_text(json.dumps({
            "base_url": api.DEFAULT_BASE_URL,
            "cdn_base_url": api.CDN_BASE_URL,
            "poll_interval": 1.0,
            "allowed_users": [],
        }, indent=2), encoding="utf-8")
        print(f"Created default config at {config_path}")

    # Read base_url from config
    cfg = json.loads(config_path.read_text(encoding="utf-8"))
    base_url = cfg.get("base_url", api.DEFAULT_BASE_URL)

    try:
        result = asyncio.run(_login_flow(base_url))
    except KeyboardInterrupt:
        print("\nLogin cancelled.")
        sys.exit(1)

    if result is None:
        print("Login failed — QR code expired or error occurred.")
        sys.exit(1)

    # Save credentials
    creds_path = addon_path / "credentials.json"
    creds = {
        "bot_token": result["bot_token"],
        "user_id": result["user_id"],
        "base_url": result["base_url"],
        "saved_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    creds_path.write_text(json.dumps(creds, indent=2), encoding="utf-8")
    os.chmod(creds_path, 0o600)
    print(f"Connected as {result['user_id']}")
    print(f"Credentials saved to {creds_path}")


async def _login_flow(base_url: str) -> dict | None:
    """Run the QR login flow. Returns credentials dict or None on failure."""
    # Step 1: Get QR code
    print("Fetching QR code...")
    qr_data = await api.get_qrcode(base_url)
    qrcode_str = qr_data.get("qrcode")
    if not qrcode_str:
        print("Error: failed to get QR code from server.")
        return None

    # Step 2: Display QR code in terminal
    try:
        import qrcode as qr_lib
        qr = qr_lib.QRCode(error_correction=qr_lib.constants.ERROR_CORRECT_L)
        qr.add_data(qr_data.get("qrcode_img_content", qrcode_str))
        qr.make(fit=True)
        qr.print_ascii(invert=True)
    except ImportError:
        print(f"QR code data: {qrcode_str}")
        print("(Install 'qrcode' package for visual QR display)")

    print("\nScan this QR code with WeChat on your phone.")
    print("Waiting for confirmation (5 minute timeout)...")

    # Step 3: Poll for confirmation
    start = time.time()
    current_base_url = base_url
    while time.time() - start < LOGIN_TIMEOUT:
        try:
            status = await api.poll_qr_status(current_base_url, qrcode_str)
        except Exception as e:
            print(f"Poll error: {e}, retrying...")
            await asyncio.sleep(POLL_INTERVAL)
            continue

        s = status.get("status", "")
        if s == "wait":
            pass  # Still waiting for scan
        elif s == "scaned":
            print("QR code scanned — confirm on your phone...")
        elif s == "confirmed":
            return {
                "bot_token": status["bot_token"],
                "user_id": status.get("ilink_user_id", status.get("ilink_bot_id", "")),
                "base_url": status.get("baseurl", current_base_url),
            }
        elif s == "expired":
            print("QR code expired.")
            return None
        elif s == "scaned_but_redirect":
            redirect_host = status.get("redirect_host", "")
            if redirect_host:
                current_base_url = f"https://{redirect_host}"
                print(f"Redirecting to {current_base_url}...")
            continue
        else:
            print(f"Unknown status: {s}")

        await asyncio.sleep(POLL_INTERVAL)

    print("Login timed out (5 minutes).")
    return None
```

- [ ] **Step 2: Verify module imports cleanly**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai.addons.wechat.login import cli_login; print('ok')"`
Expected: `ok`

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/addons/wechat/login.py
git commit -m "feat(addon): add wechat QR login flow"
```

---

### Task 4: Media Helpers

**Files:**
- Create: `src/lingtai/addons/wechat/media.py`

- [ ] **Step 1: Write the media module**

Create `src/lingtai/addons/wechat/media.py`:

```python
"""Media download/upload helpers for WeChat addon."""
from __future__ import annotations

import hashlib
import logging
import mimetypes
import os
from pathlib import Path
from typing import Any

import httpx

from . import api
from .types import (
    CDNMedia, UploadMediaType, MessageItemType,
    ImageItem, VoiceItem, FileItem, VideoItem, MessageItem,
)

log = logging.getLogger(__name__)

# Extension → UploadMediaType mapping
_UPLOAD_TYPE_MAP = {
    ".jpg": UploadMediaType.IMAGE,
    ".jpeg": UploadMediaType.IMAGE,
    ".png": UploadMediaType.IMAGE,
    ".gif": UploadMediaType.IMAGE,
    ".webp": UploadMediaType.IMAGE,
    ".bmp": UploadMediaType.IMAGE,
    ".mp4": UploadMediaType.VIDEO,
    ".avi": UploadMediaType.VIDEO,
    ".mov": UploadMediaType.VIDEO,
    ".mkv": UploadMediaType.VIDEO,
    ".wav": UploadMediaType.VOICE,
    ".mp3": UploadMediaType.VOICE,
    ".ogg": UploadMediaType.VOICE,
    ".silk": UploadMediaType.VOICE,
    ".amr": UploadMediaType.VOICE,
}

# UploadMediaType → MessageItemType mapping
_ITEM_TYPE_MAP = {
    UploadMediaType.IMAGE: MessageItemType.IMAGE,
    UploadMediaType.VIDEO: MessageItemType.VIDEO,
    UploadMediaType.VOICE: MessageItemType.VOICE,
    UploadMediaType.FILE: MessageItemType.FILE,
}


async def download_media(
    cdn_media: CDNMedia,
    dest_dir: str | Path,
    filename: str = "media",
) -> str:
    """Download media from CDN. Returns local file path."""
    dest_dir = Path(dest_dir)
    dest_dir.mkdir(parents=True, exist_ok=True)

    url = cdn_media.full_url
    if not url:
        raise ValueError("CDN media has no full_url")

    dest_path = dest_dir / filename
    async with httpx.AsyncClient() as client:
        resp = await client.get(url, timeout=60.0)
        resp.raise_for_status()
        dest_path.write_bytes(resp.content)

    return str(dest_path)


def decode_voice(silk_path: str | Path, out_path: str | Path) -> str:
    """Decode Silk audio to WAV. Returns output path.

    Requires the `pilk` package: pip install pilk
    """
    try:
        import pilk
    except ImportError:
        log.warning("pilk not installed — cannot decode Silk voice. pip install pilk")
        return str(silk_path)

    silk_path = str(silk_path)
    out_path = str(out_path)
    pilk.decode(silk_path, out_path)
    return out_path


def detect_upload_type(file_path: str | Path) -> UploadMediaType:
    """Detect UploadMediaType from file extension. Defaults to FILE."""
    ext = Path(file_path).suffix.lower()
    return _UPLOAD_TYPE_MAP.get(ext, UploadMediaType.FILE)


async def upload_media(
    file_path: str | Path,
    base_url: str,
    token: str,
    to_user_id: str,
) -> CDNMedia:
    """Upload a file to WeChat CDN. Returns CDNMedia reference for sendMessage."""
    file_path = Path(file_path)
    if not file_path.is_file():
        raise FileNotFoundError(f"File not found: {file_path}")

    data = file_path.read_bytes()
    md5 = hashlib.md5(data).hexdigest()
    media_type = detect_upload_type(file_path)

    # Get upload URL
    upload_resp = await api.get_upload_url(
        base_url, token,
        media_type=int(media_type),
        to_user_id=to_user_id,
        rawsize=len(data),
        rawfilemd5=md5,
        filesize=len(data),
    )

    upload_url = upload_resp.upload_full_url
    if not upload_url:
        raise RuntimeError("Server did not return an upload URL")

    # Upload to CDN
    async with httpx.AsyncClient() as client:
        resp = await client.put(
            upload_url,
            content=data,
            headers={"Content-Type": "application/octet-stream"},
            timeout=120.0,
        )
        resp.raise_for_status()

    return CDNMedia(
        encrypt_query_param=upload_resp.upload_param,
    )


def make_media_item(cdn_media: CDNMedia, file_path: Path) -> MessageItem:
    """Create a MessageItem for sending uploaded media."""
    upload_type = detect_upload_type(file_path)
    item_type = _ITEM_TYPE_MAP.get(upload_type, MessageItemType.FILE)

    item = MessageItem(type=int(item_type))
    if item_type == MessageItemType.IMAGE:
        item.image_item = ImageItem(media=cdn_media)
    elif item_type == MessageItemType.VIDEO:
        item.video_item = VideoItem(media=cdn_media)
    elif item_type == MessageItemType.VOICE:
        item.voice_item = VoiceItem(media=cdn_media)
    else:
        item.file_item = FileItem(media=cdn_media, file_name=file_path.name)

    return item
```

- [ ] **Step 2: Verify module imports cleanly**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai.addons.wechat.media import detect_upload_type; print('ok')"`
Expected: `ok`

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/addons/wechat/media.py
git commit -m "feat(addon): add wechat media download/upload helpers"
```

---

### Task 5: Manager (Tool Schema + Message Persistence + Bridge)

**Files:**
- Create: `src/lingtai/addons/wechat/manager.py`
- Test: `tests/test_wechat_manager.py`

This is the largest task — the manager handles tool dispatch, incoming/outgoing message conversion, and filesystem persistence.

- [ ] **Step 1: Write the manager module**

Create `src/lingtai/addons/wechat/manager.py`:

```python
"""WeChat addon manager — tool dispatch, message persistence, bridge."""
from __future__ import annotations

import asyncio
import json
import logging
import os
import re
import time
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING, Any

from .types import (
    MessageItemType, WeixinMessage, MessageItem, TextItem,
    msg_from_dict, msg_to_dict,
)
from . import api
from . import media as media_mod

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

log = logging.getLogger(__name__)

TEXT_CHUNK_LIMIT = 4000
SESSION_EXPIRED_ERRCODE = -14

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": [
                "send", "check", "read", "reply", "search",
                "contacts", "add_contact", "remove_contact",
            ],
            "description": (
                "send: send a message to a WeChat user "
                "(user_id, text; optional media_path for file/image/voice/video). "
                "check: list recent conversations with unread counts. "
                "read: read messages from a user (user_id; optional limit). "
                "reply: reply to a specific message "
                "(message_id from read results, text). "
                "search: search inbox messages by regex "
                "(query; optional user_id). "
                "contacts: list saved contacts. "
                "add_contact: save a contact (user_id, alias). "
                "remove_contact: remove a contact (alias or user_id)."
            ),
        },
        "user_id": {
            "type": "string",
            "description": "WeChat user ID (e.g. wxid_abc123@im.wechat)",
        },
        "text": {
            "type": "string",
            "description": "Message text content",
        },
        "media_path": {
            "type": "string",
            "description": (
                "Absolute path to a file to send as media. "
                "Type detected from extension: "
                ".jpg/.png=image, .mp4=video, .wav/.mp3=voice, other=file."
            ),
        },
        "message_id": {
            "type": "string",
            "description": "Message ID from read results (for reply action)",
        },
        "limit": {
            "type": "integer",
            "description": "Max messages to return (for read, default 10)",
            "default": 10,
        },
        "query": {
            "type": "string",
            "description": "Search query (regex pattern)",
        },
        "alias": {
            "type": "string",
            "description": "Human-friendly contact alias",
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "WeChat client — interact with WeChat users via iLink Bot API. "
    "Supports text, images, voice, video, and files. "
    "Use 'send' for outgoing messages (text and/or media_path). "
    "'check' to see recent conversations with unread counts. "
    "'read' to read messages from a user. "
    "'reply' to respond to a message. "
    "'search' to find messages by keyword or regex. "
    "'contacts' to manage saved contacts."
)


class WechatManager:
    """Manages WeChat addon lifecycle, tool dispatch, and message storage."""

    def __init__(
        self,
        agent: "BaseAgent",
        *,
        base_url: str = api.DEFAULT_BASE_URL,
        cdn_base_url: str = api.CDN_BASE_URL,
        token: str,
        user_id: str,
        poll_interval: float = 1.0,
        allowed_users: list[str] | None = None,
        working_dir: Path,
    ) -> None:
        self._agent = agent
        self._base_url = base_url
        self._cdn_base_url = cdn_base_url
        self._token = token
        self._user_id = user_id
        self._poll_interval = poll_interval
        self._allowed_users = set(allowed_users) if allowed_users else None
        self._working_dir = working_dir

        # Filesystem dirs
        self._wechat_dir = working_dir / "wechat"
        self._inbox_dir = self._wechat_dir / "inbox"
        self._sent_dir = self._wechat_dir / "sent"
        self._media_dir = self._wechat_dir / "media"
        for d in (self._inbox_dir, self._sent_dir, self._media_dir):
            d.mkdir(parents=True, exist_ok=True)

        # State
        self._get_updates_buf = ""
        self._context_tokens: dict[str, str] = {}  # user_id -> context_token
        self._contacts: dict[str, dict] = {}  # alias -> {user_id, name}
        self._read_ids: set[str] = set()
        self._poll_task: asyncio.Task | None = None
        self._running = False

        # Load persisted state
        self._load_state()

    def start(self) -> None:
        """Start the long-poll loop."""
        self._running = True
        try:
            loop = asyncio.get_event_loop()
        except RuntimeError:
            loop = asyncio.new_event_loop()
            asyncio.set_event_loop(loop)
        self._poll_task = loop.create_task(self._poll_loop())
        log.info("WeChat addon started for %s", self._user_id)

    def stop(self) -> None:
        """Stop the long-poll loop."""
        self._running = False
        if self._poll_task and not self._poll_task.done():
            self._poll_task.cancel()
        self._save_state()
        log.info("WeChat addon stopped")

    # ── Poll loop ──────────────────────────────────────────────

    async def _poll_loop(self) -> None:
        while self._running:
            try:
                resp = await api.get_updates(
                    self._base_url, self._token, self._get_updates_buf,
                )

                # Check for session expiry
                if resp.errcode == SESSION_EXPIRED_ERRCODE:
                    log.warning("WeChat session expired (errcode -14)")
                    self._notify_session_expired()
                    self._running = False
                    return

                if resp.get_updates_buf:
                    self._get_updates_buf = resp.get_updates_buf

                for msg in resp.msgs:
                    await self._on_incoming(msg)

            except asyncio.CancelledError:
                return
            except Exception as e:
                log.error("WeChat poll error: %s", e)

            await asyncio.sleep(self._poll_interval)

    def _notify_session_expired(self) -> None:
        """Send internal notification that WeChat session expired."""
        try:
            from lingtai_kernel.message import _make_message, MSG_REQUEST
            notification = (
                "[system] WeChat session expired. "
                "Please ask me to re-login to WeChat."
            )
            msg = _make_message(MSG_REQUEST, "system", notification)
            self._agent.inbox.put(msg)
            self._agent._wake_nap("message_received")
        except Exception as e:
            log.error("Failed to notify session expiry: %s", e)

    # ── Incoming message processing ────────────────────────────

    async def _on_incoming(self, msg: WeixinMessage) -> None:
        """Process an incoming WeChat message."""
        from_user = msg.from_user_id or ""

        # Filter by allowed_users
        if self._allowed_users and from_user not in self._allowed_users:
            return

        # Cache context token
        if msg.context_token:
            self._context_tokens[from_user] = msg.context_token

        # Build text representation
        body_parts: list[str] = []
        for item in msg.item_list:
            item_type = item.type or 0
            if item_type == MessageItemType.TEXT:
                if item.text_item and item.text_item.text:
                    body_parts.append(item.text_item.text)

            elif item_type == MessageItemType.IMAGE:
                if item.image_item and item.image_item.media:
                    try:
                        ext = ".jpg"
                        fname = f"{uuid.uuid4().hex}{ext}"
                        path = await media_mod.download_media(
                            item.image_item.media, self._media_dir, fname,
                        )
                        body_parts.append(f"[Image: {path}]")
                    except Exception as e:
                        body_parts.append(f"[Image: download failed — {e}]")

            elif item_type == MessageItemType.VOICE:
                if item.voice_item:
                    transcription = item.voice_item.text or ""
                    audio_path = ""
                    if item.voice_item.media:
                        try:
                            silk_name = f"{uuid.uuid4().hex}.silk"
                            silk_path = await media_mod.download_media(
                                item.voice_item.media, self._media_dir, silk_name,
                            )
                            wav_path = silk_path.replace(".silk", ".wav")
                            audio_path = media_mod.decode_voice(silk_path, wav_path)
                        except Exception as e:
                            audio_path = f"download failed — {e}"
                    if transcription and audio_path:
                        body_parts.append(
                            f'[Voice: "{transcription}" (audio: {audio_path})]'
                        )
                    elif transcription:
                        body_parts.append(f'[Voice: "{transcription}"]')
                    elif audio_path:
                        body_parts.append(f"[Voice: (audio: {audio_path})]")

            elif item_type == MessageItemType.FILE:
                if item.file_item and item.file_item.media:
                    try:
                        fname = item.file_item.file_name or f"{uuid.uuid4().hex}"
                        path = await media_mod.download_media(
                            item.file_item.media, self._media_dir, fname,
                        )
                        body_parts.append(f"[File: {fname} ({path})]")
                    except Exception as e:
                        body_parts.append(f"[File: download failed — {e}]")

            elif item_type == MessageItemType.VIDEO:
                if item.video_item and item.video_item.media:
                    try:
                        fname = f"{uuid.uuid4().hex}.mp4"
                        path = await media_mod.download_media(
                            item.video_item.media, self._media_dir, fname,
                        )
                        body_parts.append(f"[Video: {path}]")
                    except Exception as e:
                        body_parts.append(f"[Video: download failed — {e}]")

        body = "\n".join(body_parts) if body_parts else "(empty message)"

        # Persist to inbox
        msg_id = str(uuid.uuid4())
        msg_dir = self._inbox_dir / msg_id
        msg_dir.mkdir(parents=True, exist_ok=True)
        msg_data = {
            "id": msg_id,
            "from_user_id": from_user,
            "body": body,
            "date": datetime.now(timezone.utc).isoformat(),
            "raw_item_types": [item.type for item in msg.item_list],
        }
        (msg_dir / "message.json").write_text(
            json.dumps(msg_data, ensure_ascii=False, indent=2), encoding="utf-8",
        )

        # Notify agent
        try:
            from lingtai_kernel.message import _make_message, MSG_REQUEST
            contact = self._find_contact_by_user_id(from_user)
            display = contact.get("alias", from_user) if contact else from_user
            notification = f"[system] New WeChat message from {display}: {body[:200]}"
            kernel_msg = _make_message(MSG_REQUEST, "system", notification)
            self._agent.inbox.put(kernel_msg)
            self._agent._wake_nap("message_received")
        except Exception as e:
            log.error("Failed to notify agent: %s", e)

    # ── Tool handler dispatch ──────────────────────────────────

    def handle(self, args: dict) -> dict:
        action = args.get("action")
        try:
            if action == "send":
                return self._handle_send(args)
            elif action == "check":
                return self._handle_check(args)
            elif action == "read":
                return self._handle_read(args)
            elif action == "reply":
                return self._handle_reply(args)
            elif action == "search":
                return self._handle_search(args)
            elif action == "contacts":
                return self._handle_contacts()
            elif action == "add_contact":
                return self._handle_add_contact(args)
            elif action == "remove_contact":
                return self._handle_remove_contact(args)
            else:
                return {"error": f"Unknown wechat action: {action!r}"}
        except Exception as e:
            return {"error": str(e)}

    # ── Action handlers ────────────────────────────────────────

    def _handle_send(self, args: dict) -> dict:
        user_id = args.get("user_id")
        text = args.get("text", "")
        media_path = args.get("media_path")

        if not user_id:
            return {"error": "user_id is required for send"}
        if not text and not media_path:
            return {"error": "text or media_path is required"}

        loop = self._get_loop()
        results = []

        # Send text (chunked if needed)
        if text:
            chunks = _chunk_text(text, TEXT_CHUNK_LIMIT)
            for chunk in chunks:
                msg = WeixinMessage(
                    to_user_id=user_id,
                    context_token=self._context_tokens.get(user_id),
                    item_list=[MessageItem(
                        type=int(MessageItemType.TEXT),
                        text_item=TextItem(text=chunk),
                    )],
                )
                loop.run_until_complete(
                    api.send_message(self._base_url, self._token, msg)
                )
                results.append(f"text ({len(chunk)} chars)")

        # Send media
        if media_path:
            path = Path(media_path)
            if not path.is_file():
                return {"error": f"File not found: {media_path}"}
            cdn_media = loop.run_until_complete(
                media_mod.upload_media(path, self._base_url, self._token, user_id)
            )
            media_item = media_mod.make_media_item(cdn_media, path)
            msg = WeixinMessage(
                to_user_id=user_id,
                context_token=self._context_tokens.get(user_id),
                item_list=[media_item],
            )
            loop.run_until_complete(
                api.send_message(self._base_url, self._token, msg)
            )
            results.append(f"media ({path.name})")

        # Persist to sent
        msg_id = str(uuid.uuid4())
        msg_dir = self._sent_dir / msg_id
        msg_dir.mkdir(parents=True, exist_ok=True)
        sent_data = {
            "id": msg_id,
            "to_user_id": user_id,
            "text": text,
            "media_path": media_path,
            "date": datetime.now(timezone.utc).isoformat(),
        }
        (msg_dir / "message.json").write_text(
            json.dumps(sent_data, ensure_ascii=False, indent=2), encoding="utf-8",
        )

        return {"status": "ok", "sent": results, "message_id": msg_id}

    def _handle_check(self, args: dict) -> dict:
        """List conversations with unread counts."""
        conversations: dict[str, dict] = {}
        for msg_dir in sorted(self._inbox_dir.iterdir()):
            msg_file = msg_dir / "message.json"
            if not msg_file.is_file():
                continue
            data = json.loads(msg_file.read_text(encoding="utf-8"))
            user = data.get("from_user_id", "unknown")
            msg_id = data.get("id", "")
            if user not in conversations:
                contact = self._find_contact_by_user_id(user)
                conversations[user] = {
                    "user_id": user,
                    "alias": contact.get("alias", user) if contact else user,
                    "total": 0,
                    "unread": 0,
                    "latest": data.get("body", "")[:100],
                    "date": data.get("date", ""),
                }
            conversations[user]["total"] += 1
            if msg_id not in self._read_ids:
                conversations[user]["unread"] += 1
            conversations[user]["latest"] = data.get("body", "")[:100]
            conversations[user]["date"] = data.get("date", "")

        return {"conversations": list(conversations.values())}

    def _handle_read(self, args: dict) -> dict:
        user_id = args.get("user_id")
        limit = args.get("limit", 10)
        if not user_id:
            return {"error": "user_id is required for read"}

        messages = []
        for msg_dir in sorted(self._inbox_dir.iterdir(), reverse=True):
            msg_file = msg_dir / "message.json"
            if not msg_file.is_file():
                continue
            data = json.loads(msg_file.read_text(encoding="utf-8"))
            if data.get("from_user_id") != user_id:
                continue
            msg_id = data.get("id", "")
            self._read_ids.add(msg_id)
            messages.append(data)
            if len(messages) >= limit:
                break

        self._save_read()
        return {"messages": messages}

    def _handle_reply(self, args: dict) -> dict:
        message_id = args.get("message_id")
        text = args.get("text", "")
        if not message_id or not text:
            return {"error": "message_id and text are required for reply"}

        # Find the original message to get user_id
        msg_file = self._inbox_dir / message_id / "message.json"
        if not msg_file.is_file():
            return {"error": f"Message not found: {message_id}"}
        data = json.loads(msg_file.read_text(encoding="utf-8"))
        user_id = data.get("from_user_id")
        if not user_id:
            return {"error": "Cannot determine user_id from message"}

        return self._handle_send({"user_id": user_id, "text": text})

    def _handle_search(self, args: dict) -> dict:
        query = args.get("query", "")
        user_id_filter = args.get("user_id")
        if not query:
            return {"error": "query is required for search"}

        pattern = re.compile(query, re.IGNORECASE)
        matches = []
        for msg_dir in sorted(self._inbox_dir.iterdir(), reverse=True):
            msg_file = msg_dir / "message.json"
            if not msg_file.is_file():
                continue
            data = json.loads(msg_file.read_text(encoding="utf-8"))
            if user_id_filter and data.get("from_user_id") != user_id_filter:
                continue
            body = data.get("body", "")
            if pattern.search(body):
                matches.append(data)
            if len(matches) >= 20:
                break

        return {"matches": matches}

    def _handle_contacts(self) -> dict:
        return {"contacts": self._contacts}

    def _handle_add_contact(self, args: dict) -> dict:
        user_id = args.get("user_id")
        alias = args.get("alias")
        if not user_id or not alias:
            return {"error": "user_id and alias are required"}
        self._contacts[alias] = {
            "user_id": user_id,
            "name": args.get("name", alias),
        }
        self._save_contacts()
        return {"status": "ok", "alias": alias}

    def _handle_remove_contact(self, args: dict) -> dict:
        alias = args.get("alias")
        user_id = args.get("user_id")
        if alias and alias in self._contacts:
            del self._contacts[alias]
        elif user_id:
            self._contacts = {
                k: v for k, v in self._contacts.items()
                if v.get("user_id") != user_id
            }
        else:
            return {"error": "alias or user_id required"}
        self._save_contacts()
        return {"status": "ok"}

    # ── State persistence ──────────────────────────────────────

    def _load_state(self) -> None:
        contacts_file = self._wechat_dir / "contacts.json"
        if contacts_file.is_file():
            self._contacts = json.loads(
                contacts_file.read_text(encoding="utf-8")
            )
        read_file = self._wechat_dir / "read.json"
        if read_file.is_file():
            self._read_ids = set(
                json.loads(read_file.read_text(encoding="utf-8"))
            )
        state_file = self._wechat_dir / "state.json"
        if state_file.is_file():
            state = json.loads(state_file.read_text(encoding="utf-8"))
            self._get_updates_buf = state.get("get_updates_buf", "")
            self._context_tokens = state.get("context_tokens", {})

    def _save_state(self) -> None:
        state = {
            "get_updates_buf": self._get_updates_buf,
            "context_tokens": self._context_tokens,
        }
        (self._wechat_dir / "state.json").write_text(
            json.dumps(state, ensure_ascii=False, indent=2), encoding="utf-8",
        )

    def _save_contacts(self) -> None:
        (self._wechat_dir / "contacts.json").write_text(
            json.dumps(self._contacts, ensure_ascii=False, indent=2),
            encoding="utf-8",
        )

    def _save_read(self) -> None:
        (self._wechat_dir / "read.json").write_text(
            json.dumps(list(self._read_ids), ensure_ascii=False),
            encoding="utf-8",
        )

    def _find_contact_by_user_id(self, user_id: str) -> dict | None:
        for alias, data in self._contacts.items():
            if data.get("user_id") == user_id:
                return {"alias": alias, **data}
        return None

    def _get_loop(self) -> asyncio.AbstractEventLoop:
        try:
            return asyncio.get_event_loop()
        except RuntimeError:
            loop = asyncio.new_event_loop()
            asyncio.set_event_loop(loop)
            return loop


def _chunk_text(text: str, limit: int) -> list[str]:
    """Split text into chunks of at most `limit` characters."""
    if len(text) <= limit:
        return [text]
    chunks = []
    while text:
        chunks.append(text[:limit])
        text = text[limit:]
    return chunks
```

- [ ] **Step 2: Write manager tests**

Create `tests/test_wechat_manager.py`:

```python
import json
import os
from pathlib import Path

from lingtai.addons.wechat.manager import WechatManager, _chunk_text


def test_chunk_text_short():
    assert _chunk_text("hello", 4000) == ["hello"]


def test_chunk_text_long():
    text = "a" * 8500
    chunks = _chunk_text(text, 4000)
    assert len(chunks) == 3
    assert chunks[0] == "a" * 4000
    assert chunks[1] == "a" * 4000
    assert chunks[2] == "a" * 500
    assert "".join(chunks) == text


def test_handle_contacts_empty(tmp_path):
    """Manager contacts start empty and can be added."""

    class FakeAgent:
        _working_dir = str(tmp_path)
        inbox = None
        def _wake_nap(self, reason): pass
        def add_tool(self, *a, **kw): pass

    mgr = WechatManager(
        agent=FakeAgent(),
        token="fake_token",
        user_id="bot@im.bot",
        working_dir=tmp_path,
    )

    result = mgr.handle({"action": "contacts"})
    assert result == {"contacts": {}}


def test_handle_add_remove_contact(tmp_path):
    class FakeAgent:
        _working_dir = str(tmp_path)
        inbox = None
        def _wake_nap(self, reason): pass
        def add_tool(self, *a, **kw): pass

    mgr = WechatManager(
        agent=FakeAgent(),
        token="fake_token",
        user_id="bot@im.bot",
        working_dir=tmp_path,
    )

    # Add contact
    result = mgr.handle({
        "action": "add_contact",
        "user_id": "wxid_abc@im.wechat",
        "alias": "Alice",
    })
    assert result["status"] == "ok"

    # Verify in contacts
    result = mgr.handle({"action": "contacts"})
    assert "Alice" in result["contacts"]
    assert result["contacts"]["Alice"]["user_id"] == "wxid_abc@im.wechat"

    # Remove contact
    result = mgr.handle({"action": "remove_contact", "alias": "Alice"})
    assert result["status"] == "ok"

    result = mgr.handle({"action": "contacts"})
    assert result == {"contacts": {}}


def test_handle_check_empty(tmp_path):
    class FakeAgent:
        _working_dir = str(tmp_path)
        inbox = None
        def _wake_nap(self, reason): pass
        def add_tool(self, *a, **kw): pass

    mgr = WechatManager(
        agent=FakeAgent(),
        token="fake_token",
        user_id="bot@im.bot",
        working_dir=tmp_path,
    )

    result = mgr.handle({"action": "check"})
    assert result == {"conversations": []}


def test_handle_unknown_action(tmp_path):
    class FakeAgent:
        _working_dir = str(tmp_path)
        inbox = None
        def _wake_nap(self, reason): pass
        def add_tool(self, *a, **kw): pass

    mgr = WechatManager(
        agent=FakeAgent(),
        token="fake_token",
        user_id="bot@im.bot",
        working_dir=tmp_path,
    )

    result = mgr.handle({"action": "invalid"})
    assert "error" in result


def test_handle_send_missing_user_id(tmp_path):
    class FakeAgent:
        _working_dir = str(tmp_path)
        inbox = None
        def _wake_nap(self, reason): pass
        def add_tool(self, *a, **kw): pass

    mgr = WechatManager(
        agent=FakeAgent(),
        token="fake_token",
        user_id="bot@im.bot",
        working_dir=tmp_path,
    )

    result = mgr.handle({"action": "send", "text": "hello"})
    assert "error" in result
    assert "user_id" in result["error"]


def test_state_persistence(tmp_path):
    class FakeAgent:
        _working_dir = str(tmp_path)
        inbox = None
        def _wake_nap(self, reason): pass
        def add_tool(self, *a, **kw): pass

    mgr = WechatManager(
        agent=FakeAgent(),
        token="fake_token",
        user_id="bot@im.bot",
        working_dir=tmp_path,
    )

    # Add contact and save
    mgr.handle({
        "action": "add_contact",
        "user_id": "wxid@im.wechat",
        "alias": "Bob",
    })
    mgr._save_state()

    # Create new manager — should load persisted contacts
    mgr2 = WechatManager(
        agent=FakeAgent(),
        token="fake_token",
        user_id="bot@im.bot",
        working_dir=tmp_path,
    )
    result = mgr2.handle({"action": "contacts"})
    assert "Bob" in result["contacts"]
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_wechat_manager.py -v`
Expected: All 7 tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/addons/wechat/manager.py tests/test_wechat_manager.py
git commit -m "feat(addon): add wechat manager with tool dispatch and message persistence"
```

---

### Task 6: Setup Entry Point + Registration

**Files:**
- Modify: `src/lingtai/addons/__init__.py`
- Modify: `src/lingtai/addons/wechat/__init__.py`

- [ ] **Step 1: Write the setup entry point**

Replace `src/lingtai/addons/wechat/__init__.py` with:

```python
"""WeChat addon — iLink Bot API integration.

Connects agents to WeChat via QR code login. Supports text,
images, voice (with transcription), video, and files.

Usage (config file):
    agent = Agent(
        addons={"wechat": {"config": ".lingtai/.addons/wechat/config.json"}},
    )

Prerequisites:
    1. Run the login command to scan a QR code:
       python -c "from lingtai.addons.wechat.login import cli_login; cli_login('.lingtai/.addons/wechat')"
    2. credentials.json is created automatically after scanning.
"""
from __future__ import annotations

import json
import logging
from pathlib import Path
from typing import TYPE_CHECKING

from .manager import WechatManager, SCHEMA, DESCRIPTION

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    config: str | Path | None = None,
    base_url: str | None = None,
    cdn_base_url: str | None = None,
    bot_token: str | None = None,
    user_id: str | None = None,
    poll_interval: float | None = None,
    allowed_users: list[str] | None = None,
    **kwargs,
) -> WechatManager:
    """Set up WeChat addon — registers wechat tool, starts long-poll.

    Args:
        config: Path to config.json. Credentials are loaded from
                credentials.json in the same directory.

    Listeners are NOT started here — they start in WechatManager.start(),
    which is called by Agent.start() via the addon lifecycle.
    """
    from . import api

    if config is not None:
        config_path = Path(config)
        if not config_path.is_file():
            raise FileNotFoundError(f"WeChat config not found: {config_path}")

        file_cfg = json.loads(config_path.read_text(encoding="utf-8"))

        # Load credentials from sibling file
        creds_path = config_path.parent / "credentials.json"
        if not creds_path.is_file():
            raise FileNotFoundError(
                f"WeChat credentials not found: {creds_path}. "
                "Run the login command first: "
                'python -c "from lingtai.addons.wechat.login import cli_login; '
                f"cli_login('{config_path.parent}')\""
            )
        creds = json.loads(creds_path.read_text(encoding="utf-8"))

        if base_url is None:
            base_url = creds.get("base_url") or file_cfg.get(
                "base_url", api.DEFAULT_BASE_URL
            )
        if cdn_base_url is None:
            cdn_base_url = file_cfg.get("cdn_base_url", api.CDN_BASE_URL)
        if bot_token is None:
            bot_token = creds.get("bot_token")
        if user_id is None:
            user_id = creds.get("user_id")
        if poll_interval is None:
            poll_interval = file_cfg.get("poll_interval", 1.0)
        if allowed_users is None:
            allowed_users = file_cfg.get("allowed_users", [])

    if not bot_token:
        raise ValueError(
            "WeChat addon requires a bot_token. "
            "Run the login command to authenticate via QR code."
        )
    if not user_id:
        raise ValueError("WeChat addon requires a user_id from login.")

    working_dir = Path(agent._working_dir)

    mgr = WechatManager(
        agent=agent,
        base_url=base_url or api.DEFAULT_BASE_URL,
        cdn_base_url=cdn_base_url or api.CDN_BASE_URL,
        token=bot_token,
        user_id=user_id,
        poll_interval=poll_interval or 1.0,
        allowed_users=allowed_users if allowed_users else None,
        working_dir=working_dir,
    )

    agent.add_tool(
        "wechat", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )

    log.info("WeChat addon configured for %s", user_id)
    return mgr
```

- [ ] **Step 2: Register in _BUILTIN**

In `src/lingtai/addons/__init__.py`, add `"wechat"` to the `_BUILTIN` dict:

Change:
```python
_BUILTIN: dict[str, str] = {
    "imap": ".imap",
    "telegram": ".telegram",
    "feishu": ".feishu",
}
```

to:
```python
_BUILTIN: dict[str, str] = {
    "imap": ".imap",
    "telegram": ".telegram",
    "feishu": ".feishu",
    "wechat": ".wechat",
}
```

- [ ] **Step 3: Verify module imports cleanly**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "import lingtai.addons.wechat; print('ok')"`
Expected: `ok`

- [ ] **Step 4: Run all wechat tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_wechat_types.py tests/test_wechat_api.py tests/test_wechat_manager.py -v`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/addons/__init__.py src/lingtai/addons/wechat/__init__.py
git commit -m "feat(addon): register wechat addon with setup entry point"
```

---

### Task 7: Smoke Test

- [ ] **Step 1: Verify full import chain**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
python -c "
from lingtai.addons.wechat.types import WeixinMessage, MessageItemType
from lingtai.addons.wechat.api import get_qrcode, get_updates, send_message
from lingtai.addons.wechat.login import cli_login
from lingtai.addons.wechat.media import detect_upload_type, download_media
from lingtai.addons.wechat.manager import WechatManager, SCHEMA, DESCRIPTION
from lingtai.addons import setup_addon
print('All imports OK')
print(f'Tool schema actions: {SCHEMA[\"properties\"][\"action\"][\"enum\"]}')
print(f'Media type detection: .jpg={detect_upload_type(\"test.jpg\")}, .pdf={detect_upload_type(\"test.pdf\")}')
"
```

Expected:
```
All imports OK
Tool schema actions: ['send', 'check', 'read', 'reply', 'search', 'contacts', 'add_contact', 'remove_contact']
Media type detection: .jpg=UploadMediaType.IMAGE, .pdf=UploadMediaType.FILE
```

- [ ] **Step 2: Run all tests**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_wechat_*.py -v
```

Expected: All tests pass.
