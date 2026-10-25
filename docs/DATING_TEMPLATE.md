# تمپلیت دوستیابی (`dating`)

> کد: `backend/internal/dating/` — template.go، browse.go، advanced_search.go، economy.go، profile.go، matcher.go، moderation_ux.go، ai.go، growth.go، notifications.go  
> مرجع رفتاری: سورس kpkchat (`kpkchat/src/bots/ashena/`) + [kpkchat-analysis.md](./kpkchat-analysis.md)  
> قرارداد: **متون و جریان‌های هستهٔ چت عین kpkchat**؛ امکانات جانبی با همان رفتار (و کد) پورت شده‌اند.  
> وضعیت: کامل و live روی production (@ashnadbot «آشنا» — توکن مهاجرت‌کرده از kpkchat اصلی در ۲۰۲۶-۱۰-۱۷) — از فاز ۱ (چت) تا اقتصاد سکه، browse، اعتراف، دستیار AI و ویرایش پروفایل.

## دامنهٔ فعلی

| بخش | وضعیت |
|---|---|
| Onboarding: نام/سن/استان/شهر/جنسیت + عکس اختیاری | ✅ |
| سرچ سریع (غریبه/دختر/پسر) + جستجوی زمان‌دار با فیلتر | ✅ |
| مچینگ با امتیاز شباهت (هر ۵ ثانیه) + گوست‌ها | ✅ |
| چت ناشناس + ریپلای + رسانه (عکس/استیکر/صدا/ویدیو/فایل) | ✅ |
| قطع/بلاک ۳۰ روزه/امتیاز ۱-۵ ستاره | ✅ |
| Browse سبک Tinder (لایک/سوپرلایک/چت مستقیم) + مرور فیلترشده | ✅ |
| اقتصاد سکه + پکیج/اشتراک (درگاه catroom.ir + dev mode) | ✅ |
| اعتراف ناشناس (تایید ادمین + انتشار در کانال) + پشتیبانی | ✅ |
| چت با دستیار هوشمند (quota رایگان + سکه/اشتراک) | ✅ |
| صفحهٔ پروفایل من + ۶ ویرایش (نام/سن/جنسیت/مکان/عکس/علایق) | ✅ |
| ماموریت کانال (چندکاناله) + دعوت دوستان (referral) | ✅ |
| استریک روزانه + بوست + تخفیف | ✅ |

## منوی home (رنگی، عین kpkchat)

کیبوردهای reply با فیلد `style` (Bot API 7.11+ — `primary` آبی برند / `success` سبز) ساخته میشوند؛ مثل kpkchat:

```
[🔗 به یک ناشناس وصلم کن]   ← primary (آبی)
[💘 مرور پروفایل‌ها]         ← success (سبز)
[📖 راهنما] [💰 سکه] [👤 پروفایل من]
[🎁 ماموریت‌ها] [💌 اعتراف ناشناس]
[🤖 چت با دستیار] [📩 پشتیبانی]
```

- زیرمنوی «وصلم کن»: `[🔗 غریبه ← primary]` / `[🚺 جستجوی دختر] [🚹 جستجوی پسر]` / `[🔍 جستجوی پیشرفته]` / `[🏠 منوی اصلی]`
- «🔍 جستجوی ویژه» و «👥 جستجوی کاربران» (home): ویزارد فیلتر در **حالت browse** — رایگان و بدون جستجوی زمان‌دار → دکهٔ فیلترشده (مرور پروفایل‌ها با جنسیت/استان/شهر/سن/علایق). مسیر «جستجوی پیشرفته» همان ویزارد ولی با جستجوی زمان‌دار پولی است (`search_advanced_any`/`search_advanced_gender` + `timeout_sec` از prices).
- ویزارد پیشرفته (۲۰۲۶-۱۰-۱۸ بازنویسی شد): مراحل `adv_gender → adv_province → adv_city → adv_age → adv_interests → adv_confirm` — انتخاب استان با کیبورد ۳۱تایی دوتایی (عین kpkchat)، شهرهای همان استان + «همه شهرها»؛ فیلتر استان در `BrowseFilter.ProvinceFilter` و `filtersCompatible` مچر هم اعمال میشود.
- لیبل دکمهها به `isHomeButtonText` + `handleHome` وصلند؛ تغییر نام دکمه = به‌روزرسانی هر دو + تست ناوبری.

