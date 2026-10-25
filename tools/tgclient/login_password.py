import asyncio
import sys

from telethon import TelegramClient

from send_code import client


async def main() -> None:
    password = sys.argv[1]
    tg = client()
    await tg.connect()
    await tg.sign_in(password=password)
    me = await tg.get_me()
    print(f"LOGGED_IN id={me.id} phone={me.phone} name={me.first_name} username={me.username or '-'}")
    await tg.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
