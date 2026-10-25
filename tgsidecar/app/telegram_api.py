import os
import re
import tempfile
from pathlib import Path
from typing import Optional

from fastapi import APIRouter, Depends, Header, HTTPException
from fastapi.responses import FileResponse
from starlette.background import BackgroundTask
from pydantic import BaseModel
from telethon import errors
from telethon.tl.types import MessageEntityPre

from . import store
from .schemas import (
    LoginCompleteRequest,
    LoginStartRequest,
    UserInfo,
    serialize_chat,
    serialize_message,
    serialize_user,
)

router = APIRouter()

MAX_PHOTO_BYTES = 10 * 1024 * 1024


def require_key(x_sidecar_key: Optional[str] = Header(default=None)) -> None:
    if not store.SIDECAR_KEY or x_sidecar_key != store.SIDECAR_KEY:
        raise HTTPException(status_code=401, detail="unauthorized")


def flood(e: errors.FloodWaitError) -> HTTPException:
    return HTTPException(status_code=429, detail={"error": "FLOOD_WAIT", "seconds": e.seconds})


# protocol URIs users tap-to-copy out of the channel
_CONFIG_URI_RE = re.compile(
    r"(?:vless|vmess|ss|ssr|trojan|tuic|hysteria2?|wireguard|wg|socks)://[^\s]+",
    re.IGNORECASE,
)
_URI_TRAIL = ")]}>,.،؛!؟:;\"'"


def _utf16_len(s: str) -> int:
    return len(s.encode("utf-16-le")) // 2


def _copyable(text: str):
    """Strip stray markdown fences and mark each config URI as a Pre block.

    Telegram's monospace (Pre) entity is tap-to-copy. parse_mode=markdown is
    unusable for these posts: underscores in hosts (V2ry_Proxy, *.ir paths)
    open accidental italics and the model's ``` fences end up literal.
    Offsets are UTF-16 code units — emoji in the Persian intro shift them.
    """
    if not text:
        return text, None
    cleaned = text.replace("```", "").replace("`", "")
    entities = []
    for m in _CONFIG_URI_RE.finditer(cleaned):
        uri = m.group(0).rstrip(_URI_TRAIL)
        if not uri:
            continue
        entities.append(
            MessageEntityPre(offset=_utf16_len(cleaned[: m.start()]), length=_utf16_len(uri), language="")
        )
    return cleaned, entities or None


async def _account_or_404(account_id: str):
    try:
        return await store.get_client(account_id)
    except KeyError:
        raise HTTPException(status_code=404, detail="account not found")


@router.get("/healthz")
async def healthz() -> dict:
    return {"status": "ok"}


@router.post("/login/start", dependencies=[Depends(require_key)])
async def login_start(body: LoginStartRequest) -> dict:
    async with store.login_lock:
        pending_id, tmp = store.new_pending_session()
        client = store.make_client(tmp)
        await client.connect()
        try:
            sent = await client.send_code_request(body.phone)
        except errors.FloodWaitError as e:
            await client.disconnect()
            raise flood(e)
        except Exception:
            await client.disconnect()
            raise
        store.new_login_state(pending_id, body.phone, sent.phone_code_hash, client)
    return {"ok": True, "pending_id": pending_id}


@router.post("/login/complete", dependencies=[Depends(require_key)])
async def login_complete(body: LoginCompleteRequest) -> dict:
    async with store.login_lock:
        state = store.pop_login_state(body.pending_id)
        if state is None or state.phone != body.phone:
            raise HTTPException(status_code=404, detail="login session expired; start again")
        try:
            await state.client.sign_in(phone=body.phone, code=body.code, phone_code_hash=state.phone_code_hash)
        except errors.SessionPasswordNeededError:
            if not body.password:
                # keep the state so the client can retry with the password
                store.restore_login_state(body.pending_id, state)
                raise HTTPException(status_code=401, detail="PASSWORD_NEEDED")
            try:
                await state.client.sign_in(password=body.password)
            except errors.PasswordHashInvalidError:
                store.restore_login_state(body.pending_id, state)
                raise HTTPException(status_code=401, detail="WRONG_PASSWORD")
        except errors.FloodWaitError as e:
            store.restore_login_state(body.pending_id, state)
            raise flood(e)
        except errors.PhoneCodeInvalidError:
            raise HTTPException(status_code=400, detail="INVALID_CODE")

        account_id = store.finalize_login(body.pending_id, state)
        me = await state.client.get_me()
    return {"account_id": account_id, "user": serialize_user(me, phone=body.phone)}


