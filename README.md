# Voxi

A fast, local, GPU-accelerated voice dictation tool for Linux (Wayland/GNOME), powered by Nvidia Parakeet.

## Architecture

Voxi uses a Client-Server architecture to ensure instant startup times by keeping the heavy ASR model warm in GPU VRAM.

*   **The Daemon (Server):** A background Python service that loads the Parakeet model, handles audio recording (`sounddevice`), runs transcription, copies the result to the clipboard (`wl-clipboard`), and manages all desktop notifications.
*   **The Client (CLI):** A lightning-fast script (`voxi-toggle`) triggered by a GNOME global shortcut. It only sends a "TOGGLE" command to the daemon via Unix Domain Sockets (`/tmp/voxi.sock`).

## Workflow

1.  **Start:** Press global shortcut -> `voxi-toggle` sends a "TOGGLE" command to the daemon via the socket.
2.  **Record:** Daemon starts capturing audio in memory and triggers a "🔴 Listening" desktop notification.
3.  **Stop:** Press global shortcut again -> `voxi-toggle` sends another "TOGGLE".
4.  **Process:** Daemon stops recording, triggers a "⚙️ Transcribing" notification, and begins GPU transcription.
5.  **Output:** Daemon finishes transcription, pastes the text to the Wayland clipboard, and triggers a final "✅ Copied" notification.

## System Requirements

*   **OS:** Linux with Wayland (Tested on Debian 13 / GNOME)
*   **Hardware:** Nvidia GPU (Tested on RTX 3050 6GB)
*   **System Dependencies:** `wl-clipboard` (clipboard management), `libnotify-bin` (`notify-send` for feedback)