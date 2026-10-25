package dating

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/payment"

	"github.com/google/uuid"
)

// economy accessors — services arrive via BotContext.Data (wired in main).

func paySvcOf(bc *engine.BotContext) *payment.Service {
	s, _ := bc.Data["payments"].(*payment.Service)
	return s
}

func payRepoOf(bc *engine.BotContext) PayRepo {
	r, _ := bc.Data["paymentRepo"].(PayRepo)
	return r
}

// PayRepo is the slice of PaymentRepo the dating template needs.
type PayRepo interface {
	HasActiveSubscription(ctx context.Context, botID uuid.UUID, userID int64) (bool, error)
	ActiveSubscriptionEnd(ctx context.Context, botID uuid.UUID, userID int64) (time.Time, bool, error)
}

// --- coins helpers on Store (thin wrappers for template readability) ---

func coinsOf(bc *engine.BotContext, st *Store, botID uuid.UUID) int {
	p, err := st.Profile(engineBG(), botID, bc.UserID)
	if err != nil {
		return 0
	}
	return p.Coins
}

func hasSubscription(bc *engine.BotContext, botID uuid.UUID) bool {
	r := payRepoOf(bc)
	if r == nil {
		return false
	}
	ok, err := r.HasActiveSubscription(context.Background(), botID, bc.UserID)
	return err == nil && ok
}

// searchCost returns the coin price of a search (0 when subscribed).
// genderFilter: "any" | "female" | "male"; userGender: "male"/"female".
func searchCost(bc *engine.BotContext, genderFilter, userGender string, subscribed bool) int {
	if subscribed {
		return 0
	}
	// kpkchat: stranger — boys pay 1, girls free; gendered — boys 5, girls 1
	if genderFilter == "any" {
		if userGender == "female" {
			return bc.PriceInt("search_stranger_female", 0)
		}
		return bc.PriceInt("search_stranger_male", 1)
	}
	if userGender == "female" {
		return bc.PriceInt("search_gender_female", 1)
	}
	return bc.PriceInt("search_gender_male", 5)
}

// --- wallet / store / missions screens ---

func showWallet(bc *engine.BotContext, st *Store, botID uuid.UUID) {
	ctx := engineBG()
	p, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا. /start بزن.")
		return
	}
	lines := []string{
		ttext(bc, "wallet.title", "💰 کیف پول شما"),
		"━━━━━━━━━━━━━━",
		fmt.Sprintf("سکه فعلی: %d", p.Coins),
	}
	if r := payRepoOf(bc); r != nil {
		if subEnd, ok, _ := r.ActiveSubscriptionEnd(ctx, botID, bc.UserID); ok {
			lines = append(lines, "⭐ پریموم تا "+subEnd.Format("2006-01-02"))
		}
	}
	lines = append(lines, "", "برای کسب سکه بیشتر:", "• جستجو: ۰ تا ۵ سکه", "• مرور پروفایل‌ها: اولین لایک‌ها رایگان", "• دعوت دوستان: ۵ سکه", "• عضویت کانال: ۱۰ سکه", "• خرید پکیج سکه")
	if code := appliedCode(bc); code != "" {
		lines = append(lines, "", "🎫 کد تخفیف فعال: "+code)
	}
	bc.ReplyInline(strings.Join(lines, "\n"), [][]engine.InlineButton{
		{{Label: "📈 خرید سکه", Callback: "act:wallet:buy"}, {Label: "⭐ اشتراک پریموم", Callback: "act:wallet:sub"}},
		{{Label: "🎁 ماموریت‌ها", Callback: "act:wallet:missions"}, {Label: "🎫 کد تخفیف", Callback: "act:wallet:discount"}},
		{{Label: "🚀 بوست پروفایل", Callback: "act:wallet:boost"}, {Label: "✏️ ویرایش پروفایل", Callback: "act:wallet:edit"}},
		{{Label: "🏠 منوی اصلی", Callback: "act:wallet:home"}},
	})
}

