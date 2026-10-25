import asyncio

from telethon import TelegramClient
from telethon.tl.types import (
    Channel,
    Chat,
    User,
)

from send_code import client

KIND = {User: "user", Chat: "group", Channel: "channel"}


async def main() -> None:
    tg = client()
    await tg.connect()
    if not await tg.is_user_authorized():
        print("NOT_LOGGED_IN")
        return
    me = await tg.get_me()
    print(f"account: {me.first_name} (@{me.username or '-'}) id={me.id}\n")

    dialogs = await tg.get_dialogs()
    print(f"total dialogs: {len(dialogs)}\n")
    print(f"{'#':>3}  {'kind':<7}  {'unread':>6}  title")
    for i, d in enumerate(dialogs, 1):
        ent = d.entity
        kind = KIND.get(type(ent), "other")
        title = d.name or ""
        unread = d.unread_count or 0
        print(f"{i:>3}  {kind:<7}  {unread:>6}  {title}")
    await tg.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
