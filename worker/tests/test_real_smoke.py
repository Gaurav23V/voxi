from __future__ import annotations

import base64
import os

import pytest

from voxi_worker.asr import build_asr_adapter


@pytest.mark.skipif(
    os.getenv("VOXI_RUN_REAL_MODEL_SMOKE") != "1",
    reason="set VOXI_RUN_REAL_MODEL_SMOKE=1 to run real-model smoke tests",
)
def test_real_parakeet_smoke() -> None:
    audio_b64 = os.environ["VOXI_REAL_SMOKE_AUDIO_B64"]
    adapter = build_asr_adapter("nvidia/parakeet-tdt-0.6b-v2")
    transcript = adapter.transcribe(base64.b64decode(audio_b64), 16000, "pcm_s16le")
    assert transcript.strip()
