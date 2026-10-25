import asyncio
import os
import sys

from telethon import TelegramClient

from send_code import HASH_FILE, client


async def main() -> None:
    phone = sys.argv[1]
    code = sys.argv[2]
    with open(HASH_FILE) as f:
        phone_code_hash = f.read().strip()
    tg = client()
    await tg.connect()
    try:
        await tg.sign_in(phone=phone, code=code, phone_code_hash=phone_code_hash)
    except Exception as e:
        if "SessionPasswordNeeded" in type(e).__name__ or "2FA" in str(e):
            print("NEED_2FA_PASSWORD")
            await tg.disconnect()
            return
        raise
    me = await tg.get_me()
    print(f"LOGGED_IN id={me.id} phone={me.phone} name={me.first_name} username={me.username or '-'}")
    await tg.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
