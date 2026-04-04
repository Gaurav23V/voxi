# Voxi Daemon

## Architecture Overview

Voxi has two components:

| Component | Role | Complexity |
|---|---|---|
| `voxi-daemon` | Does all the real work | Heavy |
| `voxi-toggle` | Sends a single `TOGGLE` message to the socket | Trivial |

This split exists for **startup speed**. A GNOME keyboard shortcut waits for the triggered program to finish. If the client loaded the ASR model on every keypress, you'd wait 3–5s before recording starts. The daemon keeps the model warm in VRAM permanently — the client just fires a message and exits in milliseconds.

The daemon handles everything:
- Loading the Parakeet model into GPU VRAM
- Recording audio (`sounddevice`)
- Running transcription (NeMo/CUDA)
- Copying result to clipboard (`wl-copy`)
- Sending desktop notifications (`notify-send`)

---

## Running as a systemd User Service

### System vs. User Services

We use a **user service** (not a system service) because:
- Runs under your own user account — no `sudo`
- Has natural access to Wayland, GPU, and D-Bus from your login session
- Starts automatically at login, not at system boot

### The `.service` File

Placed at `~/.config/systemd/user/voxi.service`:

```ini
[Unit]
Description=Voxi Voice Dictation Daemon
After=graphical-session.target        # wait until Wayland is ready

[Service]
Type=simple
ExecStart=/path/to/.venv/bin/python /path/to/daemon.py
Restart=on-failure                    # auto-restart if it crashes
RestartSec=3                          # wait 3s before restarting
Environment=WAYLAND_DISPLAY=wayland-1
Environment=XDG_RUNTIME_DIR=/run/user/1000

[Install]
WantedBy=default.target               # start when user session is active
```

**What each section does:**
- `[Unit]` — metadata and ordering (start *after* graphical session)
- `[Service]` — the actual behavior: what to run, restart policy, environment
- `[Install]` — when `systemctl --user enable` is run, creates a symlink that makes the service auto-start at login

### Managing the Service

```bash
systemctl --user daemon-reload        # reload after editing the .service file
systemctl --user enable voxi.service  # auto-start on login
systemctl --user start voxi.service   # start now
systemctl --user status voxi.service  # check status + PID
journalctl --user -u voxi.service -f  # view logs
```

---

## ExecStart

`ExecStart` is the single command systemd runs to launch the daemon. Rules:
- **Must be an absolute path** — systemd doesn't have a normal `$PATH`
- **Use the virtualenv's Python directly** — no need to activate the venv; pointing to `.venv/bin/python` automatically uses the right interpreter with all dependencies installed

```ini
ExecStart=/home/user/.local/share/voxi/.venv/bin/python /home/user/.local/share/voxi/daemon.py
```

---

## Inside the Python Process

When systemd runs `ExecStart`, the OS spawns a new Python interpreter process and runs `daemon.py` from top to bottom:

```
1. Imports and setup
2. Load Parakeet model → VRAM
3. Open Unix socket at /tmp/voxi.sock
4. Enter blocking event loop (sleeps, waiting for TOGGLE)
```

### 1. Loading Parakeet into VRAM

```python
import nemo.collections.asr as nemo_asr

model = nemo_asr.models.EncDecRNNTBPEModel.from_pretrained("nvidia/parakeet-tdt-1.1b")
model = model.cuda()   # transfers weights from RAM → GPU VRAM via PCIe
model.eval()           # inference mode: disables dropout, gradient tracking
```

- `from_pretrained()` loads weights (~2–4 GB) into system RAM (downloads on first run, cached after)
- `.cuda()` copies them to GPU VRAM — this is the slow step (~2–5s at startup)
- `.eval()` optimizes for inference
- The model stays resident in VRAM for the entire lifetime of the daemon

### 2. Opening the Unix Domain Socket

A Unix socket is an IPC (inter-process communication) channel that appears as a file on disk. The daemon creates it as a server; the client connects to it to send commands.

```python
import socket, os

SOCKET_PATH = "/tmp/voxi.sock"

if os.path.exists(SOCKET_PATH):
    os.unlink(SOCKET_PATH)           # clean up stale socket if daemon was killed

server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
server.bind(SOCKET_PATH)            # creates the file
server.listen(1)
```

The client (`voxi-toggle`) simply connects and writes:
```python
client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.connect("/tmp/voxi.sock")
client.send(b"TOGGLE\n")
client.close()
```

Unix sockets vs TCP sockets: same API, but no network stack — stays inside the kernel, faster, no port numbers needed.

### 3. The Idle Loop — CPU Usage

The daemon uses a **blocking** event loop, not a busy-wait loop:

```python
# ✅ Correct — blocking I/O
while True:
    conn, _ = server.accept()    # process sleeps here until a client connects
    data = conn.recv(1024)
    conn.close()
    if data.strip() == b"TOGGLE":
        handle_toggle()
```

`server.accept()` suspends the process until a connection arrives. While suspended:
- **CPU: ~0%** — the process is not scheduled by the OS
- **RAM/VRAM: unchanged** — model stays fully resident and ready

The OS uses interrupts, not polling — the kernel wakes the process the instant a connection arrives.

---

## Daemon Lifecycle — State Machine

The daemon is always in one of three states. The **only** trigger for each transition is a `TOGGLE` command from the client (or task completion for PROCESSING → IDLE):

