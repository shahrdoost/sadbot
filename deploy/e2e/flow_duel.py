"""Targeted flow test: duel full game round — piped into sadbot-tgsidecar.

Tests: /start → بازی جدید → (move buttons) → play a move → verify result →
convert/withdraw menus → leaderboard. Text-input paths too.
"""
import asyncio
import os
import shutil
import sys
import time

sys.path.insert(0, "/app")

from telethon import TelegramClient, functions  # noqa: E402

ACCOUNT_ID = "9d65f63e35f447ebb1e3d077a6e5184b"
API_ID = int(os.environ["TG_API_ID"])
API_HASH = os.environ["TG_API_HASH"]
RESULTS = []


def open_client():
    src = f"/data/sessions/{ACCOUNT_ID}.session"
    dst = f"/tmp/e2e_{ACCOUNT_ID}.session"
    shutil.copyfile(src, dst)
    return TelegramClient(dst.replace(".session", ""), API_ID, API_HASH)


class D:
    def __init__(self, client, user):
        self.c = client
        self.u = user.lstrip("@")
        self.e = None
        self.mk = None
        self.mk_id = None

    async def open(self):
        self.e = await self.c.get_entity(self.u)

    async def snap(self):
        s = {}
        async for m in self.c.iter_messages(self.e, limit=25):
            if m.out:
                continue
            s[m.id] = ((m.message or "").strip(), m.edit_date)
        return s

    def _track(self, msgs):
        best_id, best_mk = None, None
        for m in msgs:
            if not m.out and m.reply_markup is not None:
                if best_id is None or m.id > best_id:
                    best_id, best_mk = m.id, m.reply_markup
        if best_id is not None:
            self.mk, self.mk_id = best_mk, best_id

    async def _collect(self, before, wait):
        texts, deadline = [], time.time() + wait
        while time.time() < deadline:
            cur = await self.snap()
            new = [t for mid, (t, _) in cur.items() if mid not in before and t]
            ed = [t for mid, (t, e) in cur.items()
                  if mid in before and e and e != before[mid][1] and t]
            if new or ed:
                texts = new + ed
                deadline = min(deadline, time.time() + 2.0)
            await asyncio.sleep(1.0)
        msgs = [m async for m in self.c.iter_messages(self.e, limit=12)]
        self._track(msgs)
        return {"ok": bool(texts), "texts": texts}

    async def send(self, text):
        before = await self.snap()
        await self.c.send_message(self.e, text)
        return await self._collect(before, 12.0)

    async def click(self, label):
        msgs = [m async for m in self.c.iter_messages(self.e, limit=8)]
        self._track(msgs)
        if self.mk is None:
            return {"ok": False, "texts": [], "error": "no-card"}
        for row in self.mk.rows:
            for b in row.buttons:
                if (getattr(b, "text", "") or "").strip() == label:
                    data = getattr(getattr(b, "type", None), "data", None)
                    if data is None:
                        return {"ok": False, "texts": [], "error": "no-data"}
                    before = await self.snap()
                    try:
                        await self.c(functions.messages.GetBotCallbackAnswerRequest(
                            peer=self.e, msg_id=self.mk_id, data=data))
                    except Exception as e:
                        return {"ok": False, "texts": [], "error": f"cb:{e}"}
                    return await self._collect(before, 9.0)
        return {"ok": False, "texts": [], "error": "stale-button"}

    def labels(self):
        out = []
        if self.mk:
            for row in self.mk.rows:
                for b in row.buttons:
                    out.append((getattr(b, "text", "") or "").strip())
        return out


def rec(name, resp, expect=1):
    err = resp.get("error")
    if err == "stale-button":
        RESULTS.append((name, "STALE", "not on card"))
        print(f"  🔁 {name:40} (stale)", flush=True)
        return False
    ok = (not err) and resp.get("ok") and len(resp.get("texts") or []) >= expect
    RESULTS.append((name, "PASS" if ok else "FAIL",
                    err or " | ".join((resp.get("texts") or ["<silent>"])[:1])[:120]))
    print(f"  {'✅' if ok else '❌'} {name:40} {RESULTS[-1][2][:90]}", flush=True)
    return ok


async def main():
    c = open_client()
    await c.connect()
    print("authorized:", await c.is_user_authorized(), flush=True)
    d = D(c, "@rps_duel_game_bot_bot")
    await d.open()

    print("\n--- DUEL GAME FLOW ---", flush=True)
    rec("/start", await d.send("/start"))
    print("  card:", d.labels(), flush=True)

    r = await d.click("🎮 بازی جدید")
    rec("🎮 بازی جدید", r)
    print("  card after new-game:", d.labels(), flush=True)
    for t in r["texts"]:
        print("   >", t[:150].replace("\n", " / "), flush=True)

    # If move buttons appeared, play one
    labels = d.labels()
    move = None
    for cand in ("✊ سنگ", "✋ کاغذ", "✌️ قیچی"):
        if cand in labels:
            move = cand
            break
    if move:
        r = await d.click(move)
        rec(f"play-move {move}", r)
        for t in r["texts"]:
            print("   >", t[:150].replace("\n", " / "), flush=True)
        print("  card after move:", d.labels(), flush=True)
    else:
        print("  (no move buttons — game may need rival/matchmaking)", flush=True)
        RESULTS.append(("play-move", "SKIP", "no move buttons on card"))

    # navigate menus
    for lbl in ("🏠 منو", "🔄 تبدیل", "🏆 لیدربورد", "🎁 جوایز", "👤 پروفایل"):
        r = await d.click(lbl)
        rec(lbl, r)

    # text command paths
    for cmd in ("/menu", "/help", "سلام"):
        r = await d.send(cmd)
        rec(f"text:{cmd}", r)

    await c.disconnect()
    print("\n--- DUEL RESULT ---", flush=True)
    p = sum(1 for x in RESULTS if x[1] == "PASS")
    s = sum(1 for x in RESULTS if x[1] in ("SKIP", "STALE"))
    print(f"{p}/{len(RESULTS)-s} pass, {s} skip/stale", flush=True)
    for n, st, note in RESULTS:
        if st == "FAIL":
            print(f"  ❌ {n}: {note}", flush=True)


asyncio.run(main())
