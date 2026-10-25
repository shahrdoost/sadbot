-- P3: daily streak, profile boost, paid profile edit cooldowns.

ALTER TABLE dating_profiles
    ADD COLUMN streak_count int NOT NULL DEFAULT 0,
    ADD COLUMN streak_best int NOT NULL DEFAULT 0,
    ADD COLUMN streak_last_date date,
    ADD COLUMN boosted_until timestamptz,
    ADD COLUMN name_edited_at timestamptz,
    ADD COLUMN profile_edited_at timestamptz;
