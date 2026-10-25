package dating

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

func engineBG() context.Context { return context.Background() }

// Template is the Go port of kpkchat's dating chat core
// (onboarding, quick search, anonymous chat, rating, block).
type Template struct{}

func (t *Template) Key() string { return domain.TemplateDating }

// DefaultTexts seeds the editable copy deck (merged with bot_configs.texts;
// the panel ConfigTab edits the override). Missing keys fall back to the
// hardcoded defaults via ttext().
func (t *Template) DefaultTexts() []byte {
	return []byte(`{
		"home.welcome": "خوش اومدی! 👋\nچیکار برات انجام بدم؟",
		"home.guide": "📖 راهنما\n\nبا «🔗 به یک ناشناس وصلم کن» یه مخاطب پیدا کن و ناشناس چت کن.\nدر پایان چت به طرف مقابلت امتیاز بده.\nبرای خروج از هر بخش: 🏠 منوی اصلی",
		"search.waiting": "⏳ در حال جستجو...\n\nبه‌زودی به یک ناشناس وصل می‌شی.\nبرای لغو دکمه زیر رو بزن.",
		"search.empty": "فعلاً کسی برای مرور باقی نمونده 😅\nکمی بعد دوباره سر بزن.",
		"match.search": "وصلت انجام شد!",
		"match.browse": "وصل شدید! 🎉",
		"chat.ended": "مکالمه تمام شد 🌟\nبه منوی اصلی برگشتی.",
		"ai.intro": "🤖 من یک دستیار هوشمندم (هوش مصنوعی)، نه کاربر واقعی.\n\nاینجا می‌تونی حرف بزنی، سوال بپرسی یا نظرت رو بگی.\n🎁 پیام‌های اول رایگانه؛ بعدش ۱ سکه (اشتراک پریموم = نامحدود).\n\nهر وقت خواستی: 🏠 منوی اصلی",
		"wallet.title": "💰 کیف پول شما",
		"onboarding.done": "ثبت‌نامت کامل شد! 🎉\nخوش اومدی! 👋\nچیکار برات انجام بدم؟"
	}`)
}

// ttext returns the merged text for key, or def when the key is absent
// (engine.Text returns the key itself for missing entries).
func ttext(bc *engine.BotContext, key, def string) string {
	if v := bc.Text(key); v != key {
		return v
	}
	return def
}

func (t *Template) DefaultMenus() []byte {
	return []byte(`{}`)
}

// DefaultPrices is the coin economy (kpkchat parity); per-bot overridable
// via bot_configs.prices.
func (t *Template) DefaultPrices() []byte {
	return []byte(`{
		"signup_bonus": 5,
		"referral_bonus": 5,
		"channel_bonus": 10,
		"channel_id": "",
		"search_stranger_male": 1,
		"search_stranger_female": 0,
		"search_gender_male": 5,
		"search_gender_female": 1,
		"search_advanced_any": 1,
		"search_advanced_gender": 3,
		"search_advanced_timeout": 180,
		"avatar_bonus": 10,
		"browse_free_likes": 10,
		"browse_like_cost": 1,
		"superlike_cost": 3,
		"superlike_daily_cap": 3,
		"direct_chat_cost": 5,
		"ai_free_messages": 20,
		"ai_message_cost": 1,
		"ai_max_history": 12,
		"ghost_enabled": 1,
		"ghost_pool_size": 5,
		"ghost_max_messages": 12,
		"ghost_delay_seconds": 15,
		"boost_1h_coins": 2,
		"boost_6h_coins": 5,
		"boost_24h_coins": 10,
		"streak_rewards": {"3": 5, "7": 15, "14": 30, "30": 75},
		"edit_cost": 50,
		"edit_name_cost": 100,
		"edit_cooldown_days": 7,
		"name_edit_cooldown_days": 14,
		"notify_enabled": 1,
		"notify_daily_cap": 2,
		"notify_min_gap_hours": 8,
		"notify_quiet_start": 22,
		"notify_quiet_end": 9,
		"confess_cost": 2,
		"sub_price": 280000,
		"sub_discount_pct": 30,
		"packages": [
			{"id": "pkg_1", "coins": 1, "price": 10000, "label": "۱ سکه — ۱۰,۰۰۰ تومان"},
			{"id": "pkg_5", "coins": 5, "price": 50000, "label": "۵ سکه — ۵۰,۰۰۰ تومان"},
			{"id": "pkg_10", "coins": 10, "price": 90000, "label": "۱۰ سکه — ۹۰,۰۰۰ تومان (۱۰٪ تخفیف)"},
			{"id": "pkg_40", "coins": 40, "price": 350000, "label": "۴۰ سکه — ۳۵۰,۰۰۰ تومان (۱۲٪ تخفیف)"}
		]
	}`)
}

// --- state helpers ---

