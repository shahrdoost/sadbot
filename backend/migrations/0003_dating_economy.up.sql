-- Dating economy: coins wallet + channel mission + payment package metadata.
ALTER TABLE dating_profiles ADD COLUMN coins int NOT NULL DEFAULT 0;
ALTER TABLE dating_profiles ADD COLUMN channel_bonus_at timestamptz;
ALTER TABLE dating_profiles ADD CONSTRAINT dating_profiles_coins_nonneg CHECK (coins >= 0);

ALTER TABLE payments ADD COLUMN coins int NOT NULL DEFAULT 0;
ALTER TABLE payments ADD COLUMN package_id text;

-- one subscription row per user per bot (activation upserts)
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_bot_user ON subscriptions (bot_id, user_id);
