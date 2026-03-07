from __future__ import annotations

import argparse
import base64
import io
import json

import pytest

from voxi_worker.server import build_server


def test_server_health(monkeypatch: pytest.MonkeyPatch, tmp_path) -> None:
    monkeypatch.setenv("VOXI_WORKER_MODE", "fake")
    server = build_server(
        argparse.Namespace(
            socket=str(tmp_path / "worker.sock"),
            asr_model="nvidia/parakeet-tdt-0.6b-v2",
            llm_model="gemma3:4b",
            ollama_url="http://127.0.0.1:11434",
        )
    )

    response = server.handle_request(type("Req", (), {"id": "1", "op": "health"})())

    assert response.ok is True
    assert response.device
    assert response.asr_model == "nvidia/parakeet-tdt-0.6b-v2"
    assert response.llm_model == "gemma3:4b"


def test_server_transcribe_and_clean(monkeypatch: pytest.MonkeyPatch, tmp_path) -> None:
    monkeypatch.setenv("VOXI_WORKER_MODE", "fake")
    monkeypatch.setenv("VOXI_FAKE_ASR_TRANSCRIPT", "hello there")
    monkeypatch.setenv("VOXI_FAKE_CLEANUP_TEXT", "Hello there.")
    server = build_server(
        argparse.Namespace(
            socket=str(tmp_path / "worker.sock"),
            asr_model="nvidia/parakeet-tdt-0.6b-v2",
            llm_model="gemma3:4b",
            ollama_url="http://127.0.0.1:11434",
        )
    )

    response = server.handle_request(
        type(
            "Req",
            (),
            {
                "id": "job-1",
                "op": "transcribe_and_clean",
                "audio_b64": base64.b64encode(b"1234").decode("ascii"),
                "audio_format": "pcm_s16le",
                "sample_rate_hz": 16000,
            },
        )()
    )

    assert response.ok is True
    assert response.transcript == "hello there"
    assert response.cleaned == "Hello there."


def test_server_handle_connection_ignores_broken_pipe(monkeypatch: pytest.MonkeyPatch, tmp_path) -> None:
    monkeypatch.setenv("VOXI_WORKER_MODE", "fake")
    server = build_server(
        argparse.Namespace(
            socket=str(tmp_path / "worker.sock"),
            asr_model="nvidia/parakeet-tdt-0.6b-v2",
            llm_model="gemma3:4b",
            ollama_url="http://127.0.0.1:11434",
        )
    )

    class BrokenPipeWriter(io.StringIO):
        def write(self, s: str) -> int:
            raise BrokenPipeError("client disconnected")

        def flush(self) -> None:
            raise BrokenPipeError("client disconnected")

    class FakeConn:
        def makefile(self, mode: str, encoding: str = "utf-8"):  # noqa: ARG002 - API compatibility
            if mode == "r":
                return io.StringIO(json.dumps({"id": "health-1", "op": "health"}) + "\n")
            return BrokenPipeWriter()

    server.handle_connection(FakeConn())
