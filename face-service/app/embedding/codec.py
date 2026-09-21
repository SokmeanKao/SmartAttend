import base64

import numpy as np


def encode_embedding(embedding: np.ndarray) -> str:
    vector = np.asarray(embedding, dtype=np.float32).reshape(-1)
    if not np.isfinite(vector).all():
        raise ValueError("embedding must contain only finite values")
    little_endian = vector.astype("<f4", copy=False)
    return base64.b64encode(little_endian.tobytes(order="C")).decode("ascii")


def decode_embedding(encoded: str, expected_dim: int) -> np.ndarray:
    try:
        raw = base64.b64decode(encoded, validate=True)
    except (ValueError, base64.binascii.Error) as error:
        raise ValueError("embedding is not valid base64") from error
    if len(raw) != expected_dim * 4:
        raise ValueError("embedding has an invalid byte length")
    vector = np.frombuffer(raw, dtype="<f4")
    if not np.isfinite(vector).all():
        raise ValueError("embedding must contain only finite values")
    return vector
