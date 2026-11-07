import re
from typing import Any, Optional

from pydantic import BaseModel
from telethon.tl.types import MessageEntityUrl


class LoginStartRequest(BaseModel):
    phone: str


class LoginCompleteRequest(BaseModel):
    pending_id: str
    phone: str
    code: str
    password: Optional[str] = None


class UserInfo(BaseModel):
    id: str
    first_name: str = ""
    last_name: str = ""
    username: str = ""
    phone: str = ""


class LastMessage(BaseModel):
    text: str = ""
    date: Optional[str] = None


class ChatInfo(BaseModel):
    chat_id: str
    title: str
    type: str  # user | group | channel | bot
    unread: int = 0
    pinned: bool = False
    last_message: Optional[LastMessage] = None


class SenderInfo(BaseModel):
    id: str = ""
    name: str = ""
    is_out: bool = False


class MediaInfo(BaseModel):
    kind: str = "none"  # none | photo | sticker | document | video | voice | audio | other


class MessageInfo(BaseModel):
    id: int
    date: Optional[str] = None
    sender: SenderInfo = SenderInfo()
    text: str = ""
    media: MediaInfo = MediaInfo()
    reply_to_id: Optional[int] = None
    forwarded: bool = False
    proxy_urls: list[str] = []


def iso(dt: Any) -> Optional[str]:
    return dt.isoformat() if dt else None


def serialize_user(user: Any, phone: str = "") -> UserInfo:
    return UserInfo(
        id=str(user.id),
        first_name=user.first_name or "",
        last_name=user.last_name or "",
        username=user.username or "",
        phone=phone or (user.phone or ""),
    )


def serialize_chat(dialog: Any) -> ChatInfo:
    entity = dialog.entity
    if getattr(entity, "bot", False):
        kind = "bot"
    elif hasattr(entity, "title"):
        kind = "channel" if getattr(entity, "broadcast", False) else "group"
    else:
        kind = "user"
    last = None
    if dialog.message is not None:
        last = LastMessage(text=dialog.message.message or "", date=iso(dialog.message.date))
    return ChatInfo(
        chat_id=str(dialog.id),
        title=dialog.name or "",
        type=kind,
        unread=dialog.unread_count or 0,
        pinned=bool(dialog.pinned),
        last_message=last,
    )


def media_kind(msg: Any) -> str:
    if msg.photo:
        return "photo"
    if msg.sticker:
        return "sticker"
    if msg.voice:
        return "voice"
    if msg.video:
        return "video"
    if msg.audio:
        return "audio"
    if msg.document:
        return "document"
    if msg.media:
        return "other"
    return "none"


def sender_name(msg: Any) -> SenderInfo:
    sender_id = ""
    name = ""
    if msg.sender is not None:
        sender_id = str(msg.sender.id)
        if hasattr(msg.sender, "first_name"):
            name = " ".join(x for x in [msg.sender.first_name, msg.sender.last_name] if x) or (
                getattr(msg.sender, "username", "") or ""
            )
        elif hasattr(msg.sender, "title"):
            name = msg.sender.title or ""
    elif msg.post_author:
        name = msg.post_author
    return SenderInfo(id=sender_id, name=name, is_out=bool(msg.out))


def serialize_message(msg: Any) -> MessageInfo:
    reply_to = None
    if msg.reply_to is not None:
        reply_to = getattr(msg.reply_to, "reply_to_msg_id", None)
    text = msg.message or ""
    return MessageInfo(
        id=msg.id,
        date=iso(msg.date),
        sender=sender_name(msg),
        text=text,
        media=MediaInfo(kind=media_kind(msg)),
        reply_to_id=reply_to,
        forwarded=msg.forward is not None,
        proxy_urls=message_proxy_urls(msg, text),
    )


# tappable proxy deep-links only — a bare t.me/proxy resolves to the @proxy
# profile, so parameterless links are never carried over
_PROXY_LINK_RE = re.compile(r"^(?:https?://t\.me/|tg://)(?:proxy|webproxy|socks)\?", re.I)


def _utf16_slice(text: str, offset: int, length: int) -> str:
    units = text.encode("utf-16-le")
    return units[offset * 2 : (offset + length) * 2].decode("utf-16-le", "ignore")


def message_proxy_urls(msg: Any, text: str) -> list[str]:
    """Proxy connect links of a message: URL buttons + linked entities.

    Sources like @ProxyMTProto post "Server: Unknown" blocks and carry the real
    endpoints on their inline buttons — the collector republishes those as the
    post's connect links, so they must survive serialization here.
    """
    found: list[str] = []

    def add(url: Any) -> None:
        u = (url or "").strip() if isinstance(url, str) else ""
        if u and _PROXY_LINK_RE.match(u) and u not in found:
            found.append(u)

    for ent in getattr(msg, "entities", None) or []:
        url = getattr(ent, "url", None)
        if url:
            add(url)
        elif isinstance(ent, MessageEntityUrl):
            add(_utf16_slice(text, ent.offset, ent.length))
    markup = getattr(msg, "reply_markup", None)
    for row in getattr(markup, "rows", None) or []:
        for btn in getattr(row, "buttons", None) or []:
            add(getattr(btn, "url", None))
    return found
