from __future__ import annotations

from dataclasses import dataclass
from typing import Any


@dataclass(slots=True)
class WorkerRequest:
    id: str
    op: str
    audio_format: str | None = None
    sample_rate_hz: int | None = None
    audio_b64: str | None = None

    @classmethod
    def from_dict(cls, payload: dict[str, Any]) -> "WorkerRequest":
        return cls(
            id=str(payload.get("id", "")),
            op=str(payload.get("op", "")),
            audio_format=payload.get("audio_format"),
            sample_rate_hz=payload.get("sample_rate_hz"),
            audio_b64=payload.get("audio_b64"),
        )


@dataclass(slots=True)
class WorkerResponse:
    id: str
    ok: bool
    transcript: str | None = None
    cleaned: str | None = None
    stage: str | None = None
    code: str | None = None
    message: str | None = None
    device: str | None = None
    asr_model: str | None = None
    llm_model: str | None = None

    def to_dict(self) -> dict[str, Any]:
        payload: dict[str, Any] = {"id": self.id, "ok": self.ok}
        if self.transcript is not None:
            payload["transcript"] = self.transcript
        if self.cleaned is not None:
            payload["cleaned"] = self.cleaned
        if self.stage is not None:
            payload["stage"] = self.stage
        if self.code is not None:
            payload["code"] = self.code
        if self.message is not None:
            payload["message"] = self.message
        if self.device is not None:
            payload["device"] = self.device
        if self.asr_model is not None:
            payload["asr_model"] = self.asr_model
        if self.llm_model is not None:
            payload["llm_model"] = self.llm_model
        return payload