func scene(bc *engine.BotContext) string {
	s, _ := bc.State["scene"].(string)
	return s
}

func step(bc *engine.BotContext) string {
	s, _ := bc.State["step"].(string)
	return s
}

func setStep(bc *engine.BotContext, s string) {
	bc.State["step"] = s
	bc.SaveState()
}

func dataMap(bc *engine.BotContext) map[string]any {
	d, _ := bc.State["data"].(map[string]any)
	if d == nil {
		d = map[string]any{}
	}
	return d
}

func storeOf(bc *engine.BotContext) *Store {
	s, _ := bc.Data["dating"].(*Store)
	return s
}

func botUUID(bc *engine.BotContext) uuid.UUID {
	id, _ := uuid.Parse(bc.BotID)
	return id
}

func stateKBD(rows ...[]string) [][]string { return rows }

// --- kpkchat texts (ported verbatim) ---

const humanRules = "\n\n━━━━━━━━━━━━━━\n* لطفا احترام بگذارید و برای مخاطب مقابل شعور قائل باشید\n* از کلمات رکیک استفاده نکنید\n* در پایان مکالمه با ثبت امتیاز وضعیت کاربر را مشخص کنید\n━━━━━━━━━━━━━━"

// homeKB / searchKB mirror kpkchat's colored reply keyboards (Bot API 7.11+
// button styles: primary = brand blue, success = green). Labels stay the
// sadbot ones — they are wired to handleHome / isHomeButtonText / nav tests.
var homeKB = [][]engine.KBButton{
	{{Text: "🔗 به یک ناشناس وصلم کن", Style: "primary"}},
	{{Text: "💘 مرور پروفایل‌ها", Style: "success"}},
	{{Text: "🔍 جستجوی ویژه"}, {Text: "👥 جستجوی کاربران"}},
	{{Text: "📖 راهنما"}, {Text: "💰 سکه"}, {Text: "👤 پروفایل من"}},
	{{Text: "🎁 ماموریت‌ها"}, {Text: "💌 اعتراف ناشناس"}},
	{{Text: "🤖 چت با دستیار"}, {Text: "📩 پشتیبانی"}},
}

var searchKB = [][]engine.KBButton{
	{{Text: "🔗 غریبه", Style: "primary"}},
	{{Text: "🚺 جستجوی دختر"}, {Text: "🚹 جستجوی پسر"}},
	{{Text: "🔍 جستجوی پیشرفته"}},
	{{Text: "🏠 منوی اصلی"}},
}

// Label-only views for call sites that only need the texts (tests, notify
// payloads that predate styled keyboards).
var homeRows = kbLabelRows(homeKB)
var searchRows = kbLabelRows(searchKB)

func kbLabelRows(rows [][]engine.KBButton) [][]string {
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		r := make([]string, 0, len(row))
		for _, b := range row {
			r = append(r, b.Text)
		}
		out = append(out, r)
	}
	return out
}

var chatRows = [][]string{
	{"🛑 قطع مکالمه"},
	{"🚫 بلاک"},
}

var ratingRows = [][]string{
	{"⭐", "⭐⭐", "⭐⭐⭐", "⭐⭐⭐⭐", "⭐⭐⭐⭐⭐"},
	{"🏠 منوی اصلی"},
}

// --- /start ---

func (t *Template) Start(bc *engine.BotContext, payload string) bool {
	st := storeOf(bc)
	if st == nil {
		return false
	}
	captureReferralPayload(bc, payload)
	ctx := engineBG()
	botID := botUUID(bc)

	profile, err := st.EnsureProfile(ctx, botID, bc.UserID, bc.TelegramID)
	if err != nil {
		log.Printf("[dating] ensure profile: %v", err)
		return false
	}

	// kpkchat: /start mid-chat → force end + rating
	if profile.Status == domain.DatingChatting && profile.PartnerUserID != nil {
		forceEndToRating(bc, st, botID, "start")
		return true
	}

	if !IsProfileComplete(profile) {
		resumeOnboarding(bc, profile)
		return true
	}

	goHome(bc, ttext(bc, "home.welcome", "خوش اومدی! 👋\nچیکار برات انجام بدم؟"))
	return true
}

func resumeOnboarding(bc *engine.BotContext, p *domain.DatingProfile) {
	bc.State["scene"] = "setup"
	switch {
	case p.Name == nil || *p.Name == "":
		bc.State["step"] = "setname"
		bc.SaveState()
		bc.Reply("سلام! 👋\nاسمت چیه؟")
	case p.Age == nil:
		bc.State["step"] = "setage"
		bc.SaveState()
		bc.ReplyKBD("خوش اومدی "+*p.Name+"! 🎉\nچند سالته؟", ageRows())
	case p.Province == nil || *p.Province == "":
		bc.State["step"] = "setprovince"
		bc.SaveState()
		bc.ReplyKBD("تو کدوم استان زندگی میکنی؟", provinceRows())
	case p.City == nil || *p.City == "":
		bc.State["step"] = "setcity"
		bc.SaveState()
		bc.ReplyKBD("استان "+*p.Province+" انتخاب شد.\nتو کدوم شهر هستی؟", cityRows(*p.Province))
	default:
		bc.State["step"] = "setgender"
		bc.SaveState()
		bc.ReplyKBD("دختری یا پسری؟", genderRows(pw(p.Age)))
	}
}

