-- P5: panel-driven admin operations on bot end users.
-- A blocked user keeps their profile but cannot search, browse, or chat
-- until the panel unblocks them.

ALTER TABLE dating_profiles
    ADD COLUMN blocked_at timestamptz,
    ADD COLUMN blocked_reason text;

CREATE INDEX idx_dating_profiles_blocked ON dating_profiles (bot_id) WHERE blocked_at IS NOT NULL;
