"""Discover + resolve + join VPN config channels, then extend folder "vpn".

Runs inside sadbot-tgsidecar (needs TG_API_ID/TG_API_HASH + a .session file).
Prints one line per channel: kind | id | @username | title | join_status
"""
import asyncio
import os
import sys

from telethon import TelegramClient, functions, types
from telethon.errors import (
    ChannelsTooMuchError,
    ChatAdminRequiredError,
    FloodWaitError,
    UserAlreadyParticipantError,
)

API_ID = int(os.environ["TG_API_ID"])
API_HASH = os.environ["TG_API_HASH"]
SESSION = sys.argv[1] if len(sys.argv) > 1 else "/tmp/vpnsearch"

# candidate usernames to try directly (popular free config channels)
CANDIDATES = [
    "V2ry_Proxy",
    "free_v2rayyy",
    "v2ray_free_conf",
    "DirectVPN",
    "VPNCUSTOMIZE",
    "v2rayng_vpn",
    "v2rayNG_Matsuri",
    "proxystore11",
    "Dark_V2ray",
    "VlessConfig",
    "servernett",
    "meli_proxyy",
    "PrivateVPNs",
    "free_v2rayng",
    "v2rayng_vpnn",
    "vmess_vless_v2rayng",
    "vpnowl",
    "V2rayNGvpni",
    "v2rayng_config_amin",
    "vpnclassic",
    "ProxyMTProto",
    "unlimited_v2ray",
    "v2ray_vpn_free",
    "free_v2ray_proxy",
    "config_v2ray_free",
    "v2ray_configs",
    "v2rayng_config",
    "vmess_vless",
    "vless_v2rayng",
    "free_proxy_v2ray",
    "v2ray_free_daily",
    "daily_v2ray",
    "v2ray_proxi",
    "outline_vpn",
    "v2rayngm",
    "v2ray_vless_vmess",
    "proxyfreenet",
    "v2rayng_free",
    "lightning_v2ray",
    "v2ray_wireguard",
    "s2ray_vpn",
    "vmess_v2ray",
    "vless_vmess_ss",
    "free_v2ray_config",
]

# keyword queries for global search (to discover channels without known usernames)
QUERIES = [
    "vless://",
    "vmess:// free",
    "کانفیگ وی پی ان",
    "v2ray رایگان",
    "free v2ray config",
]


async def main():
    client = TelegramClient(SESSION, API_ID, API_HASH)
    await client.connect()
    if not await client.is_user_authorized():
        print("NOT_AUTHORIZED")
        return

    found = {}  # id -> (kind, username, title)

    # 1) resolve direct candidates
    for uname in CANDIDATES:
        try:
            ent = await client.get_entity(uname)
        except Exception:
            continue
        eid = getattr(ent, "id", None)
        if eid is None:
            continue
        found[eid] = (
            type(ent).__name__,
            getattr(ent, "username", None) or "",
            getattr(ent, "title", None) or "",
        )

    # 2) global search to discover more
    for q in QUERIES:
        try:
            res = await client(
                functions.messages.SearchGlobalRequest(
                    q=q,
                    filter=types.InputMessagesFilterEmpty(),
                    min_date=None,
                    max_date=None,
                    offset_peer=types.InputPeerEmpty(),
                    offset_rate=0,
                    offset_id=0,
                    limit=40,
                )
            )
        except Exception as e:
            print(f"search_fail {q!r}: {e}")
            continue
        for c in res.chats:
            eid = getattr(c, "id", None)
            if eid is None:
                continue
            title = getattr(c, "title", None) or getattr(c, "first_name", "") or ""
            username = getattr(c, "username", None) or ""
            found[eid] = (type(c).__name__, username, title)
        await asyncio.sleep(1)

    # 3) join + report
    joined = []
    for eid, (kind, username, title) in sorted(found.items(), key=lambda kv: kv[0]):
        full = f"-100{eid}" if kind == "Channel" else str(eid)
        status = "skipped"
        try:
            ent = await client.get_entity(eid)
            try:
                await client(functions.channels.JoinChannelRequest(ent))
                status = "joined"
                joined.append(full)
            except UserAlreadyParticipantError:
                status = "already"
                joined.append(full)
            except ChannelsTooMuchError:
                status = "too_many_dialogs"
            except ChatAdminRequiredError:
                status = "admin_required"
            except FloodWaitError as e:
                status = f"floodwait_{e.seconds}"
                await asyncio.sleep(min(e.seconds, 30))
            except Exception as e:
                status = f"join_err:{type(e).__name__}"
        except Exception as e:
            status = f"resolve_err:{type(e).__name__}"
        print(f"{kind:8} {full:16} @{username or '-':28} {title[:40]:40} {status}")

    print("JOINED_REFS:")
    for r in joined:
        print(" ", r)
    await client.disconnect()


if __name__ == "__main__":
    asyncio.run(main())
