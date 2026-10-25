# معماری صدبات

> تاریخ: ۲۰۲۶-۱۰-۲۰ — مطابق کد پیاده‌سازی‌شده در این مخزن

صدبات یک پردازهٔ Go واحد است که چند بات تلگرامی را همزمان اجرا می‌کند، داده‌های همه را در یک Postgres نگه می‌دارد و یک REST API برای پنل مدیریت ارائه می‌دهد.

## نمای کلی

```
تلگرام ──long-polling──▶ runtime.Manager ──▶ engine.Engine ──▶ Template (welcome_menu | dating)
                              │                    │
                              │                    ├─ repository (Postgres: state، رویداد، پروفایل)
                              │                    └─ Sender (پاسخ به چت)
                              └─ مچر dating (هر ۵ ثانیه)

مرورگر ──HTTP──▶ api.Server (chi + JWT + RBAC) ──▶ service‌ها ──▶ repository ──▶ Postgres
```

## ساختار backend

```
backend/
├── cmd/sadbot/main.go        # wire: config → db → repos → services → engine → runtime → HTTP
├── cmd/migrate/main.go       # اعمال migrations هنگام بوت کانتینر (با درایور postgres)
├── internal/
│   ├── config/               # خواندن env + .env (بدون override متغیرهای محیطی)
│   ├── domain/               # استراکت‌های خالص + interface (بدون وابستگی)
│   ├── repository/postgres/  # یک repo به‌ازای هر aggregate (pgx pool)
│   ├── service/              # auth، admin، bot (منطق کسب‌وکار + اعتبارسنجی)
│   ├── engine/               # هستهٔ config-driven: HandleUpdate + Template registry
│   ├── engine/templates/     # WelcomeMenu (نمونه)
│   ├── dating/               # تمپلیت دوستیابی + مچر (پورت kpkchat)
│   ├── content/              # تمپلیت محتوا: جمع‌آوری خبر + AI + رندر + انتشار کانال (پورت sadcontent)
│   ├── telegram/             # کلاینت دست‌ساز Bot API + type ها
│   ├── runtime/              # چرخهٔ حیات بات‌ها (poller/worker/outqueue)
│   └── api/                  # chi router، middleware ها، handlers
└── pkg/{crypto,respond}/     # رمزنگاری AES-GCM توکن، پاسخ JSON
```

### لایهٔ domain و repository

- `domain/` فقط استراکت و interface دارد؛ هیچ وابستگی به pgx/HTTP ندارد.
- `repository/postgres/` پیاده‌سازی دسترسی داده با `pgxpool` است. هر aggregate یک فایل: `admin_repo.go`، `bot_repo.go`، `botuser_repo.go`، `event_repo.go`، `payment_repo.go`، `finance_repo.go`، `dating_repo.go`، `audit_repo.go`.

### لایهٔ service

| سرویس | مسئولیت |
|---|---|
| `AuthService` | لاگین bcrypt، صدور/پارس JWT (HS256، ۱۲ ساعته) |
| `AdminService` | CRUD مدیران، دادن/گرفتن دسترسی per-bot، ثبت audit |
| `BotService` | ساخت بات (اعتبارسنجی توکن با getMe + رمزنگاری AES-GCM)، config با optimistic lock (`version`)، start/stop؛ `NormalizeConfigInput` (رفع double-encoding مقادیر JSON رشته‌ای) |
| `AdminOpsService` | عملیات مدیریتی per-user (سکه/ریست/مسدود/استریک/پیام مستقیم)، اسکن گیرکرده/بلاک تلگرامی، هارنس AI، تنظیمات عددی، کانالها |
| `TelegramAccountService` | پروکسی tgsidecar: اکانت‌های MTProto (لاگین شماره)، لیست چت/پیام، ارسال پیام، ساخت بات از BotFather |
| `PaymentService` | خرید پکیج/اشتراک (درگاه catroom.ir)، callback عمومی، verify با CAS، سوییپر auto-verify + انقضا |

`TelegramValidator` با فراخوانی واقعی `getMe` توکن را قبل از ذخیره چک می‌کند؛ هرگز توکن بدون اعتبارسنجی ذخیره نمی‌شود.

## موتور config-driven (`internal/engine`)