func pw(p *int) int {
	if p == nil {
		return 25
	}
	return *p
}

// --- message routing by scene ---

// isHomeButtonText reports whether text is a reply-keyboard label handled by
// handleHome. Scenes with inline-only keyboards (browse) leave the persistent
// home keyboard visible, so these labels must route home instead of being
// swallowed as scene input.
func isHomeButtonText(text string) bool {
	switch text {
	case "🔗 به یک ناشناس وصلم کن", "💘 مرور پروفایل‌ها", "🤖 چت با دستیار",
		"💌 اعتراف ناشناس", "📩 پشتیبانی", "🔍 جستجوی پیشرفته",
		"🔗 غریبه", "🚺 جستجوی دختر", "🚹 جستجوی پسر",
		"📖 راهنما", "👤 پروفایل من", "🏠 منوی اصلی",
		"💰 سکه", "🎁 ماموریت‌ها", "🔍 جستجوی ویژه", "👥 جستجوی کاربران":
		return true
	}
	return false
}

func (t *Template) HandleMessage(bc *engine.BotContext, text string) bool {
	st := storeOf(bc)
	if st == nil {
		return false
	}
	// The persistent home keyboard stays visible in inline-only scenes, so any
	// home-menu label must navigate instead of being swallowed as scene input.
	// (Before this guard only browse routed properly; inside confess «📩
	// پشتیبانی» literally became the confession text.)
	switch scene(bc) {
	case "browse", "search", "ai", "confess", "support":
		if isHomeButtonText(text) {
			if scene(bc) == "browse" {
				// close the on-screen card so its buttons don't stay live
				finishCard(bc, "🏠 منوی اصلی")
			}
			bc.State["scene"] = "home"
			bc.State["step"] = ""
			bc.SaveState()
			return handleHome(bc, st, text)
		}
	}
	switch scene(bc) {
	case "setup":
		return handleSetup(bc, st, text)
	case "chat":
		return handleChat(bc, st, text)
	case "rating":
		return handleRating(bc, st, text)
	case "search":
		return handleSearchScene(bc, st, text)
	case "browse":
		return handleBrowseText(bc, st, text)
	case "ai":
		return handleAIText(bc, st, text)
	case "confess":
		return handleConfessText(bc, st, text)
	case "support":
		return handleSupportText(bc, st, text)
	case "discount":
		return handleDiscountText(bc, st, text)
	case "home":
		return handleHome(bc, st, text)
	default:
		// reconcile unknown scene → home
		goHome(bc, ttext(bc, "home.welcome", "خوش اومدی! 👋\nچیکار برات انجام بدم؟"))
		return true
	}
}

// --- onboarding (setup) ---

