"""Update folder "vpn" with the new source channels + tune settings.
Also rewrites clean_prompt so the model stops fighting our Pre-entity wrap.
Runs inside sadbot-tgsidecar.
"""
import json
import os
import sys
import urllib.request

PANEL = os.environ.get("PANEL", "http://sadbot-backend:8080/api/v1")
SIDECAR = os.environ.get("SIDECAR", "http://tgsidecar:8000")
KEY = os.environ["SIDECAR_KEY"]
PASS = os.environ["PANEL_PASS"]
BOT = "2d86d7bb-c9e2-4559-89e3-cb78aac3bfb8"
ACC = "9d65f63e35f447ebb1e3d077a6e5184b"

# numeric refs (resolve via _resolve_chat int path) + the original username
# seller-only channels (price lists, no free configs) are left out on purpose
CHATS = [
    "@V2ry_Proxy",
    "-1001269569829",  # proxystore11
    "-1001357677662",  # VlessConfig
    "-1001389758759",  # DirectVPN
    "-1001395363861",  # ProxyMTProto
    "-1001410119352",  # Outline_Vpn
    "-1001508838224",  # vmess_vless
    "-1001545626906",  # vmess_v2ray
    "-1001577254070",  # config_v2ray_free
    "-1001598760532",  # V2rayNG_free
    "-1001655923185",  # vpnclassic
    "-1001760842035",  # V2rayNGvpni
    "-1001789702327",  # PrivateVPNs
    "-1001827595090",  # free_v2ray_proxy
    "-1001831989474",  # ServerNett
    "-1001845786923",  # V2RayNG_Config
    "-1001852983773",  # v2ray_configs
    "-1001882497483",  # VPNCUSTOMIZE
]

CLEAN_PROMPT = """تو ویرایشگر متن کانال تلگرامی «آقای وی‌پی‌ان» هستی.
ورودی: متن یک پیام از کانال‌های کانفیگ/وی‌پی‌ان.

وظیفه: متن را برای انتشار در کانال خودمان بازنویسی کن:
۱) چیدمان و ایموجی‌های پراکنده را مرتب کن؛ خوانا و یکدست بنویس.
۲) لینک‌های کانفیگ (vless://، vmess://، ss://، trojan:// و...) را دقیقاً حفظ کن و
   هر کدام را روی یک خط جدا بگذار. هیچ علامت ` یا ``` ننویس — سیستم خودش
   لینک‌ها را قابل‌کپی (monospace) می‌کند.
۳) یک خط تبلیغاتی جذاب و کوتاه به انتهای پیام اضافه کن که فالو کانال
   @mr_vpn_here را بخواهد. لحن صمیمی و انگیزشی.
۴) فقط متنی را منتشر کن که برای کاربر رایگان مفید باشد: کانفیگ/لینک اتصال،
   آموزش کوتاه، یا خبر فوری درباره اینترنت/فیلترینگ.
۵) اگر متن تبلیغات فروش بود (قیمت پلن، تماس با فروشنده، «برای خرید پیام بدید»،
   شماره/آیدی فروشنده) و هیچ کانفیگ رایگانی توش نبود، فقط بنویس: NO_CONTENT
۶) اگر متن اصلاً مربوط به وی‌پی‌ان/کانفیگ/اینترنت/فیلترشکن نبود، فقط بنویس: NO_CONTENT
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
            return resp.status, json.loads(resp.read() or b"{}")
    except Exception as e:
        try:
            return e.code, json.loads(e.read() or b"{}")
        except Exception:
            return 0, {"err": str(e)}


def main():
    st, tok = req("POST", f"{PANEL}/auth/login",
                  {"email": "admin@sadbot.local", "password": PASS})
    if st >= 400:
        print("LOGIN_FAIL", st, tok)
        sys.exit(1)
    A = {"Authorization": f"Bearer {tok['access_token']}", "X-Sidecar-Key": KEY}
    sacc = {"X-Sidecar-Key": KEY}

    st, r = req("POST", f"{SIDECAR}/accounts/{ACC}/folders",
                {"name": "vpn", "chats": CHATS}, headers=sacc)
    print("folder:", st, "count=", r.get("count"), r if st >= 400 else "ok")
    if st >= 400:
        sys.exit(1)

    st, cur = req("GET", f"{PANEL}/bots/{BOT}/content/settings", headers=A)
    keys = ("version", "folder", "channel", "publish_via", "exclude", "keywords",
            "poll_minutes", "backfill_limit", "min_length", "skip_forwarded",
            "skip_replies", "render_card_summary", "text_only", "brand_key",
            "brand_name_en", "brand_name_fa", "brand_handle", "brand_glow_a",
            "brand_glow_b", "brand_glow_c", "brand_accent", "clean_prompt", "card_prompt")
    body = {k: cur[k] for k in keys}
    body["account_id"] = ACC
    body["backfill_limit"] = 4          # 20 sources × 15 would dump ~300 posts
    body["clean_prompt"] = CLEAN_PROMPT
    st, r = req("PUT", f"{PANEL}/bots/{BOT}/content/settings", body, headers=A)
    print("settings:", st, "backfill=4 clean_prompt updated" if st < 400 else r)

    st, fl = req("GET", f"{SIDECAR}/accounts/{ACC}/folders", headers=sacc)
    for f in fl.get("folders", []):
        if f.get("name") == "vpn":
            print("vpn folder peers:", len(f.get("peers", [])))


if __name__ == "__main__":
    main()
