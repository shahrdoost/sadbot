import asyncio, os, inspect
from telethon import TelegramClient, functions, types, errors

async def main():
    api_id = int(os.environ["TG_API_ID"]); api_hash = os.environ["TG_API_HASH"]
    c = TelegramClient("/tmp/vpnsearch", api_id, api_hash)
    await c.connect()
    ent = await c.get_entity("@V2ry_Proxy")
    peer = await c.get_input_entity(ent)

    base = dict(title=types.TextWithEntities(text="vpn", entities=[]),
                include_peers=[peer], exclude_peers=[], pinned_peers=[], contacts=[],
                non_contacts=[], groups=[], broadcasts=True, bots=False,
                exclude_muted=False, exclude_read=False, exclude_archived=True)
    variants = [
        ("A req0 f0", 0, 0),
        ("B req0 f9001", 0, 9001),
        ("C req9001 f9001", 9001, 9001),
        ("D req9001 f0", 9001, 0),
    ]

    for label, req_id, filt_id in variants:
        f = types.DialogFilter(**{**base, "id": filt_id})
        req = functions.messages.UpdateDialogFilterRequest(id=req_id, filter=f)
        try:
            ok = await c(req)
            print(label, "-> ok:", ok if ok is not False else "False")
            if ok is not False:
                res = await c(functions.messages.GetDialogFiltersRequest())
                for x in res.filters:
                    t = getattr(getattr(x, "title", None), "text", getattr(x, "title", None))
                    print("  NOW:", type(x).__name__, getattr(x, "id", None), t)
                break
        except errors.RPCError as e:
            print(label, "-> RPC:", str(e)[:160])
    await c.disconnect()

asyncio.run(main())
