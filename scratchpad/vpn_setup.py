"""Full VPN bot setup — runs inside sadbot-tgsidecar (has python3 + network).

Steps: login panel → clone content bot → join sources → ensure folder "vpn"
      → PUT content settings (text_only, sidecar publish, @mr_vpn_here)
      → start bot → print summary.
"""
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

PANEL = os.environ.get("PANEL", "http://sadbot-backend:8080/api/v1")
SIDECAR = os.environ.get("SIDECAR", "http://tgsidecar:8000")
KEY = os.environ["SIDECAR_KEY"]
PASS = os.environ["PANEL_PASS"]

SRC_HANDLE = "8b67"          # partial match of the crypto content bot username
DEST_CHANNEL = "-1004463474801"   # @mr_vpn_here «آقای وی پی ان»
DEST_HANDLE = "@mr_vpn_here"
ACCOUNT_SIDECAR = "9d65f63e35f447ebb1e3d077a6e5184b"
SOURCES = ["@V2ry_Proxy"]
FOLDER = "vpn"
CLONE_NAME = "بات محتوا وی‌پی‌ان"
VIA = "sidecar"

CLEAN_PROMPT = """تو ویرایشگر متن کانال تلگرامی «آقای وی‌پی‌ان» هستی.
ورودی: متن یک پیام از کانال‌های کانفیگ/وی‌پی‌ان.

وظیفه: متن را برای انتشار در کانال خودمان بازنویسی کن:
۱) چیدمان و ایموجی‌های پراکنده را مرتب کن؛ خوانا و یکدست بنویس.
۲) لینک‌های کانفیگ (vless://، vmess://، ss://، trojan:// و...) را حفظ کن اما
   حتماً داخل بلوک کد «```» بگذار تا کپی‌شان آسان باشد.
۳) یک خط تبلیغاتی جذاب و کوتاه به انتهای پیام اضافه کن که فالو کانال
   @mr_vpn_here را بخواهد. لحن صمیمی و انگیزشی.
۴) اگر متن اصلاً مربوط به وی‌پی‌ان/کانفیگ/اینترنت/فیلترشکن نبود، فقط بنویس: NO_CONTENT
خروجی: فقط متن نهایی پست، بدون توضیح اضافه."""


def req(method, url, body=None, headers=None, timeout=90):
    h = dict(headers or {})
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        h.setdefault("Content-Type", "application/json")
    r = urllib.request.Request(url, data=data, headers=h, method=method)
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            raw = resp.read()
            status = resp.status
    except urllib.error.HTTPError as e:
        raw = e.read()
        status = e.code
    try:
        parsed = json.loads(raw) if raw else {}
    except ValueError:
        parsed = {"raw": raw.decode(errors="replace")[:400]}
    return status, parsed


