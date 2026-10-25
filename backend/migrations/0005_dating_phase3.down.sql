DROP TABLE IF EXISTS support_tickets;
DROP TABLE IF EXISTS confessions;
DROP INDEX IF EXISTS idx_dating_profiles_ghost;
ALTER TABLE dating_profiles
    DROP COLUMN IF EXISTS notify_dismissed_at,
    DROP COLUMN IF EXISTS notify_count_on,
    DROP COLUMN IF EXISTS notify_sent_on,
    DROP COLUMN IF EXISTS last_notified_at,
    DROP COLUMN IF EXISTS ai_free_used,
    DROP COLUMN IF EXISTS ghost_persona,
    DROP COLUMN IF EXISTS is_ghost;
