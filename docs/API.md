# مرجع API

> Base: `/api/v1` — JSON  
> احراز هویت: `Authorization: Bearer <JWT>` (از `POST /auth/login`؛ مدت ۱۲ ساعت؛ HS256)  
> خطاهای API: `{"error": "پیام فارسی"}` با کد مناسب HTTP.

## احراز هویت

| مسیر | دسترسی | بدنه/پاسخ |
|---|---|---|
| `POST /auth/login` | عمومی | `{email, password}` → `{access_token, admin}` |
| `GET /auth/me` | authed | پروفایل admin جاری |

## مدیران (superadmin فقط)

| مسیر | توضیح |
|---|---|
| `GET /admins` | لیست |
| `POST /admins` | `{email, password(min 8), display_name, role: admin\|superadmin}` |
| `PATCH /admins/{id}` | `{display_name?, role?, status?}` |
| `DELETE /admins/{id}` | حذف (حذف خودِ لاگین‌کرده ممنوع) |
| `GET /admins/{id}/bot-access` | لیست دسترسی‌های bot آن مدیر |
| `PUT /admins/{id}/bot-access/{botId}` | `{permissions: ["bot:read", ...]}` — upsert |

## بات‌ها

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots` | authed | لیست بات‌های قابل دیدن (superadmin: همه؛ مدیر عادی: فقط grant‌شده‌ها) + فیلد `running` |
| `POST /bots` | **superadmin** | `{name, token, template_key}` — توکن با getMe اعتبارسنجی و AES-GCM رمزنگاری می‌شود؛ config پیش‌فرض تمپلیت ساخته می‌شود |
| `GET /bots/{botID}` | `bot:read` | `{bot, running}` |
| `PATCH /bots/{botID}` | `bot:config` | `{name?, template_key?, token?}` — تعویض توکن runtime در حال اجرا را ری‌استارت می‌کند |
| `DELETE /bots/{botID}` | superadmin | توقف runtime + حذف کامل (cascade) |
| `POST /bots/{botID}/start` | `bot:runtime` | استارت long-polling (خطای توکن → `status=error` + `last_error`) |
| `POST /bots/{botID}/stop` | `bot:runtime` | توقف graceful |
| `GET /bots/{botID}/config` | `bot:config` | سند config (اگر نبود، defaults تمپلیت با `version: 0`) |
| `PUT /bots/{botID}/config` | `bot:config` | `{version, texts, menus}` — **version برای optimistic lock الزامی**؛ تعارض → پیام «همزمان ویرایش شده» |

## کاربران و فعالیت

| مسیر | دسترسی | پارامترها |
|---|---|---|
| `GET /bots/{botID}/users` | `bot:users` | `q` (username/نام/telegram_id)، `from`، `to`، `page`، `size` → `{users[], total}` |
| `GET /bots/{botID}/events` | `bot:users` | `kind`، `from`، `to`، `page`، `size` → `{events[], total}` |

## مالی

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots/{botID}/payments` | `bot:finance` | لیست صفحه‌بندی‌شده |
| `POST /bots/{botID}/payments` | `bot:finance` | ثبت **دستی**: `{amount_rials, user_id?, note?, kind?}` — `status=verified`، `gateway=manual`، لاگ audit |
| `GET /bots/{botID}/stats` | `bot:read` | `{total_users, new_users_30d, revenue_30d_rials, active_subscriptions, events_24h}` |
| `GET /finance/overview` | superadmin | `{total_revenue_rials, total_users, active_subscriptions, per_bot: [{bot_id, bot_name, revenue_rials, users, active_subscriptions}]}` |
| `GET /audit-logs` | superadmin | `page`، `size` → `{logs[], total}` |

## پرداخت (درگاه)

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /payments/callback` | **عمومی** (بدون JWT) | callback درگاه catroom.ir — تایید سمت سرور + credit سکه/اشتراک با CAS |
| خرید پکیج/اشتراک | از داخل بات | از منوی «💰 سکه»؛ مسیر پنلی ندارد (callback همگانی است) |

سوییپر داخل بک‌اند: auto-verify هر دقیقه (پرداختهای <۱۰ دقیقه)، انقضای pending بعد از آن (دقیقاً یک اطلاع)، انقضای اشتراک هر ۶ ساعت.

## تنظیمات و کانال‌ها

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots/{botID}/settings` | `bot:config` | مقادیر عددی/بولین merged از prices با پیشفرضهای تمپلیت (۳۸ فیلد) |
| `PUT /bots/{botID}/settings` | `bot:config` | اعتبارسنجی دامنه + merge سطح‌کلید (کلیدهای غیرفهرست مثل `channels`/`packages` دست‌نخورده می‌مانند) |
| `GET /bots/{botID}/admin/channels` | `bot:config` | لیست کانالهای ماموریت (شکل ساخت‌یافته یا رشته) |
| `PUT /bots/{botID}/admin/channels` | `bot:config` | جایگزینی لیست کانالها |

## عملیات مدیریتی کاربر (AdminOps)