func handleSetup(bc *engine.BotContext, st *Store, text string) bool {
	ctx := engineBG()
	botID := botUUID(bc)
	profile, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		goHome(bc, "خطا رخ داد. دوباره /start بزن.")
		return true
	}

	switch step(bc) {
	case "editphoto":
		// the prompt shows the home keyboard; a press must escape, not hang.
		// coins were already spent; no stamp since nothing changed.
		if isHomeButtonText(text) {
			delete(dataMap(bc), "edit_profile")
			bc.SaveState()
			bc.State["scene"] = "home"
			goHome(bc, "عکسی ثبت نشد.")
			return true
		}
		bc.ReplyKBD("فقط عکس بفرست. برای خروج: 🏠 منوی اصلی", [][]string{{"🏠 منوی اصلی"}})
		return true

	case "setname":
		name := strings.TrimSpace(text)
		if len([]rune(name)) < 2 || strings.HasPrefix(name, "/") {
			bc.Reply("لطفاً یک اسم معتبر (حداقل ۲ حرف) بنویس.")
			return true
		}
		if _, err := st.Repo().UpdateProfile(ctx, botID, bc.UserID, postgres.ProfileUpdate{Name: &name}); err != nil {
			bc.Reply("خطا رخ داد. دوباره تلاش کن.")
			return true
		}
		if IsProfileComplete(profile) {
			// paid name edit (profile was already complete on entry)
			finishPaidEdit(bc, st, botID)
			bc.Reply("✅ اسم به‌«" + name + "» تغییر کرد.")
			goHome(bc, ttext(bc, "home.welcome", "خوش اومدی! 👋\nچیکار برات انجام بدم؟"))
			return true
		}
		bc.State["step"] = "setage"
		bc.SaveState()
		bc.ReplyKBD("خوش اومدی "+name+"! 🎉\nچند سالته؟", ageRows())
		return true

	case "setage":
		age, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || age < 18 || age > 70 {
			bc.ReplyKBD("لطفاً یک عدد بین ۱۸ تا ۷۰ از کیبورد انتخاب کن.", ageRows())
			return true
		}
		if _, err := st.Repo().UpdateProfile(ctx, botID, bc.UserID, postgres.ProfileUpdate{Age: &age}); err != nil {
			bc.ReplyKBD("خطا رخ داد. دوباره تلاش کن.", ageRows())
			return true
		}
		if _, editing := dataMap(bc)["edit_profile"]; editing {
			finishPaidEdit(bc, st, botID)
			bc.Reply("✅ سن به‌روزرسانی شد.")
			return true
		}
		bc.State["step"] = "setprovince"
		bc.SaveState()
		bc.ReplyKBD("تو کدوم استان زندگی میکنی؟", provinceRows())
		return true

	case "setprovince":
		if _, ok := PROVINCES[text]; !ok {
			bc.ReplyKBD("این استان در لیست نیست. لطفاً یکی از دکمه‌ها رو بزن.", provinceRows())
			return true
		}
		prov := text
		if _, err := st.Repo().UpdateProfile(ctx, botID, bc.UserID, postgres.ProfileUpdate{Province: &prov}); err != nil {
			bc.ReplyKBD("خطا رخ داد. دوباره تلاش کن.", provinceRows())
			return true
		}
		bc.State["step"] = "setcity"
		bc.SaveState()
		bc.ReplyKBD("استان "+prov+" انتخاب شد.\nتو کدوم شهر هستی؟", cityRows(prov))
		return true

	case "setcity":
		prov := ""
		if profile.Province != nil {
			prov = *profile.Province
		}
		valid := false
		for _, c := range Cities(prov) {
			if c == text {
				valid = true
				break
			}
		}
		if !valid {
			bc.ReplyKBD("این شهر در استان "+prov+" پیدا نشد. از دکمه‌ها انتخاب کن.", cityRows(prov))
			return true
		}
		city := text
		if _, err := st.Repo().UpdateProfile(ctx, botID, bc.UserID, postgres.ProfileUpdate{City: &city}); err != nil {
			bc.ReplyKBD("خطا رخ داد. دوباره تلاش کن.", cityRows(prov))
			return true
		}
		if _, editing := dataMap(bc)["edit_profile"]; editing {
			finishPaidEdit(bc, st, botID)
			bc.Reply("✅ محل سکونت به‌روزرسانی شد.")
			return true
		}
		bc.State["step"] = "setgender"
		bc.SaveState()
		bc.ReplyKBD("دختری یا پسری؟", genderRows(pw(profile.Age)))
		return true

	case "setgender":
		gender := ""
		switch text {
		case "خانومم", "خانوم", "👩🏻 دخترم", "👧🏻 دخترم", "دخترم":
			gender = "female"
		case "آقام", "آقا", "👨🏻 پسرم", "👦🏻 پسرم", "پسرم":
			gender = "male"
		}
		if gender == "" {
			bc.ReplyKBD("لطفاً یکی از دکمه‌ها رو بزن.", genderRows(pw(profile.Age)))
			return true
		}
		if _, err := st.Repo().UpdateProfile(ctx, botID, bc.UserID, postgres.ProfileUpdate{Gender: &gender}); err != nil {
			bc.ReplyKBD("خطا رخ داد. دوباره تلاش کن.", genderRows(pw(profile.Age)))
			return true
		}
		if _, editing := dataMap(bc)["edit_profile"]; editing {
			finishPaidEdit(bc, st, botID)
			bc.Reply("✅ جنسیت به‌روزرسانی شد.")
			return true
		}
		grantSignupBonus(bc, st, botID)
		bc.State["step"] = "setavatar"
		bc.SaveState()
		bc.ReplyKBD(
			"📸 قدم آخر!\nیه عکس پروفایل بفرست (اختیاریه).\n\n🎁 هدیهٔ بارگذاری عکس: +"+
				strconv.Itoa(bc.PriceInt("avatar_bonus", 10))+" سکه",
			[][]string{{"⏭ رد شد"}},
		)
		return true

	case "setavatar":
		// any text (skip button or otherwise) completes onboarding;
		// photo uploads are consumed by HandleMedia.
		finishOnboarding(bc)
		return true
	}
	return false
}

func finishOnboarding(bc *engine.BotContext) {
	bc.State["step"] = ""
	bc.SaveState()
	goHome(bc, ttext(bc, "onboarding.done", "ثبت‌نامت کامل شد! 🎉\nخوش اومدی! 👋\nچیکار برات انجام بدم؟"))
}

