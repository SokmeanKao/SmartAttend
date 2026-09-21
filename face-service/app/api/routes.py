from fastapi import APIRouter, File, Form, Request, UploadFile

from app.errors import FacePipelineError

router = APIRouter()


@router.post("/internal/v1/faces/embed")
async def embed(
    request: Request,
    image: UploadFile = File(...),
    expected_pose: str = Form(...),
) -> dict[str, object]:
    max_bytes = request.app.state.settings.encoded_max_bytes
    encoded = await image.read(max_bytes + 1)
    if len(encoded) > max_bytes:
        raise FacePipelineError(
            "REQUEST_TOO_LARGE", "encoded image exceeds the size limit", 413
        )
    result = request.app.state.face_pipeline.embed(encoded, expected_pose)
    return result.to_dict()
