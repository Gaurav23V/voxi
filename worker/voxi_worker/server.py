from __future__ import annotations

import argparse
import base64
import json
import os
import socket
import sys
from dataclasses import dataclass

from .asr import WorkerError, build_asr_adapter
from .cleanup import build_cleanup_adapter
from .health import RuntimeHealth
from .protocol import WorkerRequest, WorkerResponse


@dataclass(slots=True)
class WorkerServer:
    socket_path: str
    health: RuntimeHealth
    asr_adapter: object
    cleanup_adapter: object

    def serve_forever(self) -> None:
        if os.path.exists(self.socket_path):
            os.remove(self.socket_path)

        server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        server.bind(self.socket_path)
        server.listen()

        try:
            while True:
                conn, _ = server.accept()
                with conn:
                    self.handle_connection(conn)
        finally:
            server.close()
            if os.path.exists(self.socket_path):
                os.remove(self.socket_path)

    def handle_connection(self, conn: socket.socket) -> None:
        reader = conn.makefile("r", encoding="utf-8")
        writer = conn.makefile("w", encoding="utf-8")
        try:
            line = reader.readline()
            if not line:
                return
            payload = json.loads(line)
            request = WorkerRequest.from_dict(payload)
            response = self.handle_request(request)
        except json.JSONDecodeError as exc:
            response = WorkerResponse(id="", ok=False, stage="startup", code="BOOT_DEP_MISSING", message=f"invalid JSON: {exc}")
        except Exception as exc:  # pragma: no cover - guard rail
            response = WorkerResponse(id="", ok=False, stage="startup", code="BOOT_DEP_MISSING", message=str(exc))
        writer.write(json.dumps(response.to_dict()) + "\n")
        writer.flush()

    def handle_request(self, request: WorkerRequest) -> WorkerResponse:
        if request.op == "health":
            return WorkerResponse(
                id=request.id,
                ok=True,
                device=self.health.device,
                asr_model=self.health.asr_model,
                llm_model=self.health.llm_model,
            )

        if request.op != "transcribe_and_clean":
            return WorkerResponse(id=request.id, ok=False, stage="startup", code="BOOT_DEP_MISSING", message="unsupported operation")

        try:
            audio_bytes = base64.b64decode(request.audio_b64 or "", validate=False)
            transcript = self.asr_adapter.transcribe(
                audio_bytes=audio_bytes,
                sample_rate_hz=int(request.sample_rate_hz or 16000),
                audio_format=request.audio_format or "pcm_s16le",
            )
            if not transcript.strip():
                raise WorkerError("speech_recognition", "ASR_RUNTIME_FAILURE", "empty transcript")
            cleaned = self.cleanup_adapter.clean(transcript)
        except WorkerError as exc:
            return WorkerResponse(
                id=request.id,
                ok=False,
                stage=exc.stage,
                code=exc.code,
                message=exc.message,
            )

        return WorkerResponse(
            id=request.id,
            ok=True,
            transcript=transcript,
            cleaned=cleaned,
        )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Voxi worker")
    parser.add_argument("--socket", required=True)
    parser.add_argument("--asr-model", required=True)
    parser.add_argument("--llm-model", required=True)
    parser.add_argument("--ollama-url", required=True)
    return parser


def build_server(args: argparse.Namespace) -> WorkerServer:
    asr_adapter = build_asr_adapter(args.asr_model)
    cleanup_adapter = build_cleanup_adapter(args.llm_model, args.ollama_url)
    health = RuntimeHealth(
        device=asr_adapter.device,
        asr_model=args.asr_model,
        llm_model=args.llm_model,
    )
    return WorkerServer(
        socket_path=args.socket,
        health=health,
        asr_adapter=asr_adapter,
        cleanup_adapter=cleanup_adapter,
    )


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    server = build_server(args)
    print(
        json.dumps(
            {
                "stage": "startup",
                "result": "ready",
                "device": server.health.device,
                "asr_model": server.health.asr_model,
                "llm_model": server.health.llm_model,
            }
        ),
        flush=True,
    )
    server.serve_forever()
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
