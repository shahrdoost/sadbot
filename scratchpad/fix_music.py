import asyncio, os
from telethon import TelegramClient, functions, types, errors

async def main():
    api_id = int(os.environ["TG_API_ID"]); api_hash = os.environ["TG_API_HASH"]
    c = TelegramClient("/tmp/vpnsearch", api_id, api_hash)
    await c.connect()
    res = await c(functions.messages.GetDialogFiltersRequest())
    target = None
    for f in res.filters:
        if type(f).__name__ == "DialogFilter" and getattr(f, "id", None) == 196:
            target = f
    if target is None:
        print("id196 not found"); return
    # only the title was changed during probing — restore Music
    f2 = types.DialogFilter(
        id=target.id, title=types.TextWithEntities(text="Music", entities=[]),
        include_peers=target.include_peers, exclude_peers=target.exclude_peers or [],
        pinned_peers=target.pinned_peers or [], contacts=target.contacts or [],
        non_contacts=target.non_contacts or [], groups=target.groups or [],
        broadcasts=getattr(target, "broadcasts", None) or False,
        bots=getattr(target, "bots", None) or False,
        exclude_muted=getattr(target, "exclude_muted", None) or False,
        exclude_read=getattr(target, "exclude_read", None) or False,
        exclude_archived=getattr(target, "exclude_archived", None) or False,
    )
    ok = await c(functions.messages.UpdateDialogFilterRequest(id=target.id, filter=f2))
    print("restore Music ->", ok)
    res = await c(functions.messages.GetDialogFiltersRequest())
    for f in res.filters:
        print(" now:", type(f).__name__, getattr(f, "id", None),
              getattr(getattr(f, "title", None), "text", getattr(f, "title", None)),
              "peers:", len(getattr(f, "include_peers", []) or []))
    await c.disconnect()

asyncio.run(main())
