import asyncio, os
from telethon import TelegramClient

async def main():
    c = TelegramClient("/tmp/vpnverify", int(os.environ["TG_API_ID"]), os.environ["TG_API_HASH"])
    await c.connect()
    m = await c.get_messages(-1004463474801, ids=13)
    t = m.text or ""
    print("LEN:", len(t))
    print("...TAIL 300:")
    print(t[-300:])
    print("HAS_HANDLE:", "@mr_vpn_here" in t)
    # also check a couple earlier ones
    for i in (10, 8):
        m2 = await c.get_messages(-1004463474801, ids=i)
        t2 = m2.text or ""
        print(f"[{i}] len={len(t2)} handle={'@mr_vpn_here' in t2}")
    await c.disconnect()

asyncio.run(main())
