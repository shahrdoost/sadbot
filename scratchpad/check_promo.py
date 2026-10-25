import json
import os
import urllib.request

PASS = os.environ["PANEL_PASS"]
r = urllib.request.Request(
    "http://sadbot-backend:8080/api/v1/auth/login",
    data=json.dumps({"email": "admin@sadbot.local", "password": PASS}).encode(),
    headers={"Content-Type": "application/json"},
    method="POST",
)
tok = json.load(urllib.request.urlopen(r))
h = {"Authorization": "Bearer " + tok["access_token"]}
r = urllib.request.Request(
    "http://sadbot-backend:8080/api/v1/bots/2d86d7bb-c9e2-4559-89e3-cb78aac3bfb8/content/posts?page=1&size=25",
    headers=h,
)
d = json.load(urllib.request.urlopen(r))
print("total", d.get("total"), d.get("counts"))
for p in d.get("posts") or []:
    t = p.get("cleaned_text") or p.get("raw_text") or ""
    if "Barcelona" in t or "VIP" in t or "پیش‌بینی" in t or "پیش‌بینی" in t:
        print("id", p.get("id"), "status", p.get("status"), "msg", p.get("message_id"))
        idx = t.find("پیش")
        if idx >= 0:
            word = t[idx : idx + 12]
            print("word:", [hex(ord(c)) for c in word])
        print("ZWNJ form match:", "پیش‌بینی" in t)
        print("stripped-ZWNJ match:", "پیش‌بینی" in t.replace("‌", ""))
        print("text[:120]:", t[:120].replace("\n", " "))
