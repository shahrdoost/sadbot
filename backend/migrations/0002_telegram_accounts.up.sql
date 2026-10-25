CREATE TABLE IF NOT EXISTS telegram_accounts (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id             uuid NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
    label                text NOT NULL DEFAULT '',
    tg_user_id           bigint NOT NULL,
    phone                text NOT NULL,
    sidecar_account_id   text NOT NULL UNIQUE,
    created_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_telegram_accounts_admin ON telegram_accounts(admin_id);
