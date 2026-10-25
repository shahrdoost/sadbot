import asyncio
from app.store import get_client

async def main():
    client = await get_client("9d65f63e35f447ebb1e3d077a6e5184b")
    entity = await client.get_entity("@ashnadbot")

    await client.send_message(entity, "/start")
    await asyncio.sleep(2)
    await client.send_message(entity, "\U0001F381 ماموریت‌ها")
    await asyncio.sleep(2)

    async for m in client.iter_messages(entity, limit=5):
        if not m.out and m.reply_markup:
            for row in m.reply_markup.rows:
                for btn in row.buttons:
                    if hasattr(btn, "callback") and btn.callback:
                        print(f"button: {btn.text} -> {btn.callback}")
                        await btn.click()
                        print("clicked")
            break

    await asyncio.sleep(3)
    async for m in client.iter_messages(entity, limit=2):
        who = "BOT" if not m.out else "ME"
        txt = (m.text or "(no text)")[:400]
        print(f"[{who}] {txt}")

    await client.disconnect()

asyncio.run(main())
