# معماری صدبات

> تاریخ: ۲۰۲۶-۱۰-۲۹ — مطابق کد پیاده‌سازی‌شده در این مخزن

صدبات یک پردازهٔ Go واحد است که چند بات تلگرامی را همزمان اجرا می‌کند، داده‌های همه را در یک Postgres نگه می‌دارد و یک REST API برای پنل مدیریت ارائه می‌دهد.

## نمای کلی

```
تلگرام ──long-polling──▶ runtime.Manager ──▶ engine.Engine ──▶ Template
                              │                    │               │
                              │                    │               ├─ dating
                              │                    │               ├─ content
                              │                    │               ├─ bombland / duel
                              │                    │               ├─ favaani / shlinks
                              │                    │               └─ whoseeme / faal / anonychat / tapearn
                              │                    │
                              │                    ├─ repository (Postgres)
                              │                    ├─ payment.Service (یکپارچه)
                              │                    └─ Sender (پاسخ به چت)
                              └─ مچر dating / کلکتور content / سوییپر payment

مرورگر ──HTTP──▶ api.Server (chi + JWT + RBAC) ──▶ service‌ها ──▶ repository ──▶ Postgres
```

## ساختار backend

```
backend/
├── cmd/sadbot/main.go        # wire: config → db → repos → services → engine → runtime → HTTP
├── cmd/migrate/main.go       # اعمال migrations هنگام بوت کانتینر
├── internal/
│   ├── config/               # خواندن env + .env
│   ├── domain/               # استراکت‌های خالص (Bot, Game, Payment, ...)
│   ├── repository/postgres/  # repo ها: admin, bot, user, payment, game, dating, content
│   ├── service/              # auth, bot, adminops, payment, settings
│   ├── engine/               # هستهٔ config-driven: HandleUpdate + Template registry
│   ├── engine/templates/     # WelcomeMenu
│   ├── dating/               # تمپلیت دوستیابی + مچر
│   ├── content/              # تمپلیت محتوا (جمع‌آوری، AI، رندر، انتشار)
│   ├── bombland/             # بازی کارخانه (idle factory)
│   ├── duel/                 # سنگ‌کاغذ‌قیچی نفر‌به‌نفر
│   ├── favaani/              # بازی فراوانی
│   ├── shlinks/              # لینک‌ساز
│   ├── whoseeme/             # کی منو دیده
│   ├── faal/                 # فال روزانه
│   ├── anonychat/            # چت ناشناس
│   ├── tapearn/              # تپ‌کلیک
│   ├── payment/              # درگاه یکپارچه + GameCreditor
│   ├── telegram/             # کلاینت Bot API
│   ├── runtime/              # چرخهٔ حیات بات‌ها
│   └── api/                  # chi router، middleware، handlers
└── pkg/{crypto,respond}/
```

## Template interface

```go
type Template interface {
    Key() string
    DefaultTexts() []byte   // JSON متن‌ها
    DefaultMenus() []byte   // JSON صحنه‌ها و دکمه‌ها
}

// interface های اختیاری:
type Starter        interface { Start(bc *BotContext, payload string) bool }
type MessageHandler interface { HandleMessage(bc *BotContext, text string) bool }
type ActionHandler  interface { HandleAction(bc *BotContext, action string) bool }
type Priced         interface { DefaultPrices() []byte }  // قیمت‌ها/تنظیمات
```

> **قرارداد `HandleAction`**: callback data با پیشوند `act:` ساخته میشود ولی موتور قبل از dispatch آن را میبرد (`strings.TrimPrefix(action, "act:")`) — پس handler مقدار را *بدون* پیشوند میگیرد. تمپلیت‌ها باید نرمال‌سازی کنند (`if !strings.HasPrefix(action, "act:") { action = "act:" + action }`) — بدون آن switch روی `act:*` هرگز match نمیشود. جزئیات: [GAME_TEMPLATES.md](./GAME_TEMPLATES.md).

## ران‌تایم (`internal/runtime`)

`Manager` به‌ازای هر باتِ running یک `botRuntime` **در هر پلتفرم** با سه goroutine میسازد (migration 0013):

| goroutine | وظیفه |
|---|---|
| **poller** | `getUpdates` با offset و long-poll ۳۰ ثانیه‌ای (کت: short-poll ۲ ثانیه‌ای — بیدارشدن poll در cr0.ir کند است) |
| **worker** | پردازش ترتیبی آپدیت‌ها → `engine.HandleUpdate` |
| **outloop** | صف خروجی با rate limit ۲۰ msg/s؛ retry با `retry_after` |

