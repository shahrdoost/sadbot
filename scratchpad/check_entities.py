"""Check dest channel messages for MessageEntityPre (tap-to-copy) entities."""
import asyncio
import os
import sys

from telethon import TelegramClient
from telethon.tl.types import MessageEntityPre, MessageEntityCode

API_ID = int(os.environ["TG_API_ID"])
API_HASH = os.environ["TG_API_HASH"]
DEST = -1004463474801


async def main(session):
    client = TelegramClient(session, API_ID, API_HASH)
    await client.connect()
    if not await client.is_user_authorized():
        print("NOT_AUTHORIZED")
        return
    n = 0
    async for m in client.iter_messages(DEST, limit=6):
        text = m.message or ""
        kinds = []
        for e in (m.entities or []):
            kinds.append(type(e).__name__)
        pre_count = sum(1 for e in (m.entities or []) if isinstance(e, MessageEntityPre))
        code_count = sum(1 for e in (m.entities or []) if isinstance(e, MessageEntityCode))
        has_cfg = any(s in text for s in ("vless://", "vmess://", "trojan://", "ss://"))
        print(f"id={m.id} cfg={has_cfg} pre={pre_count} code={code_count} entities={kinds} len={len(text)}")
        if has_cfg:
            # show which parts are Pre-covered
            for e in (m.entities or []):
                if isinstance(e, MessageEntityPre):
                    chunk = text.encode("utf-16-le")[e.offset * 2:(e.offset + e.length) * 2].decode("utf-16-le")
                    print("   PRE:", chunk[:80])
            print("   TAIL:", text[-70:].replace("\n", " "))
        n += 1
    await client.disconnect()
    print("checked", n)


if __name__ == "__main__":
    asyncio.run(main(sys.argv[1] if len(sys.argv) > 1 else "/tmp/vpnsearch"))
