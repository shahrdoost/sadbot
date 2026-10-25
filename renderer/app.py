"""Render microservice: HTML card → PNG via a shared headless Chromium.

POST /render
  {"fmt": "post"|"story", "text": "...",
   "image_b64": "...", "image_mime": "image/jpeg",   # optional
   "brand": {"key","name_en","name_fa","handle","glow_a","glow_b","glow_c","accent"}}
  → 200 image/png

GET /healthz → {"status":"ok"}
"""

import base64
import threading
from contextlib import contextmanager
from datetime import datetime
from typing import Optional

from fastapi import FastAPI, HTTPException, Response
from pydantic import BaseModel, Field
from playwright.sync_api import sync_playwright

from generate import SIZES, build_html

app = FastAPI(title="sadbot-renderer")

MAX_IMAGE_BYTES = 8 * 1024 * 1024
MAX_TEXT_CHARS = 4000

CHROME_ARGS = ["--no-sandbox", "--force-color-profile=srgb"]


class Brand(BaseModel):
    key: str = "brand"
    name_en: str = ""
    name_fa: str = ""
    handle: str = ""
    glow_a: str = "rgba(239, 68, 68, 0.28)"
    glow_b: str = "rgba(249, 115, 22, 0.20)"
    glow_c: str = "rgba(251, 146, 60, 0.12)"
    accent: str = "#FFA070"


class RenderRequest(BaseModel):
    fmt: str = Field(pattern="^(post|story)$")
    text: str = ""
    image_b64: Optional[str] = None
    image_mime: str = "image/png"
    brand: Brand = Brand()


_state = {"playwright": None, "browser": None}
_lock = threading.Lock()  # one screenshot at a time — Chromium isn't thrilled about concurrency


def _ensure_browser():
    if _state["browser"] is None or not _state["browser"].is_connected():
        p = sync_playwright().start()
        _state["playwright"] = p
        _state["browser"] = p.chromium.launch(args=CHROME_ARGS)
    return _state["browser"]


@app.on_event("shutdown")
def _teardown():
    if _state["browser"] is not None:
        _state["browser"].close()
    if _state["playwright"] is not None:
        _state["playwright"].stop()


@app.get("/healthz")
def healthz():
    return {"status": "ok"}


@app.post("/render")
def render(req: RenderRequest):
    text = (req.text or "")[:MAX_TEXT_CHARS]
    image_b64 = req.image_b64
    if image_b64:
        # ~4/3 inflation from base64
        if len(image_b64) > MAX_IMAGE_BYTES * 4 // 3:
            raise HTTPException(status_code=413, detail="image too large")
        try:
            base64.b64decode(image_b64, validate=True)
        except Exception:
            raise HTTPException(status_code=400, detail="invalid image_b64")

    w, h = SIZES[req.fmt]
    html = build_html(
        req.fmt,
        text,
        image_b64,
        req.image_mime,
        datetime.now().strftime("%Y-%m-%d"),
        req.brand.model_dump(),
    )

    with _lock:
        try:
            browser = _ensure_browser()
            page = browser.new_page(viewport={"width": w, "height": h}, device_scale_factor=1)
            try:
                page.set_content(html, wait_until="load")
                page.wait_for_timeout(350)
                png = page.screenshot(clip={"x": 0, "y": 0, "width": w, "height": h})
            finally:
                page.close()
        except Exception as e:
            raise HTTPException(status_code=500, detail=f"render failed: {e}")

    return Response(content=png, media_type="image/png")
