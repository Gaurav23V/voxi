# TRD - Voxi Technical Requirements and Design (GNOME Fedora MVP)

## Document Control

- Version: 1.1
- Status: Draft for review
- Scope: Personal machine MVP (GNOME on Fedora, Wayland session)
- Related doc: `prd.md`

---

## 1. Purpose

This TRD defines exactly how Voxi should be implemented for the first working version on a single target environment: GNOME on Fedora.

It explains:

- what technical choices we make
- why we make each choice
- what alternatives we are explicitly not choosing for MVP
- what must be true before we start implementation

---

## 2. Scope and Constraints

## 2.1 In Scope (MVP)

- Global dictation toggle using one hotkey (`Super+I`)
- Local recording from default microphone
- Local ASR with Parakeet
- Local cleanup with LLM
- Text insertion into focused app when possible
- Clipboard fallback when insertion fails
- Minimal user feedback notifications
- Local-only data handling

## 2.2 Hard Constraints

- Must work on GNOME + Wayland on Fedora first
- Must not allow overlapping dictation runs
- Must show concise stage-specific failure messages
- Must run locally with no network dependency in the speech pipeline

---

## 3. Target Environment Assumptions

- OS: Fedora Linux
- DE: GNOME
- Display protocol: Wayland
- Audio stack: PipeWire/Pulse compatible
- LLM runtime installed locally (Ollama for MVP)
- GPU may be available, but CPU fallback must function

If any assumption fails, MVP behavior should degrade gracefully, not crash.

---

## 4. Architecture Decision Summary

| Decision | Choice | Why |
| --- | --- | --- |
| Hotkey registration | GNOME custom keyboard shortcut calling `voxi toggle` | Most reliable for GNOME MVP; avoids fragile generic global key capture |
| App architecture | Go daemon + Python ML worker (two processes) | Compiled control-plane reliability with simple implementation and strong ML ecosystem fit |
| Process boundary | Daemon (control plane) separate from worker (ASR+LLM) | Prevent ML crashes/latency from freezing UX and hotkey handling |
| ASR runtime | Parakeet via NeMo-based adapter in worker | Aligns with PRD and model requirement |
| GPU utilization policy | Prefer CUDA, verify at startup and runtime, fallback to CPU with explicit warning | Ensures predictable performance and easier debugging |
| Cleanup runtime | Ollama HTTP API | Simple local model management and warm keep-alive |
| Insertion | `wtype` primary, clipboard fallback via `wl-copy` | Best practical Wayland path on GNOME; fallback is robust |
| Notifications | `notify-send`/Freedesktop notifications | Minimal implementation, native GNOME behavior |
| Daemon lifecycle | `systemd --user` service | Auto-start, supervision, restart-on-failure, easy logs |

---

## 5. Why These Choices (Tradeoff Analysis)

## 5.1 Hotkey: GNOME Shortcut vs Portal GlobalShortcuts

### Chosen

Use GNOME keyboard settings to bind `Super+I` to:

`voxi toggle`

### Why

- Works reliably today in target environment.
- No need for app window or portal bind UX for MVP.
- Avoids compositor variability while we target one machine.

### Deferred alternative

`org.freedesktop.portal.GlobalShortcuts` is a strong future option for broader desktop support, but not required for personal GNOME-first MVP.

## 5.2 Go + Python Language Stack (with Two Processes)

### Chosen

- Daemon: Go service (control plane)
- Worker: Python subprocess

### Why

- Compiled daemon + CLI reduces environment fragility for GNOME shortcuts and systemd user services.
- Go standard library (`net`, `os/exec`, `context`, `encoding/json`) maps directly to daemon requirements.
- Go is already a strong language match for this developer, improving delivery speed and maintenance.
- Python worker preserves direct access to ASR ecosystem and local LLM integrations.
- Keeps strict process isolation while avoiding heavy control-plane complexity.

### Why not Rust right now

