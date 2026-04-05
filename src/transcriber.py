import logging

import nemo.collections.asr as nemo_asr
import numpy as np

logger = logging.getLogger(__name__)

MODEL_NAME = "nvidia/parakeet-tdt-0.6b-v2"

_model: nemo_asr.models.ASRModel | None = None


def load_model() -> None:
    global _model

    if _model is not None:
        return

    logger.info(f"Loading {MODEL_NAME} — this may take a few seconds...")
    model = nemo_asr.models.ASRModel.from_pretrained(MODEL_NAME)
    model = model.cuda()
    model.eval()
    _model = model
    logger.info("Model loaded and ready on GPU.")


def transcribe(audio: np.ndarray) -> str:
    if _model is None:
        raise RuntimeError("Model not loaded. Call load_model() first.")

    audio = audio.astype(np.float32)
    if audio.ndim > 1:
        audio = audio[:, 0]

    results = _model.transcribe([audio])
    result = results[0]

    if hasattr(result, "text"):
        return result.text.strip()
    return str(result).strip()
