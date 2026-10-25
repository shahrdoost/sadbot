# مرجع API

> Base: `/api/v1` — JSON
> احراز هویت: `Authorization: Bearer <JWT>` (از `POST /auth/login`؛ مدت ۱۲ ساعت؛ HS256)
> خطاهای API: `{"error": "پیام فارسی"}` با کد مناسب HTTP

## Health

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /api/health` | عمومی | `{"status":"ok"}` |

## احراز هویت

| مسیر | دسترسی | بدنه/پاسخ |
|---|---|---|
| `POST /auth/login` | عمومی | `{email, password}` → `{access_token, admin}` |
| `GET /auth/me` | authed | پروفایل admin جاری |

## مدیران (superadmin)

| مسیر | توضیح |
|---|---|
| `GET /admins` | لیست |
| `POST /admins` | `{email, password(min 8), display_name, role}` |
| `PATCH /admins/{id}` | `{display_name?, role?, status?}` |
| `DELETE /admins/{id}` | حذف |
| `GET /admins/{id}/bot-access` | دسترسی‌های bot |
| `PUT /admins/{id}/bot-access/{botID}` | `{permissions: [...]}` |

## بات‌ها

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots` | authed | لیست بات‌ها + `running` |
| `POST /bots` | **superadmin** | `{name, token, template_key}` |
| `GET /bots/{botID}` | `bot:read` | `{bot, running}` |
| `PATCH /bots/{botID}` | `bot:config` | `{name?, template_key?, token?}` |
| `DELETE /bots/{botID}` | superadmin | حذف کامل |
| `POST /bots/{botID}/start` | `bot:runtime` | استارت |
| `POST /bots/{botID}/stop` | `bot:runtime` | توقف |
| `GET /bots/{botID}/config` | `bot:config` | سند config |
| `PUT /bots/{botID}/config` | `bot:config` | `{version, texts, menus, prices, feature_flags}` |
| `GET /bots/{botID}/settings` | `bot:config` | تنظیمات merged |
| `PUT /bots/{botID}/settings` | `bot:config` | بروزرسانی تنظیمات |
| `GET /bots/{botID}/admin/channels` | `bot:config` | کانال‌ها |
| `PUT /bots/{botID}/admin/channels` | `bot:config` | جایگزینی کانال‌ها |

## کاربران و رویدادها

| مسیر | دسترسی | پارامترها |
|---|---|---|
| `GET /bots/{botID}/users` | `bot:users` | `q`, `from`, `to`, `page`, `size` |
| `GET /bots/{botID}/events` | `bot:users` | `kind`, `from`, `to`, `page`, `size` |

## مالی

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots/{botID}/payments` | `bot:finance` | لیست پرداخت‌ها |
| `POST /bots/{botID}/payments` | `bot:finance` | ثبت دستی |
| `GET /bots/{botID}/stats` | `bot:read` | آمار |
| `GET /finance/overview` | superadmin | مالی کل |
| `GET /audit-logs` | superadmin | لاگ‌ها |

## پرداخت (درگاه)

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /payments/callback` | **عمومی** | callback درگاه catroom.ir |

## Content (بات محتوا)

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots/{botID}/content/settings` | `bot:config` | تنظیبات محتوا |
| `PUT /bots/{botID}/content/settings` | `bot:config` | بروزرسانی |
| `GET /bots/{botID}/content/folders` | `bot:read` | فولدرهای تلگرام |
| `GET /bots/{botID}/content/chats` | `bot:read` | کانال‌های مقصد |
| `GET /bots/{botID}/content/posts` | `bot:read` | ledger پست‌ها |
| `POST /bots/{botID}/clone` | **superadmin** | کلون بات |

## Moderation (اعتراف/تیکت)

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots/{botID}/confessions` | `bot:read` | اعتراف‌ها |
| `POST /bots/{botID}/confessions/{id}/approve` | `bot:config` | تایید |
| `POST /bots/{botID}/confessions/{id}/reject` | `bot:config` | رد |
| `GET /bots/{botID}/tickets` | `bot:read` | تیکت‌ها |
| `POST /bots/{botID}/tickets/{id}/reply` | `bot:config` | پاسخ |
| `POST /bots/{botID}/tickets/{id}/close` | `bot:config` | بستن |