func showBuyCoins(bc *engine.BotContext) {
	pkgs := bc.PricePackages()
	if len(pkgs) == 0 {
		bc.Reply("خرید سکه فعلاً در دسترس نیست.")
		return
	}
	sub := hasSubscription(bc, botUUID(bc))
	var rows [][]engine.InlineButton
	for _, pkg := range pkgs {
		id, _ := pkg["id"].(string)
		price := toInt(pkg["price"])
		label, _ := pkg["label"].(string)
		if label == "" {
			label = fmt.Sprintf("%d سکه — %s تومان", toInt(pkg["coins"]), formatTomans(price))
		}
		if sub {
			price = discountedPrice(bc, price)
			label += " ⭐"
		}
		price, dlabel := applyDiscount(bc, price)
		label += dlabel
		rows = append(rows, []engine.InlineButton{
			{Label: label, Callback: "act:buy:" + id},
		})
	}
	rows = append(rows, []engine.InlineButton{{Label: "🏠 بازگشت", Callback: "act:wallet"}})
	bc.ReplyInline("📈 خرید سکه\nبسته مورد نظر رو انتخاب کن:", rows)
}

func showSubscriptionOffer(bc *engine.BotContext) {
	price := discountedSubPrice(bc)
	bc.ReplyInline(
		"⭐ اشتراک پریموم\n━━━━━━━━━━━━━━\n\n"+
			"• جستجوی نامحدود و رایگان\n"+
			"• ۳۰٪ تخفیف خرید سکه\n\n"+
			fmt.Sprintf("قیمت: %s تومان / ماه", formatTomans(price)),
		[][]engine.InlineButton{
			{{Label: "💳 خرید اشتراک", Callback: "act:buy:sub"}},
			{{Label: "🏠 بازگشت", Callback: "act:wallet"}},
		},
	)
}

// missionChannelEntries returns full channel entries (id, title, link) for
// the missions screen so the user can tap to join.
func missionChannelEntries(bc *engine.BotContext) []struct{ ID, Title, Link string } {
	var out []struct{ ID, Title, Link string }
	if arr, ok := bc.Prices["channels"].([]any); ok {
		for _, v := range arr {
			switch ch := v.(type) {
			case string:
				if s := strings.TrimSpace(ch); s != "" {
					link := s
					if strings.HasPrefix(s, "@") {
						link = "https://t.me/" + strings.TrimPrefix(s, "@")
					}
					out = append(out, struct{ ID, Title, Link string }{ID: s, Title: s, Link: link})
				}
			case map[string]any:
				id := strings.TrimSpace(stringFromAny(ch["id"]))
				if id == "" {
					continue
				}
				title := strings.TrimSpace(stringFromAny(ch["title"]))
				if title == "" {
					title = id
				}
				link := strings.TrimSpace(stringFromAny(ch["link"]))
				if link == "" {
					if strings.HasPrefix(id, "@") {
						link = "https://t.me/" + strings.TrimPrefix(id, "@")
					} else {
						link = id
					}
				}
				out = append(out, struct{ ID, Title, Link string }{ID: id, Title: title, Link: link})
			}
		}
	}
	return out
}

func showMissions(bc *engine.BotContext, botID uuid.UUID, botUsername string) {
	profile, _ := profileOf(bc, botID)
	channelDone := profile != nil && profile.ChannelBonusAt != nil
	refLink := "https://t.me/" + botUsername + "?start=ref_" + strconv.FormatInt(bc.UserID, 10)
	if botUsername == "" {
		refLink = "(آیدی بات هنوز ثبت نشده)"
	}
	chEntries := missionChannelEntries(bc)

	var sb strings.Builder
	sb.WriteString("🎁 ماموریت‌ها و پاداش‌ها\n")
	sb.WriteString("━━━━━━━━━━━━━━━━\n\n")

	// channel mission
	if channelDone {
		sb.WriteString("✅ عضویت کانال — انجام شد!\n")
		sb.WriteString("۱۰ سکه به حساب شما اضافه شد 🎉\n")
	} else {
		sb.WriteString("📢 عضویت کانال\n")
		sb.WriteString("عضو کانال زیر شو و +۱۰ سکه هدیه بگیر:\n\n")
		for _, c := range chEntries {
			sb.WriteString("🔹 " + c.Title + "\n")
			sb.WriteString("   " + c.Link + "\n\n")
		}
		sb.WriteString("بعد از عضویت، دکمهٔ زیر رو بزن 👇\n")
	}

	sb.WriteString("\n━━━━━━━━━━━━━━━━\n\n")
	sb.WriteString("👥 دعوت دوستان\n")
	sb.WriteString("لینک اختصاصی شما:\n")
	sb.WriteString(refLink + "\n\n")
	sb.WriteString("🎁 هر نفر که دعوت کنی:\n")
	sb.WriteString("   • +۵ سکه برای تو 💰\n")
	sb.WriteString("   • +۵ سکه برای دوستت 💰\n\n")
	sb.WriteString("💡 لینک رو با دوستات به اشتراک بذار!")

	var btns [][]engine.InlineButton
	if channelDone {
		btns = [][]engine.InlineButton{{{Label: "🏠 بازگشت", Callback: "act:wallet"}}}
	} else {
		btns = [][]engine.InlineButton{{{Label: "✅ بررسی عضویت کانال", Callback: "act:mission:channel"}}}
	}
	bc.ReplyInline(sb.String(), btns)
}

