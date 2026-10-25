# تمپلیت‌های بازی

> ۸ تمپلیت بازی/ابزار/سرگرمی در صدبات — همه با سیستم پرداخت یکپارچه ([PAYMENT.md](./PAYMENT.md))

## ۱. بومبلند (bombland) — کارخانه

**مکانیک**: بازی idle factory — تولید بمب، فروش، ارتقا، دعوت دوستان.

| عمل | توضیح |
|---|---|
| `/start` | نمایش وضعیت: بمب‌ها، پول، تولید خودکار |
| تولید | هر بار = +N بمب (بر اساس auto_lvl) |
| فروش | فروش همه بمب‌ها → پول |
| ارتقا | افزایش auto_lvl با پول |
| دعوت | لینک دعوت → پاداش |
| خرید پول | از طریق درگاه (`buy_money`) |

**قیمت‌ها (`prices`)**:
- `money_package_1`, `money_package_2`, ... (تومان)
- `upgrade_base_cost`, `upgrade_multiplier`

**Repo**: `BombLandRepo` — `EnsurePlayer`, `SellAll`, `TryUpgrade`, `CatchUpAuto`, `SetInvitedBy`

---

## ۲. سنگ‌کاغذ‌قیچی (duel) — مبارزه نفر‌به‌نفر

**مکانیک**: بازی RPS با انرژی، آیتم‌ها، اپال، برداشت نقدی.

| عمل | توضیح |
|---|---|
| `/start` | نمایش: امتیاز، انرژی، اپال، آیتم‌ها |
| بازی | انتخاب حریف → سنگ/کاغذ/قیچی → برنده/بازنده/مساوی |
| جایزه | شانس: آیتم (candy/chocolate/soda)، اپال، امتیاز |
| انرژی | شارژ خودکار + خرید انرژی |
| برداشت | تبدیل امتیاز → نقد (cash/physical) |

**قیمت‌ها**:
- `energy_price_toman`, `energy_amount`
- `opal_price_toman`, `score_to_toman_rate`

**Repo**: `DuelRepo` — `RechargeEnergy`, `SpendEnergy`, `AddItems`, `ConsumeItems`, `AddOpals`, `FindRival`, `TopPlayers`, `EnsureBots`, `withdrawals`

---

## ۳. بازی فراوانی (favaani) — abundance game

**مکانیک**: چالش روزانه با ثبت اسپند (لاگ هزینه) + امتیاز + استریک.

| عمل | توضیح |
|---|---|
| `/start` | نمایش روز، امتیاز، استریک |
| چالش | ثبت مبلغ اسپند امروز |
| امتیاز | بر اساس مبلغ و استریک |
| دعوت | کد دعوت → پاداش |

**Repo**: `FavaaniRepo` — `EnsureUser`, `GetUser`, `AddChallenge`, `GetChallenges`, `SetInvitedBy`

---

## ۴. لینک‌ساز (shlinks) — link shortener

**مکانیک**: ساخت لینک کوتاه + درآمد از کلیک + برداشت.

| عمل | توضیح |
|---|---|
| `/start` | نمایش موجودی، لینک‌ها |
| ساخت لینک | URL → لینک کوتاه |
| کلیک | هر کلیک معتبر = +N تومان |
| برداشت | برداشت موجودی |

**قیمت‌ها**: `coins_per_click`, `min_withdrawal`, `withdrawal_fee`

**Repo**: `ShlinksRepo` — `CreateLink`, `RecordClick`, `GetUser`, `RequestWithdrawal`

---

## ۵. کی منو دیده (whoseeme) — bait link

**مکانیک**: ساخت لینک طعمه + ردیابی بازدیدکننده + وظایف.

| عمل | توضیح |
|---|---|
| `/start` | نمایش موجودی، لینک‌ها |
| ساخت لینک | عنوان + URL → لینک طعمه |
| ردیابی | هر بازدید = +سکه |
| وظایف | دعوت، اشتراک‌گذاری |
| برداشت | برداشت موجودی |

**قیمت‌ها**: `coins_per_click`, `invite_bonus_coins`, `base_url`

