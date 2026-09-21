from pathlib import Path

import cv2
import numpy as np

from app.errors import FacePipelineError


def select_single_face(
    detections: np.ndarray | None,
    *,
    min_confidence: float,
    min_face_size: int,
) -> np.ndarray:
    if detections is None:
        qualifying = []
    else:
        qualifying = [
            face
            for face in np.asarray(detections, dtype=np.float32).reshape(-1, 15)
            if float(face[14]) >= min_confidence
            and min(float(face[2]), float(face[3])) >= min_face_size
        ]
    if not qualifying:
        raise FacePipelineError("FACE_NOT_FOUND", "no qualifying face was detected")
    if len(qualifying) > 1:
        raise FacePipelineError(
            "MULTIPLE_FACES", "more than one qualifying face was detected"
        )
    return np.asarray(qualifying[0], dtype=np.float32)


class YuNetDetector:
    def __init__(
        self,
        model_path: Path,
        *,
        score_threshold: float,
        nms_threshold: float,
        top_k: int,
        min_face_size: int,
    ) -> None:
        self._score_threshold = score_threshold
        self._min_face_size = min_face_size
        self._detector = cv2.FaceDetectorYN_create(
            str(model_path),
            "",
            (320, 320),
            score_threshold,
            nms_threshold,
            top_k,
        )

    def detect_one(self, image: np.ndarray) -> np.ndarray:
        height, width = image.shape[:2]
        self._detector.setInputSize((width, height))
        _, detections = self._detector.detect(image)
        return select_single_face(
            detections,
            min_confidence=self._score_threshold,
            min_face_size=self._min_face_size,
        )
