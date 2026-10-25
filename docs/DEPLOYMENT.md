# دیپلوی و عملیات

> تاریخ: ۲۰۲۶-۱۰-۲۰ — production واقعی: سرور `212.87.199.140`

## ۱. دیپلوی production (سرور صدبات)

### مشخصات سرور

| آیتم | مقدار |
|---|---|
| هاست | `212.87.199.140` (نام: ULBIX، Ubuntu 24.04) |
| کاربر | `root` |
| دسترسی از ایران | **فقط از پشت SOCKS5** سیستم‌کاربر: `127.0.0.1:10808` |
| منابع | ۲ CPU / ۷.۷GB RAM |
| همزیست | سرویس ایمیل **Mailu** + nginx روی پورت‌های **80/443** — **هرگز دست نزنید** |

دستور SSH از طریق ساکس:

```bash
sshpass -p '<PASSWORD>' ssh \
  -o PreferredAuthentications=password -o PubkeyAuthentication=no \
  -o 'ProxyCommand nc -X 5 -x 127.0.0.1:10808 %h %p' \
  root@212.87.199.140
```

انتقال فایل: `rsync -a -e 'sshpass -p ... ssh -o ... ProxyCommand ...' ./ root@212.87.199.140:/opt/sadbot/`

### چیدمان استک (Docker Compose)

مسیر: `/opt/sadbot` — فایل: `docker-compose.prod.yml`

| کانتینر | تصویر | پورت میزبان | توضیح |
|---|---|---|---|
| `sadbot-db` | postgres:16-alpine | — (فقط شبکهٔ compose) | ولوم `sadbot_sadbot_pgdata` |
| `sadbot-backend` | build از `backend/` | — | مایگریشن خودکار در بوت + seed ادمین |
| `sadbot-frontend` | build از `frontend/` (nginx) | **8085 → 80** | پنل + proxy مسیر `/api/` به backend |
| `sadbot-tgsidecar` | build از `tgsidecar/` (FastAPI+Telethon) | — | اکانت‌های MTProto؛ ولوم `tg_sessions`؛ فقط با `X-Sidecar-Key` قابل دسترسی |
| `sadbot-renderer` | build از `renderer/` (FastAPI+Playwright) | — | رندر کارت PNG (HTML→PNG)؛ فقط از backend (`http://renderer:8788`) |

**پورت 8085 به‌جای 80**: پورت 80 متعلق به nginx موجود (Mailu) است.

### اسرار (`.env`)

`/opt/sadbot/.env` — تولیدشده توسط `deploy/server.env.sh` با `openssl rand`:

```
POSTGRES_PASSWORD=...
JWT_SECRET=...                 # 32 بایت hex
TOKEN_ENCRYPTION_KEY=...       # 32 بایت hex (AES-GCM توکن بات‌ها)
SEED_ADMIN_EMAIL=admin@sadbot.local
SEED_ADMIN_PASSWORD=...        # رمز ادمین پنل
TGSIDECAR_KEY=...              # کلید بک‌اند ↔ tgsidecar
TG_API_ID / TG_API_HASH        # اپلیکیشن تلگرام (MTProto)
PAYMENT_*                      # درگاه (PAYMENT_DEV_MODE برای تست)
```

هیچ‌کدام از این اسرار در مخزن نیستند و نباید باشند.

### استقرار / به‌روزرسانی

**جریان رسمی (تست‌ها اجباری‌اند):**

```bash
# از ماشین محلی — همهٔ تست‌ها، بعد ارسال، بعد deploy سرور
SSHPASS='<PASSWORD>' bash deploy/release.sh
```

دو گیت تست وجود دارد:

1. **گیت محلی** — `deploy/preflight.sh` (که release.sh اجرا می‌کند):
   - بک‌اند: `go vet ./...` + `go test ./...` (اگر `DATABASE_URL` تنظیم باشد، تست‌های integration دیتینگ هم اجرا می‌شوند)
   - فرانت: `vitest run` + `tsc -b` + `oxlint`
2. **گیت سرور** — داخل `docker build`: مرحلهٔ build در هر دو Dockerfile اول `go test` / `npm test` را اجرا می‌کند و فقط اگر پاس شود باینری/dist می‌سازد. پس حتی دستی `deploy.sh` هم بدون رد شدن تست‌ها تصویر نمی‌سازد.