### گارد ناوبری مرکزی

کیبورد home در صحنه‌های inline-only (browse) یا صحنه‌های متنی (search/ai/confess/support) میماند؛ در `HandleMessage` یک گارد مرکزی هر لیبل `isHomeButtonText` را در این پنج صحنه به home برمیگرداند (وگرنه داخل confess متن دکمه به‌عنوان متن اعتراف بلعیده میشد — باگ live رفعشده). تست: `nav_matrix_test.go` (`TestHomeNavFromEveryScene`, `TestHomeNavFromInlineOnlyScenes`, `TestHomeMenuColoredButtons`).

## ماشین وضعیت

state در `bot_end_users.state` (JSONB): `scene` + `step` + `data`.

```
/setup: setname → setage → setprovince → setcity → setgender → (عکس اختیاری) → home
/home: وصلم کن → زیرمنو → غریبه/دختر/پسر → search (صف + هزینه)
/search: مچ → chat | ❌ لغو (بازگشت هزینه) | timeout (بازگشت هزینه)
/chat: 🛑 قطع / 🚫 بلاک / پیام ناموفق → rating → ⭐..⭐⭐⭐⭐⭐ → home
/home: مرور پروفایل‌ها → browse (دکه inline: لایک/سوپرلایک/چت مستقیم/بعدی)
        مچ دوطرفه (لایک متقابل یا سوپرلایک یا چت مستقیم) → chat
/home: جستجوی ویژه/کاربران → ویزارد فیلتر (browse mode: جنسیت/استان/شهر/سن/علایق) → browse فیلترشده
/home: جستجوی پیشرفته → ویزارد فیلتر (timed mode) → search زمان‌دار (timeout از prices)
```

قوانین reconcile (مثل kpkchat):
- هر پیام اول `scene` فعلی را چک میکند؛ scene ناشناس → home.
- profile در `chatting` + `/start` → force-end با pending rate برای هر دو.
- وضعیت ماندگار (status/partner) در جدول‌های dating است؛ scene فقط ناوبری UI است.

## Onboarding (متون عین kpkchat)

| مرحله | متن | کیبورد |
|---|---|---|
| setname | `سلام! 👋\nاسمت چیه؟` | متن آزاد (حداقل ۲ حرف) |
| setage | `خوش اومدی {name}! 🎉\nچند سالته؟` | دکمه‌های ۱۸ تا ۷۰ |
| setprovince | `تو کدوم استان زندگی میکنی؟` | ۳۱ استان (`dating/iran.go` عین kpkchat) |
| setcity | `استان {province} انتخاب شد.\nتو کدوم شهر هستی؟` | شهرهای همان استان |
| setgender | `دختری یا پسری؟` | هوشمند سن: ≤۳۰ «خانومم/آقام»، >۳۰ «خانوم/آقا» |
| عکس (اختیاری) | دعوت به آپلود آواتار | «⏭ رد شد» / عکس (پاداش یکباره) |
| پایان | خوش‌آمد + منوی home | منوی رنگی |

استان/شهر با `+bonus` signup (سکهٔ ثبت‌نام از `prices.signup_bonus`)؛ referral با payload `ref_<id>` در `/start` هر دو طرف را شارژ میکند.

## سرچ و مچینگ

**شروع**: چک بلاک ادمین → چک وضعیت → کسر هزینهٔ جنسیت (`search_stranger_*` / `search_gender_*`؛ مشترکین رایگان) → CAS `joined→searching` + ثبت فیلتر در `dating_search_requests`. متن انتظار مثل kpkchat؛ لغو و timeout هزینه را برمیگردانند.

