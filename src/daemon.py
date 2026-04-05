import logging
import os
import signal
import socket
import subprocess
import sys
import threading

import src.audio as audio
import src.notify as notify
import src.transcriber as transcriber

logging.basicConfig(level=logging.INFO, stream=sys.stderr)
logger = logging.getLogger(__name__)

SOCKET_PATH = "/tmp/voxi.sock"
_state = "IDLE"
_clipboard_tool: list[str] = []


def _set_state(new: str) -> None:
    global _state
    _state = new


def _get_state() -> str:
    return _state


def _detect_clipboard_tool() -> None:
    global _clipboard_tool
    session_type = os.environ.get("XDG_SESSION_TYPE", "")
    if session_type == "wayland":
        _clipboard_tool = ["wl-copy"]
        logger.info("Detected Wayland compositor — using wl-copy")
    else:
        _clipboard_tool = ["xclip", "-selection", "clipboard"]
        logger.info("Detected X11 session — using xclip")


# ---------------------------------------------------------------------------
# Worker thread
# ---------------------------------------------------------------------------

def _run_transcription(audio_data) -> None:
    try:
        text = transcriber.transcribe(audio_data)
        subprocess.run(_clipboard_tool, input=text.encode(), check=True)
        notify.done()
    except Exception:
        notify.error()
    finally:
        _set_state("IDLE")


# ---------------------------------------------------------------------------
# Toggle handler
# ---------------------------------------------------------------------------

def _handle_toggle() -> None:
    state = _get_state()

    if state == "IDLE":
        audio.start_recording()
        notify.recording()
        _set_state("RECORDING")

    elif state == "RECORDING":
        audio_data = audio.stop_recording()
        notify.transcribing()
        _set_state("PROCESSING")
        thread = threading.Thread(target=_run_transcription, args=(audio_data,))
        thread.start()

    elif state == "PROCESSING":
        notify.busy()


# ---------------------------------------------------------------------------
# SIGTERM — clean shutdown
# ---------------------------------------------------------------------------

def _sigterm_handler(signum, frame) -> None:
    logger.info("Received SIGTERM, shutting down...")
    if audio._stream is not None:  # noqa: SLF001
        audio._stream.stop()       # noqa: SLF001
        audio._stream.close()      # noqa: SLF001
    _server.close()
    if os.path.exists(SOCKET_PATH):
        os.unlink(SOCKET_PATH)
    sys.exit(0)


# ---------------------------------------------------------------------------
# Startup
# ---------------------------------------------------------------------------

def main() -> None:
    signal.signal(signal.SIGTERM, _sigterm_handler)

    logger.info("Loading Parakeet model into GPU VRAM...")
    try:
        transcriber.load_model()
        logger.info("Model ready.")
    except Exception as e:
        logger.error(f"Failed to load model: {e}")
        sys.exit(1)

    _detect_clipboard_tool()

    if os.path.exists(SOCKET_PATH):
        os.unlink(SOCKET_PATH)

    global _server
    _server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    _server.bind(SOCKET_PATH)
    _server.listen(1)
    logger.info(f"Socket listening at {SOCKET_PATH}")

    # ---------------------------------------------------------------------------
    # Main event loop
    # ---------------------------------------------------------------------------

    while True:
        conn, _ = _server.accept()
        data = conn.recv(1024)
        conn.close()

        if data.strip() == b"TOGGLE":
            _handle_toggle()


if __name__ == "__main__":
    main()