def main():
    # ---- panel login ----
    st, tok = req("POST", f"{PANEL}/auth/login",
                  {"email": "admin@sadbot.local", "password": PASS})
    if st >= 400:
        print("LOGIN_FAIL", st, tok)
        sys.exit(1)
    A = {"Authorization": f"Bearer {tok['access_token']}",
         "X-Sidecar-Key": KEY}

    # ---- find the trading content bot (source for clone) ----
    st, bots = req("GET", f"{PANEL}/bots", headers=A)
    blist = bots.get("bots", bots if isinstance(bots, list) else [])
    src = None
    for b in blist:
        blob = ((b.get("name") or "") + (b.get("telegram_username") or "")).lower()
        if b.get("template_key") == "content" and "trader" in src_handle_blob(blob):
            src = b
            break
    if src is None:  # fall back: any content bot
        src = next((b for b in blist if b.get("template_key") == "content"), None)
    if not src:
        print("NO_CONTENT_BOT", [b.get("name") for b in blist])
        sys.exit(1)
    print("source:", src["id"], src.get("name"))

    # ---- resolve panel account id for the shared MTProto account ----
    st, setg = req("GET", f"{PANEL}/bots/{src['id']}/content/settings", headers=A)
    accounts = setg.get("accounts") or []
    acc = next((a for a in accounts
                if a.get("sidecar_account_id") == ACCOUNT_SIDECAR), (accounts[0] if accounts else None))
    if not acc:
        print("NO_ACCOUNT", st, setg)
        sys.exit(1)
    panel_acc = acc["id"]
    print("panel account:", panel_acc, acc.get("label"))

    # ---- join sources ----
    for s in SOURCES:
        st, r = req("POST", f"{SIDECAR}/accounts/{ACCOUNT_SIDECAR}/join",
                    {"chat": s}, headers=A)
        print("join", s, st, r)

    # ---- ensure folder ----
    st, r = req("POST", f"{SIDECAR}/accounts/{ACCOUNT_SIDECAR}/folders",
                {"name": FOLDER, "chats": SOURCES}, headers=A)
    print("folder ensure", st, r)
    if st >= 400:
        sys.exit(1)
    st, folders = req("GET", f"{SIDECAR}/accounts/{ACCOUNT_SIDECAR}/folders", headers=A)
    for f in folders.get("folders", []):
        print("  folder:", f["name"], "->", [p["title"] for p in f["peers"]])

    # ---- clone the bot ----
    st, cloned = req("POST", f"{PANEL}/bots/{src['id']}/clone",
                     {"account_id": panel_acc, "name": CLONE_NAME},
                     headers=A, timeout=300)
    if st >= 400 or "bot" not in cloned:
        print("CLONE_FAIL", st, cloned)
        sys.exit(1)
    nb = cloned["bot"]
    print("cloned:", nb["id"], nb.get("name"), nb.get("telegram_username"),
          "cfg_err=", cloned.get("config_error"))

    # ---- put content settings ----
    st, cur = req("GET", f"{PANEL}/bots/{nb['id']}/content/settings", headers=A)
    if st >= 400:
        print("GET_SETTINGS_FAIL", st, cur)
        sys.exit(1)
    body = {
        "version": cur["version"],
        "account_id": panel_acc,
        "folder": FOLDER,
        "channel": DEST_CHANNEL,
        "publish_via": VIA,
        "text_only": True,
        # the source channel @mr_vpn_here itself is not in the folder (it has no
        # username in dialogs), exclude the already-weight-loss channels by pattern
        "exclude": [],
        "keywords": [],
        "poll_minutes": 2,
        "backfill_limit": 15,
        "min_length": 20,
        "skip_forwarded": False,
        "skip_replies": True,
        "render_card_summary": False,
        "brand_key": "mr_vpn",
        "brand_name_en": "mr_vpn_here",
        "brand_name_fa": "آقای وی‌پی‌ان",
        "brand_handle": DEST_HANDLE,
        "brand_glow_a": "rgba(34, 197, 94, 0.28)",
        "brand_glow_b": "rgba(59, 130, 246, 0.20)",
        "brand_glow_c": "rgba(16, 185, 129, 0.12)",
        "brand_accent": "#22C55E",
        "clean_prompt": CLEAN_PROMPT,
        "card_prompt": "",
    }
    st, saved = req("PUT", f"{PANEL}/bots/{nb['id']}/content/settings",
                    body, headers=A)
    print("settings PUT", st, "ok" if st < 400 else saved)
    if st >= 400:
        sys.exit(1)

    # ---- start the bot ----
    st, r = req("POST", f"{PANEL}/bots/{nb['id']}/start", headers=A, timeout=180)
    print("start", st, r)
    st, b2 = req("GET", f"{PANEL}/bots/{nb['id']}", headers=A)
    print("status:", b2.get("status"), "|", b2.get("name"), b2.get("telegram_username"))

    print("DONE bot_id=" + nb["id"])


def src_handle_blob(blob):
    return blob.replace("@", "")


if __name__ == "__main__":
    main()