- Rust provides stronger compile-time guarantees and a strong desktop/portal ecosystem.
- For this MVP, end-to-end latency is dominated by ASR/LLM inference, so daemon-language performance gains are marginal.
- The Go option gives better complexity-to-value right now without blocking a future rewrite.
- We keep daemon/worker RPC contracts stable so a future Rust daemon remains low-risk if needed.

## 5.3 Go vs Rust Decision Record (Daemon Layer)

This comparison is specific to Voxi's daemon scope (CLI, state machine, IPC, worker supervision, insertion orchestration, doctor checks).

| Criterion | Go assessment | Rust assessment | MVP decision driver |
| --- | --- | --- | --- |
| Delivery speed | Very strong | Medium | Go |
| Control-plane reliability | Strong and sufficient | Strongest | Tie (both acceptable) |
| Desktop/portal ecosystem | Good (`godbus`) | Strong (`zbus`/`ashpd`) | Deferred need, so Go |
| Systemd/service ergonomics | Strong | Strong | Tie |
| Runtime overhead | Low | Lower | Not primary for MVP |
| End-to-end latency impact | Marginal vs Rust | Marginal vs Go | Not decision driver |
| Maintainability for this owner | Strong (current expertise) | Medium (new stack) | Go |

Decision outcome:

- For MVP and near-term releases, daemon language is **Go**.
- Python remains the ML worker language.
- We preserve versioned IPC contracts so future daemon-language migration remains feasible.

## 5.4 Insertion: `wtype` First

### Chosen

- Primary: simulate typing with `wtype`
- Fallback: copy cleaned text to clipboard (`wl-copy`)

### Why

- Works on Wayland with GNOME in practical use.
- Keeps primary behavior aligned with PRD: insert directly at cursor.
- Clipboard fallback guarantees output is not lost.

### Deferred alternatives

- RemoteDesktop portal input injection (too heavy permission model for MVP)
- `ydotool` (extra daemon and privilege complexity)

---

## 6. High-Level System Architecture

```text
GNOME Shortcut (Super+I)
        |
        v
   "voxi toggle" CLI
        |
  Unix socket RPC
        v
+-----------------------+
| Voxi Daemon (Go)      |
| - state machine       |
| - audio start/stop    |
| - timeout/retry       |
| - notifications       |
| - insertion/fallback  |
+-----------+-----------+
            |
            | Unix socket RPC
            v
+-----------------------+
| ML Worker (Python)    |
| - Parakeet ASR        |
| - LLM cleanup         |
| - warm model state    |
+-----------------------+
```

---

## 7. Runtime State Machine

States:

- `Idle`
- `Recording`
- `Processing`
- `Inserting`
- `ErrorTransient` (internal only, returns to `Idle`)

Transitions:

- `Idle` + hotkey -> `Recording`
- `Recording` + hotkey -> `Processing`
- `Processing` success -> `Inserting`
- `Inserting` success/fallback -> `Idle`
- any failure -> notify -> `Idle`

Hotkey guard rules:

- Accepted only in `Idle` and `Recording`.
- In `Processing`/`Inserting`, hotkey press is ignored.
- No queueing of extra presses in MVP.

---

## 8. End-to-End Sequence

1. User presses `Super+I`.
2. GNOME runs `voxi toggle`.
3. CLI sends `toggle` to daemon.
4. Daemon enters `Recording`, starts audio capture, notifies `Listening...`.
5. User presses `Super+I` again.
6. CLI sends second `toggle`.
7. Daemon stops capture, enters `Processing`, notifies `Processing speech...`.
8. Daemon sends audio to worker (`transcribe_and_clean`).
9. Worker returns transcript + cleaned text.
10. Daemon enters `Inserting`, tries `wtype`.
11. If insertion fails, daemon copies to clipboard.
12. Daemon notifies success (`Inserted` or `Copied to clipboard`) and returns `Idle`.

---

## 9. Component Design

## 9.1 `voxi` CLI (thin client)

Responsibilities:

- Provide command surface for user and shortcut integration.
- Send local RPC to daemon.

Initial commands:

