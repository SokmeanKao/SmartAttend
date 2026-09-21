from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from app.api.routes import router
from app.config import Settings
from app.errors import FacePipelineError
from app.pipeline import EmbeddingPipeline
from scripts.download_models import verify_models


def create_app(
    *,
    settings: Settings | None = None,
    pipeline: EmbeddingPipeline | None = None,
) -> FastAPI:
    configured_settings = settings or Settings()

    @asynccontextmanager
    async def lifespan(application: FastAPI) -> AsyncIterator[None]:
        application.state.settings = configured_settings
        if pipeline is None:
            verify_models(configured_settings.model_dir)
            application.state.face_pipeline = EmbeddingPipeline(configured_settings)
        else:
            application.state.face_pipeline = pipeline
        yield

    application = FastAPI(lifespan=lifespan)
    application.include_router(router)

    @application.exception_handler(FacePipelineError)
    async def face_pipeline_error_handler(
        _request: Request, error: FacePipelineError
    ) -> JSONResponse:
        return JSONResponse(
            status_code=error.status_code,
            content={"error": {"code": error.code, "message": error.message}},
        )

    @application.get("/healthz")
    def healthz() -> dict[str, str]:
        return {"status": "ok"}

    return application


app = create_app()