## Dating

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots/{botID}/dating-users` | `bot:users` | کاربران (`?ghost=only`) |
| `GET /bots/{botID}/broadcast/preview` | `bot:read` | پیش‌نمایش |
| `POST /bots/{botID}/broadcast` | `bot:config` | برادکست |
| `GET /bots/{botID}/broadcast/runs` | `bot:read` | تاریخچه |
| `GET/POST/DELETE /bots/{botID}/discounts` | `bot:config` | کدهای تخفیف |
| `POST /bots/{botID}/confessions/seed` | `bot:config` | بذر اعتراف‌ها |

## AdminOps (مدیریت کاربر)

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots/{botID}/admin/users/{userID}` | `bot:users` | جزئیات کاربر |
| `POST /bots/{botID}/admin/users/{userID}/coins` | `bot:finance` | تنظیم سکه |
| `POST /bots/{botID}/admin/users/{userID}/reset` | `bot:config` | ریست |
| `POST /bots/{botID}/admin/users/{userID}/block` | `bot:config` | مسدود |
| `POST /bots/{botID}/admin/users/{userID}/streak` | `bot:config` | ریست استریک |
| `POST /bots/{botID}/admin/users/{userID}/message` | `bot:config` | پیام مستقیم |
| `GET /bots/{botID}/admin/stuck` | `bot:read` | کاربران گیرکرده |
| `POST /bots/{botID}/admin/stuck/reset` | `bot:config` | آزادسازی |
| `POST /bots/{botID}/admin/reset-all` | `bot:config` | ریست گروهی |

## چت‌ویور و AI

| مسیر | دسترسی | توضیح |
|---|---|---|
| `GET /bots/{botID}/admin/chats` | `bot:read` | چت‌های فعال |
| `GET /bots/{botID}/admin/chats/transcript` | `bot:read` | متن چت |
| `GET /bots/{botID}/admin/messages` | `bot:read` | فید پیام‌ها |
| `GET /bots/{botID}/admin/ai` | `bot:read` | کاربران AI |
| `GET /bots/{botID}/admin/ai/users/{userID}` | `bot:read` | تاریخچه AI |
| `PUT /bots/{botID}/admin/ai` | `bot:config` | فعال/غیرفعال AI |
| `POST /bots/{botID}/admin/telegram-blocks/scan` | `bot:config` | اسکن بلاک |
| `GET /bots/{botID}/admin/blocks` | `bot:read` | جفتهای بلاک |

## اکانت تلگرامی (MTProto)

| مسیر | دسترسی | توضیح |
|---|---|---|
| `POST /telegram-accounts/start-login` | owner-or-super | شروع لاگین |
| `POST /telegram-accounts/complete-login` | owner-or-super | تکمیل لاگین |
| `GET /telegram-accounts` | owner-or-super | لیست |
| `DELETE /telegram-accounts/{id}` | owner-or-super | حذف |
| `POST /telegram-accounts/{id}/create-bot` | owner-or-super | ساخت بات با BotFather |
| `GET /telegram-accounts/{id}/chats` | owner-or-super | چت‌ها |
| `GET/POST /telegram-accounts/{id}/chats/{chatID}/messages` | owner-or-super | پیام‌ها |
| `GET /telegram-accounts/{id}/chats/{chatID}/messages/{msgID}/photo` | owner-or-super | عکس |
| `GET /telegram-accounts/{id}/chats/{chatID}/photo` | owner-or-super | عکس چت |

## ماتریس دسترسی (RBAC)

| عمل | superadmin | admin (بدون grant) | admin (با grant) |
|---|---|---|---|
| ساخت/حذف بات | ✅ | ❌ | ❌ |
| دیدن بات | همه | فقط grant | grant |
| config/settings | ✅ | ❌ | `bot:config` |
| start/stop | ✅ | ❌ | `bot:runtime` |
| کاربران/رویداد | ✅ | ❌ | `bot:users` |
| پرداخت/مالی | ✅ | ❌ | `bot:finance` |
| مالی کل | ✅ | ❌ | ❌ |
| مدیریت مدیران | ✅ | ❌ | ❌ |

## نمونهٔ مصرف

```bash
TOKEN=$(curl -s -X POST http://HOST:8085/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@sadbot.local","password":"..."}' | jq -r .access_token)

# ساخت بات
curl -s -X POST http://HOST:8085/api/v1/bots \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"بازی من","token":"123456:ABC...","template_key":"tapearn"}'

# استارت
curl -s -X POST http://HOST:8085/api/v1/bots/{botID}/start \
  -H "Authorization: Bearer $TOKEN"
```
