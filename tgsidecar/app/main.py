from fastapi import FastAPI

from . import store
from .telegram_api import router


def create_app() -> FastAPI:
    app = FastAPI(title="sadbot tgsidecar", docs_url=None, redoc_url=None)
    app.include_router(router)
    store.ensure_sessions_dir()
    return app


app = create_app()
