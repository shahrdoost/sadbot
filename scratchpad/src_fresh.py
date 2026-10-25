import json, os, urllib.request
key = os.environ["SIDECAR_KEY"]
acc = "9d65f63e35f447ebb1e3d077a6e5184b"

def get(url):
    return json.load(urllib.request.urlopen(urllib.request.Request(url, headers={"X-Sidecar-Key": key}), timeout=30))

d = get(f"http://127.0.0.1:8000/accounts/{acc}/folders")
vpn = [f for f in d["folders"] if f["name"] == "vpn"]
peer = vpn[0]["peers"][0] if vpn else None
print("vpn peer:", peer)
if peer:
    m = get(f"http://127.0.0.1:8000/accounts/{acc}/chats/{peer['chat_id']}/messages?limit=4")
    for x in reversed(m["messages"]):
        print("src", x["id"], x["date"], (x["text"] or "")[:70].replace("\n", " "))
