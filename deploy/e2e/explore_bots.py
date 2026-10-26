"""Generic button-explorer test — piped into sadbot-tgsidecar as stdin.

Fixes vs v1: ignores our own outgoing messages; detects EDITS (bots usually
edit the card instead of sending a new message) by diffing snapshots.
"""
import asyncio
import os
import shutil
import sys
import time
import traceback

sys.path.insert(0, "/app")

from telethon import TelegramClient, functions  # noqa: E402

ACCOUNT_ID = "9d65f63e35f447ebb1e3d077a6e5184b"
API_ID = int(os.environ["TG_API_ID"])
API_HASH = os.environ["TG_API_HASH"]

RESULTS = []
SAFE_SKIP = ("خرید", "پرداخت", "buy", "pay", "درگاه", "برداشت", "withdraw")


def open_client():
    src = f"/data/sessions/{ACCOUNT_ID}.session"
    dst = f"/tmp/e2e_{ACCOUNT_ID}.session"
    shutil.copyfile(src, dst)
    return TelegramClient(dst.replace(".session", ""), API_ID, API_HASH)


class BotDriver:
    def __init__(self, client, bot_username):
        self.client = client
        self.username = bot_username.lstrip("@")
        self.entity = None
        self.last_markup = None
        self.last_markup_msg_id = None

    async def open(self):
        self.entity = await self.client.get_entity(self.username)

    async def snapshot(self):
        """Return {msg_id: (text, edit_date)} for recent BOT messages only."""
        snap = {}
        async for m in self.client.iter_messages(self.entity, limit=25):
            if m.out:
                continue
            snap[m.id] = ((m.message or m.text or "").strip(), m.edit_date)
        return snap

    def _update_markup(self, msgs):
        """Track the newest message that carries a reply markup."""
        best_id, best_mk = None, None
        for m in msgs:
            if m.out or m.reply_markup is None:
                continue
            if best_id is None or m.id > best_id:
                best_id, best_mk = m.id, m.reply_markup
        if best_id is not None:
            self.last_markup = best_mk
            self.last_markup_msg_id = best_id

    async def _collect(self, before, wait):
        """Diff snapshots: new messages + edited messages count as responses."""
        texts, deadline = [], time.time() + wait
        while time.time() < deadline:
            cur = await self.snapshot()
            new_texts = [t for mid, (t, _) in cur.items()
                         if mid not in before and t]
            edited = [t for mid, (t, ed) in cur.items()
                      if mid in before and ed and ed != before[mid][1] and t]
            if new_texts or edited:
                texts = new_texts + edited
                # settle a bit more to catch follow-ups
                deadline = min(deadline, time.time() + 2.5)
            await asyncio.sleep(1.0)
        # refresh markup from the latest messages
        msgs = [m async for m in self.client.iter_messages(self.entity, limit=15)]
        self._update_markup(msgs)
        return {"ok": bool(texts), "texts": texts}

    async def send(self, text):
        before = await self.snapshot()
        await self.client.send_message(self.entity, text)
        return await self._collect(before, 12.0)

    async def click_inline(self, btn):
        data = getattr(getattr(btn, "type", None), "data", None) or getattr(btn, "data", None)
        # refresh msg id from the NEWEST markup card (old cards linger in history)
        msgs = [m async for m in self.client.iter_messages(self.entity, limit=8)]
        self._update_markup(msgs)
        msg_id = self.last_markup_msg_id
        if data is None or msg_id is None:
            return {"ok": False, "texts": [], "error": "no-data"}
        # only click buttons that exist on the LIVE card; else it's a stale artifact
        fresh_label = self.label(btn)
        live = {self.label(b) for b in self.buttons()}
        if fresh_label not in live:
            return {"ok": False, "texts": [], "error": "stale-button"}
        for b in self.buttons():
            if self.label(b) == fresh_label:
                nd = getattr(getattr(b, "type", None), "data", None) or getattr(b, "data", None)
                if nd:
                    data = nd
                    break
        before = await self.snapshot()
        try:
            await self.client(functions.messages.GetBotCallbackAnswerRequest(
                peer=self.entity, msg_id=msg_id, data=data))
        except Exception as e:
            return {"ok": False, "texts": [], "error": f"cb:{e}"}
        return await self._collect(before, 9.0)

    def buttons(self):
        mk = self.last_markup
        if mk is None:
            return []
        out = []
        for row in (getattr(mk, "rows", None) or []):
            for b in (row.buttons or []):
                out.append(b)
        return out

    @staticmethod
    def label(btn):
        return (getattr(btn, "text", None) or "?").strip()