هر بات یک instance از یک **Template** است. رفتار پایه از `DefaultConfig()`/`DefaultMenus()` تمپلیت می‌آید و با JSON ذخیره‌شده در `bot_configs` (عمیق merge، دیتابیس برنده) بازنویسی می‌شود.

### Template interface

```go
type Template interface {
    Key() string
    DefaultTexts() []byte   // JSON متن‌ها
    DefaultMenus() []byte   // JSON صحنه‌ها و دکمه‌ها
}

// interface های اختیاری — موتور با type assertion تشخیص می‌دهد:
type Starter       interface { Start(bc *BotContext, payload string) bool }
type MessageHandler interface { HandleMessage(bc *BotContext, text string) bool }
type ActionHandler interface { HandleAction(bc *BotContext, action string) bool }
```

### pipeline هر آپدیت (`Engine.HandleUpdate`)

1. استخراج کاربر/چت؛ رد کردن ربات‌ها.
2. Upsert کاربر در `bot_end_users` + `last_seen_at`.
3. Decode state از JSONB (`scene` / `step` / `data`)؛ ناشناس → `home`.
4. Merge تنظیمات (کش در حافظهٔ همان فراخوانی).
5. لاگ رویداد ورودی (`inbound_message` / `command` / `callback`).
6. روت:
   - `/start` → اگر تمپلیت `Starter` باشد او تصمیم می‌گیرد؛ وگرنه `OnStart` پیش‌فرض (رفتن به home).
   - callback → `answerCallbackQuery` + `DispatchAction` (`scene:` | `act:` | `url:`).
   - متن → اول `MessageHandler` تمپلیت؛ اگر نخورد، lookup دکمه‌ها (label → action در کل صحنه‌ها)؛ اگر نشد پیام unknown.
   - رسانه → اگر تمپلیت `MediaHandler` باشد او مصرف میکند (مثل آواتار onboarding یا رلهٔ رسانه در چت دوستیابی)؛ وگرنه پیام unknown.

`BotContext` دسترسی‌های لازم را می‌دهد: `Reply`/`ReplyKBD`/`ReplyKBDStyled` (کیبورد reply فارسی — نسخهٔ styled رنگ دکمه می‌دهد: `primary`/`success` از Bot API 7.11+ مثل kpkchat)، `ReplyInline`/`SendPhotoKBD` (کیبورد inline + عکس با file_id)، `SendTo`/`SendKBDTo` (ارسال به چت دیگر مثل طرف مقابل چت)، `ForwardMediaTo` (فوروارد رسانه در چت)، `RecordInbound`، `State`، `Data` (کیسهٔ DI برای store ها — مثلاً `Data["dating"]`)، `Prices` (قیمت‌های merged هر بات) و `MsgID`.

interface های اختیاری بیشتر برای تمپلیت‌ها: `MediaHandler` (پذیرش رسانه مثل آواتار onboarding)، `PhotoSender` (ارسال عکس)، `CaptionEditor` (ویرایش caption کارت عکسی — `editMessageText` روی رسانه رد میشود؛ browse برای بستن کارت عکسی استفاده میکند)، `MediaForwarder` (فوروارد رسانه بین دو طرف چت).

## ران‌تایم تلگرام (`internal/runtime`)

`Manager` به‌ازای هر باتِ running یک `botRuntime` با سه goroutine دارد:

| goroutine | وظیفه |
|---|---|
| **poller** | `getUpdates` با offset و long-poll ۳۰ ثانیه‌ای؛ بک‌اف ۱s→2s→…→60s روی خطا؛ خطای 401 → توقف بات و `status=error` |
| **worker** | پردازش ترتیبی آپدیت‌ها (حفظ ترتیب per-chat) → `engine.HandleUpdate` با recover از panic |
| **outloop** | صف خروجی با rate limit پایه ۲۰ msg/s؛ retry با `retry_after` روی 429 |

- **Stop(graceful)**: کنسل context → پایان poller → drain با deadline ۵ ثانیه. `StopAll` (موقع shutdown پروسه) فقط context ها را کنسل میکند و **دیتابیس را دست نمیزند** — چون قبلاً status را `stopped` میکرد و هر deploy کل فرست بات‌ها را میکشت (رفع باگ ۲۰۲۶-۱۰-۱۸). توقف صریح از پنل همچنان `status=stopped` مینویسد.
- در بوت، همهٔ بات‌های `status='running'` دوباره استارت می‌شوند (`StartRunningBots`).
- `ClientFor(botID)` کلاینت بات در حال اجرا را در اختیار مچر می‌گذارد.
- توکن با `runtime.ConfigureTokenDecryptor` (AES-GCM از `pkg/crypto`) رمزگشایی می‌شود.

