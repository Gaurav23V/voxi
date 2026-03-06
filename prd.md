# PRD - Voxi Local Voice Dictation Tool (Detailed MVP)

## Document Status

- Version: 2.0
- Stage: MVP planning
- Platform priority: Linux desktop (Wayland first, X11 second)
- Processing model: Fully local, no cloud dependency

---

## 1. Product Overview

Voxi is a local voice dictation utility for Linux. The user presses one hotkey to start recording, presses the same hotkey to stop, and gets cleaned text inserted into the active application.

The pipeline is:

`Hotkey -> Record -> ASR (Parakeet) -> Cleanup (local LLM) -> Insert text`

This product is designed to feel instant, private, and unobtrusive.

---

## 2. Product Principles

1. **Fast by default**: use a long-running daemon and warm model processes.
2. **Local by default**: no audio/text sent to external services.
3. **Minimal UI**: only compact status/error notifications.
4. **One action loop**: dictation to text insertion, no editor UI for MVP.
5. **Fail gracefully**: clear stage-specific failures and clipboard fallback.

---

## 3. Goals

### Primary goals

1. Fast voice-to-text workflow with one hotkey toggle.
2. Reliable insertion into active apps (with fallback).
3. Full local privacy.
4. Minimal cognitive overhead during daily use.

### Success metrics (MVP)

1. End-to-end latency (stop to insert/copy):
   - Stretch target: p95 <= 2.0s on supported hardware
   - Minimum acceptable for MVP: p95 <= 3.0s
2. Hotkey press while processing starts **zero** concurrent jobs.
3. Insertion success in supported apps >= 90%; remaining attempts fallback to clipboard.
4. User-facing failures include concise stage information 100% of the time.

---

## 4. Non-Goals (MVP)

The following are explicitly out of scope:

- Streaming transcription
- Editing transcript before insertion
- Voice commands
- Multi-language support
- Long-form recording or conversation mode
- Cloud APIs or remote telemetry
- Complex settings UI
- Support for every compositor/app combination

The MVP is strictly: **dictate -> clean -> insert (or clipboard fallback)**.

---

## 5. Target Users and Core User Stories

### Target users

- Developers, writers, and operators on Linux who type frequently.
- Privacy-sensitive users who want offline voice dictation.

### Core user stories

1. As a user, I press `Super+I`, speak, press `Super+I` again, and text appears where I am typing.
2. As a user, if insertion fails, I still get usable output copied to clipboard.
3. As a user, if something fails, I see exactly which stage failed in one short message.
4. As a user, while processing is running, accidental hotkey presses do not break the workflow.

---

## 6. User Flow and Interaction Model

### Primary flow

1. User presses `Super+I`.
2. Recording starts immediately from default microphone.
3. User presses `Super+I` again.
4. Recording stops, audio finalizes.
5. ASR transcribes audio locally.
6. LLM cleans punctuation/capitalization/grammar (without changing meaning).
7. Text is inserted at cursor position.
8. If insertion is unavailable/fails, text is copied to clipboard.

### State machine

`Idle -> Recording -> Processing -> Inserting -> Idle`

### Strict hotkey behavior

- `Idle`: hotkey starts recording.
- `Recording`: hotkey stops recording.
- `Processing` or `Inserting`: hotkey press is ignored and short "Processing..." notice may be shown.

No queueing, no re-entrancy, no concurrent dictation runs in MVP.

---

## 7. UX Feedback

### Notifications

- Recording: `Listening...`
- Processing: `Processing speech...`
- Success (insert): `Inserted`
- Success (fallback): `Copied to clipboard`
- Failure title: `Transcription failed` or `Action failed`

### Failure detail line

Format:

`Stage: <stage_name> (<short_reason>)`

Examples:

- `Stage: Speech recognition (timeout)`
- `Stage: Text cleanup (model unavailable)`
- `Stage: Text insertion (focus not editable)`

Messages must be concise and auto-dismiss.

