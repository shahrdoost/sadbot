-- Phase 3: AI companion counters, ghost cold-start, notifications, moderation.
ALTER TABLE dating_profiles
    ADD COLUMN is_ghost boolean NOT NULL DEFAULT false,
    ADD COLUMN ghost_persona text,
    ADD COLUMN ai_free_used int NOT NULL DEFAULT 0,
    ADD COLUMN last_notified_at timestamptz,
    ADD COLUMN notify_sent_on date,
    ADD COLUMN notify_count_on int NOT NULL DEFAULT 0,
    ADD COLUMN notify_dismissed_at timestamptz;

CREATE INDEX idx_dating_profiles_ghost ON dating_profiles (bot_id) WHERE is_ghost;

CREATE TABLE confessions (
    id              bigserial PRIMARY KEY,
    bot_id          uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id         bigint NOT NULL REFERENCES bot_end_users(id) ON DELETE CASCADE,
    text            text NOT NULL,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    decided_at      timestamptz,
    decided_by      uuid,
    published_msg_id bigint
);
CREATE INDEX idx_confessions_bot_status ON confessions (bot_id, status, created_at DESC);

CREATE TABLE support_tickets (
    id          bigserial PRIMARY KEY,
    bot_id      uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id     bigint NOT NULL REFERENCES bot_end_users(id) ON DELETE CASCADE,
    message     text NOT NULL,
    admin_reply text,
    status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open','answered','closed')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    replied_at  timestamptz
);
CREATE INDEX idx_support_tickets_bot_status ON support_tickets (bot_id, status, created_at DESC);
