import os
from dataclasses import dataclass, field
from pathlib import Path


def _env_int(name: str, default: int) -> int:
    return int(os.getenv(name, str(default)))


def _env_float(name: str, default: float) -> float:
    return float(os.getenv(name, str(default)))


def _default_model_dir() -> Path:
    return Path(
        os.getenv(
            "MODEL_DIR",
            str(Path(__file__).resolve().parent.parent / "models"),
        )
    )


@dataclass(frozen=True)
class Settings:
    model_dir: Path = field(default_factory=_default_model_dir)
    encoded_max_bytes: int = field(
        default_factory=lambda: _env_int("IMAGE_MAX_BYTES", 5 * 1024 * 1024)
    )
    max_dimension: int = field(
        default_factory=lambda: _env_int("IMAGE_MAX_DIMENSION", 4096)
    )
    max_pixels: int = field(
        default_factory=lambda: _env_int("IMAGE_MAX_PIXELS", 16_000_000)
    )
    min_short_side: int = field(
        default_factory=lambda: _env_int("IMAGE_MIN_SHORT_SIDE", 160)
    )
    detector_score_threshold: float = field(
        default_factory=lambda: _env_float("YUNET_SCORE_THRESHOLD", 0.9)
    )
    detector_nms_threshold: float = field(
        default_factory=lambda: _env_float("YUNET_NMS_THRESHOLD", 0.3)
    )
    detector_top_k: int = field(
        default_factory=lambda: _env_int("YUNET_TOP_K", 5000)
    )
    min_face_size: int = field(
        default_factory=lambda: _env_int("FACE_MIN_SIZE", 80)
    )
    min_sharpness: float = field(
        default_factory=lambda: _env_float("FACE_MIN_SHARPNESS", 40.0)
    )
    min_exposure: float = field(
        default_factory=lambda: _env_float("FACE_MIN_EXPOSURE", 45.0)
    )
    max_exposure: float = field(
        default_factory=lambda: _env_float("FACE_MAX_EXPOSURE", 210.0)
    )
    min_quality: float = field(
        default_factory=lambda: _env_float("FACE_MIN_QUALITY", 0.35)
    )
    pose_front_max: float = field(
        default_factory=lambda: _env_float("POSE_FRONT_MAX", 0.15)
    )
    pose_side_min: float = field(
        default_factory=lambda: _env_float("POSE_SIDE_MIN", 0.25)
    )
    face_match_threshold: float = field(
        default_factory=lambda: _env_float("FACE_MATCH_THRESHOLD", 0.363)
    )

    @property
    def yunet_model_path(self) -> Path:
        return self.model_dir / "face_detection_yunet_2023mar.onnx"

    @property
    def sface_model_path(self) -> Path:
        return self.model_dir / "face_recognition_sface_2021dec.onnx"
