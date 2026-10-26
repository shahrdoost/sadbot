DROP INDEX IF EXISTS idx_dating_profiles_platform;
ALTER TABLE dating_profiles DROP COLUMN IF EXISTS platform;

ALTER TABLE bot_end_users DROP CONSTRAINT IF EXISTS bot_end_users_bot_id_platform_telegram_id_key;
ALTER TABLE bot_end_users DROP COLUMN IF EXISTS platform;
ALTER TABLE bot_end_users ADD UNIQUE (bot_id, telegram_id);

ALTER TABLE bots DROP COLUMN IF EXISTS kat_token_encrypted;
ALTER TABLE bots DROP COLUMN IF EXISTS kat_username;
