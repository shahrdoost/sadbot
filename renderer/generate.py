"""Shared render logic — ported from sadcontent/render/generate.py.

Kept faithful to the original: string-replace templates, base64 Vazirmatn
fonts, fit_text truncation, data-URI media. The HTTP app (app.py) wraps
these functions with a shared Chromium instance.
"""

import base64
import html as html_mod
from datetime import datetime
from pathlib import Path

RENDER_DIR = Path(__file__).resolve().parent
FONTS_DIR = RENDER_DIR / "fonts"
TEMPLATES = RENDER_DIR / "templates"

SIZES = {
    "post": (1080, 1080),
    "story": (1080, 1920),
}

DEFAULT_BRAND = {
    "key": "trader",
    "name_en": "mr.trade.ai",
    "name_fa": "آقای تریدر",
    "handle": "@mr.trade.ai",
    "glow_a": "rgba(239, 68, 68, 0.28)",
    "glow_b": "rgba(249, 115, 22, 0.20)",
    "glow_c": "rgba(251, 146, 60, 0.12)",
    "accent": "#FFA070",
}


def font_css() -> str:
    faces = []
    for weight, name in [
        (400, "Vazirmatn-Regular.ttf"),
        (500, "Vazirmatn-Medium.ttf"),
        (700, "Vazirmatn-Bold.ttf"),
        (800, "Vazirmatn-ExtraBold.ttf"),
        (900, "Vazirmatn-Black.ttf"),
    ]:
        path = FONTS_DIR / name
        if not path.exists():
            continue
        b64 = base64.b64encode(path.read_bytes()).decode()
        faces.append(
            f"@font-face{{font-family:'Vazirmatn';font-style:normal;"
            f"font-weight:{weight};font-display:block;"
            f"src:url(data:font/ttf;base64,{b64}) format('truetype');}}"
        )
    return "\n".join(faces)


_FONT_CSS_CACHE = None


def _get_font_css() -> str:
    global _FONT_CSS_CACHE
    if _FONT_CSS_CACHE is None:
        _FONT_CSS_CACHE = font_css()
    return _FONT_CSS_CACHE


def _data_uri(b64: str, mime: str) -> str:
    return f"data:{mime};base64,{b64}"


def fit_text(text: str, fmt: str, has_image: bool) -> tuple[str, int]:
    """Truncate long text and pick a font size that stays inside the card."""
    text = (text or "").strip()
    if not text:
        return "", 40

    if fmt == "story":
        if has_image:
            max_chars, base_px = 320, 38
        else:
            max_chars, base_px = 520, 58
    else:
        if has_image:
            max_chars, base_px = 180, 32
        else:
            max_chars, base_px = 320, 46

    if len(text) > max_chars:
        cut = text[:max_chars]
        # prefer a complete-sentence cut: last sentence ender inside the window
        best = -1
        for mark in ("!", "?", "؟", ".", "\n"):
            pos = cut.rfind(mark)
            if pos > best:
                best = pos
        if best >= max_chars * 0.45:
            text = cut[: best + 1].strip()
        elif "\n" in cut:
            text = cut.rsplit("\n", 1)[0].strip() + "…"
        else:
            cut = cut.rsplit(" ", 1)[0] if " " in cut else cut
            text = cut.strip() + "…"

    n = len(text)
    if n > max_chars * 0.75:
        px = max(int(base_px * 0.8), 28)
    else:
        px = base_px
    return text, px


def build_html(
    fmt: str,
    text: str,
    image_b64: str | None,
    image_mime: str,
    date_str: str,
    brand: dict,
) -> str:
    tpl = (TEMPLATES / f"{fmt}.html").read_text(encoding="utf-8")
    has_image = bool(image_b64)
    fitted, font_px = fit_text(text, fmt, has_image)
    esc_text = html_mod.escape(fitted, quote=False)
    media_html = ""
    if has_image:
        media_html = f'<div class="media-wrap"><img src="{_data_uri(image_b64, image_mime)}"></div>'
        card_cls = "has-image"
    else:
        card_cls = "solo"

    inline_font = f"font-size:{font_px}px !important;"
    return (
        tpl.replace("__FONT_CSS__", _get_font_css())
        .replace("__BRAND_EN__", html_mod.escape(brand.get("name_en", ""), quote=False))
        .replace("__BRAND_FA__", html_mod.escape(brand.get("name_fa", ""), quote=False))
        .replace("__HANDLE__", html_mod.escape(brand.get("handle", ""), quote=False))
        .replace("__GLOW_A__", brand.get("glow_a", DEFAULT_BRAND["glow_a"]))
        .replace("__GLOW_B__", brand.get("glow_b", DEFAULT_BRAND["glow_b"]))
        .replace("__GLOW_C__", brand.get("glow_c", DEFAULT_BRAND["glow_c"]))
        .replace("__ACCENT__", brand.get("accent", DEFAULT_BRAND["accent"]))
        .replace("__MEDIA__", media_html)
        .replace("__CARD_CLASS__", card_cls)
        .replace("__TEXT_CLASS__", card_cls)
        .replace("__TEXT__", esc_text)
        .replace("__DATE__", html_mod.escape(date_str, quote=False))
        .replace("__TEXT_STYLE__", inline_font)
    )