در حالت اضطراری فقط: `SADBOT_SKIP_PREFLIGHT=1 SSHPASS=... bash deploy/release.sh` (گیت سرور همچنان اجباری است).

**دستی (بدون release.sh):**

```bash
# ۰) تست‌ها (الزامی)
bash deploy/preflight.sh

# ۱) ارسال تغییرات (از ماشین محلی، از پشت ساکس)
rsync -a -e '<ssh wrapper>' --exclude .git --exclude node_modules \
  --exclude dist --exclude kpkchat --exclude .env \
  ./ root@212.87.199.140:/opt/sadbot/

# ۲) build + up + health (روی سرور)
cd /opt/sadbot && bash deploy/deploy.sh
```

`deploy.sh`: `docker compose build` (تست‌ها داخل build اجرا می‌شوند) → `up -d` → انتظار health از `http://127.0.0.1:8085/api/health` → نمایش وضعیت کانتینرها.

### مایگریشن‌ها

باینری `cmd/migrate` داخل ایمیج backend هست و در `entrypoint.sh` قبل از استارت اپ اجرا می‌شود (`RUN_MIGRATIONS=1`). اولین بوت: ساخت جدول‌ها + seed superadmin از env.

### راستی‌آزمایی بعد از deploy

```bash
# wrapper آماده (رمز + SOCKS):  /tmp/sb-ssh — استفاده روی سرور:
/tmp/sb-ssh root@212.87.199.140 'curl -fsS http://127.0.0.1:8085/api/health'
```

**آدرس پنل**: `http://212.87.199.140:8085` — قبلاً: `https://sadbot.tfess.co.ir`

**بات‌ها بعد از deploy** (از ۲۰۲۶-۱۰-۱۸): دیگر استارت دستی لازم نیست — `StopAll` هنگام shutdown فقط context را کنسل میکند و `status` دیتابیس را دست نمیزند؛ پس بوت بعدی `StartRunningBots` همهٔ بات‌های `running` را خودش بالا میآورد. (قبلاً هر deploy همهٔ بات‌ها را `stopped` میکرد — باگ فلَت کردن کل فرست از این رفع شد.) اگر بعد از deploy باتی مرده بود: `docker logs sadbot-backend | grep runtime` و `SELECT telegram_username, status FROM bots;` را چک کنید؛ در صورت نیاز `UPDATE bots SET status='running'` + ری‌استارت backend.

**نکات اجرایی ثبت‌شده**:
- خروجی rsync را **بیرون از پایپ** چک کنید (خطای گذرا SSH ممکن است در لوله پنهان شود و deploy روی کد قدیمی برود)؛ قبل از `deploy.sh` وجود فایلهای جدید روی سرور را grep بزنید — و بعد از build داخل خود کانتینر هم verify کنید: `docker exec sadbot-backend strings /app/sadbot | grep <نماد_جدید>`.
- **rsync تک‌فایلی مسیر را flatten میکند**: `rsync a/b.go c/d.go host:/dest/` هر دو را در `/dest/` می‌ریزد (بدون والدین). کل دایرکتوری بفرستید، یا `--relative`، یا بعدش `mv` کنید (۲۰۲۶-۱۰-۱۸ پیش آمد و deploy بی‌صدا روی کد قدیمی رفت).
- `grep -c` روی صفر خروجی exit=1 میگیرد و زنجیرهٔ `&&` را می‌شکند — برای verify از الگویی استفاده کنید که میدانید هست، یا exit code را جدا چک کنید.
- اگر SSH با `Permission denied` رد شد ۳۰-۶۰ ثانیه بعد retry کنید — auth زیر فشار اتصالهای پشت‌سرهم موقتاً رد میشود (ssh/rsyncهای سریع متوالی این‌طوری میشوند).
- تست بات زنده: از اکانت MTProto پنل (`docker exec -i -e PYTHONPATH=/app sadbot-tgsidecar python3 -`) — دکمه‌های reply خودشان متن هستند؛ دکمه‌های inline با درخواست خام `GetBotCallbackAnswerRequest(peer, msg_id, data=b"act:...")` قابل فشردن‌اند (شیء دکمهٔ wrapper متد `click()` ندارد؛ callback data در `btn.type.data`). هنگام تست کارت مرور، آخرین پیام دارای `reply_markup` را بگیرید — کارتهای قدیمی در history با دکمه می‌مانند.

