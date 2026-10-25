#!/usr/bin/env python3
"""kpkchat MongoDB JSON → sadbot Postgres migration.

bot_end_users.id / dating_profiles.user_id are bigserial, so we use a
temporary mapping table (tg → bigid) populated by INSERT ... RETURNING.
"""
import json, sys
from datetime import datetime, timezone

BOT_ID = "a6b194d9-a735-499a-b1f3-bd951cc99468"

def esc(s):
    if s is None:
        return "NULL"
    if isinstance(s, bool):
        return "TRUE" if s else "FALSE"
    if isinstance(s, (int, float)):
        return str(s)
    s = str(s).replace("'", "''")
    return f"'{s}'"

def ts(s):
    if not s:
        return "NULL"
    return f"'{s}'"

def load(name):
    with open(f"{name}.json") as f:
        return json.load(f)

def main():
    users = load("core_users")
    payments = load("core_payments")
    subs = load("core_subscriptions")
    confessions = load("confessions")
    tickets = load("support_tickets")
    discounts = load("discount_codes")

    out = []
    out.append(f"-- kpkchat → sadbot migration  ({datetime.now(timezone.utc).isoformat()})")
    out.append("BEGIN;")
    out.append("")
    out.append("CREATE TEMP TABLE _tgmap (tg bigint PRIMARY KEY, uid bigint NOT NULL);")
    out.append("")
    out.append("-- bot_end_users (bulk)")
    for u in users:
        tg = u["telegramId"]
        name = u.get("name") or ""
        username = u.get("username")
        first_seen = u.get("createdAt")
        last_seen = u.get("updatedAt")
        out.append(
            f"INSERT INTO bot_end_users (bot_id, telegram_id, username, first_name, state, is_blocked, first_seen_at, last_seen_at)"
            f" VALUES ({esc(BOT_ID)}, {tg}, {esc(username)}, {esc(name)}, '{{\"scene\":\"home\",\"data\":{{}}}}'::jsonb, false, {ts(first_seen)}, {ts(last_seen)})"
            f" ON CONFLICT (bot_id, telegram_id) DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at;"
        )

    out.append("")
    out.append("-- build tg → uid map")
    out.append(
        f"INSERT INTO _tgmap (tg, uid)"
        f" SELECT telegram_id, id FROM bot_end_users WHERE bot_id = {esc(BOT_ID)};"
    )
    out.append("")

    # ---- 2. dating_profiles ----
    out.append("-- dating_profiles")
    for u in users:
        tg = u["telegramId"]
        name = u.get("name") or ""
        gender = u.get("gender") or None
        if gender == "":
            gender = None
        p = u.get("profile", {})
        age = p.get("age")
        province = p.get("province")
        city = p.get("city")
        photo = p.get("photoFileId")
        interests = u.get("interests") or []
        rating = u.get("rating", 0)
        total_ratings = u.get("totalRatings", 0)
        rating_sum = u.get("ratingSum", 0)
        coins = u.get("coins", 0)
        is_ghost = tg < 0
        ghost_persona = None
        if is_ghost:
            idx = abs(tg) - 10001
            personas = ["mina","sara","negin","parisa","reza","kian","bagher","navid","omid","delara"]
            ghost_persona = personas[idx % len(personas)]
        interests_sql = "ARRAY[" + ",".join(esc(i) for i in interests) + "]::text[]" if interests else "'{}'::text[]"

        out.append(
            f"INSERT INTO dating_profiles (bot_id, user_id, telegram_id, name, gender, age, province, city, interests, status, rating, total_ratings, rating_sum, coins, photo_file_id, is_ghost, ghost_persona)"
            f" SELECT {esc(BOT_ID)}, m.uid, {tg}, {esc(name)}, {esc(gender)}, {esc(age)}, {esc(province)}, {esc(city)}, {interests_sql}, 'joined', {rating}, {total_ratings}, {rating_sum}, {coins}, {esc(photo)}, {is_ghost}, {esc(ghost_persona)}"
            f" FROM _tgmap m WHERE m.tg = {tg}"
            f" ON CONFLICT (bot_id, user_id) DO NOTHING;"
        )

    out.append("")

    # ---- 3. payments ----
    out.append("-- payments")
    status_map = {"verified": "verified", "expired": "failed", "failed": "failed", "pending": "failed"}
    for p in payments:
        tg = p.get("userId")
        amount = p.get("amount", 0)
        coins = p.get("coins", 0)
        pkg = p.get("packageId")
        status = status_map.get(p.get("status"), "failed")
        authority = p.get("authority")
        created = p.get("createdAt")
        kind = "coin_package" if coins > 0 else "subscription"
        note = f"migrated; authority={authority}; pkg={pkg}"
        out.append(
            f"INSERT INTO payments (bot_id, user_id, amount_rials, kind, gateway, status, coins, package_id, note, created_at)"
            f" SELECT {esc(BOT_ID)}, m.uid, {amount}, {esc(kind)}, 'catroom.ir', {esc(status)}, {coins}, {esc(pkg)}, {esc(note)}, {ts(created)}"
            f" FROM _tgmap m WHERE m.tg = {tg};"
        )

    out.append("")

    # ---- 4. subscriptions ----
    out.append("-- subscriptions")
    for s in subs:
        tg = s.get("userId")
        status = s.get("status", "active")
        if status not in ("active", "cancelled", "expired"):
            status = "expired"
        plan = s.get("planId", "premium_monthly")
        started = s.get("startedAt")
        expires = s.get("expiresAt")
        out.append(
            f"INSERT INTO subscriptions (bot_id, user_id, plan, status, started_at, expires_at)"
            f" SELECT {esc(BOT_ID)}, m.uid, {esc(plan)}, {esc(status)}, {ts(started)}, {ts(expires)}"
            f" FROM _tgmap m WHERE m.tg = {tg};"
        )

    out.append("")

    # ---- 5. confessions ----
    out.append("-- confessions")
    for c in confessions:
        tg = c.get("userId")
        text = c.get("text", "")
        created = c.get("createdAt")
        out.append(
            f"INSERT INTO confessions (bot_id, user_id, text, status, created_at)"
            f" SELECT {esc(BOT_ID)}, m.uid, {esc(text)}, 'pending', {ts(created)}"
            f" FROM _tgmap m WHERE m.tg = {tg};"
        )

    out.append("")

    # ---- 6. support_tickets ----
    out.append("-- support_tickets")
    for t in tickets:
        tg = t.get("userId")
        text = t.get("text", "")
        status = t.get("status", "open")
        if status not in ("open", "answered", "closed"):
            status = "open"
        created = t.get("createdAt")
        out.append(
            f"INSERT INTO support_tickets (bot_id, user_id, message, status, created_at)"
            f" SELECT {esc(BOT_ID)}, m.uid, {esc(text)}, {esc(status)}, {ts(created)}"
            f" FROM _tgmap m WHERE m.tg = {tg};"
        )

    out.append("")

    # ---- 7. discount_codes ----
    out.append("-- discount_codes")
    for d in discounts:
        code = d.get("code")
        if not code:
            continue
        dtype = d.get("type", "")
        value = d.get("value", 0)
        if "percentage" in dtype:
            kind = "percent"
        elif "flat" in dtype:
            kind = "flat_toman"
        elif "coin" in dtype:
            kind = "coin_bonus"
        else:
            kind = "percent"
        if value <= 0:
            value = 10
        max_uses = d.get("maxUses", 0) or 0
        used_count = d.get("usedCount", 0) or 0
        expires = d.get("expiresAt")
        created = d.get("createdAt")
        out.append(
            f"INSERT INTO discount_codes (bot_id, code, kind, value, max_uses, used_count, expires_at, created_at)"
            f" VALUES ({esc(BOT_ID)}, {esc(code)}, {esc(kind)}, {value}, {max_uses}, {used_count}, {ts(expires)}, {ts(created)})"
            f" ON CONFLICT (bot_id, code) DO NOTHING;"
        )

    out.append("")
    out.append("DROP TABLE _tgmap;")
    out.append("COMMIT;")

    print("\n".join(out))

if __name__ == "__main__":
    main()