## کلاینت تلگرام (`internal/telegram`)

کلاینت دست‌ساز روی `net/http` (بدون framework): `GetMe`، `GetUpdates` (form-encoded)، `SendMessage`، `SendPhoto`، `EditMessageText`، `EditMessageCaption`، `ForwardMedia`، `SendChatAction`، `AnswerCallbackQuery`، `GetChatMemberStatus`. خطاها به `APIError` با `RetryAfter` و `IsAuthError`/`IsForbidden` نگاشت می‌شوند.

## مچر dating (`internal/dating/matcher.go`)

- حلقهٔ سراسری هر **۵ ثانیه** برای همهٔ بات‌های running با تمپلیت `dating`.
- **مچ**: بارگذاری همهٔ searchers (قفل `FOR UPDATE SKIP LOCKED` در repo) → جفت‌گیری حریصانه با `similarityScore` (وزن‌های kpkchat: جنسیت ۱۰/۵، شهر ۸، استان ۴، سن ۵/۲، هر علاقهٔ مشترک ۳) → احترام به فیلتر جنسیت هر دو طرف → بررسی بلاک → `ClaimMatch` در یک تراکنش (CAS هر دو + حذف از صف + پاک کردن pending rate) → ست کردن `scene=chat` هر دو → اطلاع‌رسانی «وصلت انجام شد!».
- **timeout**: جستجوهای قدیمی‌تر از ۴۰ ثانیه لغو و scene به home برمی‌گردد.

ارسال اعلان‌ها از طریق `Notifier` (func تزریقی) انجام می‌شود تا تست‌پذیر باشد؛ در production این func کلاینت واقعی را از `runtime.Manager` می‌گیرد.

- **گوست‌ها (cold-start)**: کاربران نمونهٔ `is_ghost` در صف جستجو شرکت می‌کنند، در browse ظاهر می‌شوند و با موتور پاسخ (`dating/cold-start/`) در چت جواب می‌دهند — چت با گوست بدون صفحهٔ امتیازدهی تمام می‌شود. موتور نوتیفیکیشن (`dating/notifications.go`) یادآوری‌ها را با کیبورد inline می‌فرستد.
- **سوییپر پرداخت** (`payment/sweeper.go`): هر دقیقه pending های تازه (<۱۰ دقیقه) را auto-verify می‌کند و pending های کهنه را **دقیقاً یک بار** با وضعیت `failed` منقضی اطلاع‌رسانی می‌کند (وضعیت باید داخل CHECK دیتابیس باشد؛ `MarkFailed` از نوع CAS و bool)؛ اشتراک‌ها هر ۶ ساعت منقضی می‌شوند.

## کلکتور محتوا (`internal/content`)

- الگوی مشابه مچر: حلقهٔ سراسری ۳۰ ثانیه‌ای → هر باتِ `content`ِ running با فاصلهٔ `content_poll_minutes`.
- منبع: فولدر تلگرام اکانت MTProto مشترک (`tgsidecar GET /folders` — **fail-closed** اگر فولدر نبود)؛ کانال مقصد همیشه مستثنی.
- پردازش هر پیام: فیلتر → AI clean (پرامپت عین sadcontent؛ NO_CONTENT = حذف؛ خطا = متن خام) → خلاصهٔ کارت → رندر post+story با برند بات از طریق **سرویس رندر** (`renderer/` کانتینر جدا — Playwright) → ارسال با `SendPhotoUpload` به کانال مقصد (کپشن = متن کامل[:1000]) → ثبت در `content_posts` (dedup ماندگار).
- fallback: خطای رندر → ارسال متن خام؛ خطای LLM → متن خام؛ بدون `RENDERER_URL` → متن خام.
- اینستاگرام عمداً خارج از دامنه است (سیستم ADB محلی جدا می‌ماند).

## لایهٔ API (`internal/api`)