// HandleMedia consumes the avatar photo during onboarding (optional step).
func (t *Template) HandleMedia(bc *engine.BotContext, msg *telegram.Message) bool {
	// media sent inside an active chat is relayed to the partner (kpkchat
	// chat-human parity); every media kind rides one forwardMessage call
	if scene(bc) == "chat" && msg.MediaType() != "" {
		return relayChatMedia(bc, msg)
	}

	// paid photo edit (profile page → 📷 تغییر عکس پروفایل)
	if step(bc) == "editphoto" {
		fileID := msg.PhotoFileID()
		if fileID == "" {
			bc.Reply("فقط عکس بفرست (jpg، png و…).")
			return true
		}
		st := storeOf(bc)
		if st == nil {
			return false
		}
		botID := botUUID(bc)
		if _, err := st.Repo().SetPhoto(engineBG(), botID, bc.UserID, fileID); err != nil {
			bc.Reply("خطا در ذخیره عکس. دوباره تلاش کن.")
			return true
		}
		bc.Reply("✅ عکس پروفایل به‌روزرسانی شد.")
		finishPaidEdit(bc, st, botID)
		return true
	}

	if step(bc) != "setavatar" {
		return false
	}
	fileID := msg.PhotoFileID()
	if fileID == "" {
		return false
	}
	st := storeOf(bc)
	if st == nil {
		return false
	}
	botID := botUUID(bc)
	first, err := st.Repo().SetPhoto(engineBG(), botID, bc.UserID, fileID)
	if err != nil {
		bc.Reply("خطا در ذخیره عکس. دوباره تلاش کن یا «⏭ رد شد» رو بزن.")
		return true
	}
	if first {
		if bonus := bc.PriceInt("avatar_bonus", 10); bonus > 0 {
			if _, err := st.Repo().AddCoins(engineBG(), botID, bc.UserID, bonus); err != nil {
				log.Printf("[dating] avatar bonus: %v", err)
			} else {
				bc.Reply(fmt.Sprintf("📸 عکس پروفایل ثبت شد!\n+%d سکه هدیهٔ بارگذاری عکس 🎁", bonus))
			}
		}
	}
	finishOnboarding(bc)
	return true
}

// --- home menu ---

func handleHome(bc *engine.BotContext, st *Store, text string) bool {
	botID := botUUID(bc)

	// daily streak: any home interaction counts as activity (kpkchat parity)
	recordStreak(bc, st, botID)

	// advanced-search wizard steps run inside the home scene
	if strings.HasPrefix(step(bc), "adv_") {
		return handleAdvSearch(bc, st, text)
	}

	switch text {
	case "🔗 به یک ناشناس وصلم کن":
		bc.ReplyKBDStyled("جستجوی ناشناس\n\nیکی از گزینه‌ها رو بزن:", searchKB)
		return true

	case "💘 مرور پروفایل‌ها":
		return startBrowse(bc, st)

	case "🤖 چت با دستیار":
		return startAI(bc)

	case "💌 اعتراف ناشناس":
		return startConfess(bc)

	case "📩 پشتیبانی":
		return startSupport(bc)

	case "🔍 جستجوی پیشرفته":
		startAdvWizard(bc, false)
		return true

	case "🔗 غریبه":
		return startSearch(bc, st, "any")
	case "🚺 جستجوی دختر":
		return startSearch(bc, st, "female")
	case "🚹 جستجوی پسر":
		return startSearch(bc, st, "male")

	case "📖 راهنما":
		bc.ReplyKBDStyled(ttext(bc, "home.guide", "📖 راهنما\n\n"+
			"با «🔗 به یک ناشناس وصلم کن» یه مخاطب پیدا کن و ناشناس چت کن.\n"+
			"در پایان چت به طرف مقابلت امتیاز بده.\n"+
			"برای خروج از هر بخش: 🏠 منوی اصلی"), homeKB)
		return true

	case "👤 پروفایل من":
		showProfilePage(bc, st, botID)
		return true

	case "🏠 منوی اصلی":
		goHome(bc, "🏠 منوی اصلی")
		return true

	case "💰 سکه":
		showWallet(bc, st, botID)
		return true

	case "🎁 ماموریت‌ها":
		showMissions(bc, botID, botUsernameOf(bc))
		return true

	case "🔍 جستجوی ویژه", "👥 جستجوی کاربران":
		// kpkchat parity: both buttons open the filter wizard in browse mode →
		// free filtered profile deck (not a paid timed search).
		startAdvWizard(bc, true)
		return true
	}
	return false
}

func formatRating(r float64, total int) string {
	if total == 0 {
		return "بدون امتیاز"
	}
	return strconv.FormatFloat(r, 'f', 1, 64) + " از " + strconv.Itoa(total) + " رأی"
}

