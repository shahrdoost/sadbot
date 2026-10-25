"""Peek at recent posts in the destination channel to verify code-block format."""
import json
import os
import sys
import urllib.request

SIDECAR = os.environ.get("SIDECAR", "http://tgsidecar:8000")
KEY = os.environ["SIDECAR_KEY"]
DEST = "-1004463474801"


def get(url, headers):
    r = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(r, timeout=60) as resp:
        return json.loads(resp.read() or b"{}")


def main():
    h = {"X-Sidecar-Key": KEY}
    st = get(f"{SIDECAR}/accounts/9d65f63e35f447ebb1e3d077a6e5184b/chats/{DEST}/messages?limit=8", h)
    for m in st.get("messages") or []:
        t = m.get("text") or ""
        print("=" * 60)
        print("id", m.get("id"), "| has_triple:", "```" in t, "| len:", len(t))
        print(t)
        print()


if __name__ == "__main__":
    main()
