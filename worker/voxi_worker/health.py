from __future__ import annotations

import os
import shutil
import subprocess
from dataclasses import dataclass


@dataclass(slots=True)
class RuntimeHealth:
    device: str
    asr_model: str
    llm_model: str


def detect_device() -> str:
    if override := os.getenv("VOXI_WORKER_DEVICE"):
        return override

    try:
        import torch  # type: ignore

        if torch.cuda.is_available():
            return f"cuda:{torch.cuda.current_device()}"
    except Exception:
        pass

    return "cpu"


def nvidia_status() -> tuple[str, str]:
    if shutil.which("nvidia-smi") is None:
        return "missing", "nvidia-smi unavailable; CPU fallback expected"

    proc = subprocess.run(
        ["nvidia-smi", "--query-gpu=name", "--format=csv,noheader"],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        detail = (proc.stderr or proc.stdout or "nvidia-smi failed").strip()
        return "error", detail

    return "ok", (proc.stdout or "NVIDIA GPU detected").strip()
