from dataclasses import asdict, dataclass
from typing import Protocol

import numpy as np

from app.config import Settings
from app.detection.yunet import YuNetDetector
from app.embedding.codec import decode_embedding, encode_embedding
from app.errors import FacePipelineError
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


@dataclass(frozen=True)
class VerificationResult:
    matched: bool
    best_score: float
    matched_pose: str | None
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

    def verify(
        self,
        encoded_image: bytes,
        references: list[dict[str, object]],
    ) -> VerificationResult:
        if not references:
            raise FacePipelineError(
                "INVALID_REFERENCE_TEMPLATES", "at least one reference is required"
            )
        vectors: list[tuple[str, np.ndarray]] = []
        for reference in references:
            if not isinstance(reference, dict):
                raise FacePipelineError(
                    "INVALID_REFERENCE_TEMPLATES",
                    "each reference template must be an object",
                )
            if (
                reference.get("model_name") != "sface"
                or reference.get("model_version") != "2021dec"
                or reference.get("embedding_dim") != EMBEDDING_DIM
            ):
                raise FacePipelineError(
                    "MODEL_VERSION_MISMATCH",
                    "reference templates are incompatible with the active model",
                )
            if reference.get("embedding_encoding") != "float32-le-base64":
                raise FacePipelineError(
                    "INVALID_REFERENCE_TEMPLATES",
                    "reference embedding encoding is invalid",
                )
            try:
                vector = decode_embedding(
                    str(reference.get("embedding", "")), EMBEDDING_DIM
                )
                norm = float(np.linalg.norm(vector))
            except ValueError as error:
                raise FacePipelineError(
                    "INVALID_REFERENCE_TEMPLATES", str(error)
                ) from error
            if not np.isclose(norm, 1.0, atol=0.01):
                raise FacePipelineError(
                    "INVALID_REFERENCE_TEMPLATES",
                    "reference embedding is not L2 normalized",
                )
            vectors.append((str(reference.get("pose", "")), vector))

        settings = self._settings
        image = decode_image(
            encoded_image,
            encoded_max_bytes=settings.encoded_max_bytes,
            max_dimension=settings.max_dimension,
            max_pixels=settings.max_pixels,
            min_short_side=settings.min_short_side,
        )
        face = self._detector.detect_one(image)
        assess_face_quality(
            image,
            face,
            min_face_size=settings.min_face_size,
            min_sharpness=settings.min_sharpness,
            min_exposure=settings.min_exposure,
            max_exposure=settings.max_exposure,
            min_quality=settings.min_quality,
        )
        live = normalize_embedding(self._recognizer.extract(image, face))
        scores = [(pose, float(np.dot(live, vector))) for pose, vector in vectors]
        matched_pose, best_score = max(scores, key=lambda item: item[1])
        matched = best_score >= settings.face_match_threshold
        return VerificationResult(
            matched=matched,
            best_score=best_score,
            matched_pose=matched_pose if matched else None,
        )
