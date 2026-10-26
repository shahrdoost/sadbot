"""Targeted nav-escape E2E on @ashnadbot — piped into sadbot-tgsidecar.

Verifies the 2026-10-26 fix live: home-menu labels must escape profile-edit
prompts (age/name) instead of being swallowed as field input.
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


def rec(name, ok, note):
    RESULTS.append((name, ok, note))
    print(f"  {'PASS' if ok else 'FAIL'}  {name}: {note[:110]}", flush=True)


async def newest_bot_text(client, bot):
    async for m in client.iter_messages(bot, limit=5):
        if not m.out:
            return (m.message or "").strip()
    return ""


async def click_latest(client, bot, label_part):
    """Click an inline button containing label_part on the newest markup card."""
    async for m in client.iter_messages(bot, limit=15):
        if m.out or m.reply_markup is None:
            continue
        for row in m.reply_markup.rows:
            for b in row.buttons:
                if label_part in (b.text or ""):
                    data = getattr(getattr(b, "type", None), "data", None) or b.data
                    await client(functions.messages.GetBotCallbackAnswerRequest(
                        peer=bot, msg_id=m.id, data=data))
                    await asyncio.sleep(3.5)
                    return True
        break  # only the newest card
    return False


async def send(client, bot, text):
    before = 0
    async for m in client.iter_messages(bot, limit=1):
        before = m.id
    await client.send_message(bot, text)
    deadline = time.time() + 7
    while time.time() < deadline:
        await asyncio.sleep(1)
        got = ""
        async for m in client.iter_messages(bot, limit=3):
            if not m.out and m.id > before:
                got = (m.message or "").strip()
                break
        if got:
            await asyncio.sleep(1.5)
            return got
    return ""


async def main():
    src = f"/data/sessions/{ACCOUNT_ID}.session"
    dst = f"/tmp/nav_{ACCOUNT_ID}.session"
    shutil.copyfile(src, dst)
    client = TelegramClient(dst.replace(".session", ""), API_ID, API_HASH)
    await client.connect()
    bot = await client.get_entity("ashnadbot")
    print("authorized:", await client.is_user_authorized(), flush=True)

    # warm-up: clear any leftover prompt state from earlier runs
    await send(client, bot, "🏠 منوی اصلی")
    await asyncio.sleep(1)

    r = await send(client, bot, "👤 پروفایل من")
    rec("profile-page", "حساب" in r or "پروفایل" in r, r.replace("\n", " ")[:100])

    # --- age edit prompt ---
    if not await click_latest(client, bot, "تغییر سن"):
        rec("edit-age-prompt", False, "no edit-age button on newest card")
    else:
        r = await newest_bot_text(client, bot)
        if "مجازه" in r:
            rec("edit-age-prompt", True, f"cooldown gate active, skip: {r[:80]}")
        elif "سن جدید" in r:
            rec("edit-age-prompt", True, "age prompt shown")
            # the prompt keyboard itself must offer a home escape (withHomeRow)
            async for m in client.iter_messages(bot, limit=5):
                if not m.out and m.reply_markup is not None:
                    labels = [b.text for row in m.reply_markup.rows for b in row.buttons]
                    rec("age-kbd-home-row", any("منوی اصلی" in (l or "") for l in labels),
                        f"kbd: {labels[:7]}")
                    break
            # a home label must escape, not be re-asked for an age
            r2 = await send(client, bot, "📖 راهنما")
            bad = "۱۸ تا ۷۰" in r2 or ("انتخاب" in r2 and "راهنما" not in r2)
            rec("age-home-escape", (not bad) and bool(r2),
                f"after home label: {r2.replace(chr(10), ' ')[:90]}")
        else:
            rec("edit-age-prompt", False, f"unexpected reply: {r[:90]}")

    # --- name edit prompt: home label must NOT become the new name ---
    await send(client, bot, "👤 پروفایل من")
    if not await click_latest(client, bot, "تغییر نام"):
        rec("edit-name-prompt", False, "no edit-name button on newest card")
    else:
        r = await newest_bot_text(client, bot)
        if "مجازه" in r:
            rec("edit-name-prompt", True, f"cooldown gate active, skip: {r[:80]}")
        elif "نام جدید" in r or "اسمت" in r or "اسم" in r:
            rec("edit-name-prompt", True, "name prompt shown")
            r2 = await send(client, bot, "💰 سکه")
            rec("name-home-escape", "کیف پول" in r2 or "موجودی" in r2,
                f"after home label: {r2.replace(chr(10), ' ')[:90]}")
        else:
            rec("edit-name-prompt", False, f"unexpected reply: {r[:90]}")

    await client.disconnect()
    ok = sum(1 for _, o, _ in RESULTS if o)
    print(f"\nNAV E2E: {ok}/{len(RESULTS)} passed", flush=True)


asyncio.run(main())
