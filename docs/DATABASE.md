# اسکیمای دیتابیس

> PostgreSQL 16 — مایگریشن‌ها: `backend/migrations/0001_init` تا `0012_more_bots`
> قواعد کلی:
> - پول: `BIGINT` (هرگز float)؛ نمایش تومان در UI (÷۱۰).
> - هر سطر bot-scoped دارای `bot_id UUID` با FK به `bots` (`ON DELETE CASCADE`).
> - شناسه‌های موجودیت‌های پلتفرم `UUID`؛ سطرهای پر ترافیک `bigserial`.

## نقشهٔ جدول‌ها

```
admins ──< admin_bot_access >── bots ──< bot_configs (1:1)
                │                 ├──< bot_end_users ──< bot_user_events
                │                 ├──< payments ──< subscriptions
                │                 │
                │                 ├──< dating_profiles / dating_search_requests
                │                 │   dating_msg_refs / dating_pending_rates
                │                 │   dating_block_pairs / dating_likes
                │                 │
                │                 ├──< bombland_players / bombland_referrals
                │                 ├──< duel_players / duel_withdrawals / duel_pools
                │                 ├──< favaani_users / favaani_challenges / favaani_referrals
                │                 ├──< shlinks_users / shlinks_links / shlinks_clicks / shlinks_withdrawals
                │                 ├──< whoseeme_users / whoseeme_links / whoseeme_clicks
                │                 │   whoseeme_tasks / whoseeme_withdrawals
                │                 ├──< faal_users / faal_fortunes
                │                 ├──< anonychat_users / anonychat_reports
                │                 └──< tapearn_users / tapearn_taps

telegram_accounts (MTProto)
content_posts (content ledger)
audit_logs
```

## پلتفرم

### `admins`
| فیلد | نوع | توضیح |
|---|---|---|
| id | uuid PK | |
| email | citext UNIQUE | |
| password_hash | text | bcrypt |
| display_name | text | |
| role | `superadmin` \| `admin` | |
| status | `active` \| `disabled` | |

### `admin_bot_access`
`admin_id + bot_id` UNIQUE — `permissions: text[]` (`bot:read`, `bot:users`, `bot:config`, `bot:finance`, `bot:runtime`)

### `audit_logs`
`action`, `target_type`, `target_id`, `details jsonb`, `ip`

### `telegram_accounts`
`admin_id`, `label`, `tg_user_id`, `phone`, `sidecar_account_id`

## بات‌ها

### `bots`
| فیلد | توضیح |
|---|---|
| name | نام نمایشی |
| telegram_username | مثل `@mybot` |
| token_encrypted | bytea — AES-GCM |
| template_key | `welcome_menu` \| `dating` \| `content` \| `bombland` \| `duel` \| `favaani` \| `shlinks` \| `whoseeme` \| `faal` \| `anonychat` \| `tapearn` |
| status | `stopped` \| `starting` \| `running` \| `error` |

### `bot_configs` (۱:۱)
`texts`, `menus`, `feature_flags`, `prices` (JSONB) + `version` (optimistic lock)

## کاربران هر بات

### `bot_end_users`
`(bot_id, telegram_id)` UNIQUE — `state jsonb` (scene/step/data), `is_blocked`, `first_seen_at`, `last_seen_at`

### `bot_user_events`
`kind`: `inbound_message` | `outbound_message` | `callback` | `command` | `user_start` | `state_change`
ریتنشن: پیام‌ها ۹۰ روز، بقیه ۳۶۵ روز

## مالی

### `payments`
`user_id`, `amount_rials BIGINT`, `kind` (`manual`|`coin_package`|`subscription`), `gateway` (`manual`|`catroom.ir`), `status` (`pending`|`verified`|`failed`|`refunded`)

### `subscriptions`
`plan`, `status` (`active`|`cancelled`|`expired`), `amount_rials`, `started_at`, `expires_at`

## تمپلیت دوستیابی

### `dating_profiles` — PK مرکب `(bot_id, user_id)`
`name`, `gender`, `age`, `province`, `city`, `interests text[]`, `status`, `partner_user_id`, `rating`, `coins`, `photo_file_id`, `is_ghost`, `streak_count`, `blocked_at`

### `dating_search_requests`
`(bot_id, user_id)` UNIQUE — `filters jsonb`, `created_at`

### `dating_msg_refs`
`(bot_id, viewer_user_id, viewer_msg_id)` → `partner_user_id, partner_msg_id`

### `dating_pending_rates`
امتیاز عقب‌افتاده — PK `(bot_id, user_id)`

### `dating_block_pairs`
بلاک دوطرفهٔ ۳۰ روزه

### `dating_likes`
`kind` (`like`|`super`|`pass`)

## بازی‌ها (migration 0011 + 0012)

