"""Dating flow test — SEND-MINIMAL (callbacks do the heavy lifting).

Dating home = reply keyboard (press = send text). Response cards are inline
(callback-clickable). One send per section, then click every inline button.
Paced sends to avoid Telegram flood; callbacks are not send-limited.
"""
import asyncio
import os
import shutil
import sys
import time

sys.path.insert(0, "/app")

from telethon import TelegramClient, functions, errors  # noqa: E402

ACCOUNT_ID = "9d65f63e35f447ebb1e3d077a6e5184b"
API_ID = int(os.environ["TG_API_ID"])
API_HASH = os.environ["TG_API_HASH"]
RESULTS = []


def open_client():
    src = f"/data/sessions/{ACCOUNT_ID}.session"
    dst = f"/tmp/e2e_{ACCOUNT_ID}.session"
    shutil.copyfile(src, dst)
    return TelegramClient(dst.replace(".session", ""), API_ID, API_HASH,
                          flood_sleep_threshold=90)


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
        async for m in self.c.iter_messages(self.e, limit=12):
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
                deadline = min(deadline, time.time() + 1.5)
            await asyncio.sleep(1.0)
        msgs = [m async for m in self.c.iter_messages(self.e, limit=8)]
        self._track(msgs)
        return {"ok": bool(texts), "texts": texts}

    async def _safe(self, coro_factory):
        for _ in range(4):
            try:
                return await coro_factory()
            except errors.FloodWaitError as e:
                wait = min(e.seconds, 1600)
                print(f"  ⏳ FLOOD {e.seconds}s (sleep {wait}s)", flush=True)
                await asyncio.sleep(wait)
        return {"ok": False, "texts": [], "error": "flood"}

    async def send(self, text):
        before = await self.snap()

        async def _do():
            await self.c.send_message(self.e, text)
            return await self._collect(before, 8.0)
        r = await self._safe(_do)
        await asyncio.sleep(3.0)  # pace sends
        return r

    async def click(self, label):
        """Callback click on an inline button of the live card."""
        msgs = [m async for m in self.c.iter_messages(self.e, limit=8)]
        self._track(msgs)
        if self.mk is None:
            return {"ok": False, "texts": [], "error": "no-card"}
        for row in self.mk.rows:
            for b in row.buttons:
                if (getattr(b, "text", "") or "").strip() != label:
                    continue
                data = getattr(getattr(b, "type", None), "data", None)
                if data is None:
                    return {"ok": False, "texts": [], "error": "reply-kbd"}
                before = await self.snap()

                async def _do():
                    await self.c(functions.messages.GetBotCallbackAnswerRequest(
                        peer=self.e, msg_id=self.mk_id, data=data))
                    return await self._collect(before, 6.0)
                return await self._safe(_do)
        return {"ok": False, "texts": [], "error": "stale-button"}

    def labels(self):
        out = []
        if self.mk:
            for row in self.mk.rows:
                for b in row.buttons:
                    out.append((getattr(b, "text", "") or "").strip())
        return out

    def inline_labels(self):
        out = []
        if self.mk:
            for row in self.mk.rows:
                for b in row.buttons:
                    if getattr(getattr(b, "type", None), "data", None) is not None:
                        out.append((getattr(b, "text", "") or "").strip())
        return out


def rec(name, resp, expect=1):
    err = resp.get("error")
    if err == "stale-button":
        RESULTS.append((name, "STALE", "gone"))
        print(f"  🔁 {name:44} (stale)", flush=True)
        return False
    if err == "reply-kbd":
        RESULTS.append((name, "SKIP", "reply-keyboard (sends text)"))
        print(f"  ⏭ {name:44} (reply-kbd → text path)", flush=True)
        return True
    ok = (not err) and resp.get("ok") and len(resp.get("texts") or []) >= expect
    RESULTS.append((name, "PASS" if ok else "FAIL",
                    err or " | ".join((resp.get("texts") or ["<silent>"])[:1])[:110]))
    print(f"  {'✅' if ok else '❌'} {name:44} {RESULTS[-1][2][:85]}", flush=True)
    return ok


def show(r, n=2):
    for t in (r.get("texts") or [])[:n]:
        print("     >", t[:125].replace("\n", " / "), flush=True)


async def click_all_inline(d, prefix, budget=8):
    """Click every inline button on the live card; returns to home after."""
    labels = d.inline_labels()
    if labels:
        print(f"     inline: {labels}", flush=True)
    for lbl in labels[:budget]:
        if any(k in lbl for k in ("خرید", "پرداخت", "درگاه", "اشتراک", "buy", "پکیج")):
            RESULTS.append((f"{prefix}>{lbl}", "SKIP", "payment"))
            print(f"     ⏭ {lbl} (payment)", flush=True)
            continue
        r = await d.click(lbl)
        rec(f"{prefix}>{lbl}", r)
        show(r, 1)


async def main():
    c = open_client()
    await c.connect()
    print("authorized:", await c.is_user_authorized(), flush=True)
    d = D(c, "@ashnadbot")
    await d.open()

    print("\n=== /start ===", flush=True)
    r = await d.send("/start")
    rec("/start", r)
    show(r, 2)
    print("  home labels:", d.labels(), flush=True)

    # Home sections: one send each, then click inline buttons on the card.
    sections = [
        "🔗 به یک ناشناس وصلم کن",
        "💘 مرور پروفایل‌ها",
        "🔍 جستجوی ویژه",
        "👥 جستجوی کاربران",
        "📖 راهنما",
        "💰 سکه",
        "👤 پروفایل من",
        "🎁 ماموریت‌ها",
        "💌 اعتراف ناشناس",
        "🤖 چت با دستیار",
        "📩 پشتیبانی",
    ]
    for s in sections:
        print(f"\n=== HOME > {s} ===", flush=True)
        r = await d.send(s)
        rec(f"home:{s}", r)
        show(r, 2)
        await click_all_inline(d, s)

    # Connect submenu texts (reply-keyboard path)
    print("\n=== connect submenu ===", flush=True)
    await d.send("🏠 منوی اصلی")
    r = await d.send("🔗 به یک ناشناس وصلم کن")
    rec("connect-open", r)
    print("     labels:", d.labels(), flush=True)
    for lbl in ("🔗 غریبه", "🚺 جستجوی دختر", "🚹 جستجوی پسر", "🔍 جستجوی پیشرفته"):
        rr = await d.send(lbl)
        rec(f"connect:{lbl}", rr)
        show(rr, 1)
        await click_all_inline(d, f"connect:{lbl}", budget=6)
        await d.send("🏠 منوی اصلی")
        await d.send("🔗 به یک ناشناس وصلم کن")

    # text edge cases
    print("\n=== text edge cases ===", flush=True)
    for txt in ("/help", "سلام"):
        r = await d.send(txt)
        rec(f"text:{txt}", r)

    await c.disconnect()
    print("\n" + "=" * 55 + "\nDATING RESULT\n" + "=" * 55, flush=True)
    p = sum(1 for x in RESULTS if x[1] == "PASS")
    s = sum(1 for x in RESULTS if x[1] in ("SKIP", "STALE"))
    print(f"{p}/{len(RESULTS)-s} pass, {s} skip/stale", flush=True)
    for n, st, note in RESULTS:
        if st == "FAIL":
            print(f"  ❌ {n}: {note}", flush=True)


asyncio.run(main())