- **chi** با middleware: RealIP، Logger، Recoverer، CORS، سپس `Authenticator` (JWT) برای همهٔ مسیرهای `/api/v1` به‌جز `login`.
- **RBAC**: `RequireSuper` (فقط superadmin) و `RequireBotPerm(perm)` که `:botID` را از URL می‌خواند و `admin_bot_access` را چک می‌کند؛ superadmin همیشه مجاز است.
- جزئیات مسیرها: [API.md](./API.md).

## فرانت‌اند (`frontend/`)

- **Vite + React 18 + TypeScript strict + Redux Toolkit (RTK Query) + React Router 6 + Tailwind**.
- RTL کامل (`<html dir="rtl" lang="fa">`)، فونت Vazirmatn، utility های منطقی Tailwind (`ms-* pe-*`)، قالب‌بندی اعداد فارسی و «تومان» در `lib/utils.ts`.
- **RTK Query** برای همهٔ state سرور؛ توکن از `localStorage` در `prepareHeaders`.
- صفحات:
  - `/login` — ورود
  - `/` — داشبورد مالی (درآمد کل + جدول per-bot)
  - `/bots` — لیست و ساخت بات (دو حالت: از اکانت تلگرامی با گفتگوی خودکار BotFather، یا با توکن دستی)
  - `/bots/:id` — تب‌ها: کاربران (جستجو + جزئیات per-user: سکه/ریست/مسدود/استریک/پیام مستقیم)، فعالیت، پرداخت‌ها، تنظیمات (فرم عددی ۳۸ فیلد + JSON خام + ویرایشگر منو per-button با کنترل version)، سلامت و ریست (اسکن گیرکرده، بلاک‌ها، ریست گروهی)، چت‌ویور، کانال‌ها، هارنس AI، برادکست (متن/عکس/فوتر)، کدهای تخفیف، اعتراف‌ها، تیکت‌ها
  - `/telegram` — اکانت‌های MTProto: افزودن با لاگین شماره، لیست چت‌ها، نمایش پیام مثل تلگرام + کامپوزر ارسال
  - `/admins` — CRUD مدیران + دسترسی per-bot
- **دارک/لایت مود**: کلاس `dark` روی `<html>` + اسکریپت pre-paint از `localStorage.sadbot_theme` (بدون فلش) + تاگل ☀️/🌙 در سایدبار.
- پوشه‌ها: `app/` (store, hooks, router)، `features/{auth,bots,finance,admins,telegram,confessions,tickets,broadcast,discounts}`، `layouts/AdminLayout.tsx`، `lib/`.
- در dev، Vite `/api` را به `localhost:8080` proxy می‌کند؛ در production nginx همین کار را می‌کند.

## سایدکار تلگرام (`tgsidecar/`)

سرویس FastAPI + Telethon، کانتینر چهارم استک (بدون پورت میزبان؛ فقط از شبکهٔ compose با کلید `X-Sidecar-Key`). اکانت‌های MTProto (شماره/کد/2FA) را نگه می‌دارد و ارائه می‌دهد: لیست چت/پیام/عکس، ارسال پیام، و **گفتگوی خودکار با BotFather** برای ساخت بات از یک اکانت. Go از طریق `internal/tgsidecar` + `TelegramAccountService` به آن پروکسی می‌کند. جلسات در ولوم `tg_sessions` (container recreate آن را پاک میکند — بعد از هر deploy کپی مجدد لازم است). دکمه‌های reply با ارسال متن فشرده میشوند؛ دکمه‌های inline از طریق درخواست خام Telethon (`GetBotCallbackAnswerRequest`) — شیء دکمهٔ wrapper متد `click()` ندارد و callback data در `btn.type.data` است.

## فایل‌های کلیدی برای مطالعه

| فایل | نقش |
|---|---|
| `backend/cmd/sadbot/main.go` | تمام wiring |
| `backend/internal/engine/engine.go` | pipeline آپدیت‌ها |
| `backend/internal/runtime/runtime.go` | چرخهٔ حیات بات‌ها |
| `backend/internal/dating/template.go` | جریان‌های دوستیابی |
| `backend/internal/dating/matcher.go` | الگوریتم مچ |
| `backend/internal/api/server.go` | مسیرها و RBAC |
| `frontend/src/features/bots/botsApi.ts` | کل قرارداد API در فرانت |
