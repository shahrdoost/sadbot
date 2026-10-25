"""Inject a promo message into claude-logs and assert the collector drops it."""
import json
import os
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

PROMO = (
    "⚽ Barcelona 🆚 PSG ⚽\n"
    "پیش‌بینی بازی امشب رو بفرست تا از بین بهترین‌ها ۵ کانفیگ هدیه بگیری 🎁\n"
    "نحوه شرکت: نتیجه بازی رو همینجا بنویس\n"
    "فقط تا ساعت ۱۰ وقت داری"
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
    dest_before = req("GET", f"{SIDECAR}/accounts/{ACC}/chats/{DEST}/messages?limit=1",
                      headers=sacc)[1].get("messages") or []
    dest0 = dest_before[0]["id"] if dest_before else 0

    st, r = req("POST", f"{SIDECAR}/accounts/{ACC}/chats/{LBL}/messages", {"text": PROMO}, headers=sacc)
    if st >= 400:
        print("SEND FAIL", st, r)
        return 1
    src_id = r["message"]["id"]
    print("promo src id:", src_id)

    st, r = req("POST", f"{SIDECAR}/accounts/{ACC}/folders",
                {"name": "vpn", "chats": PROD_CHATS + [LBL]}, headers=sacc)
    print("folder:", st, r.get("count"))
    if st >= 400:
        return 1

    deadline = time.time() + 200
    row = None
    while time.time() < deadline:
        st, led = req("GET", f"{PANEL}/bots/{BOT}/content/posts?page=1&size=5", headers=A)
        if led.get("total", 0) > before:
            posts = led.get("posts") or []
            row = next((p for p in posts if p.get("message_id") == src_id or p.get("id")), None)
            if row:
                break
        time.sleep(10)

    if not row:
        print("TIMEOUT — no ledger row")
        result = "FAIL"
    else:
        print("ledger:", row.get("status"), "|", (row.get("cleaned_text") or "")[:60].replace("\n", " "))
        if row.get("status") == "no_content":
            print("PROMO FILTER: PASS")
            result = "PASS"
        else:
            print("PROMO FILTER: FAIL — status =", row.get("status"))
            result = "FAIL"

    dest = req("GET", f"{SIDECAR}/accounts/{ACC}/chats/{DEST}/messages?limit=2", headers=sacc)[1].get("messages") or []
    leaked = [m for m in dest if m.get("id", 0) > dest0 and "Barcelona" in (m.get("text") or "")]
    print("leaked to dest:", len(leaked))
    for m in leaked:
        req("DELETE", f"{SIDECAR}/accounts/{ACC}/chats/{DEST}/messages/{m['id']}", headers=sacc)
        print("cleaned leak", m["id"])

    req("DELETE", f"{SIDECAR}/accounts/{ACC}/chats/{LBL}/messages/{src_id}", headers=sacc)
    req("POST", f"{SIDECAR}/accounts/{ACC}/folders",
        {"name": "vpn", "chats": PROD_CHATS}, headers=sacc)
    print("restored folder, deleted src", src_id)
    print("RESULT:", result)
    return 0 if result == "PASS" else 2


if __name__ == "__main__":
    raise SystemExit(main())
