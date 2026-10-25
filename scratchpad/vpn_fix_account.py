"""Store the SIDECAR account id in content_account_id (the collector passes it
straight to the sidecar API; the panel UUID got 404 account-not-found)."""
import json, os, urllib.request, urllib.error, sys

PANEL = os.environ.get("PANEL", "http://sadbot-backend:8080/api/v1")
KEY = os.environ["SIDECAR_KEY"]; PASS = os.environ["PANEL_PASS"]
BOT = "2d86d7bb-c9e2-4559-89e3-cb78aac3bfb8"
SIDECAR_ID = "9d65f63e35f447ebb1e3d077a6e5184b"

def req(method, url, body=None, headers=None):
    h = dict(headers or {}); data = None
    if body is not None:
        data = json.dumps(body).encode(); h.setdefault("Content-Type", "application/json")
    r = urllib.request.Request(url, data=data, headers=h, method=method)
    try:
        with urllib.request.urlopen(r, timeout=60) as resp:
            return resp.status, json.loads(resp.read() or b"{}")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"{}")

st, tok = req("POST", f"{PANEL}/auth/login", {"email": "admin@sadbot.local", "password": PASS})
A = {"Authorization": f"Bearer {tok['access_token']}", "X-Sidecar-Key": KEY}
st, cur = req("GET", f"{PANEL}/bots/{BOT}/content/settings", headers=A)
if st >= 400:
    print("GET fail", st, cur); sys.exit(1)
body = {k: cur[k] for k in (
    "version", "folder", "channel", "publish_via", "exclude", "keywords",
    "poll_minutes", "backfill_limit", "min_length", "skip_forwarded",
    "skip_replies", "render_card_summary", "text_only", "brand_key",
    "brand_name_en", "brand_name_fa", "brand_handle", "brand_glow_a",
    "brand_glow_b", "brand_glow_c", "brand_accent", "clean_prompt", "card_prompt")}
body["account_id"] = SIDECAR_ID
st, r = req("PUT", f"{PANEL}/bots/{BOT}/content/settings", body, headers=A)
print("PUT", st, "ok" if st < 400 else r)
st, after = req("GET", f"{PANEL}/bots/{BOT}/content/settings", headers=A)
print("account_id now:", after.get("account_id"))
