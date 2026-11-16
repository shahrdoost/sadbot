-- Support tickets become a conversation: the user can answer inside the same
-- ticket after an admin reply instead of the reply being a dead end.
-- The ticket row keeps the first message (as the panel/API always read it);
-- every later turn — user or admin — lives here, oldest first.
CREATE TABLE support_ticket_messages (
    id         bigserial PRIMARY KEY,
    ticket_id  bigint NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    sender     text NOT NULL CHECK (sender IN ('user','admin')),
    body       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_support_ticket_messages_ticket ON support_ticket_messages (ticket_id, created_at);

-- existing replies were one-shot; carry them into the thread so old tickets
-- render in the panel exactly as they did before
INSERT INTO support_ticket_messages (ticket_id, sender, body, created_at)
SELECT id, 'admin', admin_reply, COALESCE(replied_at, created_at)
FROM support_tickets
WHERE admin_reply IS NOT NULL AND admin_reply <> '';
