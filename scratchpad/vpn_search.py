"""Global Telegram search for free-v2ray config channels (runs in sidecar)."""
import asyncio, os
from telethon import TelegramClient, functions, types

async def main(query: str):
    api_id = int(os.environ["TG_API_ID"])
    api_hash = os.environ["TG_API_HASH"]
    client = TelegramClient("/tmp/vpnsearch", api_id, api_hash)
    await client.connect()
    if not await client.is_user_authorized():
        print("NOT_AUTHORIZED")
        return
    try:
        res = await client(functions.messages.SearchGlobalRequest(
            q=query,
            filter=types.InputMessagesFilterEmpty(),
            min_date=None, max_date=None,
            offset_peer=types.InputPeerEmpty(),
            offset_rate=0, offset_id=0, limit=40,
        ))
    except Exception as e:
        print("search failed:", e)
        return
    print("raw_messages:", len(res.messages), "chats:", len(res.chats))
    for c in res.chats:
        title = getattr(c, "title", None) or getattr(c, "first_name", "") or ""
        username = getattr(c, "username", None) or ""
        print(f"CHAT {type(c).__name__:8} @{username or '-':28} {title}")
    seen = set()
    for m in res.messages:
        try:
            ent = await m.get_chat()
        except Exception:
            continue
        if ent is None:
            continue
        eid = getattr(ent, "id", None)
        if eid in seen:
            continue
        seen.add(eid)
        title = getattr(ent, "title", None) or getattr(ent, "first_name", "") or ""
        username = getattr(ent, "username", None) or ""
        kind = type(ent).__name__
        if kind in ("Channel", "Chat"):
            print(f"{kind:8} @{username or '-':28} {title}")
    await client.disconnect()

if __name__ == "__main__":
    import sys
    q = sys.argv[1] if len(sys.argv) > 1 else "v2ray رایگان"
    asyncio.run(main(q))
