# PR title

Build MVP: Voxi daemon+worker, tests, setup, and docs

## Implemented scope

- Built the Go CLI and daemon commands:
  - `voxi daemon`
  - `voxi toggle`
  - `voxi status`
  - `voxi doctor`
- Implemented the daemon state machine, recorder orchestration, worker supervision, retries/timeouts, notifications, insertion, clipboard fallback, config loading, and structured logging
- Implemented the Python worker with fake and real adapter paths for Parakeet ASR and Ollama cleanup
- Added Unix socket JSON RPC contracts for CLI<->daemon and daemon<->worker
- Added Fedora-oriented setup automation, a systemd user service unit, command shims, and end-user documentation
- Added Go and Python automated coverage for state safety, retry semantics, fallback behavior, worker crash recovery, doctor checks, and IPC
- Added follow-up hardening for clean bootstrap, CLI RPC timeouts, safe stale-socket cleanup, cleanup timeout classification, and insertion double-failure coverage

## Architecture summary

- Thin Go CLI sends JSON RPC requests over a Unix socket to the daemon
- The Go daemon owns the strict single-flight state machine:
  - `Idle -> Recording -> Processing -> Inserting -> Idle`
- Audio capture uses a PipeWire-oriented recorder adapter (`pw-record`) and normalizes output to `pcm_s16le`
- A supervised Python worker process handles Parakeet ASR and Ollama cleanup behind a stable socket contract
- Insertion uses `wtype` first and `wl-copy` as the required fallback path
- Logs are emitted as JSON lines to `~/.local/state/voxi/voxi.log` and stdout/stderr for journald capture

## How to run locally

1. `./scripts/setup.sh`
2. `systemctl --user enable --now voxi.service`
3. Configure GNOME shortcut: `Super+I -> voxi toggle`
4. Verify with `voxi doctor` and `voxi status`

Manual source workflow:

- `go build -o ./bin/voxi ./cmd/voxi`
- `python3 -m venv .venv`
- `./.venv/bin/pip install -e ./worker -r ./worker/requirements-dev.txt`
- `./bin/voxi doctor`

## Test results

- `go test ./...` -> pass
- `python3 -m pytest worker/tests` -> `10 passed, 1 skipped`
  - the skipped test is the optional real-model smoke path behind `VOXI_RUN_REAL_MODEL_SMOKE=1`
- `go build -o ./bin/voxi ./cmd/voxi` -> pass
- `bash -n scripts/setup.sh` -> pass
- `./bin/voxi doctor` -> verified actionable output on the current VM

## Artifacts

### Manual happy-path smoke with shims/fake worker

- CLI outputs:
  - `{"state":"Idle"}`
  - `Recording`
  - `Processing`
  - `{"state":"Idle"}`
- Shim log captured:
  - `wtype|Hello world`
  - `notify-send|-t 2200 Inserted`

### Manual fallback smoke with shims/fake worker

- CLI outputs:
  - `Recording`
  - `Processing`
  - `{"state":"Idle"}`
- Shim log captured:
  - two `wtype` attempts
  - `wl-copy|Hello world`
  - `notify-send|-t 2200 Copied to clipboard`

## Known gaps

- Real GNOME/Fedora desktop validation with actual microphone, Wayland focus targets, Ollama runtime, and Parakeet model weights was not available in this cloud VM; automated tests and shim-backed smoke runs cover the MVP control flow instead
- Optional real-model smoke tests are scaffolded but remain opt-in because they require heavyweight local ML dependencies and model downloads
- GitHub API PR creation returned HTTP 403 in this environment, so this report captures the exact PR content that should be used if a manual PR creation step is still required

## Follow-up tasks

- Run the documented GNOME manual matrix on a Fedora Wayland machine with real `wtype`, `wl-copy`, `pw-record`, Ollama, and Parakeet installed
- Add broader insertion compatibility validation across more Wayland applications/toolkits
- Add latency benchmarking artifacts on target hardware for the 30-run TRD stretch/acceptance measurement

## TRD checklist

- [x] Sections 4, 6, 7, 8, 9.1, 10.1, 15, 16, 18/M1
- [x] Sections 9.4, 9.5, 10.2, 12, 13, 17.3.2, 18/M2
- [x] Sections 9.6, 9.7, 11, 12, 17.3.3, 17.3.4, 18/M3
- [x] Sections 14, 15, 16, 17, 18/M4, 21
