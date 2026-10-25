-- Content template: bots that collect news from a Telegram folder (via the
-- MTProto sidecar account), clean it with an LLM, render branded PNG cards
-- and publish to their destination channel.

-- Relax the template whitelist.
ALTER TABLE bots DROP CONSTRAINT bots_template_key_check;
ALTER TABLE bots ADD CONSTRAINT bots_template_key_check
    CHECK (template_key IN ('welcome_menu','dating','content'));

-- Collected/published posts — durable dedup ledger (replaces sadcontent's
-- collected.jsonl) plus a light archive of what was published.
CREATE TABLE content_posts (
    id            bigserial PRIMARY KEY,
    bot_id        uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    source_chat_id bigint NOT NULL,
    message_id    bigint NOT NULL,
    source_title  text,
    source_link   text,
    raw_text      text,
    cleaned_text  text,
    status        text NOT NULL DEFAULT 'published'
        CHECK (status IN ('published','no_content','failed','render_failed')),
    error         text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (bot_id, source_chat_id, message_id)
);

CREATE INDEX content_posts_bot_created_idx ON content_posts (bot_id, created_at DESC);
