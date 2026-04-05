import logging
import subprocess

logger = logging.getLogger(__name__)

APP_NAME = "Voxi"


def _send(message: str, urgency: str = "normal") -> None:
    """Send a desktop notification via notify-send.

    Args:
        message: The notification body text.
        urgency: One of 'low', 'normal', or 'critical'.
    """
    try:
        subprocess.run(
            ["notify-send", "-u", urgency, APP_NAME, message],
            check=True,
        )
    except FileNotFoundError:
        logger.error(
            "notify-send not found. Install libnotify-bin: "
            "sudo apt install libnotify-bin"
        )
    except subprocess.CalledProcessError as e:
        logger.error("notify-send failed (exit %d)", e.returncode)


def recording() -> None:
    """Mic is open and audio capture has started."""
    _send("🔴 Recording...", urgency="normal")


def transcribing() -> None:
    """Audio capture stopped; GPU transcription is running."""
    _send("⚙️ Transcribing...", urgency="normal")


def done() -> None:
    """Transcription complete and result copied to clipboard."""
    _send("✅ Copied to clipboard", urgency="normal")


def busy() -> None:
    """A TOGGLE arrived while already processing; inform the user."""
    _send("🚫 Still processing, please wait", urgency="normal")


def error(message: str = "Something went wrong") -> None:
    """An unexpected error occurred in the daemon."""
    _send(f"❌ {message}", urgency="critical")
