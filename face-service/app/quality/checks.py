from io import BytesIO

import cv2
import numpy as np
from PIL import Image, ImageOps, UnidentifiedImageError

from app.errors import FacePipelineError

_ALLOWED_FORMATS = frozenset({"JPEG", "PNG"})


def decode_image(
    encoded: bytes,
    *,
    encoded_max_bytes: int,
    max_dimension: int,
    max_pixels: int,
    min_short_side: int,
) -> np.ndarray:
    if len(encoded) > encoded_max_bytes:
        raise FacePipelineError(
            "REQUEST_TOO_LARGE", "encoded image exceeds the size limit", 413
        )
    if not encoded:
        raise FacePipelineError("VALIDATION_ERROR", "image is empty", 400)

    try:
        with Image.open(BytesIO(encoded)) as source:
            if source.format not in _ALLOWED_FORMATS:
                raise FacePipelineError(
                    "VALIDATION_ERROR", "image must be JPEG or PNG", 400
                )
            width, height = source.size
            _validate_dimensions(
                width,
                height,
                max_dimension=max_dimension,
                max_pixels=max_pixels,
                min_short_side=min_short_side,
            )
            oriented = ImageOps.exif_transpose(source)
            oriented.load()
            rgb = np.asarray(oriented.convert("RGB"), dtype=np.uint8)
    except FacePipelineError:
        raise
    except (UnidentifiedImageError, OSError, ValueError) as error:
        raise FacePipelineError(
            "VALIDATION_ERROR", "image could not be decoded", 400
        ) from error

    height, width = rgb.shape[:2]
    _validate_dimensions(
        width,
        height,
        max_dimension=max_dimension,
        max_pixels=max_pixels,
        min_short_side=min_short_side,
    )
    return cv2.cvtColor(rgb, cv2.COLOR_RGB2BGR)


def _validate_dimensions(
    width: int,
    height: int,
    *,
    max_dimension: int,
    max_pixels: int,
    min_short_side: int,
) -> None:
    if width <= 0 or height <= 0:
        raise FacePipelineError("VALIDATION_ERROR", "image dimensions are invalid", 400)
    if width > max_dimension or height > max_dimension:
        raise FacePipelineError(
            "VALIDATION_ERROR", "image dimensions exceed the configured limit", 400
        )
    if width * height > max_pixels:
        raise FacePipelineError(
            "VALIDATION_ERROR", "decoded image exceeds the pixel limit", 400
        )
    if min(width, height) < min_short_side:
        raise FacePipelineError(
            "VALIDATION_ERROR", "image shortest side is too small", 400
        )


def assess_face_quality(
    image: np.ndarray,
    face: np.ndarray,
    *,
    min_face_size: int,
    min_sharpness: float,
    min_exposure: float,
    max_exposure: float,
    min_quality: float,
) -> float:
    x, y, width, height = (float(value) for value in face[:4])
    if min(width, height) < min_face_size:
        raise FacePipelineError("FACE_TOO_SMALL", "detected face is too small")

    image_height, image_width = image.shape[:2]
    left = max(0, int(np.floor(x)))
    top = max(0, int(np.floor(y)))
    right = min(image_width, int(np.ceil(x + width)))
    bottom = min(image_height, int(np.ceil(y + height)))
    if right <= left or bottom <= top:
        raise FacePipelineError("FACE_QUALITY_TOO_LOW", "face region is invalid")

    region = image[top:bottom, left:right]
    gray = cv2.cvtColor(region, cv2.COLOR_BGR2GRAY)
    sharpness = float(cv2.Laplacian(gray, cv2.CV_64F).var())
    if sharpness < min_sharpness:
        raise FacePipelineError("FACE_TOO_BLURRY", "face region is too blurry")

    exposure = float(gray.mean())
    if exposure < min_exposure or exposure > max_exposure:
        raise FacePipelineError(
            "FACE_QUALITY_TOO_LOW", "face region exposure is outside the allowed range"
        )

    size_score = min(1.0, min(width, height) / max(float(min_face_size) * 2.0, 1.0))
    sharpness_score = (
        1.0 if min_sharpness <= 0 else min(1.0, sharpness / (min_sharpness * 2.0))
    )
    exposure_score = max(0.0, 1.0 - abs(exposure - 127.5) / 127.5)
    quality_score = float(
        np.clip(
            0.35 * size_score + 0.40 * sharpness_score + 0.25 * exposure_score,
            0.0,
            1.0,
        )
    )
    if quality_score < min_quality:
        raise FacePipelineError(
            "FACE_QUALITY_TOO_LOW", "face quality score is below the configured limit"
        )
    return quality_score
