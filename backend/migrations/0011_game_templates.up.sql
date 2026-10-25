-- Game templates: bombland + duel
ALTER TABLE bots DROP CONSTRAINT IF EXISTS bots_template_key_check;
ALTER TABLE bots ADD CONSTRAINT bots_template_key_check
  CHECK (template_key IN ('welcome_menu','dating','content','bombland','duel'));

-- BombLand: idle factory economy
CREATE TABLE IF NOT EXISTS bombland_players (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  bombs int NOT NULL DEFAULT 0,
  money int NOT NULL DEFAULT 0,
  auto_lvl int NOT NULL DEFAULT 0,
  last_auto timestamptz NOT NULL DEFAULT now(),
  invited_by bigint,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, user_id)
);

CREATE TABLE IF NOT EXISTS bombland_referrals (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  inviter_id bigint NOT NULL,
  invited_id bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, inviter_id, invited_id)
);

-- Duel: rock-paper-scissors with items/energy/opals
CREATE TABLE IF NOT EXISTS duel_players (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  score int NOT NULL DEFAULT 1000,
  energy int NOT NULL DEFAULT 10,
  last_energy_update timestamptz NOT NULL DEFAULT now(),
  black_opal int NOT NULL DEFAULT 0,
  candy int NOT NULL DEFAULT 0,
  chocolate int NOT NULL DEFAULT 0,
  soda int NOT NULL DEFAULT 0,
  total_spent_toman bigint NOT NULL DEFAULT 0,
  total_withdrawn_toman bigint NOT NULL DEFAULT 0,
  is_bot boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, user_id)
);

CREATE TABLE IF NOT EXISTS duel_withdrawals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  amount_mg numeric(12,4) NOT NULL,
  amount_toman bigint NOT NULL,
  kind text NOT NULL CHECK (kind IN ('cash','physical')),
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','paid')),
  note text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_duel_withdrawals_bot_status ON duel_withdrawals (bot_id, status);

CREATE TABLE IF NOT EXISTS duel_pools (
  bot_id uuid PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
  total_toman bigint NOT NULL DEFAULT 0,
  total_distributed bigint NOT NULL DEFAULT 0
);
