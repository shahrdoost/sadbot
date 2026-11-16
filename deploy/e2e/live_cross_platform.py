"""Live cross-platform test: a Telegram user (MTProto account) and a Kat user
must match and exchange text + photo.

Runs inside the sadbot-tgsidecar container:
  docker exec -i -e PYTHONPATH=/app sadbot-tgsidecar python3 - < live_cross_platform.py

It drives only the Telegram side (the MTProto account). The Kat side is a real
user already in the bot; we wait for them to be matched with us. Prints a
clear PASS/FAIL summary and every text/photo that came back.
"""
import asyncio
import os
import sys
import time

sys.path.insert(0, "/app")

from telethon import TelegramClient  # noqa: E402
from app import store  # noqa: E402

BOT = "@ashnadbot"
ACCOUNT_ID = "9d65f63e35f447ebb1e3d077a6e5184b"
WAIT = 20.0


async def latest_id(client, entity):
    async for m in client.iter_messages(entity, limit=1):
        return m.id
    return 0


async def collect(client, entity, before_id, wait):
    """Collect messages newer than before_id for `wait` seconds."""
    out = []
    deadline = time.time() + wait
    while time.time() < deadline:
        async for m in client.iter_messages(entity, min_id=before_id, limit=20):
            if m.id > before_id and m not in out:
                out.append(m)
        if out:
            # give the bot a moment to send follow-ups
            await asyncio.sleep(2.5)
            async for m in client.iter_messages(entity, min_id=before_id, limit=20):
                if m.id > before_id and m not in out:
                    out.append(m)
            break
        await asyncio.sleep(0.75)
    out.sort(key=lambda m: m.id)
    return out


def text_of(m):
    return (m.message or "").strip()


async def main():
    client = await store.get_client(ACCOUNT_ID)
    try:
        await client.connect()
    except Exception:
        pass
    entity = await client.get_entity(BOT)
    print(f"connected as test account; bot entity = {entity.id}")

    # 1) make sure we are in the dating chat scene: /start then the stranger search
    before = await latest_id(client, entity)
    await client.send_message(entity, "/start")
    msgs = await collect(client, entity, before, WAIT)
    print("\n--- /start ---")
    for m in msgs:
        print("  bot:", text_of(m)[:110].replace("\n", " | "))

    before = await latest_id(client, entity)
    await client.send_message(entity, "🔗 به یک ناشناس وصلم کن")
    msgs = await collect(client, entity, before, WAIT)
    print("\n--- «وصلم کن» menu ---")
    for m in msgs:
        print("  bot:", text_of(m)[:110].replace("\n", " | "))

    before = await latest_id(client, entity)
    await client.send_message(entity, "🔗 غریبه")
    msgs = await collect(client, entity, before, WAIT)
    print("\n--- stranger search ---")
    for m in msgs:
        print("  bot:", text_of(m)[:160].replace("\n", " | "))

    matched = any(("وصلت انجام شد" in text_of(m)) for m in msgs)
    if not matched:
        # wait longer for the matcher (5s loop + the kat user may be idle)
        msgs = await collect(client, entity, before, 60)
        print("\n--- waiting for a match (60s) ---")
        for m in msgs:
            print("  bot:", text_of(m)[:160].replace("\n", " | "))
        matched = any(("وصلت انجام شد" in text_of(m)) for m in msgs)

    if not matched:
        print("\nRESULT: NO_MATCH — the kat user is not in the search queue.")
        return

    print("\nRESULT: MATCHED")

    # 2) text relay telegram -> kat
    before = await latest_id(client, entity)
    await client.send_message(entity, "سلام از تلگرام — تست بین‌پلتفرمی")
    await asyncio.sleep(4)
    echo = await collect(client, entity, before, 8)
    print("\n--- after sending text ---")
    for m in echo:
        print("  bot:", text_of(m)[:140].replace("\n", " | "))

    # 3) photo relay telegram -> kat
    before = await latest_id(client, entity)
    png_path = os.environ.get("PHOTO", "/tmp/cp-test.png")
    sent = await client.send_file(entity, png_path, force_document=False)
    print(f"\n  sent as photo={bool(getattr(sent, 'photo', None))} "
          f"document={bool(getattr(sent, 'document', None))}")
    await asyncio.sleep(6)
    echo = await collect(client, entity, before, 12)
    print("\n--- after sending photo ---")
    for m in echo:
        print("  bot:", text_of(m)[:140].replace("\n", " | "))

    print("\nDONE — check the kat side + panel transcript")


asyncio.run(main())