@router.get("/accounts", dependencies=[Depends(require_key)])
async def list_accounts() -> dict:
    out = []
    for account_id in store.list_account_ids():
        try:
            client = await store.get_client(account_id)
            me = await client.get_me()
            out.append({"account_id": account_id, "user": serialize_user(me)})
        except Exception:
            out.append({"account_id": account_id, "user": UserInfo(id="", first_name="؟")})
    return {"accounts": out}


@router.delete("/accounts/{account_id}", dependencies=[Depends(require_key)])
async def delete_account(account_id: str) -> dict:
    existed = await store.delete_account(account_id)
    if not existed:
        raise HTTPException(status_code=404, detail="account not found")
    return {"ok": True}


@router.get("/accounts/{account_id}/chats", dependencies=[Depends(require_key)])
async def list_chats(account_id: str) -> dict:
    client = await _account_or_404(account_id)
    try:
        dialogs = await client.get_dialogs()
    except errors.FloodWaitError as e:
        raise flood(e)
    chats = [serialize_chat(d) for d in dialogs]
    # pinned first, then most-recent activity — so a chat with a new message
    # bubbles to the top on every poll.
    from datetime import datetime

    def ts(c):
        if not c.last_message or not c.last_message.date:
            return 0.0
        try:
            return datetime.fromisoformat(c.last_message.date).timestamp()
        except ValueError:
            return 0.0

    chats.sort(key=ts, reverse=True)  # newest first (stable)
    chats.sort(key=lambda c: not c.pinned)  # pinned first (stable)
    return {"chats": chats}


@router.get("/accounts/{account_id}/folders", dependencies=[Depends(require_key)])
async def list_folders(account_id: str) -> dict:
    """Dialog folders (Telegram chat folders) with their included peers."""
    client = await _account_or_404(account_id)
    from telethon import functions, types, utils

    try:
        result = await client(functions.messages.GetDialogFiltersRequest())
    except errors.FloodWaitError as e:
        raise flood(e)

    folders = []
    for flt in getattr(result, "filters", []) or []:
        # only plain folders — skip the default "All chats" and chatlist invites
        if not isinstance(flt, types.DialogFilter):
            continue
        peers = []
        for peer in flt.include_peers or []:
            try:
                entity = await client.get_entity(peer)
            except Exception:
                continue
            if isinstance(entity, types.Channel):
                kind = "channel" if entity.broadcast else "group"
            elif isinstance(entity, types.Chat):
                kind = "group"
            else:
                kind = "user"
            peers.append(
                {
                    # marked id (-100…) — same shape as dialog.id in /chats
                    "chat_id": str(utils.get_peer_id(entity)),
                    "title": getattr(entity, "title", None) or "",
                    "username": getattr(entity, "username", None) or "",
                    "type": kind,
                }
            )
        # flt.title is a TextWithEntities from the wire — take its .text
        title = flt.title
        title = getattr(title, "text", None) or (title if isinstance(title, str) else "")
        folders.append({"name": title, "peers": peers})
    return {"folders": folders}


class JoinChatRequest(BaseModel):
    # @username or a numeric chat_id (-100… form or bare)
    chat: str


async def _resolve_chat(client, ref: str):
    """@username / bare display-name / numeric chat_id (-100…) → entity or None.

    A numeric STRING must first become an int — telethon treats a str as a
    username, so folder adds silently 404'd on ids like "-1004362422574".
    Channels with no username (claude-logs) are only reachable via dialogs.
    """
    ref = (ref or "").strip()
    if not ref:
        return None
    if ref.lstrip("-").isdigit():
        try:
            return await client.get_entity(int(ref))
        except Exception:
            return None
    try:
        return await client.get_entity(ref)
    except Exception:
        pass
    low = ref.lstrip("@").lower()
    try:
        async for d in client.iter_dialogs():
            ent = d.entity
            title = (getattr(ent, "title", None) or getattr(ent, "first_name", None) or "")
            uname = (getattr(ent, "username", None) or "")
            if low in (title.lower(), uname.lower()) or low == str(getattr(ent, "id", "")):
                return ent
    except Exception:
        return None
    return None


@router.post("/accounts/{account_id}/join", dependencies=[Depends(require_key)])
async def join_chat(account_id: str, body: JoinChatRequest) -> dict:
    """Join a public channel by @username or chat_id (already-member = ok)."""
    ref = (body.chat or "").strip()
    if not ref:
        raise HTTPException(status_code=400, detail="EMPTY_CHAT")
    client = await _account_or_404(account_id)
    from telethon.tl.functions.channels import JoinChannelRequest

    try:
        entity = await _resolve_chat(client, ref)
        if entity is None:
            raise HTTPException(status_code=404, detail="CHAT_NOT_FOUND")
        await client(JoinChannelRequest(entity))
    except errors.FloodWaitError as e:
        raise flood(e)
    except errors.UserAlreadyParticipantError:
        return {"ok": True, "already_member": True}
    except errors.RPCError as e:
        raise HTTPException(status_code=400, detail=f"RPC_ERROR: {e}")
    except (ValueError, TypeError):
        raise HTTPException(status_code=404, detail="CHAT_NOT_FOUND")
    return {"ok": True}


