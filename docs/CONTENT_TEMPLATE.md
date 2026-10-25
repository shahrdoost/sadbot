# تمپلیت محتوا (`content`)

> کد: `backend/internal/content/` — template.go، collector.go، prompts.go، renderer.go  
> سرویس رندر: `renderer/` (کانتینر جدا، Playwright + Chromium)  
> مرجع رفتاری: سورس sadcontent (`~/Desktop/Mimo/sadcontent`)  
> قرارداد: **فیلترها، پرامپت‌های AI، NO_CONTENT، خلاصهٔ کارت و fallback رندر عین sadcontent**؛ اینستاگرام خارج از دامنه.  
> وضعیت: پیاده‌سازی کامل — آمادهٔ ساخت بات از پنل.

## نمای کلی

هر باتِ `content` یک **برند محتوا** است: اخبار را از فولدر تلگرامِ یک اکانت MTProto مشترک جمع می‌کند، با AI پاک‌سازی می‌کند، کارت PNG با هویت بصری برند می‌سازد و در کانال مقصد خودش پست می‌کند. چند بات = چند برند/کانال مستقل.

```
هر content_poll_minutes (پیش‌فرض ۲ دقیقه) per-bot:
Collector (گوروتین سراسری داخل backend)
  │ ListRunningByTemplate("content")
  ├─ tgsidecar GET /folders ──▶ کانال‌های فولدر per-bot (fail-closed)
  ├─ tgsidecar GET /messages ──▶ پیام‌های جدید (dedup با content_posts)
  ├─ llm.Chat(clean_prompt) ──▶ متن تمیز (NO_CONTENT = حذف؛ خطا = متن خام)
  ├─ llm.Chat(card_prompt)  ──▶ خلاصهٔ کارت (اختیاری)
  ├─ renderer POST /render  ──▶ post.png + story.png
  └─ SendPhotoUpload ──▶ کانال مقصد (کپشن = متن کامل[:1000]) + ثبت content_posts
```

بدون صف/پیسینگ — انتشار تلگرام فوری است (صف و پیسینگ در sadcontent فقط بخش اینستاگرام بود).

## ساخت بات از پنل

1. **اکانت MTProto**: تب «اکانت تلگرامی» — لاگین با شماره (sidecar). یک اکانت می‌تواند منبع چند بات باشد.
2. **ساخت بات**: `/bots` → قالب «محتوا (اخبار کانال)» → توکن بات از BotFather.
3. **ادمین کردن بات** در کانال مقصد (Post Messages).
4. **تنظیمات** از **تب «📗 محتوا و رخدادها»** — همه داینامیک:
   - **مبدأ و مقصد**: سلکت اکانت → پوشه (فولدر، زنده از sidecar) → کانال مقصد (لیست کانال‌های اکانت + ورود دستی chat_id)؛ لیست **مستثنی‌ها** و **کلیدواژه‌ها**؛ فیلترها (poll/backfill/min_length/skip_forwarded/skip_replies/خلاصهٔ کارت)
   - **هویت برند**: نام لاتین/فارسی/هندل + ۴ رنگ (pickers رنگ) با پیش‌نمایش زنده
   - **پرامپت‌های AI**: دو textarea (CLEAN/CARD) + دکمهٔ «بازگردانی پیش‌فرض‌ها» (خالی = پیش‌فرض قالب)
   - **رخدادها**: آمار (منتشرشده/بدون‌محتوا/ناموفق/۲۴ ساعت) + جدول پیج‌شدهٔ `content_posts` با فیلتر وضعیت و جست‌وجو، لینک منبع و خطا

   ذخیره با کنترل نسخه (version) انجام می‌شود؛ تغییر مبدأ/مقصد از **پُلِ بعدی** اعمال می‌گردد. JSON خام همچنان در تب «تنظیمات و منو» در دسترس است.

   کلیدهای `prices` که فرم بالا می‌نویسد:

| کلید | پیش‌فرض | توضیح |
|---|---|---|
| `content_account_id` | `""` | شناسهٔ اکانت MTProto (sidecar) |
| `content_folder` | `"Crypto"` | عنوان فولدر تلگرام منبع (**fail-closed**: نبود = هیچ‌چیز جمع نمی‌شود) |
| `content_channel` | `""` | کانال مقصد (`@username` یا `-100…`) |
| `content_publish_via` | `"bot"` | `bot` = فرستادن با Bot API (بات باید ادمین مقصد باشد)؛ `sidecar` = فرستادن از حساب MTProto انتخاب‌شده با Telethon (بدون نیاز به ادمین‌کردن بات — همان مسیر قدیمی sadcontent). از پنل: «نحوهٔ انتشار» |
| `content_text_only` | `false` | `true` = خروجی **فقط متن** (بدون کارت/عکس/رندر/خلاصه؛ هیچ خطایی به متن خام برنمی‌گردد). سقف کپشن ۴۰۹۶ و `brand_handle` **بعد از برش** به انتهای پیام اضافه می‌شود تا در پست‌های بلند تبلیغ گم نشود. از پنل: «فقط متن (بدون کارت/عکس)» — با فعال‌شدن، بلاک برند و پرامپت CARD پنهان می‌شوند |
| `content_poll_minutes` | `2` | فاصلهٔ poll |
| `content_backfill_limit` | `20` | در اولین اجرا حداکثر چند پست آخر هر کانال |
| `content_min_length` | `40` | حداقل طول متن |
| `content_skip_forwarded` | `false` | |
| `content_skip_replies` | `false` | |
| `content_exclude` | `[]` | کانال‌های منبع که جمع نمی‌شوند |
| `content_keywords` | `[]` | خالی = همه؛ غیرخالی = فقط حاوی این کلمات |
| `render_card_summary` | `true` | خلاصهٔ AI داخل کارت (کپشن همیشه متن کامل) |
| `brand_key` / `brand_name_en` / `brand_name_fa` / `brand_handle` | | هویت برند روی کارت |
| `brand_glow_a/b/c` / `brand_accent` | | رنگ‌های گلو و هایلایت اعداد |

و کلیدهای `texts`:
| کلید | توضیح |
|---|---|
| `clean_prompt` | پرامپت پاک‌سازی (پیش‌فرض = عین sadcontent) |
| `card_prompt` | پرامپت خلاصهٔ کارت |

5. **استارت** بات از پنل.

## کلون بات (یک‌کلیکی)

دکمهٔ «⧉ کلون بات» در هدر صفحهٔ بات (یا دکمهٔ کلون داخل تب محتوا):

- بات جدید را از طریق **BotFather روی اکانت انتخابی** می‌سازد (همان flow ساخت عادی؛ ۳۰–۶۰ ثانیه)
- کل تنظیمات (قیمت‌ها، متن‌ها، منوها، feature_flags همان بات) **کپی** می‌شود
- عمداً **`content_account_id` و `content_channel` خالی می‌مانند** — کلون تا انتخاب مبدأ/مقصدِ خود هیچ‌چیز نمی‌خواند و نمی‌فرستد (رفع خطر انتشار در کانالِ مبدأ)
- امتیاز: فقط superadmin (مثل ساخت/حذف بات)؛ audit با action `bot.clone_config`
- اگر کپی config شکستخورده باشد، بات ساخته‌شده برمی‌گردد و `config_error` در پاسخ می‌آید (پیام هشدار در UI)

`POST /api/v1/bots/{botID}/clone` — body: `{account_id, name?, username_hint?}` → `201 {bot, config_error?}`

### جوین کانال + ساخت فولدر از پنل/sidecar

دو endpoint جدید سایدکار (روی اکانت MTProto) برای آماده‌سازی یک بات تازه بدون دست‌کاری دستی در تلگرام:

- `POST /accounts/{id}/join` — body `{chat: "@username یا chat_id"}` → جوین کانال عمومی (`UserAlreadyParticipantError` هم `ok` محسوب می‌شود)
- `POST /accounts/{id}/folders` — body `{name, chats: ["@u1","-100…"]}` → **ساخت یا جایگزین‌کردن پیرهای فولدری با همین نام** (بقیه فولدرها دست‌نخورده)
  - فولدر با id موجود آپدیت می‌شود؛ برای ساخت جدید **اولین id آزاد از ۲ به بالا** انتخاب می‌شود — تلگرام idهای ۰ و ۱ را رد می‌کند (`FILTER_ID_INVALID`) و تلکن ۱.۴۵ کلاس `CreateDialogFilterRequest` را ندارد، پس ساخت با `UpdateDialogFilterRequest` + id آزاد انجام می‌شود
  - هر چتِ درون `chats` اول resolve و در صورت نیاز جوین می‌شود → یک POST هم «جوین» هم «فولدر» را جمع می‌کند

