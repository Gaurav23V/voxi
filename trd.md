# TRD - Voxi Technical Requirements and Design (GNOME Fedora MVP)

## Document Control

- Version: 1.0
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

## 2.2 Out of Scope (MVP)

- Streaming partial transcripts
- Editing UI before insertion
- Multi-language routing
- Cloud services
- Multi-desktop support hardening (beyond GNOME/Fedora)

## 2.3 Hard Constraints

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
| App architecture | Python daemon + Python ML worker (two processes) | Fastest implementation for solo dev, keeps ML isolation requirement |
| Process boundary | Daemon (control plane) separate from worker (ASR+LLM) | Prevent ML crashes/latency from freezing UX and hotkey handling |
| ASR runtime | Parakeet via NeMo-based adapter in worker | Aligns with PRD and model requirement |
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

## 5.2 Python-Only Language Stack (with Two Processes)

### Chosen

- Daemon: Python asyncio service
- Worker: Python subprocess

### Why

- Fastest iteration for solo implementation.
- Direct access to ASR ecosystem.
- Keeps two-process isolation without cross-language complexity.

### Deferred alternative

Rust daemon + Python worker is a valid future hardening path, but not the fastest route to first usable daily build.

## 5.3 Insertion: `wtype` First

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
| Voxi Daemon (Python)  |
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

- Daemon captures to memory buffer (preferred for short utterances)
- If buffer exceeds threshold, spill to temp file under runtime dir

## 9.5 ASR Adapter (Parakeet)

Responsibilities:

- Normalize incoming audio format
- Invoke Parakeet model inference
- Return plain transcript text

Error classes:

- model unavailable
- timeout
- inference runtime failure
- empty transcript

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

## 9.7 Output Adapter

Order:

1. attempt direct insertion with `wtype`
2. on failure, copy to clipboard with `wl-copy`

On fallback, user message must clearly say clipboard was used.

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
asr_model: "parakeet"
llm_runtime: "ollama"
llm_model: "gemma3b"
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
- Python runtime and required ML libs
- Local Ollama runtime

Service dependencies at runtime:

- Wayland session active
- D-Bus user session active

`voxi doctor` should explicitly validate all required binaries and services.

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

## 17.1 Unit Tests

- state transition correctness
- hotkey ignore behavior during processing
- timeout/retry logic
- error mapping to user messages

## 17.2 Integration Tests

- daemon <-> worker IPC contract
- recording -> worker -> insertion happy path
- insertion failure -> clipboard fallback
- worker crash -> daemon survives and recovers

## 17.3 Manual Validation (GNOME Fedora)

- `Super+I` starts/stops reliably
- insertion in GNOME Text Editor, Firefox, terminal input
- failure message includes correct stage
- no concurrent runs possible

Exit criteria for MVP:

- all acceptance criteria from `prd.md` pass on target machine

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
- Add optional Rust daemon rewrite while keeping worker RPC stable.
- Add optional insertion adapters for broader desktop/app compatibility.
- Add richer local metrics dashboard if needed.

---

## 21. Open Questions Before Implementation

1. Which exact Parakeet checkpoint should be pinned for latency vs quality?
2. Which cleanup model gives best speed/quality on this machine (`gemma3b` vs smaller)?
3. Should we preserve previous clipboard content when fallback occurs (yes/no for MVP)?
4. Do we enforce maximum utterance duration in MVP for predictable latency?

These should be finalized before coding starts.
