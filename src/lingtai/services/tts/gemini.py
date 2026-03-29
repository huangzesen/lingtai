"""GeminiTTSService — text-to-speech via Gemini's native speech generation."""
from __future__ import annotations

import hashlib
import io
import time
import wave
from pathlib import Path

from lingtai_kernel.logging import get_logger

from . import TTSService

logger = get_logger()


class GeminiTTSService(TTSService):
    """TTS via Gemini's speech generation models.

    Creates its own ``google.genai.Client``.  No dependency on the
    Gemini *adapter* — this is a standalone service.

    Args:
        api_key: Google AI API key.
        model: Gemini model to use for TTS.
            Default: ``gemini-2.5-flash-preview-tts``.
        voice: Default voice name.  One of Puck, Charon, Kore, Fenrir,
            Aoede, Leda, Orus, Zephyr.  Default: ``Charon``.
    """

    def __init__(
        self,
        *,
        api_key: str | None = None,
        model: str = "gemini-2.5-flash-preview-tts",
        voice: str = "Charon",
    ) -> None:
        from google import genai
        from google.genai import types

        self._client = genai.Client(api_key=api_key)
        self._types = types
        self._model = model
        self._default_voice = voice

    def synthesize(
        self,
        text: str,
        *,
        voice: str | None = None,
        output_dir: Path | None = None,
        **kwargs: object,
    ) -> Path:
        """Synthesize speech via Gemini's speech generation.

        Args:
            text: Text to convert to speech.
            voice: Voice name (Puck, Charon, Kore, Fenrir, Aoede,
                Leda, Orus, Zephyr).  Falls back to the default set
                at construction.
            output_dir: Directory to save the WAV file.
            **kwargs: Extra params (``model`` overrides the default).

        Returns:
            Path to the saved WAV file.

        Raises:
            RuntimeError: If Gemini returns no audio data.
        """
        from google.genai import types

        if output_dir is None:
            output_dir = Path.cwd() / "media" / "audio"
        output_dir.mkdir(parents=True, exist_ok=True)

        effective_voice = voice or self._default_voice
        effective_model = str(kwargs.get("model", self._model))

        types = self._types
        speech_config = types.SpeechConfig(
            voice_config=types.VoiceConfig(
                prebuilt_voice_config=types.PrebuiltVoiceConfig(
                    voice_name=effective_voice,
                ),
            ),
        )

        raw = self._client.models.generate_content(
            model=effective_model,
            contents=text,
            config=types.GenerateContentConfig(
                response_modalities=["AUDIO"],
                speech_config=speech_config,
            ),
        )

        # Extract raw PCM and wrap in WAV header
        if raw.candidates:
            for part in raw.candidates[0].content.parts:
                if part.inline_data and part.inline_data.data:
                    pcm = part.inline_data.data
                    # Parse sample rate from mime_type
                    # (audio/L16;codec=pcm;rate=24000)
                    mime = part.inline_data.mime_type or ""
                    rate = 24000
                    for token in mime.split(";"):
                        if token.strip().startswith("rate="):
                            rate = int(token.strip().split("=")[1])
                    buf = io.BytesIO()
                    with wave.open(buf, "wb") as wf:
                        wf.setnchannels(1)
                        wf.setsampwidth(2)  # 16-bit
                        wf.setframerate(rate)
                        wf.writeframes(pcm)
                    wav_bytes = buf.getvalue()

                    ts = int(time.time())
                    short_hash = hashlib.md5(text.encode()).hexdigest()[:4]
                    filename = f"talk_{ts}_{short_hash}.wav"
                    out_path = output_dir / filename
                    out_path.write_bytes(wav_bytes)
                    return out_path

        raise RuntimeError("Gemini TTS returned no audio data")
