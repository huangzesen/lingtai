"""Listen capability — speech transcription and music appreciation.

Transcription is backed by a pluggable ``TranscriptionService`` (default:
WhisperTranscriptionService — local, free). Music appreciation uses
librosa and runs locally.
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING, Any

from lingtai_kernel.logging import get_logger

from ..i18n import t
from ..services.transcription import (
    TranscriptionService,
    create_transcription_service,
)

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

logger = get_logger()

PROVIDERS = {
    "providers": ["whisper", "gemini"],
    "default": "whisper",
}

def get_description(lang: str = "en") -> str:
    return t(lang, "listen.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "audio_path": {
                "type": "string",
                "description": t(lang, "listen.audio_path"),
            },
            "action": {
                "type": "string",
                "enum": ["transcribe", "appreciate"],
                "description": t(lang, "listen.action"),
            },
        },
        "required": ["audio_path", "action"],
    }



class ListenManager:
    """Manages audio transcription (via TranscriptionService) and appreciation (librosa)."""

    def __init__(
        self,
        *,
        working_dir: Path,
        transcription_service: TranscriptionService | None = None,
    ) -> None:
        self._working_dir = working_dir
        self._transcription_service = transcription_service
        self._librosa = None

    def handle(self, args: dict) -> dict:
        audio_path = args.get("audio_path")
        if not audio_path:
            return {"status": "error", "message": "Missing required parameter: audio_path"}

        action = args.get("action")
        if action not in ("transcribe", "appreciate"):
            return {"status": "error", "message": "action must be 'transcribe' or 'appreciate'"}

        path = Path(audio_path)
        if not path.is_absolute():
            path = self._working_dir / path

        if not path.is_file():
            return {"status": "error", "message": f"Audio file not found: {path}"}

        if action == "transcribe":
            return self._transcribe(path)
        return self._appreciate(path)

    # ------------------------------------------------------------------
    # Transcribe — delegates to TranscriptionService
    # ------------------------------------------------------------------

    def _transcribe(self, path: Path) -> dict:
        try:
            svc = self._ensure_transcription_service()
        except Exception as exc:
            return {"status": "error", "message": f"Failed to load transcription service: {exc}"}

        try:
            result = svc.transcribe(path)
        except Exception as exc:
            return {"status": "error", "message": f"Transcription failed: {exc}"}

        return {
            "status": "ok",
            "action": "transcribe",
            "language": result.language,
            "language_probability": result.language_probability,
            "duration": result.duration,
            "text": result.text,
            "segments": result.segments,
        }

    def _ensure_transcription_service(self) -> TranscriptionService:
        """Lazy-create default WhisperTranscriptionService if none was injected."""
        if self._transcription_service is None:
            self._transcription_service = create_transcription_service("whisper")
        return self._transcription_service

    # ------------------------------------------------------------------
    # Appreciate — librosa
    # ------------------------------------------------------------------

    def _appreciate(self, path: Path) -> dict:
        try:
            librosa = self._get_librosa()
        except Exception as exc:
            return {"status": "error", "message": f"Failed to load librosa: {exc}"}

        import numpy as np

        try:
            y, sr = librosa.load(str(path))
        except Exception as exc:
            return {"status": "error", "message": f"Failed to load audio: {exc}"}

        duration = float(librosa.get_duration(y=y, sr=sr))

        # --- Tempo & Beat ---
        tempo, beats = librosa.beat.beat_track(y=y, sr=sr)
        tempo_val = float(np.atleast_1d(tempo)[0])
        beat_times = librosa.frames_to_time(beats, sr=sr).tolist()
        beat_regularity = float(np.std(np.diff(beat_times))) if len(beat_times) > 1 else None

        # --- Key estimation (Krumhansl profiles) ---
        chroma = librosa.feature.chroma_cqt(y=y, sr=sr)
        chroma_avg = np.mean(chroma, axis=1)
        notes = ["C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"]

        major_profile = np.array([6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88])
        minor_profile = np.array([6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17])

        best_major = max(
            ((np.corrcoef(np.roll(chroma_avg, -s), major_profile)[0, 1], s) for s in range(12)),
            key=lambda x: x[0],
        )
        best_minor = max(
            ((np.corrcoef(np.roll(chroma_avg, -s), minor_profile)[0, 1], s) for s in range(12)),
            key=lambda x: x[0],
        )

        if best_major[0] > best_minor[0]:
            key = f"{notes[best_major[1]]} major"
            key_confidence = round(best_major[0], 2)
        else:
            key = f"{notes[best_minor[1]]} minor"
            key_confidence = round(best_minor[0], 2)

        # --- Spectral features ---
        spectral_centroid = float(np.mean(librosa.feature.spectral_centroid(y=y, sr=sr)))
        spectral_bandwidth = float(np.mean(librosa.feature.spectral_bandwidth(y=y, sr=sr)))
        spectral_rolloff = float(np.mean(librosa.feature.spectral_rolloff(y=y, sr=sr)))
        zcr = float(np.mean(librosa.feature.zero_crossing_rate(y)))

        # --- Dynamics ---
        rms = librosa.feature.rms(y=y)[0]
        rms_nonzero = rms[rms > 0]
        dynamic_range = float(20 * np.log10(np.max(rms) / (np.min(rms_nonzero) + 1e-10))) if len(rms_nonzero) > 0 else 0.0

        # --- Energy contour (10 segments) ---
        n_segments = 10
        seg_len = len(y) // n_segments
        energy_contour = []
        for i in range(n_segments):
            seg = y[i * seg_len : (i + 1) * seg_len]
            seg_rms = float(np.sqrt(np.mean(seg**2)))
            energy_contour.append({
                "start": round(i * seg_len / sr, 1),
                "end": round((i + 1) * seg_len / sr, 1),
                "rms": round(seg_rms, 6),
            })

        # --- Frequency band energy ---
        N = len(y)
        fft = np.fft.rfft(y)
        freqs = np.fft.rfftfreq(N, 1 / sr)
        magnitude = np.abs(fft) / N
        total_energy = float(np.sum(magnitude**2))

        bands = {}
        band_ranges = [
            ("sub_bass", 20, 60),
            ("bass", 60, 250),
            ("low_mid", 250, 500),
            ("mid", 500, 2000),
            ("upper_mid", 2000, 4000),
            ("presence", 4000, 6000),
            ("brilliance", 6000, 16000),
        ]
        for name, lo, hi in band_ranges:
            mask = (freqs >= lo) & (freqs < hi)
            band_energy = float(np.sum(magnitude[mask] ** 2))
            bands[name] = round(100 * band_energy / total_energy, 1) if total_energy > 0 else 0.0

        # --- Onsets ---
        onsets = librosa.onset.onset_detect(y=y, sr=sr)
        onset_density = round(len(onsets) / duration, 1) if duration > 0 else 0.0

        return {
            "status": "ok",
            "action": "appreciate",
            "duration": round(duration, 1),
            "tempo_bpm": round(tempo_val, 0),
            "beat_regularity_std": round(beat_regularity, 3) if beat_regularity is not None else None,
            "key": key,
            "key_confidence": key_confidence,
            "chroma_profile": {notes[i]: round(float(chroma_avg[i]), 3) for i in range(12)},
            "spectral_centroid_hz": round(spectral_centroid, 0),
            "spectral_bandwidth_hz": round(spectral_bandwidth, 0),
            "spectral_rolloff_hz": round(spectral_rolloff, 0),
            "zero_crossing_rate": round(zcr, 4),
            "dynamic_range_db": round(dynamic_range, 1),
            "frequency_bands_pct": bands,
            "energy_contour": energy_contour,
            "onset_density_per_sec": onset_density,
        }

    def _get_librosa(self):
        if self._librosa is None:
            from lingtai.venv_resolve import ensure_package
            ensure_package("librosa")
            import librosa
            self._librosa = librosa
        return self._librosa


def setup(
    agent: "BaseAgent",
    *,
    provider: str = "whisper",
    api_key: str | None = None,
    transcription_service: TranscriptionService | None = None,
    **kwargs: Any,
) -> ListenManager:
    """Set up the listen capability on an agent.

    Args:
        agent: The agent to attach the capability to.
        provider: Transcription provider — ``"whisper"`` (default, local)
            or ``"gemini"`` (cloud).
        api_key: API key for cloud providers (e.g., Gemini).
        transcription_service: Pre-built service instance. If provided,
            ``provider`` and ``api_key`` are ignored.
        **kwargs: Extra kwargs forwarded to the transcription service
            constructor (e.g., ``model_size``, ``device``).
    """
    lang = agent._config.language

    # Build transcription service if not injected
    if transcription_service is None:
        svc_kwargs = dict(kwargs)
        if api_key is not None:
            svc_kwargs["api_key"] = api_key
        transcription_service = create_transcription_service(provider, **svc_kwargs)

    mgr = ListenManager(
        working_dir=agent.working_dir,
        transcription_service=transcription_service,
    )
    agent.add_tool("listen", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang))
    return mgr
