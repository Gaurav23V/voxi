# Voxi

A fast, local, GPU-accelerated voice dictation tool for Linux (GNOME), powered by Nvidia Parakeet.

## Architecture

Voxi uses a Client-Server architecture to ensure instant startup times by keeping the heavy ASR model warm in GPU VRAM.

*   **The Daemon (Server):** A background Python service that loads the Parakeet model, handles audio recording (`sounddevice`), runs transcription, copies the result to the clipboard, and manages all desktop notifications.
*   **The Client (CLI):** A lightning-fast script (`voxi-toggle`) triggered by a GNOME global shortcut. It only sends a "TOGGLE" command to the daemon via Unix Domain Sockets (`/tmp/voxi.sock`).

## Workflow

1.  **Start:** Press global shortcut -> `voxi-toggle` sends a "TOGGLE" command to the daemon via the socket.
2.  **Record:** Daemon starts capturing audio in memory and triggers a "🔴 Listening" desktop notification.
3.  **Stop:** Press global shortcut again -> `voxi-toggle` sends another "TOGGLE".
4.  **Process:** Daemon stops recording, triggers a "⚙️ Transcribing" notification, and begins GPU transcription.
5.  **Output:** Daemon finishes transcription, pastes the text to the clipboard, and triggers a final "✅ Copied" notification.

## System Requirements

*   **OS:** Linux with GNOME (Tested on Debian 13)
*   **Hardware:** Nvidia GPU (Tested on RTX 3050 6GB)
*   **System Dependencies:** `wl-clipboard` or `xclip` (clipboard management), `libnotify-bin` (`notify-send` for feedback)

## Project Structure

```
voxi/
├── src/
│   ├── daemon.py        # Main entry point — socket server, state machine, orchestration
│   ├── audio.py         # Mic stream management via sounddevice (open, buffer, close)
│   ├── transcriber.py   # Loads Parakeet model into VRAM at startup, runs transcription
│   └── notify.py        # Thin wrapper around notify-send for desktop notifications
│
├── client/
│   └── voxi-toggle      # The client — sends a single TOGGLE command to the daemon socket
│
├── systemd/
│   └── voxi.service     # systemd user service template (auto-start on login)
│
├── scripts/
│   └── install.sh       # One-shot setup: venv, symlinks, systemd registration
│
├── docs/
│   └── daemon.md        # In-depth documentation of the daemon internals
│
└── pyproject.toml       # Project metadata and dependencies (managed by uv)
```

## Installation

> [!IMPORTANT]
> **Nvidia drivers must be installed on your system before proceeding.** This project requires a working Nvidia GPU with the appropriate driver for your distro. Driver installation is out of scope for this guide — refer to your distro's documentation or the [Nvidia driver installation guide](https://docs.nvidia.com/cuda/cuda-installation-guide-linux/).

Run the install script from the project root:

```bash
./scripts/install.sh
```

The script handles everything:
- Installing system dependencies (libportaudio2, wl-clipboard, xclip, libnotify-bin)
- Installing uv and Python 3.11
- Setting up the Python environment
- Copying the project to `~/.local/share/voxi`
- Symlinking the client to `~/.local/bin/voxi-toggle`
- Registering the systemd user service
- Starting the daemon

Once installed, bind a GNOME keyboard shortcut to `voxi-toggle`:

1. Open **Settings → Keyboard → Keyboard Shortcuts → Custom Shortcuts**
2. Click **Add Shortcut**
3. Enter a name (e.g. "Voxi") and set the command to `voxi-toggle`
4. Press your desired key combination (e.g. `Super + V`)

You're ready to use Voxi — press the shortcut once to start recording, again to stop and transcribe.

## Local Setup (Manual)

> [!IMPORTANT]
> **Nvidia drivers must be installed on your system before proceeding.** This project requires a working Nvidia GPU with the appropriate driver for your distro. Driver installation is out of scope for this guide — refer to your distro's documentation or the [Nvidia driver installation guide](https://docs.nvidia.com/cuda/cuda-installation-guide-linux/).

**Check your CUDA version and update `pyproject.toml` before running `uv sync`:**

```bash
nvidia-smi   # look for "CUDA Version: XX.X" in the top-right corner
```

Open `pyproject.toml` and update the index URL to match your CUDA version:
- CUDA 11.8 → `https://download.pytorch.org/whl/cu118`
- CUDA 12.1 → `https://download.pytorch.org/whl/cu121`
- CUDA 12.4 → `https://download.pytorch.org/whl/cu124`  ← default in this repo

Once confirmed, run the following block to set up everything:

```bash
# 1. Install uv (Python package and environment manager)
curl -LsSf https://astral.sh/uv/install.sh | sh

# 2. Install system-level dependencies
sudo apt install libportaudio2 wl-clipboard xclip libnotify-bin

# 3. Install Python 3.11 via uv
uv python install 3.11

# 4. Create the virtual environment and install all Python dependencies
uv sync
```

The `.venv/` directory will be created in the project root with all dependencies installed and ready.

## Troubleshooting

### Voxi breaks after waking PC from Sleep (CUDA Unknown Error)
By default, Nvidia drivers on Linux do not preserve GPU memory (VRAM) during system sleep/suspend. When your computer wakes up, the VRAM where the AI model was loaded becomes corrupted, causing the daemon to silently fail with a `CUDA unknown error` when you try to dictate.

To permanently fix this, Nvidia provides a kernel-level feature to safely save VRAM to your system memory during sleep. We have included an automated helper script to configure this for you.

Simply run:
```bash
sudo ./scripts/fix_nvidia_suspend.sh
```
*(This will safely update your GRUB configuration, enable the required systemd suspend services, and update your bootloader. You must **reboot your computer** after running this script).*