### `bombland_players` — کارخانه بومبلند
| فیلد | توضیح |
|---|---|
| bombs | تعداد بمب‌ها |
| money | پول (سکه) |
| auto_lvl | سطح تولید خودکار |
| last_auto | آخرین تولید خودکار |
| invited_by | کاربر دعوت‌کننده |

### `bombland_referrals`
`(bot_id, inviter_id, invited_id)`

### `duel_players` — سنگ‌کاغذ‌قیچی
| فیلد | توضیح |
|---|---|
| score | امتیاز (پیش‌فرض ۱۰۰۰) |
| energy | انرژی (پیش‌فرض ۱۰) |
| black_opal | اپال مشکی |
| candy / chocolate / soda | آیتم‌های جایزه |
| total_spent_toman / total_withdrawn_toman | آمار مالی |
| is_bot | بازیکن ربات (برای مچ سریع) |

### `duel_withdrawals`
`amount_mg numeric(12,4)`, `amount_toman bigint`, `kind` (`cash`|`physical`), `status` (`pending`|`approved`|`rejected`|`paid`)

### `duel_pools`
`total_toman`, `total_distributed` — استخر جایزه

### `favaani_users` — بازی فراوانی
`day`, `score`, `streak`, `total_spent`, `referral_code`, `referred_by`

### `favaani_challenges`
`day`, `amount`, `spend_log` — چالش‌های روزانه

### `favaani_referrals`
`(bot_id, inviter_id, invited_id)`

### `shlinks_users` — لینک‌ساز
`balance numeric(14,2)`, `total_earned`, `total_withdrawn`

### `shlinks_links`
`short_code`, `target_url`, `total_clicks`, `valid_clicks`, `total_earned`

### `shlinks_clicks`
`ip`, `user_agent`, `valid` — کلیک‌های هر لینک

### `shlinks_withdrawals`
`amount`, `status` (`pending`|`approved`|`rejected`|`paid`)

### `whoseeme_users` — کی منو دیده
`balance`, `total_earned`, `referral_code`

### `whoseeme_links`
`token`, `target_url`, `title`, `total_clicks`, `unique_clicks`

### `whoseeme_clicks`
`ip`, `user_agent`, `referrer` — بازدیدکنندگان

### `whoseeme_tasks`
`type`, `reward`, `completed_at` — وظایف (دعوت، اشتراک‌گذاری و...)

### `whoseeme_withdrawals`
`amount`, `status`

### `faal_users` — فال روزانه
`total_fortunes`, `last_fortune_date`

### `faal_fortunes`
`day`, `type` (`daily`|`love`|`money`|`health`), `text` — فال‌های گرفته‌شده

### `anonychat_users` — چت ناشناس
`gender`, `age`, `status` (`idle`|`searching`|`chatting`), `partner_user_id`, `report_count`

### `anonychat_reports`
`reporter_id`, `reported_id`, `reason` — گزارش‌ها

### `tapearn_users` — تپ‌کلیک
`money`, `energy`, `max_energy`, `tap_power`, `last_recharge`

### `tapearn_taps`
`earned`, `energy_used` — لاگ تپ‌ها

## مایگریشن‌ها

| # | عنوان | اضافه‌شده |
|---|---|---|
| 0001 | init | admins, bots, bot_configs, bot_end_users, bot_user_events, payments, subscriptions, audit_logs |
| 0002 | telegram_accounts | جدول اکانت‌های MTProto |
| 0003 | dating_economy | سکه، پکیج، اشتراک یکتا |
| 0004 | dating_browse | آواتار، dating_likes |
| 0005 | dating_phase3 | گوست، نوتیفیکیشن، confessions، support_tickets |
| 0006 | discount_broadcast | discount_codes، broadcast_runs |
| 0007 | growth | streak، boost، ویرایش پروفایل |
| 0008 | admin_ops | blocked_at، blocked_reason |
| 0009 | broadcast_media | photo، footer |
| 0010 | content | content_posts (ledger) |
| 0011 | game_templates | bombland_players، bombland_referrals، duel_players، duel_withdrawals، duel_pools |
| 0012 | more_bots | favaani_*، shlinks_*، whoseeme_*، faal_*، anonychat_*، tapearn_* |

## الگوهای اتمیک

**مچینگ** (dating):
```sql
SELECT ... FROM dating_profiles WHERE status='searching' FOR UPDATE SKIP LOCKED;
UPDATE dating_profiles SET status='chatting', partner_user_id=$p
  WHERE bot_id=$b AND user_id=$u AND status='searching';  -- CAS
DELETE FROM dating_search_requests WHERE user_id IN ($a,$b);
COMMIT;
```

**پرداخت** (payment):
```sql
UPDATE payments SET status='verified' WHERE id=$1 AND status='pending';  -- CAS
-- سپس GameCreditor.AddCoins
```

**تپ‌کلیک**:
```sql
UPDATE tapearn_users SET energy=energy-1, money=money+$earned
  WHERE bot_id=$1 AND user_id=$2 AND energy>0;  -- CAS
```
