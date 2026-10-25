import asyncio, os
from telethon import TelegramClient, utils, types

async def main():
    api_id = int(os.environ["TG_API_ID"]); api_hash = os.environ["TG_API_HASH"]
    c = TelegramClient("/tmp/vpnsearch", api_id, api_hash)
    await c.connect()
    ref = -1004362422574
    print("resolve_id:", utils.resolve_id(ref))
    try:
        pe = await c.get_input_entity(ref)
        print("get_input_entity OK:", pe)
    except Exception as e:
        print("get_input_entity FAIL:", repr(e))
    try:
        pe = await c.get_entity(ref)
        print("get_entity OK:", type(pe).__name__, getattr(pe, "title", None) or getattr(pe, "first_name", None) if pe else None,
              "@", getattr(pe, "username", None) if pe else None)
    except Exception as e:
        print("get_entity FAIL:", repr(e))
    ds = await c.get_dialogs()
    for d in ds:
        if d.id == ref or getattr(d.entity, "id", None) in (ref, -ref, utils.resolve_id(ref)[1]):
            print("dialog match:", d.id, d.title, "@", getattr(d.entity, "username", None))
            try:
                print("input from dialog entity:", await c.get_input_entity(d.entity))
            except Exception as e:
                print("input via dialog FAIL:", repr(e))
    await c.disconnect()

asyncio.run(main())
