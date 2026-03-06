# Voxi

Voxi is a local voice dictation tool for GNOME on Fedora. A GNOME keyboard shortcut runs `voxi toggle`; the first press starts recording, the second press stops recording, transcribes speech locally with Parakeet, cleans the transcript with Ollama, and inserts the result into the focused application. If direct insertion fails, Voxi falls back to the Wayland clipboard.

The MVP is intentionally minimal:

`hotkey -> record -> ASR -> cleanup -> insert / clipboard fallback`

## What is included

- Go CLI and daemon
  - `voxi daemon`
  - `voxi toggle`
  - `voxi status`
  - `voxi doctor`
- Python ML worker
  - Parakeet ASR adapter (`nvidia/parakeet-tdt-0.6b-v2`)
  - Ollama cleanup adapter (`gemma3:4b`)
- Unix socket JSON RPC between CLI, daemon, and worker
- Wayland-first insertion via `wtype` with `wl-copy` fallback
- structured JSON logging
- systemd user service template
- Fedora-aware idempotent setup script
- automated Go and Python tests, plus optional real-model smoke paths

## Architecture summary

```text
GNOME custom shortcut (Super+I -> "voxi toggle")
        |
        v
   Go CLI -> Unix socket RPC -> Go daemon
                                |
                                +-- PipeWire recorder (`pw-record`)
                                +-- notifications (`notify-send`)
                                +-- insertion (`wtype`) / clipboard fallback (`wl-copy`)
                                |
                                v
                        Python worker -> Parakeet ASR + Ollama cleanup
```

### Runtime state machine

- `Idle`
- `Recording`
- `Processing`
- `Inserting`

Hotkey presses are only accepted in `Idle` and `Recording`. Presses during `Processing` or `Inserting` are ignored to enforce the MVP’s strict single in-flight job policy.

## Quickstart

### Fedora setup

1. Clone the repository.
2. Run:

   ```bash
   ./scripts/setup.sh
   ```

3. Start or enable the daemon:

   ```bash
   systemctl --user enable --now voxi.service
   ```

4. Configure the GNOME shortcut:
   - Settings -> Keyboard -> Keyboard Shortcuts -> Custom Shortcuts
   - Name: `Voxi Dictation`
   - Command: `voxi toggle`
   - Shortcut: `Super+I`

5. Verify readiness:

   ```bash
   voxi doctor
   voxi status
   ```

## Manual development setup

If you are working from source instead of using the bootstrap script:

```bash
go build -o ./bin/voxi ./cmd/voxi
python3 -m venv .venv
./.venv/bin/pip install -e ./worker -r ./worker/requirements-dev.txt
./bin/voxi doctor
```

Create `~/.config/voxi/config.yaml` with the worker Python path:

```yaml
hotkey_command: "voxi toggle"
asr_model: "nvidia/parakeet-tdt-0.6b-v2"
llm_runtime: "ollama"
llm_model: "gemma3:4b"
insert_method: "wtype"
notification_timeout_ms: 2200
asr_timeout_ms: 1500
llm_timeout_ms: 1200
insertion_timeout_ms: 200
worker_python: "/absolute/path/to/repo/.venv/bin/python"
worker_entrypoint: "voxi_worker"
ollama_url: "http://127.0.0.1:11434"
```

## Commands

### `voxi daemon`

Starts the foreground daemon. The daemon:

- creates the daemon socket
- supervises the Python worker
- owns the state machine
- records audio
- retries worker failures once for ASR and cleanup stages
- performs insertion and clipboard fallback
- writes structured logs

### `voxi toggle`

Sends a toggle request to the daemon:

- `Idle -> Recording`
- `Recording -> Processing`
- ignored in `Processing` / `Inserting`

### `voxi status`

Prints the current daemon state as JSON:

```json
{"state":"Idle"}
```

### `voxi doctor`

Runs local dependency and readiness checks:

- required binaries
- Ollama reachability
- NVIDIA driver visibility via `nvidia-smi`
- worker runtime probe showing CUDA vs CPU fallback

## Structured logging

Voxi writes JSON lines to:

- `~/.local/state/voxi/voxi.log`
- stdout/stderr for systemd journald capture

Fields include stage, result, error code, retry count, and durations.

## Systemd user service

Install the provided user unit:

```bash
mkdir -p ~/.config/systemd/user
cp systemd/voxi.service ~/.config/systemd/user/voxi.service
systemctl --user daemon-reload
systemctl --user enable --now voxi.service
```

The service targets `graphical-session.target` for GNOME Wayland sessions.

## Tests

### Automated tests

Run the full automated test suite:

```bash
go test ./...
python3 -m pytest worker/tests
```

Or use the convenience target:

```bash
make test
```

### What is covered

- state machine transitions
- re-entrancy / single-flight behavior
- retry semantics for ASR and cleanup failures
- insertion retry and clipboard fallback
- CLI / daemon Unix-socket RPC
- doctor checks
- worker restart after crash
- Python worker protocol, cleanup adapter, health probe, and server behavior

### Optional real-model smoke tests

Real-model tests are intentionally off by default to avoid heavyweight downloads in normal automated runs.

To run the Python Parakeet smoke test:

```bash
export VOXI_RUN_REAL_MODEL_SMOKE=1
export VOXI_REAL_SMOKE_AUDIO_B64="<base64 pcm_s16le audio>"
python3 -m pytest worker/tests/test_real_smoke.py
```

You must also have the real ASR dependencies installed, including NeMo/Torch and a working Parakeet runtime.

## Troubleshooting

### `voxi doctor` reports missing binaries

Install the Fedora packages required by the TRD:

- `wtype`
- `wl-clipboard`
- `libnotify`
- `pipewire-utils`
- `golang`
- `python3-pip`

### Ollama is installed but unreachable

Make sure the local service is running:

```bash
ollama serve
```

or, if installed as a system service:

```bash
sudo systemctl enable --now ollama.service
```

### Worker probe reports CPU fallback

This is expected on machines without a validated NVIDIA/CUDA runtime. On NVIDIA systems, ensure:

- the Fedora NVIDIA driver stack is installed correctly
- `nvidia-smi` works
- Python ML dependencies were installed against a CUDA-capable runtime

### `voxi toggle` says the daemon is unavailable

Start the daemon manually:

```bash
voxi daemon
```

or inspect the user service:

```bash
systemctl --user status voxi.service
journalctl --user -u voxi.service -n 200
```

### Direct insertion fails

Voxi will fall back to `wl-copy` and notify with `Copied to clipboard`. This is the expected MVP behavior when `wtype` injection fails in the target application.

## Known limitations

- GNOME on Fedora with Wayland is the primary supported target
- GNOME shortcut setup is manual in the MVP
- no streaming transcription
- no transcript editor UI
- no multi-language support
- insertion reliability still depends on application/toolkit behavior under Wayland
- default automated tests use fake adapters rather than downloading real models

## Implementation tracking

- milestone plan: `IMPLEMENTATION_PLAN.md`
- running build status: `BUILD_STATUS.md`
- requirements docs: `prd.md`, `trd.md`
