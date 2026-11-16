# سیستم پرداخت یکپارچه

> تمام بات‌های صدبات از **یک** سیستم پرداخت استفاده می‌کنند — بدون کپی کد.

## نمای کلی

```
کاربر → «💰 خرید» → payment.BuyPackage(botID, userID, packageID)
                              ↓
                    Gateway.CreatePayment (catroom.ir)
                              ↓
                    کاربر → درگاه → پرداخت
                              ↓
        POST /payments/gateway-hook (سرور-به-سرور) / Sweeper
                              ↓
                    payment.Complete → CAS status=pending/failed→verified
                              ↓
                    GameCreditor.AddCoins(botID, userID, coins)
                              ↓
              ┌───────────────┼───────────────┐
              ↓               ↓               ↓
        bombland: money  duel: energy   dating: coins
```

> **خریدار هرگز میزبانِ پنل (`sadbot.tfess.co.ir`) را نمی‌بیند.** صفحهٔ نتیجهٔ درگاه (catroom.ir) دکمهٔ «بازگشت به برنامه» ندارد و ریدایرکت پس از تسویه فقط برای callerهای زیرِ `catroom.ir` انجام می‌شود؛ برای صدبات، نتیجه از راه هوک سرور-به-سرور + sweeper اعلام و در چت بات ارسال می‌شود. `GET /payments/callback` فقط برای hit سروری باقی است.

## اجزا

### ۱. `payment.Gateway` — درگاه

- **آدرس**: `catroom.ir` (سازگار Zibal)
- **مبلغ**: تومان × ۱۰ = ریال (BIGINT)
- **متودها**:
  - `CreatePayment(ctx, orderID, amountTomans, callbackURL, notifyURL)` → `{PaymentID, PayURL}`
  - `Verify(ctx, orderID)` → `(verified bool, err)`
  - `Status(ctx, orderID)` → وضعیت خام از درگاه (داخل `Hook` برای استعلام مجدد؛ هوک هرگز payload را باور نمی‌کند)
- **Dev Mode**: `PAYMENT_DEV_MODE=true` + `PAYMENT_DEV_USERS` → پرداخت فوراً `dev://payment-success` برمی‌گرداند (فقط برای تست)

### ۲. `payment.Service` — منطق کسب‌وکار

```go
type Service struct {
    store       PaymentStore
    coins       CoinCreditor
    gateway     *Gateway
    notifier    Notifier
    callbackURL string
    notifyURL   string // از callbackURL قابل‌اشتقاق: /callback → /gateway-hook
}
```

**متودها**:
- `BuyPackage(botID, userID, telegramID, packageID, coins, priceTomans)` → `payURL`
- `BuySubscription(botID, userID, telegramID, priceTomans)` → `payURL`
- `Complete(paymentID)` — تایید + credit (CAS)
- `Hook(paymentID, event)` — هوک سرور-به-سرور؛ وضعیت را دوباره از درگاه استعلام می‌کند (payload هرگز باور نمی‌شود)
- `VerifyUserPayment(p)` / `VerifyLatestPending(botID, userID)` — استعلام دستی / sweeper

### ۳. `GameCreditor` — روتینگ اعتبار

```go
type GameCreditor struct {
    db       *pgxpool.Pool
    dating   CoinCreditor          // fallback
    bombland *postgres.BombLandRepo
    duel     *postgres.DuelRepo
}

func (g *GameCreditor) AddCoins(ctx, botID, userID, n) (int, error) {
    // روتینگ بر اساس bots.template_key:
    // bombland → BombLandRepo (money)
    // duel     → DuelRepo (energy)
    // default  → dating CoinCreditor (dating/welcome_menu/content/…)
}
```

### ۴. `payment.Sweeper` — نگهداری خودکار

| وظیفه | فاصله | توضیح |
|---|---|---|
| Auto-verify | ۱ دقیقه | پرداخت‌های <۱۵ دقیقه → verify |
| انقضای pending | ۱ دقیقه | pending‌های کهنه → failed (یک‌بار اطلاع) |
| انقضای اشتراک | ۶ ساعت | اشتراک‌های منقضی → expired |

## جریان خرید (از داخل بات)

```go
// در template.go
payURL, err := bc.PaymentService.BuyPackage(
    ctx,
    bc.BotID,
    bc.UserID,
    bc.TelegramID,
    "package_1",     // شناسه پکیج
    1000,            // تعداد سکه/پول
    5000,            // قیمت (تومان)
)
// ارسال لینک پرداخت به کاربر
bc.ReplyInline("💰 برای خرید کلیک کن:", [][]engine.InlineButton{
    {{Label: "پرداخت", URL: payURL}},
})
```