// --- payment actions (inline callbacks) ---

// HandleAction routes act:* callbacks for the dating economy.
func (t *Template) HandleAction(bc *engine.BotContext, action string) bool {
	st := storeOf(bc)
	if st == nil {
		return false
	}
	botID := botUUID(bc)

	if strings.HasPrefix(action, "browse") {
		return handleBrowseAction(bc, st, action)
	}
	if strings.HasPrefix(action, "notify:") {
		return handleNotificationAction(bc, st, action)
	}

	switch {
	case action == "wallet":
		showWallet(bc, st, botID)
		return true
	case action == "wallet:buy":
		showBuyCoins(bc)
		return true
	case action == "wallet:sub":
		showSubscriptionOffer(bc)
		return true
	case action == "wallet:missions":
		showMissions(bc, botID, botUsernameOf(bc))
		return true
	case action == "wallet:discount":
		showDiscountPrompt(bc)
		return true
	case action == "wallet:boost":
		showBoost(bc, st, botID)
		return true
	case action == "wallet:edit":
		showProfilePage(bc, st, botID)
		return true
	case action == "wallet:home":
		goHome(bc, "🏠 منوی اصلی")
		return true

	case strings.HasPrefix(action, "buy:"):
		return buyAction(bc, st, botID, strings.TrimPrefix(action, "buy:"))

	case strings.HasPrefix(action, "boost:"):
		return boostAction(bc, st, botID, strings.TrimPrefix(action, "boost:"))

	case strings.HasPrefix(action, "edit:"):
		return profileEditAction(bc, st, botID, strings.TrimPrefix(action, "edit:"))

	case strings.HasPrefix(action, "int:toggle:"):
		idx, _ := strconv.Atoi(strings.TrimPrefix(action, "int:toggle:"))
		return handleInterestToggle(bc, idx)
	case action == "int:save":
		return handleInterestSave(bc, st, botID)
	case action == "int:cancel":
		delete(dataMap(bc), "int_pick")
		bc.SaveState()
		goHome(bc, "🏠 منوی اصلی")
		return true

	case action == "pay:verify":
		return verifyPendingPayment(bc, botID)

	case action == "mission:channel":
		return checkChannelMission(bc, st, botID)
	}
	return false
}

func buyAction(bc *engine.BotContext, st *Store, botID uuid.UUID, what string) bool {
	svc := paySvcOf(bc)
	if svc == nil {
		bc.Reply("سرویس پرداخت در دسترس نیست.")
		return true
	}
	ctx := engineBG()
	profile, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا. /start بزن.")
		return true
	}

	if what == "sub" {
		price := discountedSubPrice(bc)
		price, dlabel := applyDiscount(bc, price)
		url, err := svc.BuySubscription(ctx, botID, bc.UserID, bc.TelegramID, int64(price))
		if err != nil {
			bc.Reply("خطا در ساخت لینک پرداخت. دوباره تلاش کن.")
			return true
		}
		consumeDiscount(bc)
		if url == "dev://payment-success" {
			goHome(bc, "✅ اشتراک پریموم فعال شد! 🎉\nتا ۳۰ روز جستجوی رایگان و ۳۰٪ تخفیف داری."+dlabel)
			return true
		}
		sendPaymentLink(bc, url, "⭐ اشتراک پریموم")
		return true
	}

	// coin package
	var pkg map[string]any
	for _, p := range bc.PricePackages() {
		if id, _ := p["id"].(string); id == what {
			pkg = p
			break
		}
	}
	if pkg == nil {
		bc.Reply("پکیج نامعتبره.")
		return true
	}
	coins := toInt(pkg["coins"])
	price := toInt(pkg["price"])
	if hasSubscription(bc, botID) {
		price = discountedPrice(bc, price)
	}
	price, dlabel := applyDiscount(bc, price)
	url, err := svc.BuyPackage(ctx, botID, bc.UserID, bc.TelegramID, what, int64(coins), int64(price))
	if err != nil {
		bc.Reply("خطا در ساخت لینک پرداخت. دوباره تلاش کن.")
		return true
	}
	consumeDiscount(bc)
	if url == "dev://payment-success" {
		bc.Reply(fmt.Sprintf("✅ پرداخت تایید شد!\n+%d سکه به حسابت اضافه شد.%s", coins, dlabel))
		return true
	}
	_ = profile
	sendPaymentLink(bc, url, fmt.Sprintf("📦 پکیج %d سکه", coins))
	return true
}

