from fastapi import FastAPI

from app.api.routes import router

app = FastAPI()
app.include_router(router)


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}