**قاعدهٔ پیام لینک پرداخت (همهٔ قالب‌ها):** لینکِ خام در متن پیام نمی‌آید — فقط دکمهٔ **«💳 پرداخت»** (URL-button خودِ لینک؛ `engine.InlineButton.URL`) + دکمهٔ بازگشت به منو. دکمهٔ «✅ بررسی پرداخت» حذف شده (جای آن را هوک + sweeper می‌گیرند). در dating متن هشدار VPN هم به «روی دکمه بزن» تغییر کرده (لینکی برای کپی نیست). پشتیبانی URL-button روی کت (cr0.ir) تأیید شده.

### کانال عملیات (`OPS_CHANNEL_ID`)

هر پرداختِ تسویه‌شده — از هر بات و هر پلتفرم — یک پیام در کانال `Payment-Catroom-Gateway` می‌گذارد: نام بات، پلتفرم و شناسهٔ خریدار، مبلغ به تومان، نوع (بستهٔ سکه/اشتراک) و زمان تهران. بات `@sadops_claude_bridge_bot` ادمین آن است.

- پیام **بعد از** `MarkVerified` و بعد از موفقیت اعتباردهی فرستاده می‌شود، پس پرداختِ ردشده پیامی نمی‌سازد و تحویل دوبارهٔ همان پرداخت پیام دوم نمی‌دهد.
- خطای ارسال فقط لاگ می‌شود و **هرگز** پرداخت را برنمی‌گرداند: خریدار پول داده و سکه‌اش باید بماند.
- خاموشی = کافی است `OPS_BOT_TOKEN` یا `OPS_CHANNEL_ID` خالی باشد (`NewOpsChannel` مقدار `nil` می‌دهد).
- **ناموفق‌ها هم گزارش می‌شوند** — رد شدن توسط درگاه و تمام شدن مهلت (sweeper) هر دو یک پیام «⚠️ پرداخت ناموفق» با علت می‌سازند، از هر باتی.

### دریافت گزارش از پلتفرم‌های دیگر (`OPS_REPORT_TOKEN`)

پلتفرم‌های بیرونی (فعلاً sadgpt.ir / sadai) پرداخت‌های خودشان را به این کانال می‌فرستند:

```
POST https://sadbot.tfess.co.ir/api/v1/ops/payment-report
Authorization: Bearer <OPS_REPORT_TOKEN>
Content-Type: application/json

{"platform":"sadai","status":"success","amount":294998,
 "user_id":"usr_9f2a","item":"شارژ کیف پول API",
 "detail":"۱۴۰۵/۰۸/۲۵ ۱۳:۴۰ — نرخ دلار ۲۹۴٬۹۹۸ تومان | مانده قبل: ۱۲٬۰۰۰ → بعد: ۳۰۶٬۹۹۸ | gateway: catroom | track: TRK-99182 | ref: RF-7712",
 "external_id":"RF-7712",
 "first_name":"کوشان","username":"kooshan","phone":"+989121234567"}
```

#### قرارداد sadai (نهایی‌شده با سشن sadai-chat، ۲۰۲۶-۱۱-۱۶)

| فیلد | اجباری | محتوا |
|---|---|---|
| `platform` | ✅ | همیشه `"sadai"` |
| `status` | ✅ | `"success"` یا `"failed"` |
| `amount` | ✅ | عدد صحیح **تومان**، `Math.max(0, round(...))` |
| `user_id` | ✅ | شناسهٔ کاربر در sadai |
| `item` | ✅ | ≤۲۰۰ کاراکتر — «شارژ کیف پول API» یا نام بسته |
| `detail` | ✅ | ≤۵۰۰ کاراکتر — تاریخ شمسی + ساعت تهران با ارقام فارسی، نرخ دلار، مانده قبل→بعد، gateway/track/ref |
| `external_id` | ✅ | `referenceId` درگاه، وگرنه `reference_id`/`track_id`/`payment.id` |
| `first_name` | ➖ | فقط وقتی `display_name` موجود است |
| `username` | ➖ | فقط وقتی موجود است |
| `phone` | ➖ | فقط وقتی موجود است؛ `0912…` به `+98912…` نرمال می‌شود |

فیلد دیگر نفرستید — `DisallowUnknownFields` است و `400` می‌دهد. `at` پذیرفته می‌شود ولی sadai از آن استفاده نمی‌کند (زمان دریافت ملاک است).

#### قواعد کلی روت

- `amount` به **تومان** است، نه ریال و نه دلار. منفی نباشد.
- **نام کاربر**: سطر «کاربر» از اولین مقدار موجود پر می‌شود: `first_name`+`last_name` → `user` → `@username` → `phone` → `user_id`. بقیه در «شناسهٔ کاربر» می‌آید تا نام تکراری هم قابل ردیابی باشد.
- سطر «جزئیات» همان `detail` را عیناً نمایش می‌دهد — RTL و ارقام فارسی سالم می‌مانند.
- روت **عمومی** است ولی با توکن قفل می‌شود (`subtle.ConstantTimeCompare`؛ هدر `X-Ops-Token` هم پذیرفته می‌شود). بدون توکن → `401`.
- بدنهٔ بیش از ۸KiB، فیلد ناشناس، یا مبلغ منفی → `400`. پاسخ موفق: `{"ok":true}`.
- اگر کانال تنظیم نشده باشد، `200` با `{"ok":true,"skipped":"..."}` برمی‌گردد تا فرستنده بی‌دلیل retry نکند.
- فراخوانی از سمت sadai **fire-and-forget** با timeout ۵ ثانیه است؛ خطا یا کندی این روت هرگز روی جریان پرداخت sadai اثر ندارد. گزارش به ازای هر تراکنش یک‌بار فرستاده می‌شود (idempotent).

