from __future__ import annotations

import importlib
import importlib.util
import sys

import pytest

from voxi_worker.asr import ensure_optional_runtime_shims


def test_optional_runtime_shim_exposes_time_event_callback(monkeypatch: pytest.MonkeyPatch) -> None:
    if importlib.util.find_spec("nv_one_logger.training_telemetry") is None:
        pytest.skip("nv_one_logger training telemetry package is not installed")

    target = "nv_one_logger.training_telemetry.integration.pytorch_lightning"
    integration_pkg = "nv_one_logger.training_telemetry.integration"
    monkeypatch.delitem(sys.modules, target, raising=False)
    monkeypatch.delitem(sys.modules, integration_pkg, raising=False)

    ensure_optional_runtime_shims()

    module = importlib.import_module(target)
    assert hasattr(module, "TimeEventCallback")
    module.TimeEventCallback()


def test_optional_runtime_shim_keeps_onnx_importable() -> None:
    ensure_optional_runtime_shims()
    importlib.import_module("onnx")
