# مستندات صدبات

فهرست مستندات پلتفرم مدیریت بات تلگرامی «صدبات».

| سند | موضوع |
|---|---|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | معماری کلی: لایه‌های backend، موتور config-driven، ران‌تایم تلگرام، فرانت‌اند |
| [DATABASE.md](./DATABASE.md) | اسکیمای کامل Postgres — جدول‌ها، ایندکس‌ها، قواعد |
| [API.md](./API.md) | مرجع REST API + ماتریس دسترسی (RBAC) |
| [DATING_TEMPLATE.md](./DATING_TEMPLATE.md) | تمپلیت دوستیابی — پورت کامل kpkchat: چت، مرور، اقتصاد سکه، state machine، مچر |
| [CONTENT_TEMPLATE.md](./CONTENT_TEMPLATE.md) | تمپلیت محتوا — پورت sadcontent: جمع‌آوری از فولدر تلگرام، AI، رندر کارت، انتشار در کانال |
| [GAME_TEMPLATES.md](./GAME_TEMPLATES.md) | تمام تمپلیت‌های بازی: بومبلند، سنگ‌کاغذ‌قیچی، فراوانی، لینک‌ساز، کی منو دیده، چت ناشناس، تپ‌کلیک، فال |
| [PAYMENT.md](./PAYMENT.md) | سیستم پرداخت یکپارچه — درگاه catroom.ir، GameCreditor، سوییپر |
| [DEPLOYMENT.md](./DEPLOYMENT.md) | دیپلوی production روی سرور، داکر، runbook عملیاتی |
| [kpkchat-analysis.md](./kpkchat-analysis.md) | تحلیل کامل پروژهٔ مرجع kpkchat (مرجع رفتاری تمپلیت dating) |

## یک‌نمای پروژه

```
sadbot/
├── backend/     # Go — API، ران‌تایم بات‌ها، موتور، تمپلیت‌ها
├── frontend/    # React + Redux — پنل فارسی RTL
├── tgsidecar/   # FastAPI + Telethon — اکانت‌های MTProto و ساخت بات با BotFather
├── renderer/    # سرویس رندر کارت (Playwright) — محتوا
├── deploy/      # اسکریپت‌های دیپلوی سرور
└── docs/        # همین مستندات
```

## تمپلیت‌های موجود (۱۱ قالب)

| قالب | نوع | توضیح |
|---|---|---|
| `dating` | دوستیابی | چت، مرور، اقتصاد سکه، مچر (پورت kpkchat) |
| `content` | محتوا | جمع‌آوری از فولدر + AI + رندر + انتشار کانال |
| `welcome_menu` | منو | منوی ساده با دکمه و صحنه |
| `bombland` | بازی | کارخانه: تولید، فروش، ارتقا، دعوت |
| `duel` | بازی | سنگ‌کاغذ‌قیچی نفر‌به‌نفر + انرژی + اپال |
| `favaani` | بازی | بازی فراوانی: چالش روزانه + اسپند لاگ |
| `shlinks` | ابزار | لینک‌ساز: ساخت لینک کوتاه + درآمد کلیک |
| `whoseeme` | ابزار | کی منو دیده: لینک طعمه + ردیابی بازدیدکننده |
| `faal` | سرگرمی | فال روزانه + فال عشق/پول/سلامت |
| `anonychat` | اجتماعی | چت ناشناس: جستجو، مچ، رله پیام |
| `tapearn` | بازی | تپ‌کلیک: tap-to-earn با انرژی و ارتقا |

## سه تصمیم معماری

1. **Postgres تنها source of truth** — بدون Redis؛ state ناوبری کاربران در JSONB.
2. **موتور واحد config-driven** — رفتار هر بات = یک Template Go + پیکربندی JSON در دیتابیس؛ افزودن بات جدید بدون deploy کد.
3. **سیستم پرداخت یکپارچه** — تمام بات‌ها از یک `payment.Service` + `GameCreditor` استفاده می‌کنند؛ بدون کپی کد.
