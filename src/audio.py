import logging

import numpy as np
import sounddevice as sd

logger = logging.getLogger(__name__)

SAMPLE_RATE = 16000
CHANNELS = 1

_stream: sd.InputStream | None = None
_buffer: list[np.ndarray] = []


def _callback(indata: np.ndarray, frames: int, time, status) -> None:
    if status:
        logger.warning(f"Audio stream status: {status}")
    _buffer.append(indata.copy())


def start_recording() -> None:
    global _stream

    _buffer.clear()

    _stream = sd.InputStream(
        samplerate=SAMPLE_RATE,
        channels=CHANNELS,
        dtype="float32",
        callback=_callback,
    )
    _stream.start()


def stop_recording() -> np.ndarray:
    global _stream

    _stream.stop()
    _stream.close()
    _stream = None

    audio = np.concatenate(_buffer, axis=0)
    _buffer.clear()

    return audio