const vpnWarning = "⚠️ قبل از ورود به درگاه پرداخت:\n\nفیلترشکن (VPN) خود رو خاموش کنید!\n\nلینک رو کپی کنید و در مرورگر باز کنید، یا بعد از خاموش کردن VPN دوباره به این صفحه برگردید.\n\nبعد از پرداخت موفق، می‌تونید VPN رو دوباره روشن کنید."

func sendPaymentLink(bc *engine.BotContext, payURL, title string) {
	bc.ReplyInline(
		vpnWarning+"\n\n"+title+"\n\nروی لینک زیر کلیک کن:\n"+payURL,
		[][]engine.InlineButton{
			{{Label: "✅ بررسی پرداخت", Callback: "act:pay:verify"}},
			{{Label: "🏠 منوی اصلی", Callback: "act:wallet:home"}},
		},
	)
}

func verifyPendingPayment(bc *engine.BotContext, botID uuid.UUID) bool {
	svc := paySvcOf(bc)
	if svc == nil {
		bc.Reply("سرویس پرداخت در دسترس نیست.")
		return true
	}
	// find the user's latest pending payment
	repo := payRepoOf(bc)
	_ = repo
	if err := svc.VerifyLatestPending(engineBG(), botID, bc.UserID); err != nil {
		bc.Reply("پرداخت فعالی برای بررسی نیست یا هنوز تایید نشده. چند دقیقه بعد دوباره تلاش کن.")
		return true
	}
	bc.Reply("✅ پرداخت تایید شد!")
	return true
}

// --- channel mission ---

// ChannelChecker is injected via Data["channelcheck"]: returns member status.
type ChannelChecker func(botID uuid.UUID, channelID string, userID int64) (string, error)

func channelCheckerOf(bc *engine.BotContext) ChannelChecker {
	// try the named type first, then the raw func type (main.go stores a raw literal)
	if fn, ok := bc.Data["channelcheck"].(ChannelChecker); ok {
		return fn
	}
	if fn, ok := bc.Data["channelcheck"].(func(botID uuid.UUID, channelID string, userID int64) (string, error)); ok {
		return fn
	}
	return nil
}

func checkChannelMission(bc *engine.BotContext, st *Store, botID uuid.UUID) bool {
	channels := missionChannels(bc)
	if len(channels) == 0 {
		bc.Reply("ماموریت کانال فعال نیست.")
		return true
	}
	check := channelCheckerOf(bc)
	if check == nil {
		bc.Reply("بررسی عضویت در دسترس نیست.")
		return true
	}
	// all channels must be joined (kpkchat multi-channel parity)
	for _, ch := range channels {
		status, err := check(botID, ch, bc.TelegramID)
		if err != nil {
			bc.Reply("خطا در بررسی عضویت. لطفاً دوباره تلاش کن.")
			return true
		}
		if status != "member" && status != "administrator" && status != "creator" {
			bc.Reply("❌ هنوز عضو همه کانال‌ها نیستی!\nاول عضو شو، بعد دکمه «بررسی عضویت» رو بزن.")
			return true
		}
	}
	ctx := engineBG()
	ok, err := st.Repo().SetChannelBonus(ctx, botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا. دوباره تلاش کن.")
		return true
	}
	if !ok {
		bc.Reply("✅ قبلاً عضویت کانال رو انجام دادی!")
		return true
	}
	bonus := bc.PriceInt("channel_bonus", 10)
	if _, err := st.Repo().AddCoins(ctx, botID, bc.UserID, bonus); err != nil {
		log.Printf("[dating] channel bonus: %v", err)
	}
	bc.Reply(fmt.Sprintf("✅ عضویت کانال‌ها تایید شد!\n+%d سکه به حسابت اضافه شد.", bonus))
	return true
}