**Repo**: `WhoseemeRepo` — `EnsureUser`, `CreateLink`, `RecordClick`, `GetLinks`, `RequestWithdrawal`

---

## ۶. فال روزانه (faal) — fortune

**مکانیک**: فال روزانه + فال‌های موضوعی (عشق، پول، سلامت).

| عمل | توضیح |
|---|---|
| `/start` | فال امروز + دکمه‌ها |
| فال عشق | فال عاشقانه |
| فال پول | فال مالی |
| فال سلامت | فال سلامت |
| `/stats` | آمار فال‌ها |

**قیمت‌ها**: `fortunes: string[]`, `fortune_cooldown_hours`

**Repo**: `FaalRepo` — `EnsureUser`, `GetUser`, `AddFortune`, `GetFortunes`

---

## ۷. چت ناشناس (anonychat) — anonymous chat

**مکانیک**: جستجوی هم‌صحبت → چت ناشناس → پایان/گزارش.

| عمل | توضیح |
|---|---|
| `/start` | شروع جستجو |
| جستجو | یافتن کاربر آنلاین |
| چت | رله پیام بین دو طرف |
| پایان | پایان چت + امتیازدهی |
| گزارش | گزارش کاربر نامناسب |

**قیمت‌ها**: `search_timeout_seconds`

**Repo**: `AnonyChatRepo` — `FindPartner`, `MatchUsers`, `EndChat`, `ReportUser`

---

## ۸. تپ‌کلیک (tapearn) — tap-to-earn

**مکانیک**: بازی tap-to-earn با انرژی، ارتقا قدرت تپ.

| عمل | توضیح |
|---|---|
| `/start` | نمایش: پول، انرژی، قدرت تپ |
| تپ | هر تپ = +tap_power سکه (۱ انرژی) |
| ارتقا | افزایش tap_power با پول |
| آمار | نمایش آمار کامل |

**قیمت‌ها**: `tap_power_base`, `max_energy_base`, `recharge_per_minute`, `upgrade_base_cost`, `upgrade_cost_multiplier`

**Repo**: `TapEarnRepo` — `EnsureUser`, `RechargeEnergy`, `Tap`, `UpgradePower`

---

## ساختار مشترک

### BotContext (دسترسی تمپلیت)

```go
bc.Reply(text)                    // پاسخ ساده
bc.ReplyInline(text, buttons)     // پاسخ با کیبورد inline
bc.ReplyKBD(text, buttons)        // پاسخ با کیبورد reply
bc.SendTo(chatID, text)           // ارسال به چت دیگر
bc.State["key"] = value           // state کاربر
bc.Data["repo"]                   // DI: repo مربوطه
bc.PriceInt("key", default)       // خواندن قیمت/تنظیمات
bc.Text("key")                    // خواندن متن
```

### Config (در bot_configs.prices)

هر تمپلیت قیمت‌ها و تنظیمات خود را در `prices` نگه می‌دارد:

```json
{
  "tap_power_base": 1,
  "max_energy_base": 100,
  "recharge_per_minute": 5,
  "upgrade_base_cost": 1000,
  "upgrade_cost_multiplier": 2
}
```

### Frontend

تب‌های هر بات بازی در پنل:
- **بازی**: تنظیمات قیمت‌ها (GameSettingsTab)
- **بازیکنان**: لیست کاربران + آمار (GamePlayersTab)
- **برداشت‌ها**: درخواست‌های برداشت (WithdrawalsTab)

---

## جدول مقایسه

| بازی | ارز اصلی | انرژی | ارتقا | برداشت | پرداخت |
|---|---|---|---|---|---|
| bombland | money | ❌ | ✅ | ❌ | ✅ خرید پول |
| duel | score + opal | ✅ | ✅ | ✅ | ✅ خرید انرژی |
| favaani | score | ❌ | ❌ | ❌ | ❌ |
| shlinks | balance (تومان) | ❌ | ❌ | ✅ | ❌ |
| whoseeme | coins | ❌ | ❌ | ✅ | ❌ |
| faal | fortunes | ❌ | ❌ | ❌ | ❌ |
| anonychat | ❌ | ❌ | ❌ | ❌ | ❌ |
| tapearn | money | ✅ | ✅ | ❌ | ❌ |
