DROP TABLE IF EXISTS tapearn_taps;
DROP TABLE IF EXISTS tapearn_users;
DROP TABLE IF EXISTS anonychat_reports;
DROP TABLE IF EXISTS anonychat_users;
DROP TABLE IF EXISTS faal_fortunes;
DROP TABLE IF EXISTS faal_users;
DROP TABLE IF EXISTS whoseeme_withdrawals;
DROP TABLE IF EXISTS whoseeme_tasks;
DROP TABLE IF EXISTS whoseeme_clicks;
DROP TABLE IF EXISTS whoseeme_links;
DROP TABLE IF EXISTS whoseeme_users;
DROP TABLE IF EXISTS shlinks_withdrawals;
DROP TABLE IF EXISTS shlinks_clicks;
DROP TABLE IF EXISTS shlinks_links;
DROP TABLE IF EXISTS shlinks_users;
DROP TABLE IF EXISTS favaani_referrals;
DROP TABLE IF EXISTS favaani_challenges;
DROP TABLE IF EXISTS favaani_users;

ALTER TABLE bots DROP CONSTRAINT IF EXISTS bots_template_key_check;
ALTER TABLE bots ADD CONSTRAINT bots_template_key_check
  CHECK (template_key IN ('welcome_menu','dating','content','bombland','duel'));
