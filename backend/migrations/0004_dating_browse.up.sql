-- Phase 2: avatars + browse (Tinder-style) likes.
ALTER TABLE dating_profiles ADD COLUMN photo_file_id text;
ALTER TABLE dating_profiles ADD COLUMN avatar_bonus_at timestamptz;

CREATE TABLE dating_likes (
    bot_id       uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    from_user_id bigint NOT NULL REFERENCES bot_end_users(id) ON DELETE CASCADE,
    to_user_id   bigint NOT NULL REFERENCES bot_end_users(id) ON DELETE CASCADE,
    kind         text NOT NULL DEFAULT 'like' CHECK (kind IN ('like','super','pass')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, from_user_id, to_user_id)
);

CREATE INDEX idx_dating_likes_to ON dating_likes (bot_id, to_user_id);
CREATE INDEX idx_dating_likes_from_day ON dating_likes (bot_id, from_user_id, created_at);
