import os
import tempfile
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi.testclient import TestClient

os.environ.setdefault("TG_API_ID", "1")
os.environ.setdefault("TG_API_HASH", "testhash")
os.environ.setdefault("SIDECAR_KEY", "testkey")
os.environ.setdefault("TG_SESSIONS_DIR", tempfile.mkdtemp(prefix="tgsessions_"))

from app import store  # noqa: E402
from app.main import app  # noqa: E402

KEY = {"X-Sidecar-Key": "testkey"}


@pytest.fixture()
def client():
    store.reset_for_tests()
    with TestClient(app) as c:
        yield c
    store.reset_for_tests()


def test_healthz_no_auth(client):
    r = client.get("/healthz")
    assert r.status_code == 200
    assert r.json() == {"status": "ok"}


def test_requires_key(client):
    assert client.get("/accounts").status_code == 401
    assert client.get("/accounts", headers={"X-Sidecar-Key": "wrong"}).status_code == 401


def make_fake_client():
    c = MagicMock()
    c.connect = AsyncMock()
    c.disconnect = AsyncMock()
    c.is_user_authorized = AsyncMock(return_value=True)
    c.send_code_request = AsyncMock(return_value=MagicMock(phone_code_hash="hash123"))
    c.sign_in = AsyncMock()
    c.get_me = AsyncMock(return_value=MagicMock(
        id=42, first_name="Dastan", last_name="", username="dastan_group_com", phone="98903",
    ))
    c.session = MagicMock()
    c.session.filename = str(Path(tempfile.gettempdir()) / "tmpsession.session")
    return c


def test_login_flow_happy_path(client):
    fake = make_fake_client()
    with patch.object(store, "make_client", return_value=fake):
        r = client.post("/login/start", json={"phone": "+98903"}, headers=KEY)
        assert r.status_code == 200
        pending = r.json()["pending_id"]

        # create the tmp session file so finalize can rename it
        Path(fake.session.filename).touch()

        r = client.post(
            "/login/complete",
            json={"pending_id": pending, "phone": "+98903", "code": "12345"},
            headers=KEY,
        )
        assert r.status_code == 200
        body = r.json()
        assert body["account_id"]
        assert body["user"]["id"] == "42"
        assert body["user"]["first_name"] == "Dastan"
        fake.sign_in.assert_awaited()


def test_login_complete_2fa_branch(client):
    from telethon import errors

    fake = make_fake_client()
    fake.sign_in = AsyncMock(side_effect=errors.SessionPasswordNeededError(request=None))
    with patch.object(store, "make_client", return_value=fake):
        r = client.post("/login/start", json={"phone": "+98903"}, headers=KEY)
        pending = r.json()["pending_id"]

        r = client.post(
            "/login/complete",
            json={"pending_id": pending, "phone": "+98903", "code": "12345"},
            headers=KEY,
        )
        assert r.status_code == 401
        assert r.json()["detail"] == "PASSWORD_NEEDED"

        # now retry with password — succeeds
        fake2 = fake
        fake2.sign_in = AsyncMock(return_value=None)
        Path(fake.session.filename).touch()
        r = client.post(
            "/login/complete",
            json={"pending_id": pending, "phone": "+98903", "code": "12345", "password": "secret"},
            headers=KEY,
        )
        assert r.status_code == 200


def test_login_complete_expired_pending(client):
    r = client.post(
        "/login/complete",
        json={"pending_id": "nope", "phone": "+98903", "code": "1"},
        headers=KEY,
    )
    assert r.status_code == 404


def seed_account(account_id: str, fake):
    (store.SESSIONS_DIR / f"{account_id}.session").touch()
    store._clients[account_id] = fake


