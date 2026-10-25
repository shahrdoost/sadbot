"""Live verification for the VPN bot — run inside sadbot-tgsidecar after setup.

Checks: ledger rows (published, no failed), config snapshot, and reads the last
messages of @mr_vpn_here directly from Telegram to confirm text posts landed.
"""
import asyncio
import json
import os
import sys
import urllib.request

PANEL = os.environ.get("PANEL", "http://sadbot-backend:8080/api/v1")
SIDECAR = os.environ.get("SIDECAR", "http://tgsidecar:8000")
KEY = os.environ["SIDECAR_KEY"]
PASS = os.environ["PANEL_PASS"]
DEST = "-1004463474801"          # @mr_vpn_here
ACC = "9d65f63e35f447ebb1e3d077a6e5184b"


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
        except ValueError:
            return e.code, {}


def main():
    st, tok = req("POST", f"{PANEL}/auth/login",
                  {"email": "admin@sadbot.local", "password": PASS})
    if st >= 400:
        print("LOGIN_FAIL"); sys.exit(1)
    A = {"Authorization": f"Bearer {tok['access_token']}", "X-Sidecar-Key": KEY}

    # find the vpn bot by name
    st, bots = req("GET", f"{PANEL}/bots", headers=A)
    blist = bots.get("bots", [])
    vpn = None
    for b in blist:
        if "وی" in (b.get("name") or "") or "vpn" in (b.get("name") or "").lower():
            vpn = b
            break
    if not vpn:
        print("VPN_BOT_NOT_FOUND", [(b.get("name"), b.get("template_key")) for b in blist])
        sys.exit(1)
    print("bot:", vpn["id"], vpn.get("name"), vpn.get("telegram_username"),
          "status=", vpn.get("status"))

    st, cfg = req("GET", f"{PANEL}/bots/{vpn['id']}/content/settings", headers=A)
    if st < 400:
        keys = {k: cfg.get(k) for k in
                ("folder", "channel", "publish_via", "text_only",
                 "poll_minutes", "min_length", "brand_handle")}
        print("config:", json.dumps(keys, ensure_ascii=False))

    st, led = req("GET",
                  f"{PANEL}/bots/{vpn['id']}/content/posts?page=1&size=10",
                  headers=A)
    if st < 400:
        print("counts:", led.get("counts"), "total:", led.get("total"))
        for p in (led.get("posts") or [])[:8]:
            print("  post:", p.get("status"), "|",
                  (p.get("source_title") or "")[:24], "|",
                  (p.get("cleaned_text") or p.get("raw_text") or "")[:80].replace("\n", " "))

    # --- read the destination channel directly via Telethon ---
    asyncio.run(read_channel())


async def read_channel():
    from telethon import TelegramClient
    api_id = int(os.environ["TG_API_ID"])
    api_hash = os.environ["TG_API_HASH"]
    c = TelegramClient("/tmp/vpnverify", api_id, api_hash)
    await c.connect()
    if not await c.is_user_authorized():
        print("TG_NOT_AUTHORIZED"); return
    msgs = await c.get_messages(int(DEST), limit=6)
    print(f"--- last {len(msgs)} messages of {DEST} (@mr_vpn_here) ---")
    for m in reversed(msgs):
        kind = "photo" if m.photo else ("text" if m.text else "service")
        t = (m.text or "").replace("\n", " ")[:150]
        print(f"  [{m.id}] {m.date} {kind} | {t}")
    await c.disconnect()


if __name__ == "__main__":
    main()