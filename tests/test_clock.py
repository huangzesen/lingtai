"""Tests for clock intrinsic."""
from __future__ import annotations

import threading
import time
from unittest.mock import MagicMock

import pytest

from stoai.agent import BaseAgent
from stoai.intrinsics import ALL_INTRINSICS


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Registration
# ---------------------------------------------------------------------------


def test_clock_in_all_intrinsics():
    """Clock should be registered in ALL_INTRINSICS with handler=None."""
    assert "clock" in ALL_INTRINSICS
    info = ALL_INTRINSICS["clock"]
    assert "schema" in info
    assert "description" in info
    assert info["handler"] is None
