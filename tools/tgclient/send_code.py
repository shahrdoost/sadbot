import asyncio
import os
import sys

from telethon import TelegramClient

API_ID = 37257760
API_HASH = "766bc4e2116018f13ea5da45986c2df7"
SESSION = os.path.join(os.path.dirname(__file__), "sadbot_user")
HASH_FILE = os.path.join(os.path.dirname(__file__), "phone_code_hash.txt")
PROXY = ("socks5", "127.0.0.1", 10808)


def client() -> TelegramClient:
    return TelegramClient(SESSION, API_ID, API_HASH, proxy=PROXY)


async def main() -> None:
    phone = sys.argv[1]
    tg = client()
    await tg.connect()
    if await tg.is_user_authorized():
        me = await tg.get_me()
        print(f"ALREADY_LOGGED_IN id={me.id} phone={me.phone} name={me.first_name}")
        await tg.disconnect()
        return
    sent = await tg.send_code_request(phone)
    with open(HASH_FILE, "w") as f:
        f.write(sent.phone_code_hash)
    print("CODE_SENT")
    await tg.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