def test_list_chats_serializes_ids_as_strings(client):
    fake = make_fake_client()
    dialog = MagicMock()
    dialog.entity = MagicMock()
    dialog.entity.bot = False
    dialog.entity.broadcast = True
    dialog.entity.title = "کانال"
    dialog.name = "کانال تست"
    dialog.id = -1001234567890
    dialog.unread_count = 5
    dialog.pinned = True
    dialog.message = MagicMock(message="سلام", date=None)
    fake.get_dialogs = AsyncMock(return_value=[dialog])
    seed_account("acc1", fake)

    r = client.get("/accounts/acc1/chats", headers=KEY)
    assert r.status_code == 200
    chats = r.json()["chats"]
    assert chats[0]["chat_id"] == "-1001234567890"  # string, not number
    assert chats[0]["type"] == "channel"
    assert chats[0]["unread"] == 5


def test_list_messages(client):
    fake = make_fake_client()
    msg = MagicMock()
    msg.id = 10
    msg.date = None
    msg.sender = MagicMock(id=7, first_name="Ali", last_name="", username="ali")
    msg.message = "پیام تست"
    msg.photo = None
    msg.sticker = None
    msg.voice = None
    msg.video = None
    msg.audio = None
    msg.document = None
    msg.media = None
    msg.out = True
    msg.reply_to = MagicMock(reply_to_msg_id=3)
    fake.get_messages = AsyncMock(return_value=[msg])
    seed_account("acc1", fake)

    r = client.get("/accounts/acc1/chats/-1001/messages", headers=KEY)
    assert r.status_code == 200
    m = r.json()["messages"][0]
    assert m["id"] == 10
    assert m["sender"]["is_out"] is True
    assert m["media"]["kind"] == "none"
    assert m["reply_to_id"] == 3


def test_list_messages_oldest_first(client):
    """Telethon answers newest-first; the API must flip it for the panel."""
    fake = make_fake_client()

    def mk(mid):
        m = MagicMock()
        m.id = mid
        m.date = None
        m.sender = MagicMock(id=1, first_name="", last_name="", username="")
        m.message = f"m{mid}"
        m.photo = m.sticker = m.voice = m.video = m.audio = m.document = m.media = None
        m.out = False
        m.reply_to = None
        return m

    fake.get_messages = AsyncMock(return_value=[mk(12), mk(11), mk(10)])  # newest-first
    seed_account("accorder", fake)

    r = client.get("/accounts/accorder/chats/-1001/messages", headers=KEY)
    assert r.status_code == 200
    body = r.json()
    assert [m["id"] for m in body["messages"]] == [10, 11, 12]
    assert body["oldest_id"] == 10


def test_unknown_account_404(client):
    assert client.get("/accounts/missing/chats", headers=KEY).status_code == 404


def test_list_chats_sorted_pinned_then_newest(client):
    from datetime import datetime, timezone

    fake = make_fake_client()
    dialogs = []
    for i, (title, pinned, hour) in enumerate([
        ("کهنه", False, 1),
        ("جدید", False, 12),
        ("پین", True, 0),
    ]):
        d = MagicMock()
        d.entity = MagicMock()
        d.entity.bot = False
        d.entity.broadcast = True
        d.name = title
        d.id = -1000 - i
        d.unread_count = 0
        d.pinned = pinned
        d.message = MagicMock(message="x", date=datetime(2026, 10, 14, hour, tzinfo=timezone.utc))
        dialogs.append(d)
    fake.get_dialogs = AsyncMock(return_value=dialogs)
    seed_account("accsort", fake)

    r = client.get("/accounts/accsort/chats", headers=KEY)
    titles = [c["title"] for c in r.json()["chats"]]
    assert titles == ["پین", "جدید", "کهنه"]


def test_chat_photo_no_photo_404(client):
    fake = make_fake_client()
    entity = MagicMock()
    fake.get_entity = AsyncMock(return_value=entity)
    fake.download_profile_photo = AsyncMock(return_value=None)
    seed_account("accp", fake)

    r = client.get("/accounts/accp/chats/-1001/photo", headers=KEY)
    assert r.status_code == 404


