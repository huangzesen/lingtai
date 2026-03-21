# Media Capabilities & Mail Attachments Design

**Date:** 2026-03-15
**Status:** Draft

## Overview

Add four new media capabilities (draw, compose, talk, listen) and filesystem-based mail attachments to 灵台. Media capabilities follow the capability pattern (opt-in via `add_capability()`), routing through LLMService/LLMAdapter methods. Mail attachments extend the existing mail intrinsic with inline file transfer and a filesystem-based mailbox.

## Part 1: Media Capabilities

### Four New Capabilities

| Capability | Tool name | What it does | Output |
|-----------|-----------|-------------|--------|
| draw | `draw` | Text-to-image generation | Image file path |
| compose | `compose` | Music generation from prompt | Audio file path |
| talk | `talk` | Text-to-speech | Audio file path |
| listen | `listen` | Speech transcription + audio analysis | Text |

### Capability Modules

Four new files in `capabilities/`:

- `capabilities/draw.py` — `setup(agent, **kwargs)`
- `capabilities/compose.py` — `setup(agent, **kwargs)`
- `capabilities/talk.py` — `setup(agent, **kwargs)`
- `capabilities/listen.py` — `setup(agent, **kwargs)`

Each `setup()` registers a tool via `agent.add_tool()` and injects a system prompt section via `agent.update_system_prompt()`.

**Registration in `capabilities/__init__.py`:**

```python
_BUILTIN = {
    "bash": ...,
    "delegate": ...,
    "email": ...,
    "draw": ".draw",
    "compose": ".compose",
    "talk": ".talk",
    "listen": ".listen",
}
```

**Usage:**

```python
agent.add_capability("draw")
agent.add_capability("draw", "compose", "talk", "listen")  # multiple at once
```

### Tool Schemas

**draw:**
```json
{
  "type": "object",
  "properties": {
    "prompt": { "type": "string", "description": "Description of the image to generate" }
  },
  "required": ["prompt"]
}
```

**compose:**
```json
{
  "type": "object",
  "properties": {
    "prompt": { "type": "string", "description": "Description of the music to generate" },
    "duration_seconds": { "type": "number", "description": "Desired duration in seconds" }
  },
  "required": ["prompt"]
}
```

**talk:**
```json
{
  "type": "object",
  "properties": {
    "text": { "type": "string", "description": "Text to convert to speech" }
  },
  "required": ["text"]
}
```

**listen:**
```json
{
  "type": "object",
  "properties": {
    "audio_path": { "type": "string", "description": "Path to the audio file" },
    "mode": { "type": "string", "enum": ["transcribe", "analyze"], "description": "Transcribe speech or analyze audio content", "default": "transcribe" },
    "prompt": { "type": "string", "description": "Question about the audio (for analyze mode)" }
  },
  "required": ["audio_path"]
}
```

### Handler Pattern

No fallback chain. One path — call the LLM adapter method, succeed or fail:

1. Call `agent.service.<method>()` (routes to configured provider via LLMService)
2. If provider not configured → LLMService raises `RuntimeError`; capability catches and returns `{ status: "error", message: "No provider configured for ..." }`
3. If adapter method raises `NotImplementedError` → capability catches and returns `{ status: "error", message: "Provider does not support ..." }`
4. Success → save file to `media/` subfolder, return `{ status: "ok", file_path: "..." }`

**Path resolution:** `listen` resolves `audio_path` relative to `agent.working_dir`, same as vision resolves `image_path` relative to `_working_dir`.

**`BaseAgent.working_dir` property:** A new public `working_dir: Path` property must be added to `BaseAgent` so capability modules can access the agent's working directory without using private attributes.

### Media File Output Structure

Generated files are saved under the agent's working directory:

```
<working_dir>/
  media/
    images/       <- draw output
    music/        <- compose output
    audio/        <- talk (TTS) output
```

**File naming:** `{tool}_{timestamp}_{short_hash}.{ext}` (e.g., `draw_20260315_a3f2.png`)

Directories are auto-created on first use.

### LLM Layer Extensions

**LLMAdapter (base.py)** — new methods, all raise `NotImplementedError` in the base class (providers override to implement):

```python
def generate_image(self, prompt: str, model: str) -> bytes:
    """Text-to-image. Returns image bytes (PNG)."""
    raise NotImplementedError

def generate_music(self, prompt: str, model: str, duration_seconds: float | None = None) -> bytes:
    """Text-to-music. Returns audio bytes."""
    raise NotImplementedError

def text_to_speech(self, text: str, model: str) -> bytes:
    """TTS. Returns audio bytes."""
    raise NotImplementedError

def transcribe(self, audio_bytes: bytes, model: str) -> str:
    """Speech-to-text. Returns transcription."""
    raise NotImplementedError

def analyze_audio(self, audio_bytes: bytes, prompt: str, model: str) -> str:
    """Audio analysis. Returns text description."""
    raise NotImplementedError
```