---

## 8. Functional Requirements

### FR1 - Global hotkey

- Support global hotkey `Super+I` (configurable later).
- First press starts recording, second press stops recording.

### FR2 - Concurrency guard

- If state is `Processing` or `Inserting`, new hotkey presses do not trigger a new run.
- Implementation must enforce single in-flight dictation job.

### FR3 - Recording

- Use system default audio input.
- Capture until stop.
- Store short-lived audio in memory or temporary local file.

### FR4 - Local ASR

- Use local Parakeet model for transcription.
- Prefer GPU; allow CPU fallback.
- No remote inference.

### FR5 - Local cleanup LLM

- Use local LLM runtime (for example llama.cpp or Ollama).
- Prompt behavior:
  - fix punctuation, capitalization, minor grammar
  - preserve meaning
  - return cleaned text only

### FR6 - Text output behavior

- Attempt insertion at current cursor position.
- If insertion fails or no editable target exists, copy text to clipboard.

### FR7 - Visual feedback

- Show compact status transitions for recording, processing, success, and failures.
- Errors include stage name.

### FR8 - Local-only privacy

- Audio and text stay on device.
- No external network calls for inference or telemetry.

### FR9 - Daemon lifecycle

- App runs as long-lived user daemon to minimize latency.
- Daemon exposes a lightweight trigger interface for hotkey activation.

### FR10 - ML isolation boundary

- ASR and cleanup inference run in a separate worker process from desktop/control logic.
- Worker crashes must not crash the main daemon.

### FR11 - Retry behavior

- Retry transient failures for ASR and cleanup once.
- If ASR fails after retry, show `Transcription failed` with stage detail.

### FR12 - Basic local observability

- Capture stage timings, retries, and failure codes in local logs.

---

## 9. Architecture and Major Technical Decisions

### Decision 1: Daemon-first startup

- **Choice**: long-lived daemon with warm model worker.
- **Why**: avoids repeated model load and runtime startup cost; required for low latency.

### Decision 2: Wayland-first integration

- **Choice**: design primary path for Wayland-compatible hotkey and insertion strategy.
- **Why**: modern Linux desktops default to Wayland; X11 assumptions are unreliable there.

### Decision 3: Strict state machine for hotkeys

- **Choice**: ignore hotkey while processing/inserting.
- **Why**: prevents race conditions and overlapping jobs.

### Decision 4: Two-process ML boundary

- **Choice**: main daemon process + dedicated ML worker process.
- **Why**:
  - isolate heavy inference and potential GPU/runtime failures
  - preserve responsiveness of hotkey/UI logic
  - enable controlled worker restarts

### Decision 5: Insertion with fallback

- **Choice**: attempt direct insertion first; fallback to clipboard.
- **Why**: direct insertion is best UX, but fallback preserves utility when integration limits exist.

---

## 10. Technical Architecture (MVP)

### Components

1. **Voxi daemon**
   - owns state machine
   - handles hotkey events, notifications, output behavior
2. **Audio capture module**
   - starts/stops recording from default mic
3. **ML worker process**
   - ASR inference
   - cleanup LLM inference
4. **Insertion adapter**
   - attempts focused-app insertion
   - fallback to clipboard
5. **Config + logging module**
   - loads local config
   - emits local logs and timings

### Logical data flow

1. `toggle` command received by daemon
2. daemon transitions state (`Idle` to `Recording`, etc.)
3. audio chunk sent to ML worker
4. worker returns transcript and cleaned text
5. daemon attempts insertion
6. daemon emits notification + logs and returns to `Idle`

---

## 11. Performance, Reliability, and Error Handling

### Latency targets (MVP)

| Stage | Target |
| --- | --- |
| Audio finalize | <= 100ms |
| Parakeet transcription | <= 800ms (p95 target) |
| LLM cleanup | <= 600ms (p95 target) |
| Insertion/copy | <= 100ms |
| End-to-end stop->insert/copy | <= 2.0s stretch, <= 3.0s acceptable |

