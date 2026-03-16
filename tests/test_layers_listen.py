"""Tests for the listen capability."""
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from stoai.capabilities.listen import ListenManager, setup as setup_listen


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    return agent


class TestListenManagerTranscribe:
    def test_transcribe_success(self, tmp_path):
        audio_file = tmp_path / "speech.mp3"
        audio_file.write_bytes(b"FAKE_AUDIO")

        mock_segment = MagicMock()
        mock_segment.start = 0.0
        mock_segment.end = 2.5
        mock_segment.text = " Hello world"

        mock_info = MagicMock()
        mock_info.language = "en"
        mock_info.language_probability = 0.99
        mock_info.duration = 2.5

        mock_model = MagicMock()
        mock_model.transcribe.return_value = ([mock_segment], mock_info)

        mgr = ListenManager(working_dir=tmp_path)
        mgr._whisper_model = mock_model

        result = mgr.handle({"audio_path": str(audio_file), "action": "transcribe"})
        assert result["status"] == "ok"
        assert result["action"] == "transcribe"
        assert result["text"] == "Hello world"
        assert result["language"] == "en"
        assert len(result["segments"]) == 1

    def test_transcribe_relative_path(self, tmp_path):
        audio_file = tmp_path / "audio" / "test.mp3"
        audio_file.parent.mkdir()
        audio_file.write_bytes(b"FAKE")

        mock_model = MagicMock()
        mock_model.transcribe.return_value = ([], MagicMock(
            language="en", language_probability=0.9, duration=1.0,
        ))

        mgr = ListenManager(working_dir=tmp_path)
        mgr._whisper_model = mock_model

        result = mgr.handle({"audio_path": "audio/test.mp3", "action": "transcribe"})
        assert result["status"] == "ok"

    def test_transcribe_file_not_found(self, tmp_path):
        mgr = ListenManager(working_dir=tmp_path)
        result = mgr.handle({"audio_path": "/nonexistent/file.mp3", "action": "transcribe"})
        assert result["status"] == "error"
        assert "not found" in result["message"]

    def test_transcribe_model_load_failure(self, tmp_path):
        audio_file = tmp_path / "test.mp3"
        audio_file.write_bytes(b"FAKE")

        mgr = ListenManager(working_dir=tmp_path)
        with patch.object(mgr, "_get_whisper_model", side_effect=ImportError("no module")):
            result = mgr.handle({"audio_path": str(audio_file), "action": "transcribe"})
        assert result["status"] == "error"
        assert "Whisper" in result["message"]


class TestListenManagerAppreciate:
    def test_appreciate_success(self, tmp_path):
        audio_file = tmp_path / "music.mp3"
        audio_file.write_bytes(b"FAKE")

        import numpy as np
        mock_librosa = MagicMock()
        mock_librosa.load.return_value = (np.random.randn(22050 * 5).astype(np.float32), 22050)
        mock_librosa.get_duration.return_value = 5.0
        mock_librosa.beat.beat_track.return_value = (np.array([120.0]), np.array([10, 20, 30]))
        mock_librosa.frames_to_time.return_value = np.array([0.5, 1.0, 1.5])
        mock_librosa.feature.chroma_cqt.return_value = np.random.rand(12, 100)
        mock_librosa.feature.spectral_centroid.return_value = np.array([[2000.0]])
        mock_librosa.feature.spectral_bandwidth.return_value = np.array([[1500.0]])
        mock_librosa.feature.spectral_rolloff.return_value = np.array([[4000.0]])
        mock_librosa.feature.zero_crossing_rate.return_value = np.array([[0.05]])
        mock_librosa.feature.rms.return_value = np.array([[0.01, 0.05, 0.1]])
        mock_librosa.onset.onset_detect.return_value = np.array([1, 5, 10, 15, 20])

        mgr = ListenManager(working_dir=tmp_path)
        mgr._librosa = mock_librosa

        result = mgr.handle({"audio_path": str(audio_file), "action": "appreciate"})
        assert result["status"] == "ok"
        assert result["action"] == "appreciate"
        assert "tempo_bpm" in result
        assert "key" in result
        assert "frequency_bands_pct" in result
        assert "energy_contour" in result
        assert "spectral_centroid_hz" in result

    def test_appreciate_file_not_found(self, tmp_path):
        mgr = ListenManager(working_dir=tmp_path)
        result = mgr.handle({"audio_path": "/nonexistent/file.mp3", "action": "appreciate"})
        assert result["status"] == "error"
        assert "not found" in result["message"]

    def test_appreciate_librosa_load_failure(self, tmp_path):
        audio_file = tmp_path / "bad.mp3"
        audio_file.write_bytes(b"FAKE")

        mgr = ListenManager(working_dir=tmp_path)
        with patch.object(mgr, "_get_librosa", side_effect=ImportError("no librosa")):
            result = mgr.handle({"audio_path": str(audio_file), "action": "appreciate"})
        assert result["status"] == "error"
        assert "librosa" in result["message"]


class TestListenManagerValidation:
    def test_missing_audio_path(self, tmp_path):
        mgr = ListenManager(working_dir=tmp_path)
        result = mgr.handle({"action": "transcribe"})
        assert result["status"] == "error"
        assert "audio_path" in result["message"]

    def test_invalid_action(self, tmp_path):
        audio_file = tmp_path / "test.mp3"
        audio_file.write_bytes(b"FAKE")
        mgr = ListenManager(working_dir=tmp_path)
        result = mgr.handle({"audio_path": str(audio_file), "action": "invalid"})
        assert result["status"] == "error"
        assert "action" in result["message"]


class TestSetupListen:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mgr = setup_listen(agent)
        assert isinstance(mgr, ListenManager)
        agent.add_tool.assert_called_once()

    def test_setup_no_mcp_needed(self, tmp_path):
        """Listen runs locally — no mcp_client required."""
        agent = make_mock_agent(tmp_path)
        mgr = setup_listen(agent)
        assert isinstance(mgr, ListenManager)
