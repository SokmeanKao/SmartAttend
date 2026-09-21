import base64
import hashlib

import numpy as np
import pytest

from app.detection.yunet import select_single_face
from app.embedding.codec import decode_embedding, encode_embedding
from app.errors import FacePipelineError
from app.pose.yaw_score import classify_yaw, validate_expected_pose, yaw_score
from scripts.download_models import verify_sha256


def _detection(x: float, width: float, confidence: float = 0.95) -> np.ndarray:
    # bbox, right eye, left eye, nose, right mouth, left mouth, confidence
    return np.asarray(
        [
            x,
            10,
            width,
            width,
            x + width * 0.30,
            10 + width * 0.35,
            x + width * 0.70,
            10 + width * 0.35,
            x + width * 0.50,
            10 + width * 0.55,
            x + width * 0.35,
            10 + width * 0.75,
            x + width * 0.65,
            10 + width * 0.75,
            confidence,
        ],
        dtype=np.float32,
    )


def test_model_sha_verification_accepts_pinned_digest(tmp_path):
    model = tmp_path / "model.onnx"
    model.write_bytes(b"pinned model")

    verify_sha256(model, hashlib.sha256(b"pinned model").hexdigest().upper())


def test_model_sha_verification_rejects_mismatch(tmp_path):
    model = tmp_path / "model.onnx"
    model.write_bytes(b"tampered model")

    with pytest.raises(ValueError, match="SHA256 mismatch"):
        verify_sha256(model, "0" * 64)


def test_embedding_codec_is_float32_little_endian_round_trip():
    vector = np.arange(1, 129, dtype=np.float32)
    vector /= np.linalg.norm(vector)

    encoded = encode_embedding(vector)

    assert len(base64.b64decode(encoded)) == 128 * 4
    decoded = decode_embedding(encoded, expected_dim=128)
    assert decoded.dtype == np.dtype("<f4")
    np.testing.assert_array_equal(decoded, vector.astype("<f4"))


def test_embedding_codec_rejects_non_finite_values():
    vector = np.zeros(128, dtype=np.float32)
    vector[0] = np.nan

    with pytest.raises(ValueError, match="finite"):
        encode_embedding(vector)


def test_select_single_face_reports_face_not_found():
    with pytest.raises(FacePipelineError) as error:
        select_single_face(
            np.empty((0, 15), dtype=np.float32),
            min_confidence=0.9,
            min_face_size=80,
        )

    assert error.value.code == "FACE_NOT_FOUND"


def test_select_single_face_ignores_non_qualifying_detections():
    with pytest.raises(FacePipelineError) as error:
        select_single_face(
            np.stack([_detection(0, 120, confidence=0.5), _detection(150, 40)]),
            min_confidence=0.9,
            min_face_size=80,
        )

    assert error.value.code == "FACE_NOT_FOUND"


def test_select_single_face_reports_multiple_faces():
    with pytest.raises(FacePipelineError) as error:
        select_single_face(
            np.stack([_detection(0, 120), _detection(150, 100)]),
            min_confidence=0.9,
            min_face_size=80,
        )

    assert error.value.code == "MULTIPLE_FACES"


def test_landmark_yaw_uses_nose_position_between_eye_midpoint_and_mouth_midpoint():
    front = _detection(0, 120)
    left = front.copy()
    left[8] -= 20
    right = front.copy()
    right[8] += 20

    assert yaw_score(left) < yaw_score(front) < yaw_score(right)
    assert classify_yaw(yaw_score(front), front_max=0.15, side_min=0.25) == "FRONT"
    assert classify_yaw(yaw_score(left), front_max=0.15, side_min=0.25) == "LEFT"
    assert classify_yaw(yaw_score(right), front_max=0.15, side_min=0.25) == "RIGHT"


def test_pose_validation_rejects_out_of_band_pose():
    with pytest.raises(FacePipelineError) as error:
        validate_expected_pose(
            expected_pose="LEFT",
            score=0.0,
            front_max=0.15,
            side_min=0.25,
        )

    assert error.value.code == "INVALID_POSE"
