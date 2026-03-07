from __future__ import annotations

import base64
import importlib
import os
import sys
import tempfile
import types
import wave
from dataclasses import dataclass

from .health import detect_device


class WorkerError(RuntimeError):
    def __init__(self, stage: str, code: str, message: str) -> None:
        super().__init__(message)
        self.stage = stage
        self.code = code
        self.message = message


@dataclass(slots=True)
class ASRAdapter:
    model_name: str
    device: str

    def transcribe(self, audio_bytes: bytes, sample_rate_hz: int, audio_format: str) -> str:
        raise NotImplementedError


@dataclass(slots=True)
class FakeASRAdapter(ASRAdapter):
    transcript: str = "hello world"

    def transcribe(self, audio_bytes: bytes, sample_rate_hz: int, audio_format: str) -> str:
        behavior = os.getenv("VOXI_FAKE_ASR_BEHAVIOR", "success")
        if behavior == "timeout":
            raise WorkerError("speech_recognition", "ASR_TIMEOUT", "inference exceeded timeout")
        if behavior == "unavailable":
            raise WorkerError("speech_recognition", "ASR_MODEL_UNAVAILABLE", "model unavailable")
        if behavior == "empty":
            return ""
        if behavior == "echo-base64":
            return base64.b64encode(audio_bytes).decode("ascii")
        return os.getenv("VOXI_FAKE_ASR_TRANSCRIPT", self.transcript)


@dataclass(slots=True)
class ParakeetASRAdapter(ASRAdapter):
    _model: object | None = None

    def _load_model(self) -> object:
        if self._model is not None:
            return self._model

        ensure_optional_runtime_shims()

        try:
            import nemo.collections.asr as nemo_asr  # type: ignore
        except Exception as exc:  # pragma: no cover - exercised only in real-model mode
            raise WorkerError("speech_recognition", "ASR_MODEL_UNAVAILABLE", str(exc)) from exc

        try:
            self._model = nemo_asr.models.ASRModel.from_pretrained(model_name=self.model_name)
        except Exception as exc:  # pragma: no cover - exercised only in real-model mode
            raise WorkerError("speech_recognition", "ASR_MODEL_UNAVAILABLE", str(exc)) from exc

        return self._model

    def transcribe(self, audio_bytes: bytes, sample_rate_hz: int, audio_format: str) -> str:
        model = self._load_model()

        with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as temp_audio:
            temp_path = temp_audio.name
        try:
            if audio_format == "pcm_s16le":
                with wave.open(temp_path, "wb") as wav_file:
                    wav_file.setnchannels(1)
                    wav_file.setsampwidth(2)
                    wav_file.setframerate(sample_rate_hz)
                    wav_file.writeframes(audio_bytes)
            elif audio_format == "wav":
                with open(temp_path, "wb") as handle:
                    handle.write(audio_bytes)
            else:
                raise WorkerError("speech_recognition", "ASR_RUNTIME_FAILURE", f"unsupported audio format: {audio_format}")

            output = model.transcribe([temp_path])
            if not output:
                return ""
            first = output[0]
            return getattr(first, "text", first)
        except WorkerError:
            raise
        except Exception as exc:  # pragma: no cover - exercised only in real-model mode
            raise WorkerError("speech_recognition", "ASR_RUNTIME_FAILURE", str(exc)) from exc
        finally:
            try:
                os.remove(temp_path)
            except OSError:
                pass


def ensure_optional_runtime_shims() -> None:
    """Install lightweight shims for optional NeMo telemetry integrations.

    NeMo can import training-time OneLogger callbacks even for inference-only usage.
    Some wheels omit the optional integration module; in that case we provide a tiny
    no-op implementation so ASR model loading can proceed.
    """
    _ensure_onnx_stub()

    target_module = "nv_one_logger.training_telemetry.integration.pytorch_lightning"
    try:
        importlib.import_module(target_module)
        return
    except ModuleNotFoundError as exc:
        missing = getattr(exc, "name", "")
        if not missing.startswith("nv_one_logger.training_telemetry.integration"):
            return
    except Exception:
        return

    try:
        parent = importlib.import_module("nv_one_logger.training_telemetry")
    except Exception:
        return

    integration_pkg_name = "nv_one_logger.training_telemetry.integration"
    integration_pkg = sys.modules.get(integration_pkg_name)
    if integration_pkg is None:
        integration_pkg = types.ModuleType(integration_pkg_name)
        integration_pkg.__path__ = []  # mark as package-like module
        sys.modules[integration_pkg_name] = integration_pkg

    ptl_module = types.ModuleType(target_module)

    class TimeEventCallback:  # pragma: no cover - runtime compatibility shim
        def __init__(self, *args: object, **kwargs: object) -> None:
            pass

    setattr(ptl_module, "TimeEventCallback", TimeEventCallback)
    sys.modules[target_module] = ptl_module
    setattr(integration_pkg, "pytorch_lightning", ptl_module)
    setattr(parent, "integration", integration_pkg)


def _ensure_onnx_stub() -> None:
    try:
        importlib.import_module("onnx")
        return
    except Exception:
        pass

    sys.modules.pop("onnx", None)
    onnx_stub = types.ModuleType("onnx")
    onnx_stub.__dict__["__version__"] = "0.0-shim"
    sys.modules["onnx"] = onnx_stub


def build_asr_adapter(model_name: str) -> ASRAdapter:
    mode = os.getenv("VOXI_WORKER_MODE", "").lower()
    device = detect_device()
    if mode == "fake":
        return FakeASRAdapter(model_name=model_name, device=device)
    return ParakeetASRAdapter(model_name=model_name, device=device)