```
┌─────────┐   TOGGLE   ┌───────────┐   TOGGLE   ┌─────────────┐
│  IDLE   │ ─────────→ │ RECORDING │ ──────────→ │ PROCESSING  │
└─────────┘            └───────────┘             └──────┬──────┘
     ↑                                                   │ done
     └───────────────────────────────────────────────────┘
```

Important rules:
- A `TOGGLE` during `PROCESSING` is **ignored** (can't interrupt transcription)
- There is no automatic timeout — the daemon records until the user presses the shortcut again

---

## Full Daemon Lifecycle

### Phase 1: IDLE → RECORDING (first TOGGLE)

1. Check state — if `PROCESSING`, ignore and return
2. Flip state to `RECORDING`
3. **Start audio capture first** — `sounddevice` opens the mic and begins streaming chunks into an in-memory buffer
4. **Then notify** `🔴 Recording...` — by the time the notification renders on screen (~100–200ms), the mic is already hot and capturing

> **Why this order matters:** If the notification fires before the mic is open, the user sees the cue and starts speaking before audio is being captured — losing the first word or two. Starting capture first guarantees the mic is live before the user reacts.

```python
audio_buffer = []

def audio_callback(indata, frames, time, status):
    audio_buffer.append(indata.copy())  # called continuously by sounddevice

stream = sd.InputStream(callback=audio_callback, samplerate=16000, channels=1)
stream.start()          # mic is live NOW
notify("🔴 Recording...")  # user sees this ~100-200ms later
```

The audio callback runs on a thread managed by `sounddevice`. The main thread stays free to listen on the socket for the second TOGGLE.

### Phase 2: RECORDING → PROCESSING (second TOGGLE)

1. Stop the audio stream (`stream.stop()` / `stream.close()`)
2. Flip state to `PROCESSING`
3. Notify `⚙️ Transcribing...`
4. **Spin up a worker thread** for transcription (see Threading Model below)
5. Main thread returns immediately to `server.accept()` — socket stays alive

### Phase 3: PROCESSING → IDLE (transcription done)

This happens inside the worker thread:

1. Concatenate all audio chunks into one array:
   ```python
   audio_data = np.concatenate(audio_buffer, axis=0)
   audio_buffer.clear()
   ```
2. Run transcription — the GPU-heavy step:
   ```python
   transcription = model.transcribe([audio_data])[0]
   ```
   Duration scales with audio length (~0.5–1s for a 10s clip on an RTX 3050).
3. Copy to clipboard:
   ```python
   subprocess.run(["wl-copy"], input=transcription.encode(), check=True)
   ```
4. Notify `✅ Copied to clipboard`
5. Flip state back to `IDLE`

---

## Threading Model

The daemon runs transcription in a **background worker thread** rather than blocking the main thread. This keeps the socket responsive during the ~1s GPU operation.

```
Main thread:    [accept] ──────────────────────────── [accept TOGGLE → "🚫 busy"] ──── [accept]
Worker thread:            [concatenate][GPU transcribe][wl-copy][notify ✅][state=IDLE]
```

**Shared state between threads** — just one flag:
```python
state = "IDLE"  # values: "IDLE", "RECORDING", "PROCESSING"
```

When a TOGGLE arrives during `PROCESSING`:
- Main thread reads `state == "PROCESSING"`
- Immediately sends a `🚫 Still processing, please wait` notification
- Does nothing else — no queuing

This gives the user immediate feedback instead of silent nothing, without any complex queuing logic.

---

## How systemd Watches the Process

When systemd starts the daemon, the OS assigns it a **PID** (process ID). systemd stores this and subscribes to kernel events for that PID.

```
systemctl --user status voxi.service
  Active: active (running)
  Main PID: 4821 (python)    ← systemd tracks this
```

If the process exits for any reason, the kernel instantly notifies systemd (zero CPU cost — interrupt-driven). systemd then checks the `Restart=` policy:

- `Restart=on-failure` → restarts if exit code ≠ 0 or killed by signal
- `RestartSec=3` → waits 3 seconds, then runs `ExecStart` again

systemd does **not** inspect memory or code — it only watches at the process level (alive or dead).

---

## Notifications Reference

All notifications use `notify-send` via subprocess:

| Trigger | Message | Urgency |
|---|---|---|
| Mic opens | 🔴 Recording... | `low` |
| Second TOGGLE received | ⚙️ Transcribing... | `low` |
| Transcription done | ✅ Copied to clipboard | `normal` |
| TOGGLE during PROCESSING | 🚫 Still processing, please wait | `low` |
| Any error | ❌ Error message | `critical` |

---

## Design Implications for the Daemon Code

- **Use `logging` not `print()`** — systemd captures stdout/stderr and routes them to the journal automatically
- **Handle `SIGTERM`** — systemd sends SIGTERM on `stop`; clean up the socket file before exiting
- **Use absolute paths** — the working directory under systemd is `/`
- **Set `WAYLAND_DISPLAY` explicitly** — don't rely on environment inheritance; set it in the `.service` file
- **Start mic before notifying** — `stream.start()` is synchronous; the mic is live before `notify-send` fires
- **Transcription runs in a worker thread** — keeps the socket responsive; state is coordinated via a single shared `state` string