### Timeout and retry policy

| Stage | Timeout | Retry |
| --- | --- | --- |
| ASR | 1.5s | 1 retry on transient failures |
| LLM cleanup | 1.2s | 1 retry on transient failures |
| Insertion | 0.2s | 1 retry, then clipboard fallback |

### Failure taxonomy (user-facing stage labels)

- `Recording`
- `Audio finalize`
- `Speech recognition`
- `Text cleanup`
- `Text insertion`
- `Startup`

### Required failure UX

- If ASR fails after final retry:
  - Title: `Transcription failed`
  - Detail: `Stage: Speech recognition (<short_reason>)`

- Generic failure format:
  - Title: `Action failed`
  - Detail: `Stage: <stage_name> (<short_reason>)`

Internal logs may contain full technical details; user notifications stay concise.

---

## 12. Platform Support Matrix

| Environment | Support level | Notes |
| --- | --- | --- |
| Wayland (GNOME/KDE baseline) | Primary | First-class path for MVP |
| X11 | Secondary | Best-effort fallback path |
| GPU present | Preferred | Lower latency for ASR/LLM |
| CPU-only | Supported (degraded) | May miss stretch latency target |

MVP acceptance requires reliable operation on at least one Wayland desktop configuration.

---

## 13. Privacy and Security Requirements

1. All inference and processing is local.
2. No audio/text payloads are transmitted off device.
3. Temporary artifacts are local and short-lived.
4. Local logs must avoid storing raw audio.

---

## 14. Configuration (MVP + Near-MVP)

Config file location example:

`~/.config/voxi/config.yaml`

Example fields:

```yaml
hotkey: Super+I
asr_model: parakeet
llm_runtime: ollama
llm_model: gemma3b
output_mode: auto_insert_then_clipboard
notification_timeout_ms: 2200
```

---

## 15. Deployment and Runtime

### Runtime assumptions

- Linux desktop session
- Local Parakeet available
- Local LLM runtime available

### Process management

- Daemon runs as a user service (recommended: systemd user service).
- ML worker launched and supervised by daemon.

---

## 16. MVP Acceptance Criteria

MVP is complete when all items below pass:

1. User presses `Super+I` and recording begins immediately.
2. User presses `Super+I` again and processing begins.
3. While processing/inserting, hotkey press does not start another run.
4. Local ASR returns transcript for normal speech input.
5. Local LLM cleanup preserves meaning and returns cleaned text.
6. Text is inserted in a supported active app.
7. If insertion fails, text is copied to clipboard and user is informed.
8. If ASR fails after retry, user sees:
   - `Transcription failed`
   - concise stage detail line
9. End-to-end latency meets minimum acceptable MVP target on supported machine.
10. No cloud calls occur during dictation flow.

---

## 17. Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Wayland hotkey variability across environments | High | Explicit support matrix; compositor-specific adapter strategy |
| Insertion reliability across apps/toolkits | High | Multi-strategy insertion + clipboard fallback |
| Model cold start latency | High | Daemon-first warm worker |
| ML worker instability (GPU/runtime errors) | Medium | Process isolation + supervised restart |
| Meaning drift in cleanup output | Medium | strict prompt + deterministic settings |

---

## 18. Milestones (Suggested)

### M1 - Core loop prototype

- daemon state machine
- hotkey toggle
- recording + ASR + clipboard output

### M2 - Cleanup and insertion reliability

- LLM cleanup integration
- insertion adapter + fallback handling
- failure messages with stage info

### M3 - Hardening

- retries/timeouts
- metrics and logs
- Wayland baseline validation + packaging

---

## 19. Explicit Out-of-Scope Reminder

Do not add these in MVP:

- transcript editor UI
- streaming partial captions
- command mode
- advanced workflow automation
- cloud sync/integration

The best MVP is still the shortest path to dependable daily use.