def record(bot, button, resp, expect=1):
    err = resp.get("error")
    if err == "stale-button":
        RESULTS.append({"bot": bot, "button": button, "status": "STALE",
                        "note": "not on live card anymore (nav artifact)"})
        print(f"  🔁 {button[:42]:42} (stale — button gone from card)", flush=True)
        return False
    ok = (not err) and resp.get("ok") and len(resp.get("texts") or []) >= expect
    RESULTS.append({"bot": bot, "button": button, "status": "PASS" if ok else "FAIL",
                    "note": err or " | ".join((resp.get("texts") or ["<silent>"])[:1])[:130]})
    flag = "✅" if ok else "❌"
    print(f"  {flag} {button[:42]:42} {RESULTS[-1]['note'][:95]}", flush=True)


def is_homeish(label):
    return any(k in label for k in ("منوی اصلی", "خانه", "🏠", "بازگشت", "منو"))


async def explore(drv, botname, max_depth=2, max_buttons=20):
    visited = set()
    budget = [max_buttons]

    async def dfs(depth, path):
        while budget[0] > 0 and depth <= max_depth:
            nxt = None
            for b in drv.buttons():
                lbl = drv.label(b)
                if lbl in visited:
                    continue
                if any(s in lbl.lower() for s in SAFE_SKIP):
                    visited.add(lbl)
                    RESULTS.append({"bot": botname, "button": f"{path}>{lbl}", "status": "SKIP",
                                    "note": "payment/withdraw — not clicked"})
                    print(f"  ⏭ {path}>{lbl[:36]:36} (skipped)", flush=True)
                    continue
                nxt = (lbl, b)
                break
            if nxt is None:
                return
            lbl, b = nxt
            visited.add(lbl)
            budget[0] -= 1
            resp = await drv.click_inline(b)
            record(botname, f"{path}>{lbl}", resp)
            if not resp.get("error"):
                await dfs(depth + 1, f"{path}>{lbl}")
                # best-effort return to this menu after a child subtree
                for hb in drv.buttons():
                    hl = drv.label(hb)
                    if is_homeish(hl) and hl not in visited:
                        await drv.click_inline(hb)
                        break

    resp = await drv.send("/start")
    record(botname, "/start", resp)
    await dfs(1, "")


async def run_one(client, username):
    print(f"\n{'='*60}\nBOT {username}\n{'='*60}", flush=True)
    drv = BotDriver(client, username)
    try:
        await drv.open()
    except Exception as e:
        RESULTS.append({"bot": username, "button": "<open>", "status": "FAIL", "note": str(e)[:130]})
        print(f"  ❌ open failed: {e}", flush=True)
        return
    try:
        await explore(drv, username)
    except Exception:
        RESULTS.append({"bot": username, "button": "<explore>", "status": "FAIL",
                        "note": traceback.format_exc()[-150:]})
        print("  ❌ crashed:\n" + traceback.format_exc(), flush=True)


async def main():
    bots = [b.strip() for b in os.environ.get("BOTS", "").split(",") if b.strip()] or [
        "@tapearnc_bot", "@faal_bakht_bot",
    ]
    client = open_client()
    await client.connect()
    print("authorized:", await client.is_user_authorized(), flush=True)
    for u in bots:
        await run_one(client, u)
    await client.disconnect()

    print("\n" + "=" * 60 + "\nSUMMARY\n" + "=" * 60, flush=True)
    by_bot = {}
    for r in RESULTS:
        by_bot.setdefault(r["bot"], []).append(r)
    tp = tt = 0
    for bot, rows in by_bot.items():
        p = sum(1 for x in rows if x["status"] == "PASS")
        s = sum(1 for x in rows if x["status"] == "SKIP")
        st = sum(1 for x in rows if x["status"] == "STALE")
        tp += p
        tt += len(rows) - s - st
        print(f"{bot:34} {p}/{len(rows)-s-st} pass  ({s} skipped, {st} stale)", flush=True)
        for x in rows:
            if x["status"] == "FAIL":
                print(f"   ❌ {x['button'][:42]}: {x['note'][:100]}", flush=True)
    print(f"\nTOTAL: {tp}/{tt} passed", flush=True)


asyncio.run(main())
