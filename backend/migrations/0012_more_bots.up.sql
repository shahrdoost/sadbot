-- More bot templates: favaani, shlinks, whoseeme, faal, anonychat, tapearn
ALTER TABLE bots DROP CONSTRAINT IF EXISTS bots_template_key_check;
ALTER TABLE bots ADD CONSTRAINT bots_template_key_check
  CHECK (template_key IN ('welcome_menu','dating','content','bombland','duel','favaani','shlinks','whoseeme','faal','anonychat','tapearn'));

-- Favaani (money mindset / abundance game)
CREATE TABLE IF NOT EXISTS favaani_users (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  username text,
  day int NOT NULL DEFAULT 1,
  score int NOT NULL DEFAULT 0,
  streak int NOT NULL DEFAULT 0,
  total_spent bigint NOT NULL DEFAULT 0,
  referral_code text,
  referred_by bigint,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, user_id)
);

CREATE TABLE IF NOT EXISTS favaani_challenges (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  day int NOT NULL,
  amount bigint NOT NULL,
  spend_log text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_favaani_challenges_user ON favaani_challenges (bot_id, user_id, day);

CREATE TABLE IF NOT EXISTS favaani_referrals (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  inviter_id bigint NOT NULL,
  invited_id bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, inviter_id, invited_id)
);

-- Shlinks (link shortener with earnings)
CREATE TABLE IF NOT EXISTS shlinks_users (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  username text,
  balance numeric(14,2) NOT NULL DEFAULT 0,
  total_earned numeric(14,2) NOT NULL DEFAULT 0,
  total_withdrawn numeric(14,2) NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, user_id)
);

CREATE TABLE IF NOT EXISTS shlinks_links (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  short_code text NOT NULL,
  target_url text NOT NULL,
  total_clicks int NOT NULL DEFAULT 0,
  valid_clicks int NOT NULL DEFAULT 0,
  total_earned numeric(14,2) NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (bot_id, short_code)
);

CREATE TABLE IF NOT EXISTS shlinks_clicks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  link_id uuid REFERENCES shlinks_links(id) ON DELETE CASCADE,
  ip text,
  user_agent text,
  valid boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS shlinks_withdrawals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  amount numeric(14,2) NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','paid')),
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Whoseeme (who saw my link tracker)
CREATE TABLE IF NOT EXISTS whoseeme_users (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  username text,
  coins int NOT NULL DEFAULT 0,
  balance numeric(14,2) NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, user_id)
);

CREATE TABLE IF NOT EXISTS whoseeme_links (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  token text NOT NULL,
  bait_text text,
  bait_image text,
  content text,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (bot_id, token)
);

CREATE TABLE IF NOT EXISTS whoseeme_clicks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  link_id uuid REFERENCES whoseeme_links(id) ON DELETE CASCADE,
  viewer_name text,
  viewer_username text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS whoseeme_tasks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  task_type text NOT NULL,
  reward int NOT NULL DEFAULT 0,
  completed boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS whoseeme_withdrawals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  amount numeric(14,2) NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','paid')),
  created_at timestamptz NOT NULL DEFAULT now()
);

-- Faal (fortune telling)
CREATE TABLE IF NOT EXISTS faal_users (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  username text,
  fortune_count int NOT NULL DEFAULT 0,
  last_fortune_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, user_id)
);

CREATE TABLE IF NOT EXISTS faal_fortunes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  fortune_text text NOT NULL,
  fortune_type text NOT NULL DEFAULT 'daily',
  created_at timestamptz NOT NULL DEFAULT now()
);

-- AnonyChat (anonymous chat)
CREATE TABLE IF NOT EXISTS anonychat_users (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  username text,
  status text NOT NULL DEFAULT 'idle' CHECK (status IN ('idle','searching','chatting')),
  partner_id bigint,
  chat_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, user_id)
);

CREATE TABLE IF NOT EXISTS anonychat_reports (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  reporter_id bigint NOT NULL,
  reported_id bigint NOT NULL,
  reason text,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- TapEarn (tap-to-earn game)
CREATE TABLE IF NOT EXISTS tapearn_users (
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  username text,
  money bigint NOT NULL DEFAULT 0,
  energy int NOT NULL DEFAULT 100,
  max_energy int NOT NULL DEFAULT 100,
  last_energy_update timestamptz NOT NULL DEFAULT now(),
  last_tap timestamptz,
  tap_power int NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (bot_id, user_id)
);

CREATE TABLE IF NOT EXISTS tapearn_taps (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id uuid NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id bigint NOT NULL,
  amount bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
