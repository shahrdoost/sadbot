"""One-shot E2E for the text-only ad-line fix, then full cleanup.
Adds claude-logs to the folder (backfill=1 so only the newest message is
processed), posts one config-like test message there as DA, waits for the
collector to publish it into @mr_vpn_here, asserts the brand line, then
deletes both messages and restores folder + settings."""
import json
import os
import sys
import time
import urllib.error
import urllib.request

PANEL = os.environ.get("PANEL", "http://sadbot-backend:8080/api/v1")
SIDECAR = os.environ.get("SIDECAR", "http://tgsidecar:8000")
KEY = os.environ["SIDECAR_KEY"]
PASS = os.environ["PANEL_PASS"]
BOT = "2d86d7bb-c9e2-4559-89e3-cb78aac3bfb8"
ACC_SIDECAR = "9d65f63e35f447ebb1e3d077a6e5184b"
LBL = "claude-logs"
LBL_ID = "-1004362422574"
DEST = "-1004463474801"
TEST_SRC = (
    "🖥 سرور جدید V2rayNG — پیام تستی سیستم (حذف می‌شود)\n"
    "تمامی نت‌ها: 💜\n"
    "```\n"
    "vless://12345678-1234-4321-8765-123456789abc@example-test.com:443"
    "?security=tls&type=ws&path=%2Ftest&mode=multi#test-e2e\n"
    "```\n"
    "کانال اصلی را دنبال کنید."
)


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
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read() or b"{}")
        except ValueError:
            return e.code, {"raw": e.read().decode(errors="replace")[:300]}


def main():
    st, tok = req("POST", f"{PANEL}/auth/login",
                  {"email": "admin@sadbot.local", "password": PASS})
    A = {"Authorization": f"Bearer {tok['access_token']}", "X-Sidecar-Key": KEY}
    sacc = {"X-Sidecar-Key": KEY}
    ok = True

    # wipe leftovers of a previous aborted run (known test msg id)
    for leftover in (827, 826, 825):
        st, r = req("DELETE", f"{SIDECAR}/accounts/{ACC_SIDECAR}/chats/{LBL_ID}/messages/{leftover}",
                    headers=sacc)
        print("old test msg", leftover, ":", st)

    # folder extended AFTER the test message is posted (see below), so the
    # collector's first claude-logs backfill can only ever pick the test msg
    # 1) settings: backfill=1 (only the newest claude-logs msg gets processed)
    st, cur = req("GET", f"{PANEL}/bots/{BOT}/content/settings", headers=A)
    body = {k: cur[k] for k in (
        "version", "folder", "channel", "publish_via", "exclude", "keywords",
        "poll_minutes", "backfill_limit", "min_length", "skip_forwarded",
        "skip_replies", "render_card_summary", "text_only", "brand_key",
        "brand_name_en", "brand_name_fa", "brand_handle", "brand_glow_a",
        "brand_glow_b", "brand_glow_c", "brand_accent", "clean_prompt", "card_prompt")}
    body["backfill_limit"] = 1
    body["account_id"] = ACC_SIDECAR
    st, r = req("PUT", f"{PANEL}/bots/{BOT}/content/settings", body, headers=A)
    print("settings backfill=1:", st, "ok" if st < 400 else r)

    # 2) post the test message into claude-logs as DA *first*
    before = req("GET", f"{PANEL}/bots/{BOT}/content/posts?page=1&size=1", headers=A)[1].get("total", 0)
    st, r = req("POST", f"{SIDECAR}/accounts/{ACC_SIDECAR}/chats/{LBL_ID}/messages",
                {"text": TEST_SRC}, headers=sacc)
    if st >= 400:
        print("SEND FAILED", st, r)
        sys.exit(1)
    src_id = r["message"]["id"]
    print("test src msg id:", src_id)

    # 3) now extend the folder with claude-logs (backfill=1 + newest = test msg)
    st, r = req("POST", f"{SIDECAR}/accounts/{ACC_SIDECAR}/folders",
                {"name": "vpn", "chats": ["@V2ry_Proxy", LBL_ID]}, headers=sacc)
    print("folder now:", st, r)
    if st >= 400:
        sys.exit(1)

    # 4) wait for the collector (poll ≤ 2min + processing)
    deadline = time.time() + 240
    new_post = None
    while time.time() < deadline:
        st, led = req("GET", f"{PANEL}/bots/{BOT}/content/posts?page=1&size=5", headers=A)
        posts = led.get("posts") or []
        if led.get("total", 0) > before:
            new_post = next((p for p in posts
                              if p.get("source_title") in (LBL, "claude-logs")
                              or p.get("message_id")), None)
            if posts and posts[0].get("status") in ("published", "no_content", "failed"):
                new_post = posts[0]
                break
        time.sleep(10)
    if not new_post:
        print("WAIT TIMEOUT — no new ledger row")
        ok = False
        new_post = {}
    else:
        print("new ledger row:", new_post.get("status"), "|",
              (new_post.get("cleaned_text") or "")[:80].replace("\n", " "))

    # 5) read the destination's newest text and assert the brand line
    st, r = req("GET", f"{SIDECAR}/accounts/{ACC_SIDECAR}/chats/{DEST}/messages?limit=3",
                headers=sacc)
    texts = [m for m in (r.get("messages") or []) if m.get("text")]
    dest = texts[-1] if texts else None
    dest_id = None
    if dest:
        dest_id = dest["id"]
        has_handle = "@mr_vpn_here" in dest["text"]
        print("dest msg", dest_id, "handle_present:", has_handle,
              "| tail:", dest["text"][-90:].replace("\n", " "))
        if not has_handle or "test-e2e" not in dest["text"]:
            print("AD-LINE CHECK FAILED")
            ok = False
        else:
            print("AD-LINE CHECK PASSED")
    else:
        print("no dest message found")
        ok = False

    # 6) cleanup: delete dest + source messages, restore folder/settings
    if dest_id:
        st, r = req("DELETE", f"{SIDECAR}/accounts/{ACC_SIDECAR}/chats/{DEST}/messages/{dest_id}",
                    headers=sacc)
        print("cleanup dest:", st, r)
    st, r = req("DELETE", f"{SIDECAR}/accounts/{ACC_SIDECAR}/chats/{LBL_ID}/messages/{src_id}",
                headers=sacc)
    print("cleanup src:", st, r)
    st, r = req("POST", f"{SIDECAR}/accounts/{ACC_SIDECAR}/folders",
                {"name": "vpn", "chats": ["@V2ry_Proxy"]}, headers=sacc)
    print("restore folder:", st, r)
    st, cur = req("GET", f"{PANEL}/bots/{BOT}/content/settings", headers=A)
    body = {k: cur[k] for k in (
        "version", "folder", "channel", "publish_via", "exclude", "keywords",
        "poll_minutes", "backfill_limit", "min_length", "skip_forwarded",
        "skip_replies", "render_card_summary", "text_only", "brand_key",
        "brand_name_en", "brand_name_fa", "brand_handle", "brand_glow_a",
        "brand_glow_b", "brand_glow_c", "brand_accent", "clean_prompt", "card_prompt")}
    body["backfill_limit"] = 15
    body["account_id"] = ACC_SIDECAR
    st, r = req("PUT", f"{PANEL}/bots/{BOT}/content/settings", body, headers=A)
    print("restore backfill:", st, r)
    print("RESULT:", "PASS" if ok else "FAIL")
    sys.exit(0 if ok else 2)


if __name__ == "__main__":
    main()