// --- search ---

// blockedByAdmin reports whether the panel has blocked this user. Blocked
// users keep their profile but cannot search, browse, or chat.
func blockedByAdmin(bc *engine.BotContext, st *Store) bool {
	blocked, err := st.Repo().IsAdminBlocked(engineBG(), botUUID(bc), bc.UserID)
	if err != nil {
		return false
	}
	return blocked
}

// denyIfBlocked answers a blocked user and reports whether the caller should
// stop processing the update.
func denyIfBlocked(bc *engine.BotContext, st *Store) bool {
	if !blockedByAdmin(bc, st) {
		return false
	}
	bc.ReplyKBDStyled(ttext(bc, "blocked.account",
		"🚫 دسترسی حساب شما توسط پشتیبانی موقتاً محدود شده است.\n"+
			"برای پیگیری از بخش «📩 پشتیبانی» پیام بده."), homeKB)
	return true
}

func startSearch(bc *engine.BotContext, st *Store, genderFilter string) bool {
	if denyIfBlocked(bc, st) {
		return true
	}
	ctx := engineBG()
	botID := botUUID(bc)
	p, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا رخ داد. /start بزن.")
		return true
	}
	if p.Status == domain.DatingChatting {
		bc.Reply("الان در چت هستی. اول مکالمه رو تموم کن.")
		return true
	}
	if p.Status == domain.DatingSearching {
		bc.ReplyKBD("الان در جستجو هستی. صبر کن یا «❌ لغو جستجو» بزن.", [][]string{{"❌ لغو جستجو"}, {"🏠 منوی اصلی"}})
		return true
	}
	cost := searchCost(bc, genderFilter, derefStr(p.Gender), hasSubscription(bc, botID))
	if cost > 0 {
		ok, err := st.Repo().SpendCoins(ctx, botID, bc.UserID, cost)
		if err != nil || !ok {
			bc.ReplyKBDStyled(fmt.Sprintf("💰 سکه کافی نیست! (هزینهٔ جستجو: %d سکه)\nاز بخش «💰 سکه» سکه بخر.", cost), homeKB)
			return true
		}
	}
	if err := st.StartSearch(ctx, botID, bc.UserID, genderFilter, cost); err != nil {
		if cost > 0 {
			_, _ = st.Repo().AddCoins(ctx, botID, bc.UserID, cost) // rollback
		}
		bc.ReplyKBDStyled("خطا در شروع جستجو. دوباره تلاش کن.", searchKB)
		return true
	}
	bc.State["scene"] = "search"
	bc.State["step"] = ""
	bc.SaveState()
	bc.ReplyKBD(searchingText(bc), [][]string{{"❌ لغو جستجو"}, {"🏠 منوی اصلی"}})
	return true
}

func searchingText(bc *engine.BotContext) string {
	return ttext(bc, "search.waiting", "⏳ در حال جستجو...\n\n"+
		"به‌زودی به یک ناشناس وصل می‌شی.\n"+
		"برای لغو دکمه زیر رو بزن.")
}

func handleSearchScene(bc *engine.BotContext, st *Store, text string) bool {
	switch text {
	case "❌ لغو جستجو":
		ctx := engineBG()
		if err := st.CancelSearch(ctx, botUUID(bc), bc.UserID); err != nil {
			bc.Reply("خطا در لغو. دوباره تلاش کن.")
			return true
		}
		bc.State["scene"] = "home"
		bc.SaveState()
		goHome(bc, "جستجو لغو شد.")
		return true
	default:
		// home-menu labels are routed by the central guard in HandleMessage;
		// anything else re-shows the waiting state (keeps the queue entry).
		bc.ReplyKBD(searchingText(bc), [][]string{{"❌ لغو جستجو"}, {"🏠 منوی اصلی"}})
		return true
	}
}

// --- chat ---