- `voxi daemon` - start daemon foreground
- `voxi toggle` - send toggle request
- `voxi status` - print current daemon state
- `voxi doctor` - verify dependencies and runtime readiness

## 9.2 Daemon (control plane)

Responsibilities:

- Own state machine
- Handle toggle events
- Start/stop recording
- Call worker
- Enforce retry/timeout rules
- Insert/fallback behavior
- User notifications
- Logging and metrics

Non-responsibilities:

- Direct model inference logic (worker owns that)

Implementation language:

- Go
- recommended libraries: `cobra` (CLI), `encoding/json` (RPC payloads), Unix sockets via Go stdlib

## 9.3 ML Worker

Responsibilities:

- Load ASR model at worker startup
- Run ASR on provided audio
- Call local LLM runtime for cleanup
- Return structured success/error response

Non-responsibilities:

- Hotkey logic
- Insertion logic
- UI notifications

## 9.4 Audio Capture Module

MVP requirements:

- Capture from default mic
- 16kHz mono PCM path for ASR compatibility
- Fast start and stop

Implementation approach:

- Daemon controls recording start/stop boundaries.
- Capture implementation should avoid complex native bindings in MVP:
  - preferred path: subprocess capture tooling (for example PipeWire/Pulse-compatible CLI tools)
- Store short utterances in memory where practical; use runtime temp files when needed.

Utterance duration policy (MVP):

- No hard maximum utterance duration is enforced.
- Recording duration is user-controlled via hotkey start/stop.
- Longer utterances are supported with the understanding that processing latency scales with input length.

## 9.5 ASR Adapter (Parakeet)

Responsibilities:

- Normalize incoming audio format
- Invoke Parakeet model inference
- Return plain transcript text

Pinned MVP ASR checkpoint:

- `nvidia/parakeet-tdt-0.6b-v2`
- English-focused default for current scope.
- Future model swaps are allowed as a separate optimization task.

Error classes:

- model unavailable
- timeout
- inference runtime failure
- empty transcript

GPU requirements and checks:

- Worker must attempt CUDA device selection first.
- At startup, worker logs selected device (`cuda:<id>` or `cpu`).
- If GPU is expected but unavailable, daemon must surface a clear warning.
- During transcription, logs must include the device used for that run.
- `voxi doctor` must validate GPU availability and report:
  - NVIDIA GPU detected or not
  - driver/runtime health (for example `nvidia-smi` accessibility)
  - whether ASR runtime is using CUDA or CPU fallback

## 9.6 Cleanup Adapter (Ollama)

Use:

- `POST /api/generate`
- `stream: false`
- deterministic options (temperature close to 0)
- configured `keep_alive` to reduce cold load penalty

Prompt contract:

- fix punctuation/capitalization/minor grammar
- preserve meaning
- return cleaned text only

Pinned MVP cleanup model:

- `gemma3:4b`
- Model changes are allowed later as a small, isolated optimization task.

## 9.7 Output Adapter

Order:

1. attempt direct insertion with `wtype`
2. on failure, copy to clipboard with `wl-copy`

On fallback, user message must clearly say clipboard was used.

Clipboard policy (MVP):

- Do not preserve previous clipboard contents when fallback occurs.
- Fallback behavior prioritizes reliable delivery of current dictated text.

## 9.8 Notification Adapter

Use `notify-send` with short TTL.

Required states:

- Listening...
- Processing speech...
- Inserted
- Copied to clipboard
- Failure message with stage detail

## 9.9 Configuration Module

Path:

`~/.config/voxi/config.yaml`