def test_chat_photo_streams_jpeg(client):
    fake = make_fake_client()
    entity = MagicMock()
    fake.get_entity = AsyncMock(return_value=entity)
    photo_path = Path(tempfile.gettempdir()) / f"tgp_{os.urandom(6).hex()}"
    photo_path.write_bytes(b"\xff\xd8\xff\xd9")
    fake.download_profile_photo = AsyncMock(return_value=str(photo_path))
    seed_account("accp2", fake)

    r = client.get("/accounts/accp2/chats/-1001/photo", headers=KEY)
    assert r.status_code == 200
    assert r.headers["content-type"] == "image/jpeg"
    assert r.content == b"\xff\xd8\xff\xd9"


def test_username_candidates_rules():
    from app.botfather import username_candidates

    cands = username_candidates("بات تست صدبات")
    assert cands, "candidates must not be empty"
    for c in cands:
        assert 5 <= len(c) <= 32, c
        assert c.endswith("bot") or c.endswith("_bot"), c
        assert c.replace("_", "").isalnum(), c
    # hint first
    withhint = username_candidates("بات", hint="MyCoolIdea")
    assert withhint[0].startswith("MyCoolIdea")


def test_transliterate_persian():
    from app.botfather import transliterate

    t = transliterate("بات تستی")
    assert t == "bat tsti", t


def test_botfather_endpoint_success(client):
    from app import botfather as bfm

    fake = make_fake_client()
    seed_account("accbf", fake)

    async def fake_create(client, name, hint):
        return {"token": "123456789:AAABCDEFGHIJKLMNOPQRSTUVWXYZabcd", "username": "testbot1bot"}

    with client as c:
        import app.botfather

        orig = app.botfather.create_bot
        app.botfather.create_bot = fake_create
        try:
            r = c.post("/accounts/accbf/botfather/create-bot", json={"name": "بات تست"}, headers=KEY)
            assert r.status_code == 200, r.text
            body = r.json()
            assert body["username"] == "testbot1bot"
            assert body["token"].startswith("123456789:")
        finally:
            app.botfather.create_bot = orig


def test_botfather_endpoint_no_username(client):
    from app import botfather as bfm

    fake = make_fake_client()
    seed_account("accbf2", fake)

    async def fake_create(client, name, hint):
        raise bfm.NoUsername()

    import app.botfather

    orig = app.botfather.create_bot
    app.botfather.create_bot = fake_create
    try:
        r = client.post("/accounts/accbf2/botfather/create-bot", json={"name": "x"}, headers=KEY)
        assert r.status_code == 409
        assert r.json()["detail"] == "NO_FREE_USERNAME"
    finally:
        app.botfather.create_bot = orig


def _sent_message_mock(msg_id=42, text="سلام دنیا"):
    msg = MagicMock()
    msg.id = msg_id
    msg.date = None
    msg.sender = MagicMock(id=42, first_name="Dastan", last_name="", username="dastan")
    msg.message = text
    msg.photo = None
    msg.sticker = None
    msg.voice = None
    msg.video = None
    msg.audio = None
    msg.document = None
    msg.media = None
    msg.out = True
    msg.reply_to = None
    return msg


def test_send_message_success(client):
    fake = make_fake_client()
    fake.send_message = AsyncMock(return_value=_sent_message_mock())
    seed_account("accs", fake)

    r = client.post("/accounts/accs/chats/-1001/messages", json={"text": "سلام دنیا"}, headers=KEY)
    assert r.status_code == 200, r.text
    m = r.json()["message"]
    assert m["id"] == 42
    assert m["text"] == "سلام دنیا"
    assert m["sender"]["is_out"] is True
    fake.send_message.assert_awaited_once_with(-1001, "سلام دنیا")


def test_send_message_empty_text(client):
    fake = make_fake_client()
    fake.send_message = AsyncMock()
    seed_account("accs2", fake)

    r = client.post("/accounts/accs2/chats/-1001/messages", json={"text": "   "}, headers=KEY)
    assert r.status_code == 400
    assert r.json()["detail"] == "EMPTY_TEXT"
    fake.send_message.assert_not_awaited()


