-- Cross-platform retry queue that survives a redeploy.
--
-- The in-process queue loses parked messages when the backend restarts, and a
-- restart is precisely what a platform outage looks like. A relayed message
-- (text/photo/match notice) that fails gets a row here and is retried by the
-- runner until it lands or is ruled permanent (recipient blocked the bot).
CREATE TABLE outbound_retries (
    id         bigserial PRIMARY KEY,
    bot_id     uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    -- what to send: kind + payload hold everything needed without touching the
    -- original update (which is gone by retry time)
    kind       text NOT NULL CHECK (kind IN ('text','photo','match')),
    platform   text NOT NULL,
    chat_id    bigint NOT NULL,
    target     text,            -- text body, or the source file_id for photo
    source_platform text,       -- where a photo's file_id lives (photo kind)
    created_at timestamptz NOT NULL DEFAULT now(),
    attempts   int NOT NULL DEFAULT 0,
    next_at    timestamptz NOT NULL DEFAULT now(),
    last_error text
);
CREATE INDEX idx_outbound_retries_due ON outbound_retries (next_at) WHERE attempts < 5;