class EnsureFolderRequest(BaseModel):
    name: str
    chats: list[str]  # @usernames and/or numeric chat_ids


@router.post("/accounts/{account_id}/folders", dependencies=[Depends(require_key)])
async def ensure_folder(account_id: str, body: EnsureFolderRequest) -> dict:
    """Create (or replace the peer list of) a dialog folder by name — the
    collector only reads GetDialogFilters, so a content bot's folder has to
    exist on the ACCOUNT side. Same-name folder keeps its id; other folders
    untouched."""
    name = (body.name or "").strip()
    if not name:
        raise HTTPException(status_code=400, detail="EMPTY_NAME")
    if not body.chats:
        raise HTTPException(status_code=400, detail="EMPTY_CHATS")
    client = await _account_or_404(account_id)
    from telethon import functions, types
    from telethon.tl.functions.channels import JoinChannelRequest

    # resolve every chat → InputPeer (join first if public and not a member)
    peers = []
    for ref in body.chats:
        ref = ref.strip()
        if not ref:
            continue
        entity = await _resolve_chat(client, ref)
        if entity is None:
            raise HTTPException(status_code=404, detail=f"CHAT_NOT_FOUND: {ref}")
        try:
            await client(JoinChannelRequest(entity))
        except errors.FloodWaitError as fh:
            raise flood(fh)
        except errors.RPCError:
            pass  # private/already-member — get_input_entity below decides
        peers.append(await client.get_input_entity(entity))
    if not peers:
        raise HTTPException(status_code=400, detail="EMPTY_CHATS")

    try:
        current = await client(functions.messages.GetDialogFiltersRequest())
    except errors.FloodWaitError as e:
        raise flood(e)
    filters = list(getattr(current, "filters", []) or [])

    target = None
    for flt in filters:
        if not isinstance(flt, types.DialogFilter):
            continue
        t = flt.title
        t = getattr(t, "text", None) or (t if isinstance(t, str) else "")
        if t == name:
            target = flt
            break

    if target is None:
        used_ids = {flt.id for flt in filters if isinstance(flt, types.DialogFilter)}
        # Telegram rejects id 0/1 (0 is the "no folder" sentinel) → the first
        # free id ≥ 2 creates the folder (verified live: 2 accepted, 0/1 →
        # FILTER_ID_INVALID). telethon 1.45 has no CreateDialogFilterRequest —
        # UpdateDialogFilterRequest + a fresh id doubles as create.
        new_id = next((i for i in range(2, 1000) if i not in used_ids), None)
        if new_id is None:
            raise HTTPException(status_code=409, detail="FOLDER_LIMIT")
        new_filter = types.DialogFilter(
            id=new_id,
            title=types.TextWithEntities(text=name, entities=[]),
            include_peers=peers,
            exclude_peers=[],
            pinned_peers=[],
        )
        req = functions.messages.UpdateDialogFilterRequest(id=new_id, filter=new_filter)
    else:
        new_filter = types.DialogFilter(
            id=target.id,
            title=target.title,
            include_peers=peers,
            exclude_peers=getattr(target, "exclude_peers", []) or [],
            pinned_peers=getattr(target, "pinned_peers", []) or [],
        )
        req = functions.messages.UpdateDialogFilterRequest(id=target.id, filter=new_filter)

    try:
        ok = await client(req)
    except errors.FloodWaitError as e:
        raise flood(e)
    except errors.RPCError as e:
        raise HTTPException(status_code=400, detail=f"RPC_ERROR: {e}")
    if ok is False:
        raise HTTPException(status_code=400, detail="FOLDER_SAVE_REFUSED")
    return {"ok": True, "name": name, "count": len(peers)}