نمونهٔ واقعیِ رسیده در کانال:

```
💳 پرداخت موفق
پلتفرم: سعدی (sadgpt.ir)
مبلغ: 294,998 تومان
کاربر: کوشان
شناسهٔ کاربر: @kooshan — +989121234567 — usr_9f2a
مورد: شارژ کیف پول API
جزئیات: ۱۴۰۵/۰۸/۲۵ ۱۳:۴۰ — نرخ دلار ۲۹۴٬۹۹۸ تومان | مانده قبل: ۱۲٬۰۰۰ → بعد: ۳۰۶٬۹۹۸ | gateway: catroom | track: TRK-99182 | ref: RF-7712
شمارهٔ تراکنش: RF-7712
زمان: 2026-11-16 13:47
```

## Callback و هوک

```
GET  /api/v1/payments/callback?pid=...   (hit سروری — خریدار اینجا نمی‌آید؛ ?orderId= هم پذیرفته می‌شود)
POST /api/v1/payments/gateway-hook  {orderId, status}   (سرور-به-سرور از درگاه)
```

- **عمومی** (بدون JWT)
- تایید سمت سرور (هرگز به client/هوک اعتماد نمی‌کند — وضعیت دوباره از درگاه استعلام می‌شود)
- **CAS**: `UPDATE payments SET status='verified' WHERE id=$1 AND status IN ('pending','failed')` — از `failed` هم تایید می‌شود (بازیابی پرداختی که دیر تسویه شده؛ credit فقط یک‌بار انجام می‌شود)
- سپس `GameCreditor.AddCoins`
- هوک گیرنده فقط wake-up است؛ `verified` دوباره `Complete` می‌شود و `failed` فقط وقتی خودِ درگاه هم failed باشد (ضد spoof)
- اندپوینت callback قدیمی برای سازگاری مانده؛ تسویهٔ خریداران از طریق هوک + sweeper انجام می‌شود

## ساختار پکیج (در prices)

```json
{
  "packages": [
    {"id": "package_1", "coins": 1000, "price": 5000, "label": "۱۰۰۰ سکه"},
    {"id": "package_2", "coins": 5000, "price": 20000, "label": "۵۰۰۰ سکه"}
  ]
}
```

## جدول مقایسهٔ ارز بازی‌ها

| بازی | ارز | مسیر credit | روش خرید |
|---|---|---|---|
| bombland | money | `bombland_players.money` | خرید پول |
| duel | energy | `duel_players.energy` | خرید انرژی |
| dating | coins | `dating_profiles.coins` | خرید سکه |
| tapearn | money | `tapearn_users.money` | ❌ (فقط بازی) |
| shlinks | balance | `shlinks_users.balance` | ❌ (فقط کلیک) |

## متغیرهای محیطی

```bash
PAYMENT_HOST_URL=https://catroom.ir   # میزبان درگاه (پیش‌فرض)
PAYMENT_CALLBACK_URL=https://...      # callback عمومی؛ notifyURL خودکار از /callback ← /gateway-hook
PAYMENT_DEV_MODE=true                 # فعال‌سازی حالت توسعه (verify آنی)
PAYMENT_DEV_USERS=906919989           # شماره‌های تلگرام مجاز در dev mode (کاما-جدا)
```

## تست

```go
// Dev mode (PAYMENT_DEV_MODE=true و کاربر در PAYMENT_DEV_USERS)
// — پرداخت فوراً تسویه می‌شود و URL خاصی به کاربر نشان داده نمی‌شود
service := payment.NewService(store, creditor, gateway, notifier, "http://localhost/callback")
payURL, _ := service.BuyPackage(ctx, botID, userID, tgID, "pkg1", 100, 1000)
// payURL = "dev://payment-success" — قالب‌ها آن را تشخیص داده و رسید فوری نشان می‌دهند
```

## الگوهای اتمیک

```sql
-- تایید پرداخت (CAS — از pending و failed)
UPDATE payments SET status='verified', verified_at=now()
  WHERE id=$1 AND status IN ('pending','failed');

-- credit سکه (بازی bombland)
UPDATE bombland_players SET money=money+$3
  WHERE bot_id=$1 AND user_id=$2;

-- credit انرژی (بازی duel)
UPDATE duel_players SET energy=energy+$3
  WHERE bot_id=$1 AND user_id=$2;
```