## رفتار (پورت وفادار)

- **فیلترها**: متن خالی+بدون مدیا رد؛ min_length؛ forward/reply طبق تنظیمات؛ keywords
- **AI clean**: گارد <۳۰ کاراکتر (به مدل نمی‌رود — همان متن منتشر می‌شود)؛ NO_CONTENT = حذف کامل + رکورد `no_content`؛ جواب مکالمه‌ای (لطفاً/ارسال کنید/متنی که/بفرمایید) = حذف؛ خروجی <۲۰ کاراکتر = حذف؛ **خطای LLM = متن خام منتشر می‌شود**
- **خلاصهٔ کارت**: گارد <۴۰ کاراکتر؛ خطا/جواب مکالمه‌ای → متن کامل داخل کارت؛ کپشن = همیشه متن کامل[:1000]
- **عکس منبع**: فقط photo (sidecar GetPhoto → JPEG)؛ به‌صورت data-URI داخل کارت
- **رندر**: post (1080×1080) + story (1080×1920) با فونت Vazirmatn و هایلایت اعداد accent برند؛ **خطای رندر = ارسال متن خام** (رکورد `render_failed`)
- **پیام رسانه‌ای بدون متن**: رد می‌شود (عین sadcontent — فقط-عکس بدون متن با min_length حذف می‌شود)
- **فاصله**: ۱.۵s بین پردازش پیام‌ها، ۱s بین آپلود کارت‌ها
- **dedup**: جدول `content_posts` UNIQUE(bot_id, source_chat_id, message_id) — منبع حقیقت ماندگار (به‌جای collected.jsonl)
- **اولین اجرا per-source**: حداکثر `content_backfill_limit` پست آخر، قدیمی‌ترین اول

## AI

از env سراسری `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` (مشترک با دستیار dating). برای sadgpt: `LLM_BASE_URL=https://sadgpt.ir/v1` + `LLM_MODEL=xiaomi/mimo-v2.5`. بدون کلید: پاک‌سازی غیرفعال → متن خام.

## سرویس رندر (`renderer/`)

- کانتینر پنجم استک: FastAPI + Playwright Chromium، پورت داخلی 8788
- `POST /render` — `{fmt, text, image_b64?, image_mime?, brand}` → PNG bytes
- قالب‌ها و فونت‌ها عین sadcontent (کپی‌شده از `sadcontent/render/`)
- `RENDERER_URL` در env بک‌اند؛ خالی = رندر غیرفعال (fallback متن خام)
- تست: `cd renderer && python -m pytest test_generate.py`

## جدول `content_posts`

رکورد هر پیام پردازش‌شده با وضعیت‌های `published` | `no_content` (آگهی حذف‌شده) | `render_failed` (متن خام فرستاده شد) | `failed` (ارسال هم خطا داد). مبنای dedup و آرشیو.

## مهاجرت از sadcontent

1. کپی session: `sadcontent/session/sadcontent.session` → ولوم `tg_sessions` سایدکار (یا لاگین مجدد در پنل)
2. توقف launchd قدیمی: `launchctl unload ~/Library/LaunchAgents/com.sadcontent.bot.plist`
3. ساخت بات content از پنل با همان توکن بات + کانال `@Crypto-Content`
4. استارت و رصد `docker logs sadbot-backend | grep content`

## تست

```bash
cd backend
go test ./internal/content/    # unit با fake های sidecar/llm/renderer/publisher
```

پوشش: مسیر خوش‌مسیر (۲ PNG + رکورد published)، حذف کانال مقصد، NO_CONTENT، fallback خطای LLM، جواب مکالمه‌ای، fallback خطا/عدم‌دسترسی رندر، dedup، backfill limit، fail-closed فولدر، فیلترها (min_length/forwarded/keywords)، guard های AI، brand/link parsing، poll interval.
