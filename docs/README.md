# مستندات صدبات

فهرست مستندات پلتفرم مدیریت بات تلگرامی «صدبات».

| سند | موضوع |
|---|---|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | معماری کلی: لایه‌های backend، موتور config-driven، ران‌تایم تلگرام، فرانت‌اند |
| [DATABASE.md](./DATABASE.md) | اسکیمای کامل Postgres — جدول‌ها، ایندکس‌ها، قواعد |
| [API.md](./API.md) | مرجع REST API + ماتریس دسترسی (RBAC) |
| [DATING_TEMPLATE.md](./DATING_TEMPLATE.md) | تمپلیت دوستیابی — پورت کامل kpkchat: چت، مرور، اقتصاد سکه، state machine، مچر |
| [CONTENT_TEMPLATE.md](./CONTENT_TEMPLATE.md) | تمپلیت محتوا — پورت sadcontent: جمع‌آوری از فولدر تلگرام، AI، رندر کارت، انتشار در کانال |
| [DEPLOYMENT.md](./DEPLOYMENT.md) | دیپلوی production روی سرور، داکر، runbook عملیاتی |
| [kpkchat-analysis.md](./kpkchat-analysis.md) | تحلیل کامل پروژهٔ مرجع kpkchat (مرجع رفتاری تمپلیت dating) |

## یک‌نمای پروژه

```
sadbot/
├── backend/     # Go — API، ران‌تایم بات‌ها، موتور، تمپلیت‌ها
├── frontend/    # React + Redux — پنل فارسی RTL
├── tgsidecar/   # FastAPI + Telethon — اکانت‌های MTProto و ساخت بات با BotFather
├── deploy/      # اسکریپت‌های دیپلوی سرور
└── docs/        # همین مستندات
```

**سه تصمیم معماری که همهٔ مستندات بر پایهٔ آنهاست:**

1. **Postgres تنها source of truth** — بدون Redis؛ state ناوبری کاربران در JSONB.
2. **موتور واحد config-driven** — رفتار هر بات = یک Template Go + پیکربندی JSON در دیتابیس؛ افزودن بات جدید بدون deploy کد.
3. **درس‌های kpkchat** — اقدام مالی/مشترک اتمیک (قفل + CAS + rollback)، روتینگ با action key پایدار نه متن دکمه، پول فقط BIGINT ریال.