**مچر** (هر ۵ ثانیه، هر باتِ datingِ running):
1. بارگذاری searchers با `FOR UPDATE SKIP LOCKED`.
2. جفت‌گیری حریصانه با `similarityScore` (وزن‌های kpkchat: جنسیت ۱۰/۵، شهر ۸، استان ۴، سن ۵/۲، هر علاقهٔ مشترک ۳).
3. فیلتر جنسیت/شهر/بازهٔ سن هر دو طرف (جستجوی پیشرفته `filtersCompatible`).
4. `ClaimMatch` اتمیک → scene هر دو `chat` + اطلاع «وصلت انجام شد!».
5. **گوست‌ها**: کاربران نمونهٔ `is_ghost` در همان صف شرکت میکنند؛ چت با گوست با موتور پاسخ هوشمند (`cold-start/`) ادامه مییابد و **بدون صفحهٔ امتیازدهی** تمام میشود.
6. **timeout**: از `filters.timeout_sec` (پیشفرض ۴۰s؛ جستجوی پیشرفته ۱۸۰s)؛ لغو + بازگشت هزینه + نوتیفیکیشن.

## Browse (سبک Tinder)

- **کارت** (۲۰۲۶-۱۰-۱۸ بازطراحی شد، عین kpkchat): کپشن فشرده — `💕 نام 👩` / `🎂 سن ساله` / `📍 شهر، استان` / `⭐ امتیاز` / `🎯 علایق · جداشده` (سقف ۵) / خط پایانی سکه (رایگان باقی‌مانده / هزینه / پریموم). عکس با `SendPhotoKBD`، وگرنه متن؛ دکمه‌ها: لایک (سقف رایگان روزانه `browse_free_likes` بعد `browse_like_cost`)، سوپرلایک (`superlike_cost` + سقف روزانه + نوتیفیکیشن ویژه)، چت مستقیم (`direct_chat_cost`؛ مشترکین رایگان)، بعدی، منوی اصلی.
- **بستن کارت هنگام خروج** (۲۰۲۶-۱۰-۱۸ رفع باگ): شناسهٔ پیام کارت در state ذخیره میشود (`browse_card_msg` + `browse_card_photo`) چون خروج با کیبورد reply به‌صورت *پیام متنی* می‌رسد و MsgID آن به پیام کاربر اشاره دارد نه کارت. `finishCard` در همهٔ مسیرهای خروج صدا زده میشود — دکمهٔ inline «منوی اصلی»، گارد لیبل‌های home، متن تایپشده (قبل از کارت بعدی) و connect — و کارت را بدون دکمه ویرایش میکند. کارت عکس‌دار با `editMessageCaption` (اختیاری `engine.CaptionEditor` در sender) بسته میشود چون `editMessageText` روی رسانه رد میشود.
- لایک/سوپرلایک متقابل → connect؛ رد (`kind=pass`) تا تکرار کارت ثبت میشود.
- **مرور فیلترشده** («جستجوی ویژه/کاربران»): `BrowseCandidatesFiltered` با جنسیت/استان/شهر/سن/علایق از ویزارد؛ رایگان؛ فیلترها با خروج/خالی شدن دکه پاک میشوند.

## اقتصاد سکه

- کیف پول (`💰 سکه`): موجودی + روشهای کسب + پکیج‌ها (درگاه catroom.ir سازگار Zibal؛ `PAYMENT_DEV_MODE` برای تست فوری) + اشتراک ۳۰ روزه.
- ماموریت‌ها (`🎁 ماموریت‌ها`): عضویت کانال(ها) با تایید سروری (`+bonus`) — چندکاناله از `prices.channels`؛ لینک دعوت (`+bonus` دوطرفه).
- سوییپر پرداخت: auto-verify هر ۱ دقیقه (۱۰ دقیقه اول) + انقضای pending بعد از آن (**فقط یک بار** اطلاع‌رسانی — رفع باگ اسپم «مهلت پرداخت تمام شد»)؛ انقضای اشتراک هر ۶ ساعت.
- تخفیف (`discount_codes`) و بوست پروفایل (`growth.go`).

