import asyncio, os
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
    # try small ids
    for i in (0, 1, 2, 3, 100, 255, 256, 1000, 65535):
        try:
            ok = await c(functions.messages.UpdateDialogFilterRequest(id=i, filter=types.DialogFilter(**{**base, "id": i})))
            print("req/filt id", i, "->", ok)
            if ok is False:
                continue
            res = await c(functions.messages.GetDialogFiltersRequest())
            extra = [ (type(x).__name__, getattr(x,'id',None), getattr(getattr(x,'title',None),'text',getattr(x,'title',None))) for x in res.filters ]
            print("folders now:", extra)
            break
        except errors.RPCError as e:
            print("id", i, "-> RPC:", str(e)[:120])
    await c.disconnect()

asyncio.run(main())
