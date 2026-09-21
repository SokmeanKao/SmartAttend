from dataclasses import dataclass
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from app.config import Settings
from app.errors import FacePipelineError
from app.main import create_app


@dataclass
class _Result:
    def to_dict(self) -> dict[str, object]:
        return {
            "quality_score": 0.9,
            "pose": {"label": "FRONT", "yaw_score": 0.0},
            "embedding": "AAAA",
            "embedding_encoding": "float32-le-base64",
            "embedding_dim": 128,
            "model_name": "sface",
            "model_version": "2021dec",
        }


class _Pipeline:
    def __init__(self, error: FacePipelineError | None = None) -> None:
        self.error = error
        self.calls: list[tuple[bytes, str]] = []

    def embed(self, image: bytes, expected_pose: str) -> _Result:
        self.calls.append((image, expected_pose))
        if self.error:
            raise self.error
        return _Result()


def test_embed_route_uses_pipeline_and_returns_contract():
    pipeline = _Pipeline()
    app = create_app(pipeline=pipeline)

    with TestClient(app) as client:
        response = client.post(
            "/internal/v1/faces/embed",
            files={"image": ("face.png", b"image bytes", "image/png")},
            data={"expected_pose": "front"},
        )

    assert response.status_code == 200
    assert response.json()["model_version"] == "2021dec"
    assert pipeline.calls == [(b"image bytes", "front")]


def test_embed_route_returns_structured_face_error():
    pipeline = _Pipeline(
        FacePipelineError("FACE_NOT_FOUND", "no qualifying face was detected")
    )
    app = create_app(pipeline=pipeline)

    with TestClient(app) as client:
        response = client.post(
            "/internal/v1/faces/embed",
            files={"image": ("face.png", b"image bytes", "image/png")},
            data={"expected_pose": "FRONT"},
        )

    assert response.status_code == 422
    assert response.json() == {
        "error": {
            "code": "FACE_NOT_FOUND",
            "message": "no qualifying face was detected",
        }
    }


def test_startup_refuses_model_hash_mismatch(tmp_path: Path):
    (tmp_path / "face_detection_yunet_2023mar.onnx").write_bytes(b"tampered")
    (tmp_path / "face_recognition_sface_2021dec.onnx").write_bytes(b"tampered")
    app = create_app(settings=Settings(model_dir=tmp_path))

    with pytest.raises(ValueError, match="SHA256 mismatch"):
        with TestClient(app):
            pass
