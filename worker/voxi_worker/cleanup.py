from __future__ import annotations

import json
import os
import socket
import urllib.error
import urllib.request
from dataclasses import dataclass

from .asr import WorkerError


PROMPT_TEMPLATE = """Clean up the following dictated text.
- Fix punctuation, capitalization, and minor grammar only.
- Preserve meaning exactly.
- Return cleaned text only.

Text:
{text}
"""

DEFAULT_CLEANUP_TIMEOUT_MS = 8000
MIN_CLEANUP_TIMEOUT_MS = 8000


@dataclass(slots=True)
class CleanupAdapter:
    model_name: str
    ollama_url: str

    def clean(self, text: str) -> str:
        raise NotImplementedError


@dataclass(slots=True)
class FakeCleanupAdapter(CleanupAdapter):
    def clean(self, text: str) -> str:
        behavior = os.getenv("VOXI_FAKE_CLEANUP_BEHAVIOR", "success")
        if behavior == "timeout":
            raise WorkerError("text_cleanup", "LLM_TIMEOUT", "request exceeded timeout")
        if behavior == "unavailable":
            raise WorkerError("text_cleanup", "LLM_UNAVAILABLE", "model unavailable")
        if behavior == "identity":
            return text
        return os.getenv("VOXI_FAKE_CLEANUP_TEXT", text.strip().capitalize())


@dataclass(slots=True)
class OllamaCleanupAdapter(CleanupAdapter):
    timeout_s: float = DEFAULT_CLEANUP_TIMEOUT_MS / 1000.0

    def clean(self, text: str) -> str:
        payload = {
            "model": self.model_name,
            "stream": False,
            "keep_alive": "10m",
            "options": {"temperature": 0},
            "prompt": PROMPT_TEMPLATE.format(text=text.strip()),
        }
        body = json.dumps(payload).encode("utf-8")
        request = urllib.request.Request(
            self.ollama_url.rstrip("/") + "/api/generate",
            data=body,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout_s) as response:
                raw = response.read().decode("utf-8")
        except socket.timeout as exc:
            raise WorkerError("text_cleanup", "LLM_TIMEOUT", "request exceeded timeout") from exc
        except TimeoutError as exc:
            raise WorkerError("text_cleanup", "LLM_TIMEOUT", "request exceeded timeout") from exc
        except urllib.error.HTTPError as exc:
            code = "LLM_TIMEOUT" if exc.code in {408, 504} else "LLM_UNAVAILABLE"
            raise WorkerError("text_cleanup", code, f"HTTP {exc.code}") from exc
        except urllib.error.URLError as exc:
            if is_timeout_reason(exc.reason):
                raise WorkerError("text_cleanup", "LLM_TIMEOUT", "request exceeded timeout") from exc
            raise WorkerError("text_cleanup", "LLM_UNAVAILABLE", str(exc.reason)) from exc

        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError as exc:
            raise WorkerError("text_cleanup", "LLM_UNAVAILABLE", "invalid Ollama response") from exc

        cleaned = str(parsed.get("response", "")).strip()
        if not cleaned:
            raise WorkerError("text_cleanup", "LLM_UNAVAILABLE", "empty Ollama response")

        return cleaned


def build_cleanup_adapter(model_name: str, ollama_url: str, llm_timeout_ms: int | None = None) -> CleanupAdapter:
    mode = os.getenv("VOXI_WORKER_MODE", "").lower()
    if mode == "fake":
        return FakeCleanupAdapter(model_name=model_name, ollama_url=ollama_url)
    timeout_ms = normalize_cleanup_timeout_ms(llm_timeout_ms)
    return OllamaCleanupAdapter(model_name=model_name, ollama_url=ollama_url, timeout_s=timeout_ms / 1000.0)


def normalize_cleanup_timeout_ms(value: int | None) -> int:
    if value is None or value <= 0:
        return DEFAULT_CLEANUP_TIMEOUT_MS
    return max(value, MIN_CLEANUP_TIMEOUT_MS)


def is_timeout_reason(reason: object) -> bool:
    if isinstance(reason, (TimeoutError, socket.timeout)):
        return True
    return "timed out" in str(reason).lower()
