ALTER TABLE payments DROP COLUMN IF EXISTS package_id;
ALTER TABLE payments DROP COLUMN IF EXISTS coins;
DROP INDEX IF EXISTS idx_subscriptions_bot_user;
ALTER TABLE dating_profiles DROP CONSTRAINT IF EXISTS dating_profiles_coins_nonneg;
ALTER TABLE dating_profiles DROP COLUMN IF EXISTS channel_bonus_at;
ALTER TABLE dating_profiles DROP COLUMN IF EXISTS coins;
