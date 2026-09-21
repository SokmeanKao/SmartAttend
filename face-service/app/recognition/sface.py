from pathlib import Path

import cv2
import numpy as np

from app.errors import FacePipelineError

EMBEDDING_DIM = 128


def normalize_embedding(embedding: np.ndarray) -> np.ndarray:
    vector = np.asarray(embedding, dtype=np.float32).reshape(-1)
    if vector.size != EMBEDDING_DIM:
        raise FacePipelineError(
            "MODEL_VERSION_MISMATCH",
            f"SFace returned dimension {vector.size}, expected {EMBEDDING_DIM}",
        )
    if not np.isfinite(vector).all():
        raise FacePipelineError(
            "FACE_QUALITY_TOO_LOW", "SFace returned a non-finite embedding"
        )
    norm = float(np.linalg.norm(vector))
    if not np.isfinite(norm) or norm <= 1e-12:
        raise FacePipelineError(
            "FACE_QUALITY_TOO_LOW", "SFace returned an invalid embedding"
        )
    normalized = np.asarray(vector / norm, dtype=np.float32)
    if not np.isfinite(normalized).all():
        raise FacePipelineError(
            "FACE_QUALITY_TOO_LOW", "normalized embedding is non-finite"
        )
    return normalized


class SFaceRecognizer:
    def __init__(self, model_path: Path) -> None:
        self._recognizer = cv2.FaceRecognizerSF_create(str(model_path), "")

    def extract(self, image: np.ndarray, face: np.ndarray) -> np.ndarray:
        aligned = self._recognizer.alignCrop(
            image, np.asarray(face, dtype=np.float32)
        )
        feature = self._recognizer.feature(aligned)
        return normalize_embedding(feature)