## پروفایل من (پورت «حساب من» kpkchat)

صفحهٔ inline غنی (نام/جنسیت/سن/استان/شهر/علایق/امتیاز/عکس/سکه/اشتراک) + ۶ ویرایش: نام (هزینه + کول‌داون)، سن/جنسیت/مکان/عکس (`edit_cost` + کول‌داون هفتگی مشترک؛ اولین تغییر هر فیلد رایگان)، علاقه‌مندی‌ها (رایگان، حداکثر ۵ تا از ۱۵ گزینه). prompt عکس با گارد home فرار میکند بدون ثبت کول‌داون.

## اعتراف و پشتیبانی

- **اعتراف ناشناس**: متن ≥۵ حرف → پیش‌نمایش → تایید (`confess_cost`) → ثبت برای تایید ادمین (پنل) → انتشار در کانال + اطلاع به کاربر. کاربران نمونه هم اعتراف تولید میکنند (seed).
- **پشتیبانی**: متن ≥۳ حرف → تیکت (پنل) + پاسخ ادمین از پنل به چت کاربر.

## دستیار هوشمند (`🤖 چت با دستیار`)

quota رایگان (`ai_free_messages`) بعد هر پیام `ai_message_cost` (مشترکین نامحدود)؛ gate از `prices.ai_enabled` (پنل میتواند قطع کند)؛ شخصیت بر اساس جنسیت؛ تاریخچه محدود در state.

## پنل مدیریت (AdminOps)

از پنل وب per-bot: تنظیمات عددی (۳۸ فیلد)، ویرایش کانالها، هارنس AI (لیست کاربران mid-AI + toggle)، چت‌ویور و فید پیامها، برادکست (متن + عکس + فوتر)، کدهای تخفیف، اعترافها/تیکتها، اسکن بلاک تلگرامی، ریست/مسدود/سکه/پیام مستقیم per-user.

## تست

- `flow_test.go` — جریان کامل چت (Postgres واقعی + Sender جعلی).
- `nav_matrix_test.go` — ماتریس ناوبری از هر صحنه + رنگی بودن کیبورد + فرار از prompt عکس.
- `filtered_browse_test.go` — ویزارد browse mode (فیلتر جنسیت/استان/شهر/سن، رایگان بودن، دکهٔ خالی).
- `browse_test.go` / `phase3_*` / `p4_test.go` — لایک متقابل، سوپرلایک، کانالها، گوست‌ها.
- `profile_edit_test.go` — صفحه و ۶ ویرایش + کول‌داون.
- `economy_test.go` / `discount_flow_test.go` / `growth_test.go` / `ai_gate_test.go` / `adminops_test.go`.
- `internal/payment/expiry_test.go` — انقضای پرداخت دقیقاً یک بار (رفع اسپم).

```bash
cd backend
DATABASE_URL=postgres://... go test ./internal/dating/ ./internal/payment/
```

## E2E زنده

با اکانت MTProto پنل (`tgsidecar`) میتوان بات را مثل یک کاربر واقعی چت کرد: پیام متنی دکمهها را بفرست (کیبورد reply = متن). درایور: `docker exec -i -e PYTHONPATH=/app sadbot-tgsidecar python3 -` با `get_client("<sidecar_account_id>")`. **دکمه‌های inline هم قابل فشردن هستند** — شیء دکمهٔ Telethon در این wrapper متد `click()` ندارد؛ callback data در `btn.type.data` است و باید درخواست خام بزنید:

```python
from telethon import functions
await client(functions.messages.GetBotCallbackAnswerRequest(
    peer=entity, msg_id=card.id, data=b"act:browse:next"))
```

نکته: کارت‌های قدیمی در history می‌مانند — هنگام تست حتماً کارت *جدید* را از `iter_messages` بگیرید ( آخرین پیامِ دارای `reply_markup` ) وگرنه کارت کهنهٔ ویرایش‌نشده را می‌بینید.