def test_send_message_write_forbidden(client):
    from telethon import errors

    fake = make_fake_client()
    fake.send_message = AsyncMock(side_effect=errors.ChatWriteForbiddenError(request=None))
    seed_account("accs3", fake)

    r = client.post("/accounts/accs3/chats/-1001/messages", json={"text": "hi"}, headers=KEY)
    assert r.status_code == 400
    assert r.json()["detail"] == "WRITE_FORBIDDEN"


def test_send_message_flood_wait(client):
    from telethon import errors

    fake = make_fake_client()
    fake.send_message = AsyncMock(side_effect=errors.FloodWaitError(request=None, capture=17))
    seed_account("accs4", fake)

    r = client.post("/accounts/accs4/chats/-1001/messages", json={"text": "hi"}, headers=KEY)
    assert r.status_code == 429
    assert r.json()["detail"] == {"error": "FLOOD_WAIT", "seconds": 17}


def test_send_message_unknown_account_404(client):
    r = client.post("/accounts/missing/chats/-1001/messages", json={"text": "hi"}, headers=KEY)
    assert r.status_code == 404



def test_list_folders_returns_dialog_filters(client):
    from datetime import datetime, timezone

    from telethon import types

    fake = make_fake_client()
    channel = types.Channel(
        id=1842020090,
        title="اخبار کریپتو",
        photo=types.ChatPhotoEmpty(),
        date=datetime.now(timezone.utc),
        broadcast=True,
    )
    channel.username = "cryptonews"
    flt = types.DialogFilter(
        id=1,
        title=types.TextWithEntities(text="Crypto", entities=[]),
        include_peers=[types.InputPeerChannel(channel.id, 0)],
        exclude_peers=[],
        pinned_peers=[],
    )
    result = MagicMock()
    result.filters = [flt, types.DialogFilterDefault()]

    class _Awaitable:  # await client(request) must resolve to result
        def __await__(self):
            async def _():
                return result

            return _().__await__()

    fake.side_effect = lambda *_a, **_k: _Awaitable()
    fake.get_entity = AsyncMock(return_value=channel)
    seed_account("accf1", fake)

    r = client.get("/accounts/accf1/folders", headers=KEY)
    assert r.status_code == 200
    folders = r.json()["folders"]
    assert len(folders) == 1  # DialogFilterDefault skipped
    assert folders[0]["name"] == "Crypto"
    peer = folders[0]["peers"][0]
    assert peer["type"] == "channel"
    assert peer["username"] == "cryptonews"
    assert peer["chat_id"] == "-1001842020090"  # marked id, same shape as /chats


def test_list_folders_unknown_account_404(client):
    r = client.get("/accounts/missing/folders", headers=KEY)
    assert r.status_code == 404


def test_send_photo_happy_path(client):
    import base64

    fake = make_fake_client()
    fake.send_file = AsyncMock(return_value=_sent_message_mock(msg_id=77, text=""))
    seed_account("accsp", fake)

    png = b"\x89PNG\r\n\x1a\nfake"
    r = client.post(
        "/accounts/accsp/chats/-1001/photo",
        json={"image_b64": base64.b64encode(png).decode(), "filename": "card.png", "caption": "کپشن"},
        headers=KEY,
    )
    assert r.status_code == 200, r.text
    m = r.json()["message"]
    assert m["id"] == 77
    fake.send_file.assert_awaited_once()  # BytesIO + caption passed through


def test_send_photo_rejects_bad_b64(client):
    seed_account("accsp2", make_fake_client())
    r = client.post(
        "/accounts/accsp2/chats/-1001/photo",
        json={"image_b64": "!!!not-base64!!!"},
        headers=KEY,
    )
    assert r.status_code == 400
    assert r.json()["detail"] == "INVALID_B64"
