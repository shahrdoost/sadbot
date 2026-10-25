# اسکیمای دیتابیس

> PostgreSQL 16 — مایگریشن‌ها: `backend/migrations/0001_init` تا `0010_content` (۰۰۰۲+: جدول‌ها و ستونهای اقتصاد/مرور/فاز۳/رشد/ادمین‌آپس — خلاصه در انتها)  
> قواعد کلی:
> - پول: `amount_rials BIGINT` (هرگز float)؛ نمایش تومان در لایهٔ UI (÷۱۰).
> - هر سطر bot-scoped دارای `bot_id UUID` با FK به `bots` (و `ON DELETE CASCADE`).
> - شناسه‌های موجودیت‌های پلتفرم `UUID`؛ سطرهای پر ترافیک `bigserial`.

## نقشهٔ جدول‌ها

```
admins ──< admin_bot_access >── bots ──< bot_configs (1:1)
                │                 ├──< bot_end_users ──< bot_user_events
                │                 ├──< payments ──< subscriptions
                │                 └──< dating_profiles / dating_search_requests
                │                      dating_msg_refs / dating_pending_rates
                │                      dating_block_pairs
audit_logs (actors: admins)
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
| created_at / updated_at | timestamptz | |

### `admin_bot_access` — دسترسی per-bot
| فیلد | توضیح |
|---|---|
| admin_id + bot_id | UNIQUE با هم |
| permissions | `text[]` — مقادیر معتبر: `bot:read`, `bot:users`, `bot:config`, `bot:finance`, `bot:runtime` |
| granted_by | uuid ← admins |

### `audit_logs`
هر تغییر مهم (ساخت/حذف بات، grant دسترسی، پرداخت دستی، start/stop، آپدیت config) یک سطر با `action`, `target_type`, `target_id`, `details jsonb`, `ip` ثبت می‌کند.

## بات‌ها

### `bots`
| فیلد | توضیح |
|---|---|
| name | نام نمایشی |
| telegram_username | مثل `@mybot` (از getMe) |
| token_encrypted | bytea — **AES-GCM** با کلید `TOKEN_ENCRYPTION_KEY` |
| template_key | `welcome_menu` \| `dating` |
| status | `stopped` \| `starting` \| `running` \| `error` |
| last_error | علت خطا (مثلاً 401) |
| token_validated_at | زمان موفق getMe |
| created_by | uuid ← admins |

### `bot_configs` — سند پیکربندی هر بات (۱:۱)
چهار JSONB + کنترل نسخه:

| فیلد | محتوا |
|---|---|
| texts | متن‌های UI (`{"welcome": "...", ...}`) |
| menus | صحنه‌ها و دکمه‌ها (شکل زیر) |
| feature_flags | برای فازهای بعدی |
| prices | قیمت‌ها (برای اقتصاد سکهٔ آینده — BIGINT ریال) |
| version | **optimistic lock** — PUT فقط با version صحیح؛ `version = version + 1` |

شکل `menus` (تمپلیت‌های scene-محور مثل welcome_menu):

```json
{ "scenes": {
    "home": {
      "text_key": "welcome",
      "rows": [
        [{"label": "📖 درباره ما", "action": "scene:about"}],
        [{"label": "⚙️ تنظیمات", "action": "act:settings.notify"}]
      ]
    }
} }
```

اکشن‌ها: `scene:<name>` (جابجایی صحنه)، `act:<key>` (رفتن به handler تمپلیت)، `url:<href>`.

## کاربران هر بات

### `bot_end_users`
| فیلد | توضیح |
|---|---|
| bot_id + telegram_id | **UNIQUE** — یک انسان در هر بات یک سطر |
| username / first_name / last_name | از پیام تلگرام |
| **state** | `jsonb` — `{"scene":"...","step":"...","data":{...}}`؛ تنها state ناوبری (بدون Redis) |
| is_blocked | رزرو برای فیلتر آینده |
| first_seen_at / last_seen_at | آنالیتیکس |

- Upsert در هر آپدیت (`ON CONFLICT ... DO UPDATE last_seen_at`).
- `SetStateScene` فقط فیلد scene را با `jsonb_set` عوض می‌کند (مسیر matcher/force-end).
- ایندکس: `(bot_id, last_seen_at DESC)`.

### `bot_user_events` — لاگ فعالیت
| فیلد | توضیح |
|---|---|
| kind | `inbound_message` \| `outbound_message` \| `callback` \| `command` \| `user_start` \| `state_change` |
| text | حداکثر ۱۰۰۰ کاراکتر |
| meta | jsonb (مثلاً `msg_id`) |

**ریتنشن**: job داخلیِ باینری — پیام‌ها بعد از `EVENT_RETENTION_DAYS` (پیش‌فرض ۹۰ روز)، بقیه بعد از ۳۶۵ روز حذف می‌شوند.

## مالی

### `payments`
| فیلد | توضیح |
|---|---|
| user_id | nullable ← bot_end_users (پرداخت بدون کاربر شناخته‌شده هم ممکن) |
| amount_rials | BIGINT ≥ 0 |
| kind | `manual` \| `coin_package` \| `subscription` |
| gateway | `manual` (ثبت دستی) یا `catroom.ir` (درگاه واقعی سازگار Zibal) |
| status | `pending` \| `verified` \| `failed` \| `refunded` |
| recorded_by | ← admins (ثبت دستی) |

### `subscriptions`
`plan` (پیش‌فرض `premium_monthly`)، `status` (`active|cancelled|expired`)، `amount_rials`، `payment_id`، `started_at`/`expires_at`. ایندکس `(bot_id, status, expires_at)`.

## تمپلیت دوستیابی

### `dating_profiles` — PK مرکب `(bot_id, user_id)`
| فیلد | توضیح |
|---|---|
| telegram_id | برای ارسال پیام بدون join |
| name / gender / age / province / city | خروجی onboarding (gender: `male|female`) |
| interests | `text[]` — علاقه‌مندی‌ها (حداکثر ۵ انتخاب) |
| status | `joined` \| `searching` \| `chatting` — معادل CoreUser کپچت |
| partner_user_id | شناسهٔ bot_end_users طرف مقابل |
| rating / total_ratings / rating_sum | میانگین متحرک امتیاز (round 1 رقم) |

ایندکس‌های partial: `searching` و `chatting` برای مچر.

### `dating_search_requests`
صف جستجو: `(bot_id, user_id) UNIQUE` + `filters jsonb` + `created_at` (مبنای timeout — پیشفرض ۴۰s؛ جستجوی پیشرفته `timeout_sec` از prices، معمولاً ۱۸۰s). شکل `filters`: سرچ ساده `{"gender":"any|male|female","cost":N}`؛ جستجوی پیشرفته علاوه بر آن `{"province","city","age_min","age_max","interests","timeout_sec","advanced":true}` — مچر همه را در `filtersCompatible` هر دو طرف اعمال میکند.

### `dating_msg_refs` — نقشهٔ ریپلای (نسخهٔ Postgres از chat-reply-map کپچت)
چون `message_id` در چت دو نفر فرق دارد: `(bot_id, viewer_user_id, viewer_msg_id)` → `partner_user_id, partner_msg_id`. هنگام ریپلای، id معتبر طرف مقابل resolve و استفاده می‌شود؛ اگر anchor خراب بود، پیام بدون reply فرستاده می‌شود (نه «ترک کرد»).

### `dating_pending_rates`
امتیاز عقب‌افتاده: قبل از پاک کردن `partnerid` برای **هر دو** طرف ثبت می‌شود (باگ کلاسیک kpkchat). PK: `(bot_id, user_id)`.

### `dating_block_pairs`
بلاک دوطرفهٔ ۳۰ روزه: `(bot_id, blocker_id, blocked_id)` + `expires_at`. چک بلاک هم جهت است.


## مایگریشن‌های ۰۰۰۲ تا ۰۰۰۹ (خلاصه)

| # | فایل | اضافه‌شده |
|---|---|---|
| 0002 | telegram_accounts | جدول اکانت‌های MTProto: `admin_id`, `label`, `tg_user_id`, `phone`, `sidecar_account_id` |
| 0003 | dating_economy | `dating_profiles.coins/channel_bonus_at`؛ `payments.coins/package_id`؛ ایندکس یکتای اشتراک `(bot_id, user_id)` |
| 0004 | dating_browse | `dating_profiles.photo_file_id/avatar_bonus_at`؛ جدول `dating_likes` (kind `like/super/pass`، PK مرکب bot+from+to) |
| 0005 | dating_phase3 | `dating_profiles.is_ghost/ghost_persona/ai_free_used` + ستونهای نوتیفیکیشن (`last_notified_at`, `notify_sent_on`, `notify_count_on`, `notify_dismissed_at`)؛ جدول `confessions` (status pending/approved/rejected)؛ جدول `support_tickets` (open/closed) |
| 0006 | discount_broadcast | جدول `discount_codes` (کد/درصد/سقف/انقضا)؛ جدول `broadcast_runs` (متن، تعداد، وضعیت) |
| 0007 | growth | `dating_profiles.streak_count/streak_best/streak_last_date/boosted_until/name_edited_at/profile_edited_at` |
| 0008 | admin_ops | `dating_profiles.blocked_at/blocked_reason` + ایندکس partial مسدودها |
| 0009 | broadcast_media | `broadcast_runs.photo/footer` (عکس file_id یا URL + فوتر زیر کپشن) |
| 0010 | content | شل‌شدن CHECK تمپلیت (`+content`)؛ جدول `content_posts` (ledger جمع‌آوری/انتشار هر بات محتوا — dedup با UNIQUE(bot_id, source_chat_id, message_id)) |

نکات:
- `bot_end_users.is_blocked` از اسکنر پینگ تلگرام (403 = ربات بلاکشده) پر میشود و مخاطبان برادکست آن را رد میکنند؛ با `dating_profiles.blocked_at` (مسدودسازی دستی از پنل) فرق دارد.
- وضعیت `payments.status` فقط `pending|verified|failed|refunded` (CHECK) — سوییپر انقضا باید `failed` بزند نه مقدار دلخواه.
- ستونهای قیمتی/پکیج/کانال داخل `bot_configs.prices` (JSONB) زندگی میکنند، نه جدول‌های رابطه‌ای.

## الگوهای اتمیک (درس kpkchat)

**مچینگ** — همه در یک تراکنش:
```sql
SELECT ... FROM dating_profiles WHERE status='searching' FOR UPDATE SKIP LOCKED;
UPDATE dating_profiles SET status='chatting', partner_user_id=$p
  WHERE bot_id=$b AND user_id=$u AND status='searching';  -- CAS
DELETE FROM dating_search_requests WHERE user_id IN ($a,$b);
DELETE FROM dating_pending_rates   WHERE user_id IN ($a,$b);
COMMIT;
```
اگر CAS دوم شکست بخورد، تراکنش rollback می‌شود و هیچ‌کس «نیمه‌مچ» نمی‌ماند.

**پایان چت**: اول `dating_pending_rates` برای هر دو، بعد پاک کردن partner و برگرداندن به `joined` — هرگز partnerid قبل از ثبت امتیاز پاک نمی‌شود.