@router.get("/accounts/{account_id}/chats/{chat_id}/photo", dependencies=[Depends(require_key)])
async def chat_photo(account_id: str, chat_id: str) -> FileResponse:
    client = await _account_or_404(account_id)
    try:
        entity = await client.get_entity(int(chat_id))
    except errors.FloodWaitError as e:
        raise flood(e)
    except (ValueError, TypeError):
        raise HTTPException(status_code=404, detail="chat not found")
    tmp = Path(tempfile.gettempdir()) / f"tgchatphoto_{os.urandom(8).hex()}"
    try:
        path = await client.download_profile_photo(entity, file=str(tmp))
        if not path:
            raise HTTPException(status_code=404, detail="no photo")
        path = Path(path)
        if path.stat().st_size > MAX_PHOTO_BYTES:
            path.unlink(missing_ok=True)
            raise HTTPException(status_code=413, detail="photo too large")
        return FileResponse(path, media_type="image/jpeg", filename="chat.jpg", background=BackgroundTask(path.unlink, missing_ok=True))
    except errors.FloodWaitError as e:
        tmp.unlink(missing_ok=True)
        raise flood(e)


@router.get("/accounts/{account_id}/chats/{chat_id}/messages", dependencies=[Depends(require_key)])
async def list_messages(account_id: str, chat_id: str, limit: int = 50, offset_id: int = 0) -> dict:
    client = await _account_or_404(account_id)
    limit = max(1, min(limit, 100))
    try:
        msgs = await client.get_messages(int(chat_id), limit=limit, offset_id=offset_id)
    except errors.FloodWaitError as e:
        raise flood(e)
    except (ValueError, TypeError):
        raise HTTPException(status_code=404, detail="chat not found")
    # Telethon returns newest-first; the panel renders top→bottom as oldest→newest
    messages = [serialize_message(m) for m in reversed(msgs)]
    oldest = min((m.id for m in msgs), default=0)
    return {"messages": messages, "oldest_id": oldest}


@router.get("/accounts/{account_id}/chats/{chat_id}/messages/{msg_id}/photo", dependencies=[Depends(require_key)])
async def message_photo(account_id: str, chat_id: str, msg_id: int) -> FileResponse:
    client = await _account_or_404(account_id)
    try:
        msg = await client.get_messages(int(chat_id), ids=msg_id)
    except errors.FloodWaitError as e:
        raise flood(e)
    if msg is None or not getattr(msg, "photo", None):
        raise HTTPException(status_code=404, detail="photo not found")
    tmp = Path(tempfile.gettempdir()) / f"tgphoto_{os.urandom(8).hex()}"
    try:
        path = await client.download_media(msg.photo, file=str(tmp))
        if not path:
            raise HTTPException(status_code=404, detail="download failed")
        path = Path(path)
        if path.stat().st_size > MAX_PHOTO_BYTES:
            path.unlink(missing_ok=True)
            raise HTTPException(status_code=413, detail="photo too large")
        return FileResponse(path, media_type="image/jpeg", filename="photo.jpg", background=BackgroundTask(path.unlink, missing_ok=True))
    except errors.FloodWaitError as e:
        tmp.unlink(missing_ok=True)
        raise flood(e)


@router.get("/accounts/{account_id}/chats/{chat_id}/participant", dependencies=[Depends(require_key)])
async def get_participant(account_id: str, chat_id: str, user_id: int) -> dict:
    """Is this user a member of the chat? 200 {participant:...} | 404 NOT_MEMBER."""
    from telethon.tl.functions.channels import GetParticipantRequest

    client = await _account_or_404(account_id)
    try:
        entity = await client.get_entity(int(chat_id))
        res = await client(GetParticipantRequest(channel=entity, participant=user_id))
    except (ValueError, TypeError):
        raise HTTPException(status_code=404, detail="chat not found")
    except errors.RPCError:
        raise HTTPException(status_code=404, detail="NOT_MEMBER")
    kind = type(res.participant).__name__
    return {"participant": {
        "user_id": user_id,
        "kind": kind,
        "banned": "Banned" in kind,
        "left": "Left" in kind,
    }}


@router.delete("/accounts/{account_id}/chats/{chat_id}/messages/{msg_id}", dependencies=[Depends(require_key)])
async def delete_message(account_id: str, chat_id: str, msg_id: int) -> dict:
    """Delete one message (cleanup of verification posts)."""
    client = await _account_or_404(account_id)
    try:
        await client.delete_messages(int(chat_id), [msg_id])
    except errors.FloodWaitError as e:
        raise flood(e)
    except errors.RPCError:
        raise HTTPException(status_code=400, detail="RPC_ERROR")
    except (ValueError, TypeError):
        raise HTTPException(status_code=404, detail="chat not found")
    return {"ok": True, "deleted": msg_id}


class SendMessageRequest(BaseModel):
    text: str


