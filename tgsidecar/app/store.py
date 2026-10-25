import asyncio
import os
import uuid
from dataclasses import dataclass, field
from pathlib import Path

from telethon import TelegramClient

API_ID = int(os.environ.get("TG_API_ID", "0"))
API_HASH = os.environ.get("TG_API_HASH", "")
SIDECAR_KEY = os.environ.get("SIDECAR_KEY", "")
SESSIONS_DIR = Path(os.environ.get("TG_SESSIONS_DIR", "/data/sessions"))
# optional e.g. socks5://127.0.0.1:10808 — required where Telegram is blocked
PROXY_URL = os.environ.get("TG_PROXY", "")

# account_id -> live TelegramClient
_clients: dict[str, TelegramClient] = {}
# pending_id -> in-flight login
_login_states: dict[str, "LoginState"] = {}
# serializes login/send-code per sidecar to avoid Telegram flood races
login_lock = asyncio.Lock()
# per-account locks for the BotFather create-bot conversation
_bf_locks: dict[str, asyncio.Lock] = {}


def bf_lock(account_id: str) -> asyncio.Lock:
    return _bf_locks.setdefault(account_id, asyncio.Lock())


@dataclass
class LoginState:
    phone: str
    phone_code_hash: str
    client: TelegramClient
    tmp_session: Path


def _proxy_args():
    if not PROXY_URL:
        return {}
    from python_socks import parse_proxy_url

    proxy_type, host, port, username, password = parse_proxy_url(PROXY_URL)
    proxy = (proxy_type, host, port)
    if username:
        proxy += (username, password)
    return {"proxy": proxy}


def make_client(session_path: Path) -> TelegramClient:
    """Build a Telethon client for a session path (without .session suffix)."""
    return TelegramClient(str(session_path), API_ID, API_HASH, **_proxy_args())


def session_file(account_id: str) -> Path:
    return SESSIONS_DIR / f"{account_id}.session"


def ensure_sessions_dir() -> None:
    SESSIONS_DIR.mkdir(parents=True, exist_ok=True)


def new_pending_session() -> tuple[str, Path]:
    """Create a pending login id + its session path INSIDE the sessions volume.

    The session must live on the same filesystem as the final location so
    finalize_login can rename() it (cross-device links are not allowed).
    """
    import shutil
    import time

    ensure_sessions_dir()
    # opportunistic cleanup of abandoned logins (> 1h)
    cutoff = time.time() - 3600
    for p in SESSIONS_DIR.glob("pending_*.session*"):
        try:
            if p.stat().st_mtime < cutoff:
                p.unlink(missing_ok=True)
        except OSError:
            pass
    pending_id = uuid.uuid4().hex
    return pending_id, SESSIONS_DIR / f"pending_{pending_id}"


def list_account_ids() -> list[str]:
    ensure_sessions_dir()
    return sorted(
        p.stem for p in SESSIONS_DIR.glob("*.session") if not p.stem.startswith("pending_")
    )


async def get_client(account_id: str) -> TelegramClient:
    """Return an authorized client, loading the session file lazily."""
    cached = _clients.get(account_id)
    if cached is not None and await cached.is_user_authorized():
        return cached
    path = session_file(account_id)
    if not path.exists():
        raise KeyError(account_id)
    client = make_client(Path(os.path.splitext(str(path))[0]))
    await client.connect()
    if not await client.is_user_authorized():
        await client.disconnect()
        raise KeyError(account_id)
    _clients[account_id] = client
    return client


async def delete_account(account_id: str) -> bool:
    client = _clients.pop(account_id, None)
    if client is not None:
        try:
            await client.disconnect()
        except Exception:
            pass
    path = session_file(account_id)
    existed = path.exists()
    path.unlink(missing_ok=True)
    Path(str(path) + "-journal").unlink(missing_ok=True)
    return existed


def new_login_state(pending_id: str, phone: str, phone_code_hash: str, client: TelegramClient) -> None:
    _login_states[pending_id] = LoginState(phone, phone_code_hash, client, Path(""))


def pop_login_state(pending_id: str) -> LoginState | None:
    return _login_states.pop(pending_id, None)


def restore_login_state(pending_id: str, state: LoginState) -> None:
    """Put a popped state back (e.g. after a PASSWORD_NEEDED retry)."""
    _login_states[pending_id] = state


def finalize_login(pending_id: str, state: LoginState) -> str:
    """Move the authorized tmp session to {account_id}.session and return account_id."""
    import shutil

    ensure_sessions_dir()
    account_id = uuid.uuid4().hex
    src = Path(state.client.session.filename)
    dst = session_file(account_id)
    try:
        src.rename(dst)
    except OSError:
        # fallback: copy + unlink (never lose an authorized session)
        shutil.copy2(src, dst)
        src.unlink(missing_ok=True)
    _clients[account_id] = state.client
    return account_id


def reset_for_tests() -> None:
    _clients.clear()
    _login_states.clear()