MVP fields:

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
```

---

## 10. Interface Contracts

## 10.1 CLI -> Daemon RPC

Transport:

- Unix domain socket:
  - `/run/user/<uid>/voxi/daemon.sock`

Request:

```json
{"id":"req-123","op":"toggle"}
```

Response:

```json
{"id":"req-123","ok":true,"state":"Recording"}
```

## 10.2 Daemon -> Worker RPC

Transport:

- Unix domain socket:
  - `/run/user/<uid>/voxi/worker.sock`

Request:

```json
{
  "id":"job-101",
  "op":"transcribe_and_clean",
  "audio_format":"pcm_s16le",
  "sample_rate_hz":16000,
  "audio_b64":"<base64>"
}
```

Response (success):

```json
{
  "id":"job-101",
  "ok":true,
  "transcript":"hello this is a test message",
  "cleaned":"Hello, this is a test message."
}
```

Response (failure):

```json
{
  "id":"job-101",
  "ok":false,
  "stage":"speech_recognition",
  "code":"ASR_TIMEOUT",
  "message":"inference exceeded timeout"
}
```

---

## 11. Error Taxonomy and User Messaging

User-facing stage labels:

- `Recording`
- `Audio finalize`
- `Speech recognition`
- `Text cleanup`
- `Text insertion`
- `Startup`

Message format:

- Title: `Transcription failed` or `Action failed`
- Body: `Stage: <stage_name> (<short_reason>)`

Example:

- `Transcription failed`
- `Stage: Speech recognition (retry limit reached)`

Internal error codes (MVP):

- `REC_MIC_UNAVAILABLE`
- `REC_EMPTY_AUDIO`
- `ASR_TIMEOUT`
- `ASR_MODEL_UNAVAILABLE`
- `LLM_TIMEOUT`
- `LLM_UNAVAILABLE`
- `INS_WTYPE_FAILED`
- `INS_CLIPBOARD_FAILED`
- `BOOT_DEP_MISSING`

---

## 12. Timeout and Retry Policy

| Stage | Timeout | Retry policy |
| --- | --- | --- |
| ASR | 1.5s | retry once on transient failure |
| LLM cleanup | 1.2s | retry once on transient failure |
| Insertion | 0.2s | retry once then clipboard fallback |

Rules:

- No retry for invalid/empty audio.
- No retry for missing dependency until user fixes environment.
- After final ASR failure, always show `Transcription failed`.

---

## 13. Performance Requirements and Startup Strategy

Target budgets:

- Audio finalize: <= 100ms
- ASR p95: <= 800ms
- LLM cleanup p95: <= 600ms
- Insertion/fallback: <= 100ms
- End-to-end (stop to insert/copy): <= 2.0s stretch, <= 3.0s acceptable

Warm-start strategy:

- Daemon starts at login.
- Worker starts with daemon and loads ASR model once.
- Optional startup warm call to Ollama to reduce first-use delay.

---

## 14. Dependency Requirements (Fedora)

System packages:

- `wtype`
- `wl-clipboard`
- `libnotify` (or package providing `notify-send`)
- Go toolchain
- Python runtime and required ML libs
- Local Ollama runtime

GPU-related runtime dependencies (when NVIDIA is present):

- NVIDIA driver stack correctly installed and loaded
- CUDA-capable runtime path compatible with selected ASR stack
- `nvidia-smi` available for health checks

Service dependencies at runtime:

- Wayland session active
- D-Bus user session active

`voxi doctor` should explicitly validate all required binaries and services.

For GPU-capable systems, `voxi doctor` must also print whether CUDA acceleration is active or CPU fallback is currently in use.

---

## 15. Deployment and Process Management

## 15.1 systemd User Service

Unit: `~/.config/systemd/user/voxi.service`

Behavior:

- start after graphical session
- restart on failure
- log to journald

## 15.2 Hotkey Setup

User setup in GNOME Settings:

- Name: `Voxi Dictation`
- Command: `voxi toggle`
- Shortcut: `Super+I`

This is the official MVP hotkey path.

---

## 16. Logging and Observability

Log format:

- structured JSON lines
- include `timestamp`, `stage`, `duration_ms`, `result`, `error_code`, `retry_count`

Storage:

- `~/.local/state/voxi/voxi.log`
- journald mirror via systemd user service

MVP metrics:

- end-to-end latency
- stage-level latencies
- insertion success vs clipboard fallback rate
- failure counts by stage/code

No remote telemetry in MVP.

---

## 17. Testing Strategy

Test strategy is layered to reduce risk early:

- Unit tests validate deterministic logic and state safety.
- Integration tests validate process boundaries and real adapters.
- Manual GNOME validation confirms desktop behavior that CI cannot fully simulate.

## 17.1 Test Harness and Fixtures

Required reusable fixtures:

- Fake worker server with configurable behavior:
  - success response
  - timeout
  - transient error then success
  - permanent failure
  - crash on request
- Command shims for `wtype`, `wl-copy`, and `notify-send`:
  - controllable exit codes
  - captured arguments for assertions
- Deterministic audio fixtures:
  - normal short utterance sample
  - silence sample
  - invalid/corrupt sample
- Structured log assertion helper:
  - validates expected `stage`, `result`, `error_code`, `retry_count`

Test harness requirement:

- No test should require actual model downloads or GPU for default automated runs.
- Real model/GPU validation is covered in manual and optional extended test jobs.

## 17.2 Unit Tests

### 17.2.1 State Machine Correctness

- `Idle` + toggle -> `Recording`
- `Recording` + toggle -> `Processing`
- `Processing` + toggle -> remains `Processing` (ignored)
- `Inserting` + toggle -> remains `Inserting` (ignored)
- `Processing` success -> `Inserting` -> `Idle`
- any terminal error -> `Idle`

### 17.2.2 Re-Entrancy and Concurrency Guards

- Rapid repeated `toggle` events during `Processing` create no new jobs.
- Exactly one in-flight dictation job allowed at any time.
- Duplicate completion events do not cause duplicate insertions.

### 17.2.3 Retry and Timeout Semantics

- ASR transient timeout -> one retry -> success path continues.
- ASR timeout after retry -> final failure with `Transcription failed`.
- LLM transient timeout -> one retry -> success path continues.
- LLM timeout after retry -> final failure with stage-specific message.
- Non-transient failures do not retry.

### 17.2.4 Error Mapping and User Messaging

- Internal error codes map to correct user stage labels.
- Message format always matches:
  - `Title`
  - `Stage: <stage_name> (<short_reason>)`
- Missing-dependency errors map to actionable startup/doctor messages.

### 17.2.5 CLI Command Semantics

- `voxi toggle` returns success when daemon socket is healthy.
- `voxi toggle` returns actionable error when daemon is unavailable.
- `voxi status` returns valid state output schema.
- `voxi doctor` aggregates checks and returns non-zero on fatal readiness issues.

## 17.3 Integration Tests

### 17.3.1 CLI <-> Daemon Contract

- CLI connects to daemon Unix socket successfully.
- Invalid daemon response payload is handled safely.
- Socket permission/path errors produce actionable output.

### 17.3.2 Daemon <-> Worker IPC Contract

- Request/response schema compatibility check.
- Unknown/missing fields handled defensively.
- Corrupt JSON from worker leads to controlled failure and reset to `Idle`.

### 17.3.3 Happy Path Flow

- recording -> worker transcription+cleanup -> insertion success.
- verifies state transitions, notifications, and success metrics/log events.

### 17.3.4 Insertion and Fallback Behavior

- `wtype` success -> no clipboard fallback.
- `wtype` failure -> `wl-copy` fallback succeeds -> user informed.
- `wtype` failure + `wl-copy` failure -> terminal insertion error surfaced cleanly.

### 17.3.5 Worker Resilience

- Worker crash during processing -> daemon survives.
- Daemon restarts worker based on policy.
- Request fails gracefully and system returns to `Idle`.

### 17.3.6 Doctor and Dependency Checks

- Missing `wtype` detected.
- Missing `wl-copy` detected.
- Ollama not reachable detected.
- GPU checks report CUDA active vs CPU fallback status.

## 17.4 Non-Functional and Reliability Tests

### 17.4.1 Latency Verification

On target machine, collect at least 30 dictation runs and measure:

- stop -> inserted/copied end-to-end latency
- stage-level latency breakdown

Acceptance thresholds:

- p95 end-to-end <= 3.0s (minimum acceptable)
- target stretch goal remains <= 2.0s

### 17.4.2 Long-Run Stability

- Run daemon for >= 2 hours with periodic toggles.
- Assert:
  - no process leaks
  - no state deadlocks
  - stable memory growth profile (no obvious leak trend)

### 17.4.3 Restart Behavior

- Restart daemon service while idle and during processing.
- Validate clean recovery and no stuck socket state.

## 17.5 Manual Validation (GNOME Fedora)

Manual matrix (minimum):

- Hotkey behavior
  - `Super+I` start/stop works consistently.
  - press during processing is ignored.
- App insertion targets
  - GNOME Text Editor
  - Firefox text field
  - terminal input line
- Failure UX
  - stage-specific failure text visible and concise
  - clipboard fallback message shown when insertion fails
- Service lifecycle
  - service starts with user session
  - `voxi toggle` works from terminal and GNOME shortcut

## 17.6 Release Gates and Exit Criteria

Before MVP implementation is considered complete:

1. All critical unit tests pass.
2. All integration tests pass.
3. Manual GNOME matrix passes.
4. No known issue allows concurrent overlapping dictation jobs.
5. Failure messaging includes correct stage context.
6. All acceptance criteria from `prd.md` pass on target machine.

Recommended policy:

- P0 test failures block merge.
- P1 tests can be deferred only with explicit tracking issue.

---

## 18. Milestones

## M1 - Control Plane Skeleton

- daemon, CLI, socket RPC, state machine, notifications

## M2 - Recording + ASR Integration

- mic capture, Parakeet inference, transcript return

## M3 - Cleanup + Insertion

- Ollama cleanup, `wtype` insertion, clipboard fallback

## M4 - Reliability and Hardening

- retries/timeouts, error taxonomy, structured logging, doctor command

---

## 19. Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| `wtype` behavior varies by app | medium | keep clipboard fallback as guaranteed output path |
| first-run model load latency | medium | daemon+worker warm start, optional pre-warm call |
| ML runtime crash | high | strict process isolation + worker restart policy |
| dependency drift on Fedora updates | medium | `voxi doctor` checks and clear actionable errors |
| prompt drift rewriting meaning | medium | deterministic settings + strict prompt contract |

---

## 20. Future Evolution (Post-MVP)

- Add portal-based global shortcuts for less manual setup.
- Add optional insertion adapters for broader desktop/app compatibility.
- Add richer local metrics dashboard if needed.
- Re-evaluate Rust daemon only if reliability or portal integration complexity justifies the migration cost.

---

## 21. Bootstrap Setup Script Strategy (Deferred Implementation)

This project will include a single bootstrap entrypoint for fresh Fedora setups:

- Primary script: `scripts/setup.sh`
- Optional helper (if needed for orchestration): `scripts/setup.py`

Design intent:

- user clones repo and runs one command
- script provisions everything required for Voxi on target system
- script is safe to re-run many times (idempotent)

Requirements:

1. One-command bootstrap
   - `./scripts/setup.sh`
2. Minimal preconditions
   - no manual dependency installation expected where avoidable
   - unavoidable prerequisites (for example `sudo` access and internet) must be documented in `README.md`
3. Idempotency
   - before each action, script checks whether requirement is already satisfied
   - if already configured, script skips step and prints reason
4. GPU/driver awareness
   - detect NVIDIA hardware presence
   - if driver stack already healthy, do not reinstall
   - if missing and required, install/configure using Fedora-compatible path
5. Environment provisioning
   - install system packages
   - install Go dependencies/toolchain as needed
   - install Python dependencies
6. Project setup
   - install/activate CLI commands (`voxi`, including `voxi toggle`)
   - install and enable user service where applicable
   - run readiness checks (`voxi doctor`)

Important sequencing note:

- This script is intentionally deferred until after implementation and test hardening.
- We will finalize it at the end, once we confirm exact working dependencies and runtime behavior on the target machine.

---

## 22. Open Questions Before Implementation

No blocking open questions currently.
