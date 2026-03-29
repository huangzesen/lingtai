"""WhisperTranscriptionService — local speech-to-text via faster-whisper.

Runs entirely locally, no API key needed. Requires ``faster-whisper``
(``pip install faster-whisper``).
"""
from __future__ import annotations

from pathlib import Path

from . import TranscriptionResult, TranscriptionService


class WhisperTranscriptionService(TranscriptionService):
    """Local transcription using faster-whisper (CTranslate2 Whisper).

    The model is loaded lazily on first ``transcribe()`` call and kept
    in memory for subsequent calls.

    Args:
        model_size: Whisper model size (default ``"base"``).
            Options: tiny, base, small, medium, large-v2, large-v3, etc.
        device: Compute device (default ``"cpu"``). Use ``"cuda"`` for GPU.
        compute_type: CTranslate2 compute type (default ``"int8"``).
    """

    def __init__(
        self,
        *,
        model_size: str = "base",
        device: str = "cpu",
        compute_type: str = "int8",
    ) -> None:
        self._model_size = model_size
        self._device = device
        self._compute_type = compute_type
        self._model = None

    def transcribe(self, audio_path: Path, **kwargs) -> TranscriptionResult:
        """Transcribe audio using faster-whisper.

        Args:
            audio_path: Path to the audio file.
            **kwargs: Forwarded to ``WhisperModel.transcribe()``.

        Returns:
            TranscriptionResult with text, language, segments, etc.
        """
        model = self._ensure_model()
        segments, info = model.transcribe(str(audio_path), **kwargs)
        segments = list(segments)

        transcript = []
        for seg in segments:
            transcript.append({
                "start": round(seg.start, 1),
                "end": round(seg.end, 1),
                "text": seg.text.strip(),
            })

        full_text = " ".join(seg["text"] for seg in transcript)

        return TranscriptionResult(
            text=full_text,
            language=info.language,
            language_probability=round(info.language_probability, 2),
            duration=round(info.duration, 1),
            segments=transcript,
        )

    def _ensure_model(self):
        """Lazy-load the Whisper model."""
        if self._model is None:
            from lingtai.venv_resolve import ensure_package
            ensure_package("faster-whisper", "faster_whisper")
            from faster_whisper import WhisperModel

            self._model = WhisperModel(
                self._model_size,
                device=self._device,
                compute_type=self._compute_type,
            )
        return self._model