| مسیر | دسترسی | بدنه/توضیح |
|---|---|---|
| `GET /admin/users/{userID}` | `bot:users` | پروفایل + سکه + ۱۰ پرداخت آخر + ۳۰ پیام آخر + تعداد لایک |
| `POST /admin/users/{userID}/coins` | `bot:finance` | `{delta}` (۱-۱۰۰۰۰۰۰؛ کف صفر) → `{coins}` |
| `POST /admin/users/{userID}/reset` | `bot:config` | ریست محتوا (سکه/امتیاز/استریک/چت/لایکها) — پروفایل می‌ماند |
| `POST /admin/users/{userID}/block` | `bot:config` | `{blocked: bool, reason?}` — جستجو/مرور/چت مسدود می‌شود |
| `POST /admin/users/{userID}/streak` | `bot:config` | ریست استریک روزانه |
| `POST /admin/users/{userID}/message` | `bot:config` | `{text}` — پیام مستقیم از ربات (بات باید running باشد) |
| `GET /admin/stuck` | `bot:read` | کاربران گیرکرده در صف/چت |
| `POST /admin/stuck/reset` | `bot:config` | آزادسازی گروهی گیرکردهها |
| `POST /admin/reset-all` | `bot:config` | بدنه `{confirm:"confirm"}` — همهٔ چتها به حالت joined |
| `POST /admin/telegram-blocks/scan` | `bot:config` | `{days}` — پینگ مجدد کاربران غیرفعال؛ 403 → `is_blocked` (از مخاطبان برادکست حذف) |
| `GET /admin/blocks` | `bot:read` | جفتهای بلاک فعال |

## چت‌ویور و دستیار AI

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /admin/chats` | `bot:read` | چتهای فعال جاری |
| `GET /admin/chats/transcript?a=&b=` | `bot:read` | متن کامل یک چت |
| `GET /admin/messages` | `bot:read` | فید سراسری آخرین پیامهای چت |
| `GET /admin/ai` | `bot:read` | کاربران وسط چت AI + تعداد پیامها |
| `GET /admin/ai/users/{userID}` | `bot:read` | تاریخچهٔ AI یک کاربر |
| `PUT /admin/ai` | `bot:config` | `{ai_enabled: 0|1}` — فقط روی prices اثر می‌کند |

## برادکست، تخفیف، اعتراف، تیکت

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /dating-users` | `bot:users` | کاربران دوستیابی (`?ghost=only` فیلتر گوست) |
| `GET /broadcast/preview` | `bot:read` | تعداد مخاطبان قابل دسترس |
| `POST /broadcast` | `bot:config` | `{text, photo?, footer?}` — ارسال گروهی (عکس = file_id یا URL) |
| `GET /broadcast/runs` | `bot:read` | تاریخچهٔ اجراها |
| `GET/POST/DELETE /discounts` | `bot:config` | کدهای تخفیف |
| `GET /confessions` · `POST /confessions/{id}/approve|reject` | read / `bot:config` | اعترافها (تایید → انتشار در کانال) |
| `POST /confessions/seed` | `bot:config` | بذر اعترافهای کاربران نمونه |
| `GET /tickets` · `POST /tickets/{id}/reply|close` | read / `bot:config` | تیکتهای پشتیبانی (reply = پیام به چت کاربر) |

## اکانت تلگرامی (MTProto — tgsidecar)

| مسیر | دسترسی | توضیح |
|---|---|---|
| `POST /telegram-accounts/start-login` | owner-or-super | `{phone}` → ارسال کد |
| `POST /telegram-accounts/complete-login` | owner-or-super | `{phone, code, password?}` — 2FA |
| `GET/DELETE /telegram-accounts[/{id}]` | owner-or-super | لیست/حذف |
| `POST /telegram-accounts/{id}/create-bot` | owner-or-super | ساخت بات با گفتگوی خودکار BotFather |
| `GET /telegram-accounts/{id}/chats` | owner-or-super | لیست چت‌ها |
| `GET/POST .../chats/{chatID}/messages` | owner-or-super | خواندن/ارسال پیام (دکمه‌های reply = متن) |
| `GET .../photo` | owner-or-super | عکس پیام/چت |

## ماتریس دسترسی (RBAC)

| عمل | superadmin | admin (بدون grant) | admin (با grant) |
|---|---|---|---|
| ساخت/حذف بات | ✅ | ❌ | ❌ |
| دیدن لیست بات | همه | فقط grant‌ها | grant‌ها |
| دیدن کاربران/رویداد | ✅ | ❌ | فقط با `bot:users` |
| ویرایش config | ✅ | ❌ | فقط با `bot:config` |
| start/stop | ✅ | ❌ | فقط با `bot:runtime` |
| پرداخت/مالی bot | ✅ | ❌ | فقط با `bot:finance` |
| مالی کل سیستم | ✅ | ❌ | ❌ |
| مدیریت مدیران | ✅ | ❌ | ❌ |

## نمونهٔ مصرف

```bash
TOKEN=$(curl -s -X POST http://HOST:8085/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@sadbot.local","password":"..."}' | jq -r .access_token)

curl -s -X POST http://HOST:8085/api/v1/bots \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"بات اول","token":"123456:ABC...","template_key":"dating"}'
```
