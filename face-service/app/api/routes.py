import base64
import struct

from fastapi import APIRouter, File, Form, UploadFile

router = APIRouter()

_UNIT_EMBEDDING = base64.b64encode(
    struct.pack("<128f", 1.0, *([0.0] * 127))
).decode("ascii")


@router.post("/internal/v1/faces/embed")
async def embed(
    image: UploadFile = File(...),
    expected_pose: str = Form(...),
) -> dict[str, object]:
    await image.read()
    pose = expected_pose.upper()
    yaw_scores = {"FRONT": 0.0, "LEFT": -0.4, "RIGHT": 0.4}
    return {
        "quality_score": 0.91,
        "pose": {"label": pose, "yaw_score": yaw_scores.get(pose, 0.0)},
        "embedding": _UNIT_EMBEDDING,
        "embedding_encoding": "float32-le-base64",
        "embedding_dim": 128,
        "model_name": "sface",
        "model_version": "2021dec",
    }
