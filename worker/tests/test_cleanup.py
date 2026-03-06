from __future__ import annotations

import json
import os
import socket
import threading
import urllib.error
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest

from voxi_worker.asr import WorkerError
from voxi_worker.cleanup import FakeCleanupAdapter, OllamaCleanupAdapter


def test_fake_cleanup_identity(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("VOXI_FAKE_CLEANUP_BEHAVIOR", "identity")
    adapter = FakeCleanupAdapter(model_name="gemma3:4b", ollama_url="http://127.0.0.1:11434")
    assert adapter.clean("hello there") == "hello there"


def test_fake_cleanup_timeout(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("VOXI_FAKE_CLEANUP_BEHAVIOR", "timeout")
    adapter = FakeCleanupAdapter(model_name="gemma3:4b", ollama_url="http://127.0.0.1:11434")
    with pytest.raises(WorkerError) as excinfo:
        adapter.clean("hello")
    assert excinfo.value.code == "LLM_TIMEOUT"


class _Handler(BaseHTTPRequestHandler):
    def do_POST(self) -> None:  # noqa: N802
        payload = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        body = json.dumps({"response": payload["prompt"].splitlines()[-1].strip().capitalize()}).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: object) -> None:  # noqa: A003
        return


def test_ollama_cleanup_adapter() -> None:
    server = HTTPServer(("127.0.0.1", 0), _Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    try:
        adapter = OllamaCleanupAdapter(
            model_name="gemma3:4b",
            ollama_url=f"http://127.0.0.1:{server.server_port}",
        )
        assert adapter.clean("hello from test") == "Hello from test"
    finally:
        server.shutdown()
        thread.join(timeout=1)


def test_ollama_cleanup_timeout_from_urlerror(monkeypatch: pytest.MonkeyPatch) -> None:
    adapter = OllamaCleanupAdapter(model_name="gemma3:4b", ollama_url="http://127.0.0.1:11434")

    def raise_timeout(*args: object, **kwargs: object) -> object:
        raise urllib.error.URLError(socket.timeout("timed out"))

    monkeypatch.setattr("urllib.request.urlopen", raise_timeout)

    with pytest.raises(WorkerError) as excinfo:
        adapter.clean("hello")

    assert excinfo.value.code == "LLM_TIMEOUT"


def test_ollama_cleanup_unavailable_from_urlerror(monkeypatch: pytest.MonkeyPatch) -> None:
    adapter = OllamaCleanupAdapter(model_name="gemma3:4b", ollama_url="http://127.0.0.1:11434")

    def raise_unavailable(*args: object, **kwargs: object) -> object:
        raise urllib.error.URLError("connection refused")

    monkeypatch.setattr("urllib.request.urlopen", raise_unavailable)

    with pytest.raises(WorkerError) as excinfo:
        adapter.clean("hello")

    assert excinfo.value.code == "LLM_UNAVAILABLE"
