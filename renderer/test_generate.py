"""Smoke tests — no Chromium required (fit_text + template substitution)."""

from generate import build_html, fit_text


def test_fit_text_short_passthrough():
    text, px = fit_text("قیمت بیت‌کوین رشد کرد", "post", False)
    assert text == "قیمت بیت‌کوین رشد کرد"
    assert px == 46


def test_fit_text_truncates_with_ellipsis():
    long = "کلمه " * 200  # > 320 chars for post/solo
    text, px = fit_text(long, "post", False)
    assert len(text) <= 321  # 320 + possible …
    assert text.endswith("…")


def test_fit_text_prefers_sentence_boundary():
    base = "جملهٔ اول تمام شد. "
    long = base * 30
    text, _ = fit_text(long, "post", False)
    # cut should land on a sentence end, not mid-word with …
    assert text.endswith(".")


def test_fit_text_shrinks_font_when_long():
    almost = "x" * 300  # > 0.75*320 after truncation window
    _, px = fit_text(almost, "post", False)
    assert px == 36  # int(46*0.8)


def test_build_html_solo_substitutions():
    html = build_html(
        "post",
        "متن آزمایشی",
        None,
        "image/png",
        "2026-10-20",
        {"name_en": "test.brand", "name_fa": "تست", "handle": "@test",
         "glow_a": "rgba(1,2,3,0.1)", "glow_b": "rgba(4,5,6,0.1)",
         "glow_c": "rgba(7,8,9,0.1)", "accent": "#123456"},
    )
    assert "__FONT_CSS__" not in html
    assert "__BRAND_EN__" not in html
    assert "test.brand" in html
    assert "#123456" in html
    assert "moot" not in html
    assert "Vazirmatn" in html  # fonts inlined
    assert "solo" in html


def test_build_html_with_image():
    # 1x1 transparent png
    png_b64 = (
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk"
        "+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
    )
    html = build_html(
        "story", "خبر", png_b64, "image/png", "2026-10-20",
        {"name_en": "b", "name_fa": "ب", "handle": "@b"},
    )
    assert "data:image/png;base64," in html
    assert "has-image" in html


def test_build_html_escapes_script():
    html = build_html(
        "post", '<script>alert(1)</script>', None, "image/png", "d", {}
    )
    assert "<script>alert(1)</script>" not in html
    assert "&lt;script&gt;" in html