**دو پلتفرم، دادهٔ مشترک**: هر بات میتواند قلوهٔ کت (`kat_token`) هم داشته باشد — دو runtime مستقل (تلگرام + کت) روی همان data کار میکنند و `engine.Router` ارسال/فوروارد بین‌پلتفرمی را به کلاینت درست مسیر میدهد. هویت کاربر `(bot_id, platform, telegram_id)` است. رسانه در کت (بدون `forwardMessage`) graceful degrade میشود. کت reply keyboard ندارد (خاموش میشود) — کلاینت آنها را به inline تبدیل میکند و موتور callback های label را مثل متن تایپ‌شده route میکند.

**مقاومت در برابر قطعی یک سمت** (چون بات‌های دوپلتفرمی نباید با قطعِ cr0.ir یا تلگرام از پا بیفتند):

- **وراثت وضعیت**: خطای یک سمت (توکن باطل = 401) فقط همان سمت را میکُشد؛ `bots.status=error` فقط وقتی **هیچ سمتی** در کار نیست ثبت میشود تا بوت بعدی باتِ نیمه‌سالم را دوباره بالا بیاورد (`running` باقی میماند). خطای تک‌پلتفرمی = degraded، نه error.
- **حلقهٔ تعمیر** (`runtime.Manager.Run` — هر ۶۰ ثانیه): سمت‌های رهاشده با خطای گذرا را دوباره استارت میکند؛ سمت‌های با توکن باطل (`m.failed`) تا استارت دستی از پنل (که همان لحظه توکن جدید میگیرد) skip میشوند.
- **مچینگ امن**: جفت بین‌پلتفرمی فقط وقتی commit میشود که هر دو سمت زنده باشند (`Matcher.SetReachFunc` = `Manager.PlatformRunning`)؛ وگرنه در `parked` میماند و با برگشت سمت، drain اتمیک claim + اطلاع‌رسانی میکند. دیگر چتِ فانتوم (status=chatting، یک طرف بی‌خبر) ساخته نمیشود.
- **ارسال‌های در راه** (`dating/outbound.go`): خطای گذرای relay متن/رسانه/اعلان‌ها به صف retry با backoff (۵ تا ۱۲۰ ثانیه، سقف ۵ تلاش و ۵۰۰ مورد) میرود و چت زنده میماند؛ فقط 403 (کاربر بات را بلاک کرده) دائمی است و چت را با منطق قبلی تمام میکند.

## موتور config-driven (`internal/engine`)

هر بات یک instance از یک **Template** است. رفتار از `DefaultTexts()`/`DefaultMenus()`/`DefaultPrices()` می‌آید و با JSON ذخیره‌شده در `bot_configs` (عمیق merge) بازنویسی می‌شود.

### pipeline هر آپدیت

1. استخراج کاربر/چت
2. Upsert کاربر در `bot_end_users`
3. Decode state از JSONB
4. Merge تنظیمات
5. لاگ رویداد
6. روت: `/start` → Starter | callback → ActionHandler (داده با پیشوند `act:`/`scene:`/`url:`) یا label callback (مثل متن تایپ‌شده) | متن → MessageHandler → دکمه‌ها → unknown

### BotContext

`Reply`/`ReplyKBD`/`ReplyKBDStyled`/`ReplyInline`/`SendPhotoKBD`/`SendTo`/`ForwardMediaTo`/`RecordInbound`/`State`/`Data`/`Prices`/`MsgID`

- **replace-view** (opt-in با `ViewReplaceable.ReplaceViews()` → `BotContext.AutoView`): صفحه‌های کیبورد/کارت یکدیگر را جایگزین میکنند — **send-then-delete** (پیام قبلی فقط بعد از فرودِ جایگزین حذف میشود؛ ارسال ناموفق = صفحهٔ قبلی میماند). tracking با `view_msg` در state؛ `UntrackView` برای پیام‌هایی که باید بمانند. `SendPhotoKBD` روی خطا به پیام متنی fallback میکند.
- **دکمهٔ URL**: `engine.InlineButton{Label, Callback, URL}` — دکمه با `URL` بدون callback کار میکند (مثل لینک پرداخت).

## سیستم پرداخت یکپارچه

**همهٔ بات‌ها** از یک `payment.Service` استفاده می‌کنند:

```
خرید پکیج → payment.BuyPackage → درگاه catroom.ir → هوک سرور-به-سرور/Sweeper → payment.Complete
                                                              ↓
                                                     GameCreditor.AddCoins(botID, userID, n)
                                                              ↓
                                            ┌─────────────────┼─────────────────┐
                                            ↓                 ↓                 ↓
                                      bombland: money   duel: energy   dating: coins
```