### runbook

| سناریو | اقدام |
|---|---|
| ری‌استارت استک | `docker compose -f docker-compose.prod.yml --env-file .env restart` |
| لاگ بک‌اند | `docker logs -f sadbot-backend` |
| وضعیت کانتینرها | `docker compose ... ps` |
| پشتیبان از دیتابیس | `docker exec sadbot-db pg_dump -U sadbot sadbot > backup_$(date +%F).sql` |
| بازگردانی | `docker exec -i sadbot-db psql -U sadbot sadbot < backup.sql` |
| توقف یک بات از پنل | دکمهٔ «توقف» در صفحهٔ بات (یا `POST /bots/{id}/stop`) |
| بات در status=error | `last_error` را در صفحهٔ بات ببینید؛ معمولاً توکن نامعتبر — توکن جدید بدهید |
| بات بعد از deploy «stopped» شد | (از ۲۰۲۶-۱۰-۱۸ دیگر عادی نیست — StopAll دیگر status را تغییر نمیدهد) اگر دیدید: `docker logs sadbot-backend \| grep runtime` + چک status در دیتابیس؛ در صورت نیاز `UPDATE bots SET status='running'` + ری‌استارت backend |
| پیام منقضی-شد تکرار میرفت | (2026-10-17 رفع شد) سوییپر با مقدار `expired` که CHECK رد میکند → ردیف برای همیشه pending → اسپم هر دقیقه. حالا CAS با `failed` + اطلاع فقط یک بار؛ 남اگرده: `UPDATE payments SET status='failed' WHERE status='pending' AND created_at < now()-interval '10 minutes'` |
| برای دیدن لاگ سوییپر | `docker logs sadbot-backend \| grep payment` |

### نکات ایمنی

- backend و db هیچ پورت میزبان ندارند؛ فقط از طریق nginx فرانت در دسترس‌اند.
- رمز عبور SSH را در اسکریپت‌ها هاردکد نکنید؛ از sshpass موقت یا کلید استفاده کنید (کلید نصب‌شده موجود، اما احراز با publickey توسط این سرور رد می‌شود — مسیر رمز پایدار است).
- به nginx/Mailu سرور دست نزنید؛ تغییر مسیر دامنه باید با شناسایی config موجود و با احتیاط انجام شود.

## ۲. دیپلوی محلی (توسعه)

```bash
# دیتابیس (Docker یا brew)
docker compose up -d          # یا: brew services start postgresql@16 && createdb sadbot

cd backend
cp .env.example .env          # DATABASE_URL، JWT_SECRET، TOKEN_ENCRYPTION_KEY
migrate -path migrations -database "$DATABASE_URL" up
go run ./cmd/sadbot           # :8080

cd frontend
npm install && npm run dev    # :5173 — proxy /api → :8080
```

تست‌ها:

```bash
# همه با هم (همان گیت قبل از دیپلوی):
bash deploy/preflight.sh

# یا جداگانه:
cd backend
go test ./...                 # unit + flow dating (نیازمند DATABASE_URL برای flow)
go vet ./...
cd ../frontend
npm test                      # vitest (utils + صفحهٔ ورود)
npx tsc -b
npm run lint
```

## ۳. ساختار فایل‌های داکر

| فایل | نقش |
|---|---|
| `docker-compose.prod.yml` | استک production (db + backend + frontend) |
| `backend/Dockerfile` | multi-stage: `golang:1.27-alpine` build → `alpine:3.20` runtime |
| `backend/deploy/entrypoint.sh` | انتظار db → migrate → اجرای sadbot |
| `backend/cmd/migrate/` | کلاینت مایگریشن با درایور postgres (جایگزین CLI رسمی که درایور ندارد) |
| `frontend/Dockerfile` | `node:22-alpine` build → `nginx:1.27-alpine` |
| `frontend/deploy/nginx.conf` | SPA + proxy `/api/` به `backend:8080` (با resolver دینامیک برای مقاومت به ری‌استارت) |
| `deploy/server.env.sh` | تولید `.env` با اسرار رندوم روی سرور |
| `deploy/deploy.sh` | build + up + health روی سرور |

> نکته: در `Dockerfile` بک‌اند، `golang:1.27-alpine` الزامی است چون `go.mod` نسخهٔ go 1.27.1 را می‌خواهد.
