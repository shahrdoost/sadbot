-- P1: discount codes + broadcast runs (per-bot admin tools).

CREATE TABLE discount_codes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id      uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    code        text NOT NULL,
    kind        text NOT NULL DEFAULT 'percent' CHECK (kind IN ('percent','flat_toman','coin_bonus')),
    value       int NOT NULL CHECK (value > 0),
    max_uses    int NOT NULL DEFAULT 0, -- 0 = unlimited
    used_count  int NOT NULL DEFAULT 0,
    expires_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bot_id, code)
);

CREATE TABLE broadcast_runs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id      uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    admin_id    uuid,
    text        text NOT NULL,
    audience    int NOT NULL DEFAULT 0,
    sent        int NOT NULL DEFAULT 0,
    failed      int NOT NULL DEFAULT 0,
    status      text NOT NULL DEFAULT 'running' CHECK (status IN ('running','done')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE INDEX idx_broadcast_runs_bot ON broadcast_runs (bot_id, created_at DESC);
