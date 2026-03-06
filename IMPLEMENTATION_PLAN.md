# Voxi MVP Implementation Plan

This file mirrors the approved implementation plan used to execute the MVP build.

## Milestones

- [ ] M1 — Control plane skeleton
- [ ] M2 — Recording + ASR integration
- [ ] M3 — Cleanup + insertion
- [ ] M4 — Reliability, doctor, setup, docs, and hardening

## Milestone mapping

### M1 — Control plane skeleton
- Go module and project scaffolding
- `voxi daemon`, `voxi toggle`, `voxi status`
- daemon socket RPC
- state machine and notification wrapper
- config, runtime paths, structured logging
- systemd user service template

### M2 — Recording + ASR integration
- recorder abstraction and PipeWire adapter
- Python worker RPC server
- Parakeet adapter and worker supervision
- ASR timeout and retry behavior

### M3 — Cleanup + insertion
- Ollama cleanup adapter
- insertion via `wtype`
- clipboard fallback via `wl-copy`
- end-to-end record -> transcribe -> clean -> insert/copy flow

### M4 — Reliability and hardening
- `voxi doctor`
- full automated test harness and reliability coverage
- setup script
- final README, troubleshooting, and usage docs
