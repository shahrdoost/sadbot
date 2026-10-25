DROP TABLE IF EXISTS content_posts;

ALTER TABLE bots DROP CONSTRAINT bots_template_key_check;
ALTER TABLE bots ADD CONSTRAINT bots_template_key_check
    CHECK (template_key IN ('welcome_menu','dating'));
