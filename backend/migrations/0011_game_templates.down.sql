DROP TABLE IF EXISTS duel_pools;
DROP TABLE IF EXISTS duel_withdrawals;
DROP TABLE IF EXISTS duel_players;
DROP TABLE IF EXISTS bombland_referrals;
DROP TABLE IF EXISTS bombland_players;

ALTER TABLE bots DROP CONSTRAINT IF EXISTS bots_template_key_check;
ALTER TABLE bots ADD CONSTRAINT bots_template_key_check
  CHECK (template_key IN ('welcome_menu','dating','content'));
