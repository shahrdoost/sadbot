"""Automate the BotFather /newbot conversation from a user account."""
import asyncio
import re
import time
from typing import Optional

from telethon import errors

TOKEN_RE = re.compile(r"\d{6,12}:[A-Za-z0-9_-]{30,}")

# phonetic Persian/Arabic → Latin (good enough for usernames)
_TRANSLIT = {
    "ا": "a", "آ": "a", "أ": "a", "إ": "a", "ٱ": "a",
    "ب": "b", "پ": "p", "ت": "t", "ث": "s", "ج": "j", "چ": "ch",
    "ح": "h", "خ": "kh", "د": "d", "ذ": "z", "ر": "r", "ز": "z",
    "ژ": "zh", "س": "s", "ش": "sh", "ص": "s", "ض": "z", "ط": "t",
    "ظ": "z", "ع": "a", "غ": "gh", "ف": "f", "ق": "gh", "ک": "k",
    "گ": "g", "ل": "l", "م": "m", "ن": "n", "و": "v", "ه": "h",
    "ة": "h", "ی": "i", "ي": "i", "ئ": "i", "ؤ": "o", "إ": "e",
    "ك": "k", "ة": "h", "ٔ": "", "ً": "", "ٌ": "", "ٍ": "", "َ": "",
    "ُ": "", "ِ": "", "ّ": "", "ْ": "",
}


class FlowError(Exception):
    pass


class Timeout(FlowError):
    """BotFather did not answer in time."""


class NoUsername(FlowError):
    """Every candidate username was rejected."""


class FloodRetry(FlowError):
    def __init__(self, seconds: int):
        super().__init__(f"flood wait {seconds}")
        self.seconds = seconds


def transliterate(name: str) -> str:
    out = []
    for ch in name:
        if ch in _TRANSLIT:
            out.append(_TRANSLIT[ch])
        elif ch.isascii() and ch.isalnum():
            out.append(ch.lower())
        elif ch == " " or ch == "_":
            out.append(" ")
    return " ".join("".join(out).split())


def username_candidates(name: str, hint: Optional[str] = None) -> list[str]:
    """Ordered unique BotFather-legal candidates derived from name/hint."""
    bases: list[str] = []
    if hint and hint.strip():
        bases.append(hint.strip())
    base = transliterate(name).replace(" ", "")
    if base:
        bases.append(base)
    if not bases:
        bases = ["sadbot"]

    cands: list[str] = []
    for b in bases:
        b = re.sub(r"[^A-Za-z0-9_]", "", b).strip("_") or "sadbot"
        options = [f"{b}_bot", f"{b}bot"]
        options += [f"{b}{i}bot" for i in range(2, 31)]
        options += [f"{b}_{i}_bot" for i in range(2, 11)]
        cands.extend(options)
    # always some generic fallbacks at the end
    cands += [f"sadbot_{i}bot" for i in range(1, 21)]

    seen: set[str] = set()
    out: list[str] = []
    for c in cands:
        c = c[:32]
        if len(c) < 5 or not c.endswith(("bot", "_bot")):
            # ensure the suffix survives truncation
            c = (c[:-3] + "bot") if len(c) > 3 else "xbot"
        if c not in seen:
            seen.add(c)
            out.append(c)
    return out


async def wait_reply(client, entity, after_id: int, timeout: float = 20.0):
    """Poll the chat for the first incoming message newer than after_id."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            msgs = await client.get_messages(entity, limit=10)
        except errors.FloodWaitError:
            raise
        for m in msgs:
            if m.id > after_id and not m.out:
                return m
        await asyncio.sleep(1.5)
    raise Timeout()


async def create_bot(client, name: str, hint: Optional[str] = None) -> dict:
    """Drive the /newbot conversation; return {token, username}."""
    botfather = await client.get_entity("BotFather")

    # /newbot — BotFather rate-limits repeated attempts ("too many attempts")
    reply = None
    for _ in range(4):
        sent = await client.send_message(botfather, "/newbot")
        reply = await wait_reply(client, botfather, sent.id)
        low = (reply.text or "").lower()
        if "too many attempts" in low:
            m = re.search(r"(\d+) seconds", low)
            wait = int(m.group(1)) if m else 30
            # long cooldowns (hours after hammering /newbot) must fail fast —
            # sleeping here would hang the whole create-bot request for hours
            if wait > 180:
                mins = wait // 60
                raise FlowError(
                    f"BotFather محدودیت ساخت زده است؛ حدود {mins} دقیقه دیگر دوباره تلاش کنید"
                )
            await asyncio.sleep(wait + 2)
            continue
        break
    else:
        raise FlowError("BotFather محدودیت ساخت دارد؛ کمی بعد دوباره تلاش کنید")

    text = (reply.text or "").lower()
    if "call it" not in text and "name" not in text:
        raise FlowError(reply.text or "unexpected BotFather reply")

    sent = await client.send_message(botfather, name)
    reply = await wait_reply(client, botfather, sent.id)  # username prompt / error
    text = (reply.text or "").lower()
    if "too many attempts" in text:
        raise FlowError("BotFather محدودیت ساخت دارد؛ کمی بعد دوباره تلاش کنید")
    if "username" not in text and "taken" not in text:
        # name rejected or unexpected prompt — surface whatever it said
        raise FlowError(reply.text or "unexpected BotFather reply")

    for cand in username_candidates(name, hint):
        sent = await client.send_message(botfather, cand)
        reply = await wait_reply(client, botfather, sent.id)
        body = reply.text or ""
        low = body.lower()
        if "too many requests" in low:
            raise FloodRetry(60)
        token_match = TOKEN_RE.search(body)
        if token_match:
            return {"token": token_match.group(0), "username": cand}
        # success often arrives as "Done!" with the token in the NEXT message
        if "done!" in low or "congratulations" in low:
            nxt = await wait_reply(client, botfather, reply.id, timeout=15.0)
            token_match = TOKEN_RE.search(nxt.text or "")
            if token_match:
                return {"token": token_match.group(0), "username": cand}
            raise FlowError("توکن در پاسخ BotFather پیدا نشد")
        # taken / invalid / anything else → next candidate
        continue

    raise NoUsername()
