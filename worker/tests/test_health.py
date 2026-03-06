from __future__ import annotations

import pytest

from voxi_worker.health import detect_device


def test_detect_device_env_override(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("VOXI_WORKER_DEVICE", "cuda:0")
    assert detect_device() == "cuda:0"
