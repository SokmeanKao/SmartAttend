from io import BytesIO
from pathlib import Path

import numpy as np
import pytest
from PIL import Image

from app.config import Settings
from app.embedding.codec import decode_embedding, encode_embedding
from app.errors import FacePipelineError
from app.pipeline import EmbeddingPipeline
from app.quality.checks import assess_face_quality, decode_image


def _front_detection() -> np.ndarray:
    return np.asarray(
        [
            20,
            20,
            120,
            120,
            56,
            62,
            104,
            62,
            80,
            86,
            62,
            110,
            98,
            110,
            0.99,
        ],
        dtype=np.float32,
    )


def _png_bytes(rgb: tuple[int, int, int] = (255, 0, 0)) -> bytes:
    output = BytesIO()
    Image.new("RGB", (200, 180), rgb).save(output, format="PNG")
    return output.getvalue()


def test_decode_image_returns_bgr_uint8():
    image = decode_image(
        _png_bytes(),
        encoded_max_bytes=1024 * 1024,
        max_dimension=4096,
        max_pixels=16_000_000,
        min_short_side=160,
    )

    assert image.dtype == np.uint8
    assert image.shape == (180, 200, 3)
    assert image[0, 0].tolist() == [0, 0, 255]


def test_decode_image_applies_exif_orientation():
    output = BytesIO()
    exif = Image.Exif()
    exif[274] = 6
    Image.new("RGB", (200, 180), (1, 2, 3)).save(
        output, format="JPEG", exif=exif
    )

    image = decode_image(
        output.getvalue(),
        encoded_max_bytes=1024 * 1024,
        max_dimension=4096,
        max_pixels=16_000_000,
        min_short_side=160,
    )

    assert image.shape == (200, 180, 3)


def test_decode_image_rejects_decoded_dimension_limit():
    with pytest.raises(FacePipelineError) as error:
        decode_image(
            _png_bytes(),
            encoded_max_bytes=1024 * 1024,
            max_dimension=190,
            max_pixels=16_000_000,
            min_short_side=160,
        )

    assert error.value.code == "VALIDATION_ERROR"


def test_face_quality_rejects_blurry_region():
    image = np.full((180, 200, 3), 127, dtype=np.uint8)

    with pytest.raises(FacePipelineError) as error:
        assess_face_quality(
            image,
            _front_detection(),
            min_face_size=80,
            min_sharpness=20.0,
            min_exposure=40.0,
            max_exposure=215.0,
            min_quality=0.35,
        )

    assert error.value.code == "FACE_TOO_BLURRY"


class _Detector:
    def detect_one(self, image: np.ndarray) -> np.ndarray:
        return _front_detection()


class _Recognizer:
    def extract(self, image: np.ndarray, face: np.ndarray) -> np.ndarray:
        return np.arange(1, 129, dtype=np.float32)


def test_pipeline_returns_real_normalized_embedding_metadata():
    settings = Settings(
        min_sharpness=0.0,
        min_quality=0.0,
        min_exposure=0.0,
        max_exposure=255.0,
    )
    pipeline = EmbeddingPipeline(
        settings=settings,
        detector=_Detector(),
        recognizer=_Recognizer(),
    )

    result = pipeline.embed(_png_bytes(), "FRONT")
    vector = decode_embedding(result.embedding, expected_dim=128)

    assert result.embedding_encoding == "float32-le-base64"
    assert result.embedding_dim == 128
    assert result.model_name == "sface"
    assert result.model_version == "2021dec"
    assert result.pose.label == "FRONT"
    assert np.linalg.norm(vector) == pytest.approx(1.0, abs=1e-6)


def test_pipeline_rejects_wrong_expected_pose():
    settings = Settings(
        min_sharpness=0.0,
        min_quality=0.0,
        min_exposure=0.0,
        max_exposure=255.0,
    )
    pipeline = EmbeddingPipeline(
        settings=settings,
        detector=_Detector(),
        recognizer=_Recognizer(),
    )

    with pytest.raises(FacePipelineError) as error:
        pipeline.embed(_png_bytes(), "LEFT")

    assert error.value.code == "INVALID_POSE"


def test_verify_uses_maximum_cosine_similarity_and_threshold():
    settings = Settings(
        min_sharpness=0.0,
        min_quality=0.0,
        min_exposure=0.0,
        max_exposure=255.0,
        face_match_threshold=0.363,
    )
    pipeline = EmbeddingPipeline(
        settings=settings,
        detector=_Detector(),
        recognizer=_Recognizer(),
    )
    live = np.arange(1, 129, dtype=np.float32)
    live /= np.linalg.norm(live)
    low = -live
    high = live.copy()

    result = pipeline.verify(
        _png_bytes(),
        [
            {
                "pose": "LEFT",
                "embedding": encode_embedding(low),
                "embedding_encoding": "float32-le-base64",
                "embedding_dim": 128,
                "model_name": "sface",
                "model_version": "2021dec",
            },
            {
                "pose": "FRONT",
                "embedding": encode_embedding(high),
                "embedding_encoding": "float32-le-base64",
                "embedding_dim": 128,
                "model_name": "sface",
                "model_version": "2021dec",
            },
        ],
    )

    assert result.matched is True
    assert result.best_score == pytest.approx(1.0)
    assert result.matched_pose == "FRONT"
    assert not hasattr(result, "embedding")


def test_verify_rejects_model_mismatch():
    settings = Settings(
        min_sharpness=0.0,
        min_quality=0.0,
        min_exposure=0.0,
        max_exposure=255.0,
    )
    pipeline = EmbeddingPipeline(
        settings=settings,
        detector=_Detector(),
        recognizer=_Recognizer(),
    )
    reference = {
        "pose": "FRONT",
        "embedding": encode_embedding(np.ones(128, dtype=np.float32) / np.sqrt(128)),
        "embedding_encoding": "float32-le-base64",
        "embedding_dim": 128,
        "model_name": "other-model",
        "model_version": "2021dec",
    }

    with pytest.raises(FacePipelineError) as error:
        pipeline.verify(_png_bytes(), [reference])

    assert error.value.code == "MODEL_VERSION_MISMATCH"


def test_real_yunet_sface_positive_path_when_models_are_available():
    settings = Settings()
    if not settings.yunet_model_path.is_file() or not settings.sface_model_path.is_file():
        pytest.skip("pinned models are downloaded during the Docker build")
    fixture = Path(__file__).parent / "fixtures" / "front-face-public-domain.jpg"

    result = EmbeddingPipeline(settings).embed(fixture.read_bytes(), "FRONT")
    vector = decode_embedding(result.embedding, expected_dim=128)

    assert result.pose.label == "FRONT"
    assert result.quality_score >= settings.min_quality
    assert np.isfinite(vector).all()
    assert np.linalg.norm(vector) == pytest.approx(1.0, abs=1e-6)
