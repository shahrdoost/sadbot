-- Kat platform support: per-bot Kat token + platform-tagged user identity.
-- Users are separate per platform (a Kat user is a distinct account), but the
-- bot/infra is shared, so identity becomes (bot_id, platform, telegram_id) —
-- numeric ids can collide across platforms.

ALTER TABLE bots ADD COLUMN kat_token_encrypted bytea;
ALTER TABLE bots ADD COLUMN kat_username text;

ALTER TABLE bot_end_users ADD COLUMN platform text NOT NULL DEFAULT 'telegram';
ALTER TABLE bot_end_users DROP CONSTRAINT bot_end_users_bot_id_telegram_id_key;
ALTER TABLE bot_end_users ADD UNIQUE (bot_id, platform, telegram_id);

-- dating_profiles keeps a denormalized chat address (telegram_id); platform
-- rides along so sends can route to the right endpoint without a join.
ALTER TABLE dating_profiles ADD COLUMN platform text NOT NULL DEFAULT 'telegram';
CREATE INDEX idx_dating_profiles_platform ON dating_profiles (bot_id, platform, telegram_id);
