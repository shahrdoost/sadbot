"""E2E bot tester — runs inside sadbot-tgsidecar container via `python3 -`.

Usage: pipe this file into `docker exec -i -e PYTHONPATH=/app sadbot-tgsidecar python3 -`
with BOT_FILTER env (comma-separated usernames) to limit scope.
"""
import asyncio
import os
import sys
import time
import traceback

sys.path.insert(0, "/app")

from telethon import TelegramClient, functions, types  # noqa: E402
from app import store  # noqa: E402

ACCOUNT_ID = "9d65f63e35f447ebb1e3d077a6e5184b"
TIMEOUT = 12.0          # seconds to wait for bot reply
CLICK_TIMEOUT = 8.0     # seconds to wait after inline click
RESULTS = []            # list of dicts: bot, button, status, note


class BotDriver:
    def __init__(self, client, bot_username):
        self.client = client
        self.username = bot_username.lstrip("@")
        self.entity = None
        self.last_markup = None
        self.last_markup_msg_id = None

    async def open(self):
        self.entity = await self.client.get_entity(self.username)

    async def send(self, text):
        """Send a text message and collect bot replies for TIMEOUT seconds."""
        before = await self._latest_msg_id()
        await self.client.send_message(self.entity, text)
        return await self._collect_replies(before, TIMEOUT)

    async def click_inline(self, btn, label=""):
        """Press an inline button (btn is a KeyboardButtonRow item)."""
        data = getattr(btn, "data", None) or getattr(getattr(btn, "type", None), "data", None)
        msg_id = self.last_markup_msg_id
        if data is None or msg_id is None:
            return {"ok": False, "texts": [], "error": f"no data/msg_id for {label}"}
        before = await self._latest_msg_id()
        try:
            await self.client(functions.messages.GetBotCallbackAnswerRequest(
                peer=self.entity, msg_id=msg_id, data=data))
        except Exception as e:
            return {"ok": False, "texts": [], "error": f"callback err: {e}"}
        return await self._collect_replies(before, CLICK_TIMEOUT)

    async def _latest_msg_id(self):
        async for m in self.client.iter_messages(self.entity, limit=1):
            return m.id
        return 0

    async def _collect_replies(self, before_id, wait):
        """Collect new bot messages; track latest reply-markup for inline clicks."""
        texts, deadline = [], time.time() + wait
        seen = set()
        while time.time() < deadline:
            new_found = False
            async for m in self.client.iter_messages(self.entity, limit=15):
                if m.id <= before_id or m.id in seen:
                    continue
                seen.add(m.id)
                new_found = True
                body = (m.message or m.text or "").strip()
                if body:
                    texts.append(body)
                if m.reply_markup is not None:
                    self.last_markup = m.reply_markup
                    self.last_markup_msg_id = m.id
            if new_found:
                deadline = min(deadline, time.time() + 2.5)  # settle window
            await asyncio.sleep(1.2)
        return {"ok": bool(texts) or self.last_markup is not None,
                "texts": texts,
                "markup": self.last_markup}

    def buttons(self):
        """Flatten current reply-markup into [(row_idx, btn), ...]."""
        mk = self.last_markup
        if mk is None:
            return []
        out = []
        rows = getattr(mk, "rows", None) or []
        for ri, row in enumerate(rows):
            for b in (row.buttons or []):
                out.append((ri, b))
        return out

    @staticmethod
    def btn_label(btn):
        return (getattr(btn, "text", None) or getattr(btn, "text", None) or "?").strip()


def record(bot, button, resp, expect_min=1):
    ok = resp.get("ok") and len(resp.get("texts", [])) >= expect_min
    if resp.get("error"):
        ok = False
    RESULTS.append({
        "bot": bot, "button": button,
        "status": "PASS" if ok else "FAIL",
        "note": resp.get("error") or " | ".join((resp.get("texts") or ["<no text>"])[:1])[:120],
    })
    flag = "✅" if ok else "❌"
    print(f"  {flag} {button!r:45} {RESULTS[-1]['note'][:100]}", flush=True)
    return ok


async def drain_home(drv, home_labels, max_rounds=6):
    """Press any visible home-ish button to get back to a known state."""
    for _ in range(max_rounds):
        btns = drv.buttons()
        if not btns:
            return
        for _, b in btns:
            if BotDriver.btn_label(b) in home_labels:
                r = await drv.click_inline(b, "→home")
                if r.get("texts"):
                    return
        # fallback: send /start
        await drv.send("/start")
        return


# ---------------------------------------------------------------------------
# Per-bot test plans. Each receives (drv, record) and exercises buttons.
# ---------------------------------------------------------------------------

HOME_LABELS_DATING = {"🏠 منوی اصلی", "منوی اصلی", "🏠 خانه"}


async def test_smoke(drv, botname):
    """Send /start and record the first response."""
    r = await drv.send("/start")
    record(botname, "/start", r)


async def run_bot(client, username, plan, label):
    print(f"\n=== {label} ({username}) ===", flush=True)
    drv = BotDriver(client, username)
    try:
        await drv.open()
    except Exception as e:
        RESULTS.append({"bot": label, "button": "<open>", "status": "FAIL", "note": str(e)[:120]})
        print(f"  ❌ cannot open: {e}", flush=True)
        return
    try:
        await plan(drv, label)
    except Exception:
        RESULTS.append({"bot": label, "button": "<plan>", "status": "FAIL",
                        "note": traceback.format_exc()[-160:]})
        print("  ❌ plan crashed:\n" + traceback.format_exc(), flush=True)


def summarize():
    print("\n" + "=" * 70, flush=True)
    print("SUMMARY", flush=True)
    print("=" * 70, flush=True)
    by_bot = {}
    for r in RESULTS:
        by_bot.setdefault(r["bot"], []).append(r)
    total_pass = total = 0
    for bot, rows in by_bot.items():
        p = sum(1 for x in rows if x["status"] == "PASS")
        total_pass += p
        total += len(rows)
        print(f"{bot:22} {p}/{len(rows)} passed", flush=True)
        for x in rows:
            if x["status"] == "FAIL":
                print(f"   ❌ {x['button']}: {x['note'][:110]}", flush=True)
    print(f"\nTOTAL: {total_pass}/{total} passed", flush=True)
    return total_pass, total
