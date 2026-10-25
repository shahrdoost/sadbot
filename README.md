# صدبات (Sadbot)

پلتفرم مرکزی مدیریت چند بات تلگرامی — بساز، اجرا کن، مدیریت کن.

صدبات یک سرور واحد است که چند بات تلگرامی را همزمان اجرا می‌کند؛ هر بات اطلاعات، کاربران، فعالیت و خریدهای خودش را دارد و همه در یک دیتابیس Postgres ذخیره می‌شوند. یک پنل مدیریت فارسی (RTL) برای داشبورد مالی، مدیریت بات‌ها، کاربران هر بات، پرداخت‌ها و RBAC بین مدیران وجود دارد.

## امکانات

- **ران‌تایم کامل**: هر بات با توکنی که از پنل وارد می‌کنی، مستقیماً داخل صدبات (long-polling در Go) اجرا می‌شود.
- **تمپلیت‌ها**: رفتار هر بات از یک Template Go + پیکربندی JSON در دیتابیس ساخته می‌شود.
  - `welcome_menu` — بات نمونه (منوی خوش‌آمد + تنظیمات با state ماندگار)
  - `dating` — بازنویسی کامل kpkchat: onboarding فارسی + عکس، سرچ ناشناس زمان‌دار (غریبه/دختر/پسر/پیشرفته)، مچینگ با امتیاز شباهت هر ۵ ثانیه + گوست‌ها، چت ناشناس با ریپلای و رسانه، browse سبک Tinder (لایک/سوپرلایک/چت مستقیم) + مرور فیلترشده، اقتصاد سکه (پکیج/اشتراک/کانال/دعوت/تخفیف)، اعتراف ناشناس و پشتیبانی، چت با دستیار هوشمند، صفحهٔ پروفایل با ویرایش کامل — منوهای رنگی مثل kpkchat
- **پنل مدیریت فارسی (RTL)**: داشبورد مالی، کاربران + جزئیات (سکه/ریست/مسدود/پیام مستقیم)، چت‌ویور، برادکست با عکس، کانالها، هارنس AI، تنظیمات عددی، سلامت و ریست، تبهای اعتراف/تیکت، دارک‌مود.
- **MTProto (tgsidecar)**: اکانت تلگرامی برای ساخت بات از BotFather، مشاهدهٔ چتها و ارسال پیام از پنل (FastAPI + Telethon کنار بک‌اند).
- **پرداخت واقعی**: درگاه catroom.ir (سازگار Zibal) + callback عمومی + auto-verify سوییپر + dev-mode برای تست.
- **RBAC**: superadmin و مدیر؛ هر مدیر به هر بات دسترسی‌های جدا می‌گیرد (`bot:read`, `bot:users`, `bot:config`, `bot:finance`, `bot:runtime`).
- **بخش مالی**: درآمد کل سیستم + جدول درآمد به تفکیک بات + ثبت پرداخت دستی.
- **آنالیتیکس per-bot**: کاربران با جستجو، لاگ فعالیت (پیام/دستور/تپ دکمه)، آمار ۳۰ روزه، اسکن بلاک تلگرامی.
- **امنیت**: توکن بات‌ها با AES-GCM رمزنگاری می‌شود؛ توکن قبل از ذخیره با `getMe` تلگرام اعتبارسنجی می‌شود.

## ساختار پروژه

```
sadbot/
├── backend/          # Go — API + ران‌تایم تلگرام + موتور بات
│   ├── cmd/sadbot/   # نقطهٔ ورود
│   ├── internal/
│   │   ├── api/          # chi router + JWT + RBAC
│   │   ├── engine/       # موتور config-driven + Template registry
│   │   ├── dating/       # تمپلیت دوستیابی (پورت kpkchat)
│   │   ├── telegram/     # کلاینت دست‌ساز Bot API + poller/manager
│   │   ├── runtime/      # چرخهٔ حیات بات‌ها (start/stop/backoff)
│   │   ├── service/      # منطق کسب‌وکار
│   │   ├── repository/   # دسترسی Postgres (pgx)
│   │   └── domain/       # استراکت‌های خالص
│   └── migrations/   # golang-migrate
├── frontend/         # Vite + React + Redux Toolkit + Tailwind (فارسی RTL)
├── deploy/           # اسکریپت‌های دیپلوی سرور
└── docs/             # مستندات کامل — فهرست در [docs/README.md](docs/README.md)
```

