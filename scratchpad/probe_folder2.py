import asyncio, os
from telethon import TelegramClient, functions, types, errors

async def main():
    api_id = int(os.environ["TG_API_ID"]); api_hash = os.environ["TG_API_HASH"]
    c = TelegramClient("/tmp/vpnsearch", api_id, api_hash)
    await c.connect()

    res = await c(functions.messages.GetDialogFiltersRequest())
    for f in res.filters:
        print("have:", type(f).__name__, getattr(f, "id", None))
        if type(f).__name__ == "DialogFilter" and getattr(getattr(f, "title", None), "text", None) == "Music":
            music = f
    # 1) rewrite Music as-is (no content change)
    try:
        ok = await c(functions.messages.UpdateDialogFilterRequest(id=music.id, filter=music))
        print("rewrite Music id=%s ->" % music.id, ok)
    except errors.RPCError as e:
        print("rewrite Music RPC:", str(e)[:200])
    # 2) rewrite Music with its peers moved to a NEW folder name (same id)
    try:
        f2 = types.DialogFilter(
            id=music.id,
            title=types.TextWithEntities(text="vpn", entities=[]),
            include_peers=music.include_peers, exclude_peers=music.exclude_peers or [],
            pinned_peers=music.pinned_peers or [], contacts=music.contacts or [],
            non_contacts=music.non_contacts or [], groups=music.groups or [],
            broadcasts=getattr(music, "broadcasts", None) or False,
            bots=getattr(music, "bots", None) or False,
            exclude_muted=getattr(music, "exclude_muted", None) or False,
            exclude_read=getattr(music, "exclude_read", None) or False,
            exclude_archived=getattr(music, "exclude_archived", None) or False,
        )
        ok = await c(functions.messages.UpdateDialogFilterRequest(id=music.id, filter=f2))
        print("rename Music->vpn ->", ok)
    except errors.RPCError as e:
        print("rename RPC:", str(e)[:200])
    await c.disconnect()

asyncio.run(main())
