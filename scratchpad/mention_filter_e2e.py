"""Assert source-channel @handles never reach the destination channel."""
import json
import os
import re
import time
import urllib.request

PANEL = "http://sadbot-backend:8080/api/v1"
SIDECAR = "http://tgsidecar:8000"
KEY = os.environ["SIDECAR_KEY"]
PASS = os.environ["PANEL_PASS"]
BOT = "2d86d7bb-c9e2-4559-89e3-cb78aac3bfb8"
ACC = "9d65f63e35f447ebb1e3d077a6e5184b"
LBL = "-1004362422574"
DEST = "-1004463474801"
PROD_CHATS = [
    "@V2ry_Proxy",
    "-1001269569829", "-1001357677662", "-1001389758759", "-1001395363861",
    "-1001410119352", "-1001508838224", "-1001545626906", "-1001577254070",
    "-1001598760532", "-1001655923185", "-1001760842035", "-1001789702327",
    "-1001827595090", "-1001831989474", "-1001845786923", "-1001852983773",
    "-1001882497483",
]

SRC = (
    "🖥 سرور جدید V2rayNG — تست حذف آیدی\n"
    "تمامی نت‌ها: 💜\n"
    "```\n"
    "vless://12345678-1234-4321-8765-123456789abc@example-test.com:443"
    "?security=tls&type=ws&path=%2Ftest&mode=multi#test-mention-e2e\n"
    "```\n"
    "🔗 مرجع این خبر: @ProxyMTProto\n"
    "https://t.me/VlessConfig/99\n"
    "@SERVERNETT"
)


def req(method, url, body=None, headers=None, timeout=60):
    h = dict(headers or {})
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        h.setdefault("Content-Type", "application/json")
    r = urllib.request.Request(url, data=data, headers=h, method=method)
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read() or b"{}")
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read() or b"{}")
        except Exception:
            return e.code, {}


def main():
    st, tok = req("POST", f"{PANEL}/auth/login",
                  {"email": "admin@sadbot.local", "password": PASS})
    A = {"Authorization": f"Bearer {tok['access_token']}", "X-Sidecar-Key": KEY}
    sacc = {"X-Sidecar-Key": KEY}

    before = req("GET", f"{PANEL}/bots/{BOT}/content/posts?page=1&size=1", headers=A)[1].get("total", 0)
    dest0 = 0
    dest_before = req("GET", f"{SIDECAR}/accounts/{ACC}/chats/{DEST}/messages?limit=1",
                      headers=sacc)[1].get("messages") or []
    if dest_before:
        dest0 = dest_before[0]["id"]

    st, r = req("POST", f"{SIDECAR}/accounts/{ACC}/chats/{LBL}/messages", {"text": SRC}, headers=sacc)
    if st >= 400:
        print("SEND FAIL", st, r)
        return 1
    src_id = r["message"]["id"]
    print("src id:", src_id)

    req("POST", f"{SIDECAR}/accounts/{ACC}/folders",
        {"name": "vpn", "chats": PROD_CHATS + [LBL]}, headers=sacc)

    deadline = time.time() + 220
    row = None
    while time.time() < deadline:
        st, led = req("GET", f"{PANEL}/bots/{BOT}/content/posts?page=1&size=5", headers=A)
        if led.get("total", 0) > before:
            posts = led.get("posts") or []
            row = next((p for p in posts if p.get("message_id") == src_id), None) or (posts[0] if posts else None)
            if row and row.get("status") in ("published", "no_content", "failed"):
                break
        time.sleep(10)

    result = "PASS"
    if not row:
        print("TIMEOUT")
        result = "FAIL"
    else:
        text = row.get("cleaned_text") or ""
        print("ledger:", row.get("status"), "|", text[:90].replace("\n", " "))
        # ignore @host inside config URIs (vless://id@host is not a handle)
        scan = re.sub(
            r"(?i)(?:vless|vmess|ss|trojan|tuic)://\S+", "", text,
        )
        bad = re.findall(r"@[A-Za-z][A-Za-z0-9_]{3,}", scan)
        bad = [b for b in bad if b.lower() != "@mr_vpn_here"]
        leaks = [x for x in ("@ProxyMTProto", "@SERVERNETT", "t.me/VlessConfig", "مرجع این خبر") if x in text]
        print("foreign mentions:", bad, "leaks:", leaks)
        if leaks or bad:
            print("MENTION STRIP: FAIL")
            result = "FAIL"
        elif row.get("status") != "published":
            print("MENTION STRIP: FAIL — not published:", row.get("status"))
            result = "FAIL"
        else:
            print("MENTION STRIP: PASS")

    dest = req("GET", f"{SIDECAR}/accounts/{ACC}/chats/{DEST}/messages?limit=3", headers=sacc)[1].get("messages") or []
    for m in dest:
        t = m.get("text") or ""
        if m.get("id", 0) > dest0 and ("ProxyMTProto" in t or "SERVERNETT" in t or "VlessConfig" in t):
            req("DELETE", f"{SIDECAR}/accounts/{ACC}/chats/{DEST}/messages/{m['id']}", headers=sacc)
            print("removed leak from dest", m["id"])
            result = "FAIL"
        if m.get("id", 0) > dest0 and "test-mention-e2e" in t:
            req("DELETE", f"{SIDECAR}/accounts/{ACC}/chats/{DEST}/messages/{m['id']}", headers=sacc)
            print("cleaned test post", m["id"])

    req("DELETE", f"{SIDECAR}/accounts/{ACC}/chats/{LBL}/messages/{src_id}", headers=sacc)
    req("POST", f"{SIDECAR}/accounts/{ACC}/folders",
        {"name": "vpn", "chats": PROD_CHATS}, headers=sacc)
    print("restored")
    print("RESULT:", result)
    return 0 if result == "PASS" else 2


if __name__ == "__main__":
    raise SystemExit(main())
