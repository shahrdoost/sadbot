-- P6-D: broadcast with an optional photo + optional footer text.
-- photo: Telegram file_id (from a user's upload) or an https URL.
-- footer: appended under the caption on photo broadcasts.
ALTER TABLE broadcast_runs
    ADD COLUMN photo text,
    ADD COLUMN footer text;