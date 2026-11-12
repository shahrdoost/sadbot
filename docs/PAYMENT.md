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
