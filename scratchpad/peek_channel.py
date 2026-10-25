"""Peek recent messages of a public channel via @username."""
import asyncio, os, sys
from telethon import TelegramClient

async def main(ref: str, n: int = 8):
    api_id = int(os.environ["TG_API_ID"]); api_hash = os.environ["TG_API_HASH"]
    c = TelegramClient("/tmp/vpnsearch", api_id, api_hash)
    await c.connect()
    if not await c.is_user_authorized():
        print("NOT_AUTHORIZED"); return
    try:
        ent = await c.get_entity(ref)
    except Exception as e:
        print("entity failed:", e); await c.disconnect(); return
    print("TITLE:", getattr(ent, "title", "?"), "| @", getattr(ent, "username", "-"), "| members:", getattr(ent, "participants_count", "?"))
    msgs = await c.get_messages(ent, limit=n)
    texts = 0
    for m in reversed(msgs):
        if not m.text: continue
        texts += 1
        t = m.text.replace("\n", " ")[:200]
        print(f"  [{m.id}] {t}")
    print("text_messages:", texts)
    await c.disconnect()

asyncio.run(main(sys.argv[1], int(sys.argv[2]) if len(sys.argv)>2 else 8))