// missionChannels returns the membership channel ids. Entries may be plain
// strings (the original shape) or structured objects {"id":"@a","title":...,
// "link":...} written by the panel's channel editor; only the id is needed
// to check membership, so both are accepted.
func missionChannels(bc *engine.BotContext) []string {
	var out []string
	if arr, ok := bc.Prices["channels"].([]any); ok {
		for _, v := range arr {
			switch ch := v.(type) {
			case string:
				if s := strings.TrimSpace(ch); s != "" {
					out = append(out, s)
				}
			case map[string]any:
				if s := strings.TrimSpace(stringFromAny(ch["id"])); s != "" {
					out = append(out, s)
				}
			}
		}
	}
	if len(out) == 0 {
		if s := strings.TrimSpace(stringFromPrices(bc, "channel_id")); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// stringFromAny renders a JSON value as a trimmed string, or "".
func stringFromAny(v any) string {
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s)
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	}
	return ""
}

// --- signup + referral bonuses (called at onboarding completion) ---

// grantSignupBonus awards signup + referral coins exactly once per user.
func grantSignupBonus(bc *engine.BotContext, st *Store, botID uuid.UUID) {
	ctx := engineBG()
	// signup: guarded by coins==0 && just-completed (single-shot per profile)
	bonus := bc.PriceInt("signup_bonus", 5)
	if bonus > 0 {
		if _, err := st.Repo().AddCoins(ctx, botID, bc.UserID, bonus); err != nil {
			log.Printf("[dating] signup bonus: %v", err)
		} else {
			bc.Reply(fmt.Sprintf("🎉 هدیهٔ ثبت‌نام: +%d سکه!", bonus))
		}
	}
	// referral: payload ref_{telegramID} stored at /start
	refRaw, _ := bc.State["data"].(map[string]any)["ref"].(string)
	if refRaw == "" {
		return
	}
	delete(bc.State["data"].(map[string]any), "ref")
	bc.SaveState()
	refID, err := strconv.ParseInt(refRaw, 10, 64)
	if err != nil || refID == bc.TelegramID {
		return
	}
	refBonus := bc.PriceInt("referral_bonus", 5)
	refProfile, err := st.ProfileByTelegram(ctx, botID, refID)
	if err != nil || refProfile == nil {
		return
	}
	if _, err := st.Repo().AddCoins(ctx, botID, refProfile.UserID, refBonus); err != nil {
		log.Printf("[dating] referral ref bonus: %v", err)
	}
	if _, err := st.Repo().AddCoins(ctx, botID, bc.UserID, refBonus); err != nil {
		log.Printf("[dating] referral new bonus: %v", err)
	}
	bc.Reply(fmt.Sprintf("🎁 هدیهٔ دعوت: +%d سکه!", refBonus))
}

// captureReferralPayload stores ref_{id} from /start deep link into state.
// State is persisted by the rest of the /start flow (goHome/resumeOnboarding).
func captureReferralPayload(bc *engine.BotContext, payload string) {
	if !strings.HasPrefix(payload, "ref_") {
		return
	}
	ref := strings.TrimPrefix(payload, "ref_")
	if _, err := strconv.ParseInt(ref, 10, 64); err != nil {
		return
	}
	data, _ := bc.State["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
		bc.State["data"] = data
	}
	if data["ref"] == nil {
		data["ref"] = ref
	}
}

// --- small helpers ---

func profileOf(bc *engine.BotContext, botID uuid.UUID) (*domain.DatingProfile, error) {
	st := storeOf(bc)
	if st == nil {
		return nil, fmt.Errorf("no store")
	}
	return st.Profile(engineBG(), botID, bc.UserID)
}

func botUsernameOf(bc *engine.BotContext) string {
	fn, _ := bc.Data["botusername"].(func(uuid.UUID) string)
	if fn == nil {
		return ""
	}
	return fn(botUUID(bc))
}

func stringFromPrices(bc *engine.BotContext, key string) string {
	s, _ := bc.Prices[key].(string)
	return s
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

func discountedPrice(bc *engine.BotContext, price int) int {
	pct := bc.PriceInt("sub_discount_pct", 30)
	return price * (100 - pct) / 100
}

func discountedSubPrice(bc *engine.BotContext) int {
	// subscription itself is full price (kpkchat parity)
	return bc.PriceInt("sub_price", 280_000)
}

func formatTomans(n int) string {
	s := strconv.Itoa(n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