## مستندات

| سند | موضوع |
|---|---|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | معماری backend، موتور، ران‌تایم، فرانت |
| [docs/DATABASE.md](docs/DATABASE.md) | اسکیمای کامل Postgres |
| [docs/API.md](docs/API.md) | مرجع REST API + ماتریس RBAC |
| [docs/DATING_TEMPLATE.md](docs/DATING_TEMPLATE.md) | رفتار تمپلیت دوستیابی (پورت kpkchat) |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | دیپلوی production و runbook |
| [docs/kpkchat-analysis.md](docs/kpkchat-analysis.md) | تحلیل پروژهٔ مرجع kpkchat |

## راه‌اندازی

### پیش‌نیاز

- Go 1.22+، Node 20+، PostgreSQL 16 (محلی یا Docker)

### دیتابیس

با Docker:

```bash
docker compose up -d
```

یا Postgres محلی (مثل brew):

```bash
brew install postgresql@16
brew services start postgresql@16
createuser -s sadbot
createdb -O sadbot sadbot
```

### راه‌اندازی backend

```bash
cd backend
cp .env.example .env
# .env را پر کن — TOKEN_ENCRYPTION_KEY: openssl rand -hex 32
export $(grep -v '^#' .env | xargs)
migrate -path migrations -database "$DATABASE_URL" up
make seed          # ساخت superadmin از SEED_ADMIN_*
make backend       # اجرا روی :8080
```

### راه‌اندازی frontend

```bash
cd frontend
npm install
npm run dev        # http://localhost:5173 (proxy به :8080)
```

### ورود

- ایمیل/رمز پیش‌فرض از `.env`: `SEED_ADMIN_EMAIL` / `SEED_ADMIN_PASSWORD`
- بعد از ورود: از «بات‌ها» یک بات بسازید (توکن از @BotFather)، تمپلیت را انتخاب و «اجرا» بزنید.

### تست‌ها

```bash
cd backend
make test          # unit + تست جریان کامل dating با Postgres واقعی
make vet
```

## API خلاصه

| مسیر | توضیح |
|---|---|
| `POST /api/v1/auth/login` | ورود، دریافت JWT |
| `GET/POST /api/v1/bots` | لیست/ساخت بات (ساخت = superadmin) |
| `POST /api/v1/bots/{id}/start` · `stop` | کنترل ران‌تایم |
| `GET/PUT /api/v1/bots/{id}/config` | متن‌ها و منوهای JSON هر بات (نسخه‌دار) |
| `GET /api/v1/bots/{id}/users` · `events` | کاربران و لاگ فعالیت |
| `GET/POST /api/v1/bots/{id}/payments` | پرداخت‌ها (ثبت دستی) |
| `GET /api/v1/finance/overview` | مالی جمعی سوپرادمین |
| `PUT /api/v1/admins/{id}/bot-access/{botId}` | دادن دسترسی به مدیر |

## تصمیم‌های معماری (درس‌های kpkchat)

- **یک حقیقت ماندگار**: Postgres — بدون Redis. state ناوبری هر کاربر در JSONB (`bot_end_users.state`) و reconcile در هر پیام.
- **اقدام مالی/مشترک اتمیک**: مچینگ با `SELECT … FOR UPDATE SKIP LOCKED` + آپدیت شرطی (CAS) + rollback.
- **روتینگ با label**: تغییر نام دکمه توسط ادمین روت را نمی‌شکند.
- **پول فقط BIGINT ریال**؛ نمایش تومان در فرانت.

تحلیل کامل kpkchat (مرجع رفتاری تمپلیت dating): [docs/kpkchat-analysis.md](docs/kpkchat-analysis.md)
