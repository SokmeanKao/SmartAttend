from dataclasses import asdict, dataclass
from typing import Protocol

import numpy as np

from app.config import Settings
from app.detection.yunet import YuNetDetector
from app.embedding.codec import encode_embedding
from app.pose.yaw_score import validate_expected_pose, yaw_score
from app.quality.checks import assess_face_quality, decode_image
from app.recognition.sface import EMBEDDING_DIM, SFaceRecognizer, normalize_embedding


class Detector(Protocol):
    def detect_one(self, image: np.ndarray) -> np.ndarray: ...


class Recognizer(Protocol):
    def extract(self, image: np.ndarray, face: np.ndarray) -> np.ndarray: ...


@dataclass(frozen=True)
class DetectedPose:
    label: str
    yaw_score: float


@dataclass(frozen=True)
class EmbeddingResult:
    quality_score: float
    pose: DetectedPose
    embedding: str
    embedding_encoding: str = "float32-le-base64"
    embedding_dim: int = EMBEDDING_DIM
    model_name: str = "sface"
    model_version: str = "2021dec"

    def to_dict(self) -> dict[str, object]:
        return asdict(self)


class EmbeddingPipeline:
    def __init__(
        self,
        settings: Settings,
        detector: Detector | None = None,
        recognizer: Recognizer | None = None,
    ) -> None:
        self._settings = settings
        self._detector = detector or YuNetDetector(
            settings.yunet_model_path,
            score_threshold=settings.detector_score_threshold,
            nms_threshold=settings.detector_nms_threshold,
            top_k=settings.detector_top_k,
            min_face_size=settings.min_face_size,
        )
        self._recognizer = recognizer or SFaceRecognizer(settings.sface_model_path)

    def embed(self, encoded_image: bytes, expected_pose: str) -> EmbeddingResult:
        settings = self._settings
        image = decode_image(
            encoded_image,
            encoded_max_bytes=settings.encoded_max_bytes,
            max_dimension=settings.max_dimension,
            max_pixels=settings.max_pixels,
            min_short_side=settings.min_short_side,
        )
        face = self._detector.detect_one(image)
        quality_score = assess_face_quality(
            image,
            face,
            min_face_size=settings.min_face_size,
            min_sharpness=settings.min_sharpness,
            min_exposure=settings.min_exposure,
            max_exposure=settings.max_exposure,
            min_quality=settings.min_quality,
        )
        score = yaw_score(face)
        label = validate_expected_pose(
            expected_pose=expected_pose,
            score=score,
            front_max=settings.pose_front_max,
            side_min=settings.pose_side_min,
        )
        embedding = normalize_embedding(self._recognizer.extract(image, face))
        return EmbeddingResult(
            quality_score=quality_score,
            pose=DetectedPose(label=label, yaw_score=score),
            embedding=encode_embedding(embedding),
        )
