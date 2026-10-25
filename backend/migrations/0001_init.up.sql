CREATE EXTENSION IF NOT EXISTS citext;

-- ============ platform admins ============

CREATE TABLE admins (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext NOT NULL UNIQUE,
    password_hash text NOT NULL,
    display_name  text NOT NULL,
    role          text NOT NULL DEFAULT 'admin' CHECK (role IN ('superadmin','admin')),
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- ============ bots ============

CREATE TABLE bots (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name               text NOT NULL,
    telegram_username  text,
    token_encrypted    bytea NOT NULL,
    template_key       text NOT NULL DEFAULT 'welcome_menu' CHECK (template_key IN ('welcome_menu','dating')),
    status             text NOT NULL DEFAULT 'stopped' CHECK (status IN ('stopped','starting','running','error')),
    last_error         text,
    token_validated_at timestamptz,
    created_by         uuid NOT NULL REFERENCES admins(id),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE admin_bot_access (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id    uuid NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
    bot_id      uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    permissions text[] NOT NULL DEFAULT '{}',
    granted_by  uuid REFERENCES admins(id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (admin_id, bot_id)
);

CREATE TABLE bot_configs (
    bot_id        uuid PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
    texts         jsonb NOT NULL DEFAULT '{}',
    menus         jsonb NOT NULL DEFAULT '{}',
    feature_flags jsonb NOT NULL DEFAULT '{}',
    prices        jsonb NOT NULL DEFAULT '{}',
    version       int NOT NULL DEFAULT 1,
    updated_by    uuid REFERENCES admins(id),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- ============ bot end users & events ============

CREATE TABLE bot_end_users (
    id           bigserial PRIMARY KEY,
    bot_id       uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    telegram_id  bigint NOT NULL,
    username     text,
    first_name   text,
    last_name    text,
    state        jsonb NOT NULL DEFAULT '{"scene":"home","data":{}}',
    is_blocked   boolean NOT NULL DEFAULT false,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bot_id, telegram_id)
);
CREATE INDEX idx_bot_end_users_last_seen ON bot_end_users (bot_id, last_seen_at DESC);

CREATE TABLE bot_user_events (
    id         bigserial PRIMARY KEY,
    bot_id     uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES bot_end_users(id) ON DELETE CASCADE,
    kind       text NOT NULL CHECK (kind IN ('inbound_message','outbound_message','callback','command','user_start','state_change')),
    text       text,
    meta       jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_bot_user_events_recent ON bot_user_events (bot_id, created_at DESC);
CREATE INDEX idx_bot_user_events_kind ON bot_user_events (bot_id, kind, created_at DESC);

-- ============ finance ============

CREATE TABLE payments (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id        uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id       bigint REFERENCES bot_end_users(id) ON DELETE SET NULL,
    amount_rials  bigint NOT NULL CHECK (amount_rials >= 0),
    kind          text NOT NULL DEFAULT 'manual' CHECK (kind IN ('coin_package','subscription','manual')),
    gateway       text NOT NULL DEFAULT 'manual',
    external_ref  text,
    status        text NOT NULL DEFAULT 'verified' CHECK (status IN ('pending','verified','failed','refunded')),
    note          text,
    recorded_by   uuid REFERENCES admins(id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    verified_at   timestamptz
);
CREATE INDEX idx_payments_bot_created ON payments (bot_id, created_at DESC);
CREATE INDEX idx_payments_status ON payments (status);

CREATE TABLE subscriptions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id       uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id      bigint NOT NULL REFERENCES bot_end_users(id) ON DELETE CASCADE,
    plan         text NOT NULL DEFAULT 'premium_monthly',
    status       text NOT NULL DEFAULT 'active' CHECK (status IN ('active','cancelled','expired')),
    amount_rials bigint NOT NULL DEFAULT 0,
    payment_id   uuid REFERENCES payments(id),
    started_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL
);
CREATE INDEX idx_subscriptions_bot_status ON subscriptions (bot_id, status, expires_at);

-- ============ audit ============

CREATE TABLE audit_logs (
    id              bigserial PRIMARY KEY,
    actor_admin_id  uuid REFERENCES admins(id),
    action          text NOT NULL,
    target_type     text NOT NULL,
    target_id       text NOT NULL,
    bot_id          uuid REFERENCES bots(id),
    details         jsonb NOT NULL DEFAULT '{}',
    ip              inet,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_recent ON audit_logs (created_at DESC);

-- ============ dating template ============

CREATE TABLE dating_profiles (
    bot_id          uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id         bigint NOT NULL REFERENCES bot_end_users(id) ON DELETE CASCADE,
    telegram_id     bigint NOT NULL,
    name            text,
    gender          text CHECK (gender IN ('male','female')),
    age             int,
    province        text,
    city            text,
    interests       text[] NOT NULL DEFAULT '{}',
    status          text NOT NULL DEFAULT 'joined' CHECK (status IN ('joined','searching','chatting')),
    partner_user_id bigint,
    rating          numeric(3,2) NOT NULL DEFAULT 0,
    total_ratings   int NOT NULL DEFAULT 0,
    rating_sum      int NOT NULL DEFAULT 0,
    last_seen_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, user_id)
);
CREATE INDEX idx_dating_profiles_searching ON dating_profiles (bot_id, status) WHERE status = 'searching';
CREATE INDEX idx_dating_profiles_chatting ON dating_profiles (bot_id, status) WHERE status = 'chatting';
CREATE INDEX idx_dating_profiles_gender ON dating_profiles (bot_id, gender);

CREATE TABLE dating_search_requests (
    id         bigserial PRIMARY KEY,
    bot_id     uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES bot_end_users(id) ON DELETE CASCADE,
    filters    jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bot_id, user_id)
);
CREATE INDEX idx_dating_search_requests_bot ON dating_search_requests (bot_id, created_at);

CREATE TABLE dating_msg_refs (
    bot_id            uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    viewer_user_id    bigint NOT NULL,
    viewer_msg_id     bigint NOT NULL,
    partner_user_id   bigint NOT NULL,
    partner_msg_id    bigint NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, viewer_user_id, viewer_msg_id)
);

CREATE TABLE dating_pending_rates (
    bot_id           uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id          bigint NOT NULL REFERENCES bot_end_users(id) ON DELETE CASCADE,
    partner_user_id  bigint NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, user_id)
);

CREATE TABLE dating_block_pairs (
    bot_id      uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    blocker_id  bigint NOT NULL,
    blocked_id  bigint NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    PRIMARY KEY (bot_id, blocker_id, blocked_id)
);
CREATE INDEX idx_dating_block_pairs_blocked ON dating_block_pairs (bot_id, blocked_id, expires_at);
