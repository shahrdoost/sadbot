import json
import os
import urllib.request

KEY = os.environ["SIDECAR_KEY"]
ACC = "9d65f63e35f447ebb1e3d077a6e5184b"
DEST = "-1004463474801"


def get(url):
    r = urllib.request.Request(url, headers={"X-Sidecar-Key": KEY})
    with urllib.request.urlopen(r, timeout=30) as resp:
        return json.loads(resp.read() or b"{}")


fl = get(f"http://tgsidecar:8000/accounts/{ACC}/folders")
for f in fl.get("folders", []):
    if f.get("name") == "vpn":
        peers = f.get("peers", [])
        print("vpn peers:", len(peers))
        for p in peers:
            print(" ", p.get("chat_id") or p.get("id"), "|", (p.get("title") or "")[:32], "|", p.get("username"))

print("\n--- dest channel last messages ---")
msgs = get(f"http://tgsidecar:8000/accounts/{ACC}/chats/{DEST}/messages?limit=6")
for m in reversed(msgs.get("messages") or []):
    t = m.get("text") or ""
    has_cfg = "vless://" in t or "vmess://" in t or "trojan://" in t
    print("id", m.get("id"), "| cfg:", has_cfg, "| len:", len(t))
    print(t[:600])
    print("-" * 50)
