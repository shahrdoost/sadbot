# معماری صدبات

> تاریخ: ۲۰۲۶-۱۰-۲۵ — مطابق کد پیاده‌سازی‌شده در این مخزن

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

## ران‌تایم تلگرام (`internal/runtime`)

`Manager` به‌ازای هر باتِ running یک `botRuntime` با سه goroutine:

| goroutine | وظیفه |
|---|---|
| **poller** | `getUpdates` با offset و long-poll ۳۰ ثانیه‌ای |
| **worker** | پردازش ترتیبی آپدیت‌ها → `engine.HandleUpdate` |
| **outloop** | صف خروجی با rate limit ۲۰ msg/s؛ retry با `retry_after` |

## موتور config-driven (`internal/engine`)

هر بات یک instance از یک **Template** است. رفتار از `DefaultTexts()`/`DefaultMenus()`/`DefaultPrices()` می‌آید و با JSON ذخیره‌شده در `bot_configs` (عمیق merge) بازنویسی می‌شود.

### pipeline هر آپدیت

1. استخراج کاربر/چت
2. Upsert کاربر در `bot_end_users`
3. Decode state از JSONB
4. Merge تنظیمات
5. لاگ رویداد
6. روت: `/start` → Starter | callback → ActionHandler | متن → MessageHandler → دکمه‌ها → unknown

### BotContext

`Reply`/`ReplyKBD`/`ReplyKBDStyled`/`ReplyInline`/`SendPhotoKBD`/`SendTo`/`ForwardMediaTo`/`RecordInbound`/`State`/`Data`/`Prices`/`MsgID`

## سیستم پرداخت یکپارچه

**همهٔ بات‌ها** از یک `payment.Service` استفاده می‌کنند:

```
خرید پکیج → payment.BuyPackage → درگاه catroom.ir → callback → payment.Complete
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
- **گوست‌ها**: کاربران `is_ghost` با موتور پاسخ

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

## فایل‌های کلیدی

| فایل | نقش |
|---|---|
| `backend/cmd/sadbot/main.go` | تمام wiring |
| `backend/internal/engine/engine.go` | pipeline آپدیت‌ها |
| `backend/internal/payment/service.go` | سیستم پرداخت یکپارچه |
| `backend/internal/payment/game_creditor.go` | روتینگ اعتبار بازی‌ها |
| `backend/internal/api/server.go` | مسیرها و RBAC |
| `frontend/src/features/bots/botsApi.ts` | قرارداد API در فرانت |