// relayChatMedia copies a photo/sticker/voice/video/file the user sent inside
// the chat scene to their partner. It mirrors handleChat's guards so a blocked
// or ended chat can't sneak media across.
func relayChatMedia(bc *engine.BotContext, msg *telegram.Message) bool {
	ctx := engineBG()
	st := storeOf(bc)
	if st == nil {
		return false
	}
	botID := botUUID(bc)
	label := msg.MediaTypeLabel()

	if denyIfBlocked(bc, st) {
		return true
	}

	p, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا. /start بزن.")
		return true
	}
	if p.Status != domain.DatingChatting || p.PartnerUserID == nil {
		bc.State["scene"] = "home"
		bc.SaveState()
		goHome(bc, "چت فعالی نداری.\n🏠 منوی اصلی")
		return true
	}
	if partnerIsGhost(ctx, st, botID, *p.PartnerUserID) {
		bc.Reply(fmt.Sprintf("کاربر جعلی رسانه دریافت نمی‌کنه. %s رو متنی بفرست.", label))
		return true
	}
	partnerTelegram, err := partnerTelegramID(ctx, st, botID, *p.PartnerUserID)
	if err != nil || partnerTelegram == 0 {
		forceEndToRating(bc, st, botID, "partner_dead")
		return true
	}
	if blocked, _ := st.IsBlocked(ctx, botID, bc.UserID, *p.PartnerUserID); blocked {
		forceEndToRating(bc, st, botID, "blocked")
		return true
	}

	if _, err := bc.ForwardMediaTo(partnerTelegram, bc.ChatID, msg.MessageID); err != nil {
		if errors.Is(err, engine.ErrMediaUnsupported) {
			bc.Reply("فقط متن منتقل میشه؛ رسانه در دسترس نیست.")
		} else {
			forceEndToRating(bc, st, botID, "partner_unreachable")
		}
		return true
	}
	// keep the transcript complete: the engine never logs media updates
	bc.RecordInbound(label, map[string]any{"media": msg.MediaType()})
	return true
}

func handleChat(bc *engine.BotContext, st *Store, text string) bool {
	ctx := engineBG()
	botID := botUUID(bc)

	// a blocked user can still leave the chat, but cannot send messages
	if text != "🛑 قطع مکالمه" && text != "🚫 بلاک" && denyIfBlocked(bc, st) {
		return true
	}

	switch text {
	case "🛑 قطع مکالمه":
		p, err := st.Profile(ctx, botID, bc.UserID)
		if err != nil || p.PartnerUserID == nil {
			bc.State["scene"] = "home"
			bc.SaveState()
			goHome(bc, "🏠 منوی اصلی")
			return true
		}
		forceEndToRating(bc, st, botID, "self")
		return true

	case "🚫 بلاک":
		p, err := st.Profile(ctx, botID, bc.UserID)
		if err != nil || p.PartnerUserID == nil {
			bc.ReplyKBD("مخاطبی برای بلاک وجود نداره.", chatRows)
			return true
		}
		partnerTelegram, err := partnerTelegramID(ctx, st, botID, *p.PartnerUserID)
		if err == nil && partnerTelegram > 0 {
			_, _ = bc.SendKBDTo(partnerTelegram,
				"🚫 مخاطب بلاک شد.\nمکالمه شما پایان یافت.\nبه مخاطبت امتیاز بده:",
				ratingRows)
			// notify rating scene on partner side: set scene via... partner state lives in DB;
			// partner's next message will hit reconcile → pending rate.
		}
		if err := st.BlockAndEnd(ctx, botID, bc.UserID, *p.PartnerUserID); err != nil {
			bc.ReplyKBD("خطا در بلاک. دوباره تلاش کن.", chatRows)
			return true
		}
		bc.State["scene"] = "home"
		bc.SaveState()
		goHome(bc, "🚫 مخاطب بلاک شد.\n🏠 منوی اصلی")
		return true
	}

	// plain text → forward to partner
	p, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا. /start بزن.")
		return true
	}
	if p.Status != domain.DatingChatting || p.PartnerUserID == nil {
		bc.State["scene"] = "home"
		bc.SaveState()
		goHome(bc, "چت فعالی نداری.\n🏠 منوی اصلی")
		return true
	}
	if partnerIsGhost(ctx, st, botID, *p.PartnerUserID) {
		return ghostHandleMessage(bc, st, botID, *p.PartnerUserID, text)
	}
	partnerTelegram, err := partnerTelegramID(ctx, st, botID, *p.PartnerUserID)
	if err != nil || partnerTelegram == 0 {
		forceEndToRating(bc, st, botID, "partner_dead")
		return true
	}
	blocked, _ := st.IsBlocked(ctx, botID, bc.UserID, *p.PartnerUserID)
	if blocked {
		forceEndToRating(bc, st, botID, "blocked")
		return true
	}
	// reply mapping (kpkchat chat-reply-map)
	replyTo, _ := st.ResolveReply(ctx, botID, bc.UserID, bc.MsgID)
	sent, err := bc.SendTo(partnerTelegram, text, replyTo)
	if err != nil {
		// retry once without reply anchor (kpkchat fix)
		sent, err = bc.SendTo(partnerTelegram, text, 0)
		if err != nil {
			forceEndToRating(bc, st, botID, "partner_unreachable")
			return true
		}
	}
	// log the forwarded message on the partner's side so the panel feed
	// shows both directions of the conversation
	_ = st.Repo().RecordOutboundForPartner(ctx, botID, *p.PartnerUserID, text)
	_ = st.BindReply(ctx, botID, bc.UserID, bc.MsgID, *p.PartnerUserID, sent.MessageID)
	_ = st.BindReply(ctx, botID, *p.PartnerUserID, sent.MessageID, bc.UserID, bc.MsgID)
	return true
}