- **GameCreditor**: روتینگ اعتبار بر اساس `template_key` بات
- **Gateway**: سازگار Zibal، مبلغ تومان×۱۰ = ریال
- **Sweeper**: auto-verify هر دقیقه + انقضای pending + انقضای اشتراک

## مچر dating (`internal/dating/matcher.go`)

- حلقهٔ سراسری هر **۵ ثانیه**
- جفت‌گیری حریصانه با `similarityScore`
- **timeout**: جستجوهای >۴۰ ثانیه (یا `timeout_sec` پیشرفته)
- **گوست‌ها**: کاربران `is_ghost` با موتور پاسخ (پرسونای LLM از پروفایل خودشان؛ fallback اسکریپتی) — **پیش‌فرض `ghost_enabled=0`**؛ فقط با فعال‌سازی per-bot در prices

## کلکتور محتوا (`internal/content`)

- حلقهٔ ۳۰ ثانیه‌ای → هر بات content
- منبع: فولدر تلگرام → AI clean → رندر → انتشار کانال
- fallback: خطای رندر/LLM → متن خام

## بازی‌ها

| بازی | Repo | مکانیک اصلی |
|---|---|---|
| bombland | `BombLandRepo` | تولید/فروش/ارتقا/دعوت + خرید پول |
| duel | `DuelRepo` | RPS + انرژی + اپال + برداشت |
| favaani | `FavaaniRepo` | چالش روزانه + لاگ اسپند |
| shlinks | `ShlinksRepo` | لینک کوتاه + درآمد کلیک + برداشت |
| whoseeme | `WhoseemeRepo` | لینک طعمه + ردیابی + وظایف |
| faal | `FaalRepo` | فال روزانه + cooldown |
| anonychat | `AnonyChatRepo` | جستجو/مچ/رله/پایان |
| tapearn | `TapEarnRepo` | tap-to-earn + انرژی + ارتقا |

## لایهٔ API (`internal/api`)

- **chi** + JWT + RBAC
- `RequireSuper` / `RequireBotPerm(perm)`
- جزئیات: [API.md](./API.md)

## فرانت‌اند (`frontend/`)

- Vite + React 18 + TypeScript + Redux Toolkit + Tailwind
- RTL کامل، فونت Vazirmatn
- صفحات: داشبورد، بات‌ها (با تب‌های بازی/بازیکنان/برداشت‌ها)، پرداخت، اکانت‌ها، ادمین‌ها

## سایدکار تلگرام (`tgsidecar/`)

FastAPI + Telethon:
- اکانت‌های MTProto (شماره/کد/2FA)
- لیست چت/پیام/عکس، ارسال پیام
- **گفتگوی خودکار BotFather** برای ساخت بات
- جلسات در ولوم `tg_sessions`
- **فرمت‌بندی ارسال** (`_copyable`): URIهای کانفیگ (`vless://` و…) → بلاک Pre (قابل کپی با یک لمس)؛ **بلوک‌های پروکسی MTProto** (Server/Port/Secret — با برچسب فارسی/انگلیسی و ایموجی) → یک لینک قابل‌کلیک `t.me/proxy` («اتصال مستقیم به پروکسی»)؛ خط «در تنظیمات پروکسی وارد کنید» حذف میشود؛ سرور URL-مانند (`host/path`) به host تقلیل مییابد؛ سرور غیرواقعی (مثل `Unknown`) **بدون لینک** میماند (لینک `t.me/proxy` بدون پارامتر پروفایل @proxy را باز میکرد — نباید ساخته شود). **لینک‌های اتصالِ پیام منبع** (دکمه‌های URL و entityها — `proxy_urls` در `serialize_message`) توسط collector استخراج و جایگزین بلوکِ متن میشوند: کانال‌هایی مثل @ProxyMTProto سرور واقعی را پشت «Server: Unknown» و دکمه‌ها پنهان میکنند؛ بدون این، پستِ بازنشری فقط بلوک مرده داشت.

## فایل‌های کلیدی

| فایل | نقش |
|---|---|
| `backend/cmd/sadbot/main.go` | تمام wiring |
| `backend/internal/engine/engine.go` | pipeline آپدیت‌ها |
| `backend/internal/payment/service.go` | سیستم پرداخت یکپارچه |
| `backend/internal/payment/game_creditor.go` | روتینگ اعتبار بازی‌ها |
| `backend/internal/api/server.go` | مسیرها و RBAC |
| `frontend/src/features/bots/botsApi.ts` | قرارداد API در فرانت |
