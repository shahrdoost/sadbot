# سیستم پرداخت یکپارچه

> تمام بات‌های صدبات از **یک** سیستم پرداخت استفاده می‌کنند — بدون کپی کد.

## نمای کلی

```
کاربر → «💰 خرید» → payment.BuyPackage(botID, userID, packageID)
                              ↓
                    Gateway.StartPayment (catroom.ir)
                              ↓
                    کاربر → درگاه → پرداخت
                              ↓
        POST /payments/gateway-hook (سرور-به-سرور) / Sweeper
                              ↓
                    payment.Complete → CAS status=pending→verified
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
  - `StartPayment(amountRials, callbackURL, description)` → `{payURL, trackID}`
  - `VerifyPayment(trackID, amountRials)` → `{verified, refID}`
- **Dev Mode**: بدون `GATEWAY_MERCHANT` → پرداخت شبیه‌سازی‌شده (فقط برای تست)

### ۲. `payment.Service` — منطق کسب‌وکار

```go
type Service struct {
    store     PaymentStore
    coins     CoinCreditor
    gateway   *Gateway
    notifier  Notifier
    callback  string
}
```

**متودها**:
- `BuyPackage(botID, userID, telegramID, packageID, coins, priceTomans)` → `payURL`
- `BuySubscription(botID, userID, telegramID, priceTomans)` → `payURL`
- `Complete(paymentID)` — تایید + credit (CAS)
- `HandleCallback(trackID, status)` — callback عمومی

### ۳. `GameCreditor` — روتینگ اعتبار

```go
type GameCreditor struct {
    db      *pgxpool.Pool
    dating  CoinCreditor  // fallback
}

func (g *GameCreditor) AddCoins(ctx, botID, userID, n) (int, error) {
    // روتینگ بر اساس bots.template_key:
    // bombland → UPDATE bombland_players SET money=money+$n
    // duel     → DuelRepo.AddEnergy
    // default  → dating CoinCreditor
}
```

### ۴. `payment.Sweeper` — نگهداری خودکار

| وظیفه | فاصله | توضیح |
|---|---|---|
| Auto-verify | ۱ دقیقه | پرداخت‌های <۱۰ دقیقه → verify |
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
GET  /api/v1/payments/callback?trackId=...&status=...   (hit سروری — خریدار اینجا نمی‌آید)
POST /api/v1/payments/gateway-hook  {orderId, status}   (سرور-به-سرور از درگاه)
```

- **عمومی** (بدون JWT)
- تایید سمت سرور (هرگز به client/هوک اعتماد نمی‌کند — وضعیت دوباره از درگاه استعلام می‌شود)
- **CAS**: `UPDATE payments SET status='verified' WHERE id=$1 AND status='pending'`
- سپس `GameCreditor.AddCoins`
- هوک گیرنده فقط wake-up است؛ `verified` دوباره `Complete` می‌شود و `failed` فقط وقتی خودِ درگاه هم failed باشد (ضد spoof)
- اندپوینت callback قدیمی برای سازگاری مانده؛ تسویهٔ خریداران از طریق هوک + sweeper انجام می‌شود

## ساختار پکیج (در prices)

```json
{
  "packages": [
    {"id": "package_1", "coins": 1000, "price_toman": 5000, "label": "۱۰۰۰ سکه"},
    {"id": "package_2", "coins": 5000, "price_toman": 20000, "label": "۵۰۰۰ سکه"}
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
GATEWAY_MERCHANT=...        # کد پذیرنده catroom.ir
GATEWAY_MODE=production|dev
CALLBACK_URL=https://...    # آدرس callback عمومی
```

## تست

```go
// Dev mode بدون merchant — پرداخت شبیه‌سازی‌شده
service := payment.NewService(store, creditor, nil, notifier, "http://localhost/callback")
payURL, _ := service.BuyPackage(ctx, botID, userID, tgID, "pkg1", 100, 1000)
// payURL = "http://localhost/callback?trackId=test&status=OK"
```

## الگوهای اتمیک

```sql
-- تایید پرداخت (CAS)
UPDATE payments SET status='verified', verified_at=now()
  WHERE id=$1 AND status='pending';

-- credit سکه (بازی bombland)
UPDATE bombland_players SET money=money+$3
  WHERE bot_id=$1 AND user_id=$2;

-- credit انرژی (بازی duel)
UPDATE duel_players SET energy=energy+$3
  WHERE bot_id=$1 AND user_id=$2;
```