func partnerTelegramID(ctx context.Context, st *Store, botID uuid.UUID, partnerUserID int64) (int64, error) {
	partnerProfile, err := st.Profile(ctx, botID, partnerUserID)
	if err != nil {
		return 0, err
	}
	return partnerProfile.TelegramID, nil
}

// --- rating ---

func handleRating(bc *engine.BotContext, st *Store, text string) bool {
	ctx := engineBG()
	botID := botUUID(bc)

	if text == "🏠 منوی اصلی" {
		// skip rating (kpkchat allows skip via home button)
		_ = st.Repo().ClearPendingRate(ctx, botID, bc.UserID)
		bc.State["scene"] = "home"
		bc.SaveState()
		goHome(bc, "🏠 منوی اصلی")
		return true
	}

	score := map[string]int{"⭐": 1, "⭐⭐": 2, "⭐⭐⭐": 3, "⭐⭐⭐⭐": 4, "⭐⭐⭐⭐⭐": 5}[text]
	if score == 0 {
		bc.ReplyKBD("لطفا یکی از گزینه‌های امتیاز رو انتخاب کن.", ratingRows)
		return true
	}

	pending, err := st.Repo().GetPendingRate(ctx, botID, bc.UserID)
	if err != nil || pending == nil {
		bc.State["scene"] = "home"
		bc.SaveState()
		goHome(bc, "🏠 منوی اصلی")
		return true
	}
	if err := st.ApplyRating(ctx, botID, bc.UserID, pending.PartnerUserID, score); err != nil {
		bc.ReplyKBD("خطا در ثبت امتیاز. دوباره تلاش کن.", ratingRows)
		return true
	}
	bc.State["scene"] = "home"
	bc.SaveState()
	goHome(bc, "✅ امتیازت ثبت شد.\nخوش اومدی! چیکار برات انجام بدم؟")
	return true
}

// --- shared helpers ---

func goHome(bc *engine.BotContext, text string) {
	bc.State["scene"] = "home"
	bc.State["step"] = ""
	bc.SaveState()
	bc.ReplyKBDStyled(text, homeKB)
}

func forceEndToRating(bc *engine.BotContext, st *Store, botID uuid.UUID, reason string) {
	ctx := engineBG()
	p, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		bc.State["scene"] = "home"
		bc.SaveState()
		goHome(bc, "🏠 منوی اصلی")
		return
	}
	if p.PartnerUserID != nil && partnerIsGhost(ctx, st, botID, *p.PartnerUserID) {
		// ghost chats end without ratings
		endGhostChatFromProfile(bc, st, botID, bc.UserID, *p.PartnerUserID)
		return
	}
	if p.PartnerUserID != nil {
		partnerTelegram, perr := partnerTelegramID(ctx, st, botID, *p.PartnerUserID)
		if perr == nil && partnerTelegram > 0 {
			msg := "مخاطب چت رو ترک کرد.\nبه مخاطبت امتیاز بده:"
			if reason == "partner_dead" || reason == "partner_unreachable" {
				msg = "مخاطب در دسترس نیست.\nمکالمه پایان یافت."
			}
			_, _ = bc.SendKBDTo(partnerTelegram, msg, ratingRows)
		}
		if err := st.EndChatForBoth(ctx, botID, bc.UserID, *p.PartnerUserID); err != nil {
			log.Printf("[dating] end chat both: %v", err)
		}
		// partner must land in rating scene too (kpkchat pendingRate behavior)
		_ = st.SetScene(ctx, bc.BotID, *p.PartnerUserID, "rating")
	} else {
		_, _ = st.Repo().EndChat(ctx, botID, bc.UserID)
	}
	// register self pending if partner existed
	if p.PartnerUserID != nil {
		_ = st.Repo().SetPendingRate(ctx, botID, bc.UserID, *p.PartnerUserID)
	}
	bc.State["scene"] = "rating"
	bc.State["step"] = ""
	bc.SaveState()
	bc.ReplyKBD("مکالمه پایان یافت. به مخاطبت امتیاز بده:", ratingRows)
}

// --- keyboards ---

func ageRows() [][]string {
	var rows [][]string
	var row []string
	for age := 18; age <= 70; age++ {
		row = append(row, strconv.Itoa(age))
		if len(row) == 5 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func provinceRows() [][]string {
	var rows [][]string
	var row []string
	for _, p := range ProvinceNames() {
		row = append(row, p)
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func cityRows(province string) [][]string {
	var rows [][]string
	var row []string
	for _, c := range Cities(province) {
		row = append(row, c)
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// genderSmart labels from kpkchat: age ≤ 30 informal, > 30 formal.
func genderRows(age int) [][]string {
	if age <= 30 {
		return [][]string{{"خانومم", "آقام"}}
	}
	return [][]string{{"خانوم", "آقا"}}
}