**LLMService (service.py)** — new gateway methods, same pattern as `generate_vision()` / `web_search()`:

```python
def generate_image(self, prompt: str) -> bytes:
    """Route to configured image_provider."""

def generate_music(self, prompt: str, duration_seconds: float | None = None) -> bytes:
    """Route to configured music_provider."""

def text_to_speech(self, text: str) -> bytes:
    """Route to configured tts_provider."""

def transcribe(self, audio_bytes: bytes) -> str:
    """Route to configured audio_provider."""

def analyze_audio(self, audio_bytes: bytes, prompt: str) -> str:
    """Route to configured audio_provider."""
```

Each resolves a provider name from config (e.g., `image_provider`, `music_provider`, `tts_provider`, `audio_provider`), gets the adapter, and calls the method. Raises `RuntimeError` if the provider is not configured. Model is resolved from `provider_defaults[provider_name]["model"]`, matching the existing `generate_vision()` / `web_search()` pattern.

## Part 2: Mail Attachments

### Message Model

Add `attachments` field to the mail message:

```python
@dataclass
class MailMessage:
    sender: str
    recipient: str
    body: str
    attachments: list[str] = field(default_factory=list)  # file paths on sender side
```

### Filesystem-Based Mailbox

Each received message is persisted as a folder:

```
<working_dir>/
  mailbox/
    <uuid4>/
      message.json         <- sender, body, timestamp, metadata
      attachments/
        draw_abc.png       <- actual file bytes, decoded and saved
    <uuid4>/
      message.json
      attachments/
        song.mp3
        image.png
```

Message directories use `uuid4` names for thread-safety (multiple TCP connections can deliver concurrently) and global uniqueness.

### Wire Protocol (TCP Transport)

Attachments are transferred inline via base64 encoding:

1. **Sender side:** MailService reads attachment files from sender's filesystem, base64-encodes the bytes, includes them in the serialized message alongside filenames.
2. **Wire format:** Extended message JSON includes an `attachments` array of `{ filename: str, data: str (base64) }`.
3. **Receiver side:** MailService decodes, creates `mailbox/<uuid>/attachments/` directory, saves files there. The delivered `MailMessage.attachments` contains the local paths (e.g., `mailbox/a3f2.../attachments/draw_abc.png`).

**Size constraint:** The current TCPMailService has a 10MB wire limit. This must be increased (e.g., to 100MB) to accommodate base64-encoded attachments. The `send()` method should return `False` if the message exceeds the limit, and the mail handler should report the error to the agent.

### Attachment Usage Convention

Files always remain in the mailbox. When an agent wants to use an attachment elsewhere:

- Create a **symlink** at the desired location pointing to the mailbox copy
- Example: `media/images/received_portrait.png -> mailbox/msg_001/attachments/portrait.png`
- The mailbox is the source of truth; symlinks are references

This convention is communicated to the agent via the mail system prompt section.

### Mail Intrinsic Schema Update

Add optional `attachments` parameter:

```json
{
  "attachments": {
    "type": "array",
    "items": { "type": "string" },
    "description": "List of file paths to attach to the message"
  }
}
```

### Email Capability Updates

The email capability inherits attachment support from mail. Updates:

- `send_email` tool gains `attachments` parameter
- `read_email` / inbox display shows attachment file paths
- `forward` carries attachments
- `reply` / `reply_all` can optionally include attachments

## No New Service ABCs

Unlike vision/search which have dedicated service ABCs, the four media capabilities route entirely through the LLM layer (LLMService -> LLMAdapter). No `DrawService`, `ComposeService`, `TTSService`, or `ListenService` ABCs. Provider implementations are LLM adapter methods.

## Files to Create/Modify

### New files:
- `src/lingtai/capabilities/draw.py`
- `src/lingtai/capabilities/compose.py`
- `src/lingtai/capabilities/talk.py`
- `src/lingtai/capabilities/listen.py`

### Modified files:
- `src/lingtai/agent.py` — add public `working_dir: Path` property
- `src/lingtai/capabilities/__init__.py` — register 4 new capabilities in `_BUILTIN`
- `src/lingtai/llm/base.py` — add 5 new LLMAdapter methods
- `src/lingtai/llm/service.py` — add 5 new LLMService gateway methods + 4 provider config keys
- `src/lingtai/services/mail.py` — add `attachments` to MailMessage, update MailService ABC, update TCPMailService wire protocol, add filesystem-based mailbox persistence
- `src/lingtai/intrinsics/mail.py` — add `attachments` to schema
- `src/lingtai/capabilities/email.py` — support attachments in send/read/forward/reply
- `src/lingtai/prompt.py` or agent system prompt sections — attachment symlink convention instructions