@router.post("/accounts/{account_id}/chats/{chat_id}/messages", dependencies=[Depends(require_key)])
async def send_message(account_id: str, chat_id: str, body: SendMessageRequest) -> dict:
    text = (body.text or "").strip()
    if not text:
        raise HTTPException(status_code=400, detail="EMPTY_TEXT")
    if len(text) > 4096:
        raise HTTPException(status_code=400, detail="TEXT_TOO_LONG")
    client = await _account_or_404(account_id)
    body_text, ents = _copyable(text)
    try:
        if ents:
            msg = await client.send_message(int(chat_id), body_text, formatting_entities=ents, parse_mode=None)
        else:
            msg = await client.send_message(int(chat_id), text)
    except errors.FloodWaitError as e:
        raise flood(e)
    except errors.ChatWriteForbiddenError:
        raise HTTPException(status_code=400, detail="WRITE_FORBIDDEN")
    except errors.ChannelPrivateError:
        raise HTTPException(status_code=400, detail="CHANNEL_PRIVATE")
    except errors.UserIsBlockedError:
        raise HTTPException(status_code=400, detail="USER_BLOCKED_BOT")
    except errors.PeerIdInvalidError:
        raise HTTPException(status_code=400, detail="PEER_INVALID")
    except errors.RPCError:
        raise HTTPException(status_code=400, detail="RPC_ERROR")
    except (ValueError, TypeError):
        raise HTTPException(status_code=404, detail="chat not found")
    return {"message": serialize_message(msg)}


class SendPhotoRequest(BaseModel):
    # PNG bytes, base64 (phone can't take raw binary in JSON)
    image_b64: str
    filename: str = "photo.png"
    caption: str = ""


@router.post("/accounts/{account_id}/chats/{chat_id}/photo", dependencies=[Depends(require_key)])
async def send_photo(account_id: str, chat_id: str, body: SendPhotoRequest) -> dict:
    """Publish a photo with caption from the MTProto account (sadcontent's old
    telethon path — used when the destination channel has no bot admin)."""
    import base64

    if not body.image_b64:
        raise HTTPException(status_code=400, detail="EMPTY_IMAGE")
    try:
        data = base64.b64decode(body.image_b64)
    except Exception:
        raise HTTPException(status_code=400, detail="INVALID_B64")
    if len(data) > MAX_PHOTO_BYTES:
        raise HTTPException(status_code=413, detail="image too large")
    caption = (body.caption or "")[:1024]
    client = await _account_or_404(account_id)
    cap_text, cap_ents = _copyable(caption)
    try:
        import io as _io

        buf = _io.BytesIO(data)
        # telethon decides photo-vs-document from the file extension
        # (utils.is_image → _get_extension): a bare BytesIO has no .name, so
        # every card would land as a captionless document.
        buf.name = body.filename or "card.png"
        if cap_ents:
            msg = await client.send_file(
                int(chat_id),
                file=buf,
                caption=cap_text or None,
                formatting_entities=cap_ents,
                parse_mode=None,
            )
        else:
            msg = await client.send_file(
                int(chat_id),
                file=buf,
                caption=caption or None,
            )
    except errors.FloodWaitError as e:
        raise flood(e)
    except errors.ChatWriteForbiddenError:
        raise HTTPException(status_code=400, detail="WRITE_FORBIDDEN")
    except errors.ChannelPrivateError:
        raise HTTPException(status_code=400, detail="CHANNEL_PRIVATE")
    except errors.RPCError:
        raise HTTPException(status_code=400, detail="RPC_ERROR")
    except (ValueError, TypeError):
        raise HTTPException(status_code=404, detail="chat not found")
    return {"message": serialize_message(msg)}


class BotFatherCreateRequest(BaseModel):
    name: str
    username_hint: Optional[str] = None


@router.post("/accounts/{account_id}/botfather/create-bot", dependencies=[Depends(require_key)])
async def botfather_create_bot(account_id: str, body: BotFatherCreateRequest) -> dict:
    if not body.name or not body.name.strip():
        raise HTTPException(status_code=400, detail="name required")
    client = await _account_or_404(account_id)
    from . import botfather

    async with store.bf_lock(account_id):
        try:
            return await botfather.create_bot(client, body.name.strip(), body.username_hint)
        except botfather.FloodRetry as e:
            raise HTTPException(status_code=429, detail={"error": "FLOOD_WAIT", "seconds": e.seconds})
        except botfather.NoUsername:
            raise HTTPException(status_code=409, detail="NO_FREE_USERNAME")
        except botfather.Timeout:
            raise HTTPException(status_code=504, detail="BOTFATHER_TIMEOUT")
        except botfather.FlowError as e:
            raise HTTPException(status_code=409, detail=f"BOTFATHER_ERROR: {e}")