package dating

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"
)

// Browse is the Tinder-style card deck (phase 2): free like quota per day,
// paid extra likes, superlike with daily cap, direct chat, mutual connect.
// All costs/limits come from the bot's merged prices (per-bot dynamic).

func dayStart() time.Time { return time.Now().UTC().Truncate(24 * time.Hour) }

// browseLikeCost returns the coin cost of one like (0 = free).
func browseLikeCost(bc *engine.BotContext, subscribed bool, likesToday int) int {
	if subscribed {
		return 0
	}
	if likesToday < bc.PriceInt("browse_free_likes", 10) {
		return 0
	}
	return bc.PriceInt("browse_like_cost", 1)
}

// oppositeGender for the browse deck ("" viewer gender → all).
func oppositeGender(viewerGender string) string {
	switch viewerGender {
	case "male":
		return "female"
	case "female":
		return "male"
	}
	return ""
}

// browseCaption renders the card text.
func browseCaption(t *domain.DatingProfile, likeCost, freeLeft int, subscribed bool) string {
	name := "?"
	if t.Name != nil {
		name = *t.Name
	}
	age := "?"
	if t.Age != nil {
		age = strconv.Itoa(*t.Age)
	}
	genderFA := ""
	if t.Gender != nil {
		if *t.Gender == "female" {
			genderFA = " 👩"
		} else {
			genderFA = " 👨"
		}
	}
	city := ""
	if t.City != nil && *t.City != "" {
		city = "📍 " + *t.City
	}
	province := ""
	if t.Province != nil && *t.Province != "" && (t.City == nil || *t.City == "" || *t.Province != *t.City) {
		if city != "" {
			province = "، " + *t.Province
		} else {
			province = "📍 " + *t.Province
		}
	}

	var sb strings.Builder
	sb.WriteString("━━━━━━━━━━━━━━━━\n")
	sb.WriteString("💕 " + name + genderFA + "\n")
	sb.WriteString("🎂 " + age + " ساله\n")
	if city != "" || province != "" {
		sb.WriteString(city + province + "\n")
	}
	sb.WriteString("⭐ " + formatRating(t.Rating, t.TotalRatings) + "\n")

	// interests
	if len(t.Interests) > 0 {
		shown := t.Interests
		if len(shown) > 5 {
			shown = shown[:5]
		}
		sb.WriteString("🎯 " + strings.Join(shown, " · ") + "\n")
	}

	sb.WriteString("━━━━━━━━━━━━━━━━\n")
	switch {
	case subscribed:
		sb.WriteString("💎 لایک رایگان (پریموم)")
	case likeCost == 0:
		sb.WriteString(fmt.Sprintf("💚 لایک رایگان (%d باقی‌مانده)", freeLeft))
	default:
		sb.WriteString(fmt.Sprintf("💛 لایک: %d سکه", likeCost))
	}
	return sb.String()
}

func browseButtons(likeCost, superCost, directCost int) [][]engine.InlineButton {
	likeLabel := "❤️ لایک"
	if likeCost > 0 {
		likeLabel = fmt.Sprintf("❤️ لایک (%d سکه)", likeCost)
	}
	superLabel := "⭐ سوپرلایک"
	if superCost > 0 {
		superLabel = fmt.Sprintf("⭐ سوپرلایک (%d سکه)", superCost)
	}
	directLabel := "💬 چت مستقیم"
	if directCost > 0 {
		directLabel = fmt.Sprintf("💬 چت مستقیم (%d سکه)", directCost)
	}
	return [][]engine.InlineButton{
		{{Label: likeLabel, Callback: "act:browse:like"}, {Label: superLabel, Callback: "act:browse:super"}},
		{{Label: directLabel, Callback: "act:browse:chat"}, {Label: "👉 بعدی ←", Callback: "act:browse:next"}},
		{{Label: "🏠 منوی اصلی", Callback: "act:browse:exit"}},
	}
}

// startBrowse enters the browse scene and shows the first card. Plain entry
// («💘 مرور پروفایل‌ها») always starts an unfiltered deck.
func startBrowse(bc *engine.BotContext, st *Store) bool {
	if denyIfBlocked(bc, st) {
		return true
	}
	clearBrowseFilters(bc)
	bc.State["scene"] = "browse"
	bc.State["step"] = ""
	bc.SaveState()
	return showNextCard(bc, st)
}

// startFilteredBrowse enters browse with wizard-chosen constraints
// (kpkchat «جستجوی کاربران» browse mode: free filtered deck). gender "" or
// "any" keeps the default opposite-gender baseline.
func startFilteredBrowse(bc *engine.BotContext, st *Store, gender, province, city string, interests []string, ageMin, ageMax int) bool {
	if denyIfBlocked(bc, st) {
		return true
	}
	data := dataMap(bc)
	if gender == "female" || gender == "male" {
		data["browse_gender"] = gender
	} else {
		delete(data, "browse_gender")
	}
	if province != "" {
		data["browse_province"] = province
	} else {
		delete(data, "browse_province")
	}
	if city != "" {
		data["browse_city"] = city
	} else {
		delete(data, "browse_city")
	}
	if len(interests) > 0 {
		data["browse_interests"] = interests
	} else {
		delete(data, "browse_interests")
	}
	if ageMin > 0 {
		data["browse_age_min"] = ageMin
	} else {
		delete(data, "browse_age_min")
	}
	if ageMax > 0 {
		data["browse_age_max"] = ageMax
	} else {
		delete(data, "browse_age_max")
	}
	bc.State["scene"] = "browse"
	bc.State["step"] = ""
	bc.SaveState()
	return showNextCard(bc, st)
}

func clearBrowseFilters(bc *engine.BotContext) {
	data := dataMap(bc)
	delete(data, "browse_gender")
	delete(data, "browse_province")
	delete(data, "browse_city")
	delete(data, "browse_interests")
	delete(data, "browse_age_min")
	delete(data, "browse_age_max")
}

func showNextCard(bc *engine.BotContext, st *Store) bool {
	ctx := engineBG()
	botID := botUUID(bc)
	viewer, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا. /start بزن.")
		return true
	}
	data := dataMap(bc)
	want := oppositeGender(derefStr(viewer.Gender))
	if g := stateString(data["browse_gender"]); g == "male" || g == "female" {
		want = g
	}
	filt := postgres.BrowseFilter{
		Gender:         want,
		ProvinceFilter: stateString(data["browse_province"]),
		City:           stateString(data["browse_city"]),
		Interests:      toStringSlice(data["browse_interests"]),
		AgeMin:         stateInt(data["browse_age_min"]),
		AgeMax:         stateInt(data["browse_age_max"]),
	}
	cands, err := st.Repo().BrowseCandidatesFiltered(ctx, botID, bc.UserID, filt, 1)
	if err != nil {
		bc.Reply("خطا در مرور پروفایل‌ها. دوباره تلاش کن.")
		return true
	}
	if len(cands) == 0 {
		delete(data, "browse_target")
		clearBrowseFilters(bc)
		bc.SaveState()
		goHome(bc, ttext(bc, "search.empty", "فعلاً کسی برای مرور باقی نمونده 😅\nکمی بعد دوباره سر بزن."))
		return true
	}
	t := cands[0]
	data["browse_target"] = t.UserID

	likesToday, _ := st.Repo().CountLikesSince(ctx, botID, bc.UserID, dayStart())
	sub := hasSubscription(bc, botID)
	freeQuota := bc.PriceInt("browse_free_likes", 10)
	freeLeft := freeQuota - likesToday
	if freeLeft < 0 {
		freeLeft = 0
	}
	likeCost := browseLikeCost(bc, sub, likesToday)
	superCost := bc.PriceInt("superlike_cost", 3)
	directCost := bc.PriceInt("direct_chat_cost", 5)
	caption := browseCaption(&t, likeCost, freeLeft, sub)
	buttons := browseButtons(likeCost, superCost, directCost)
	// track the card message so exits (reply-keyboard home label has no
	// callback MsgID pointing at the card) can close it in place
	msgID := int64(0)
	isPhoto := false
	if t.PhotoFileID != nil && *t.PhotoFileID != "" {
		if m := bc.SendPhotoKBD(bc.ChatID, *t.PhotoFileID, caption, buttons); m != nil {
			msgID = m.MessageID
			isPhoto = len(m.Photo) > 0
		}
	} else {
		if m := bc.ReplyInline(caption, buttons); m != nil {
			msgID = m.MessageID
		}
	}
	if msgID > 0 {
		data["browse_card_msg"] = msgID
		if isPhoto {
			data["browse_card_photo"] = true
		} else {
			delete(data, "browse_card_photo")
		}
	}
	bc.SaveState()
	return true
}

// handleBrowseText handles plain text while in the browse scene. The home
// reply keyboard stays visible (cards use inline buttons), so home-menu
// labels must navigate instead of advancing the deck.
func handleBrowseText(bc *engine.BotContext, st *Store, text string) bool {
	// home-menu labels are routed by the central guard in HandleMessage;
	// any other text advances the deck — close the old card first so its
	// buttons don't stay live on a stale message.
	finishCard(bc, "➡️ رد شد")
	return showNextCard(bc, st)
}

// handleBrowseAction routes act:browse* callbacks.
func handleBrowseAction(bc *engine.BotContext, st *Store, action string) bool {
	switch action {
	case "browse":
		return startBrowse(bc, st)
	case "browse:exit":
		finishCard(bc, "🏠 منوی اصلی")
		clearBrowseFilters(bc)
		goHome(bc, "🏠 منوی اصلی")
		return true
	case "browse:next":
		return browsePass(bc, st)
	case "browse:like":
		return browseReact(bc, st, "like")
	case "browse:super":
		return browseReact(bc, st, "super")
	case "browse:chat":
		return browseDirectChat(bc, st)
	}
	return false
}

func browseTarget(bc *engine.BotContext) (int64, bool) {
	data := dataMap(bc)
	switch v := data["browse_target"].(type) {
	case float64:
		return int64(v), v > 0
	case int64:
		return v, v > 0
	case int:
		return int64(v), v > 0
	}
	return 0, false
}

func browsePass(bc *engine.BotContext, st *Store) bool {
	ctx := engineBG()
	botID := botUUID(bc)
	if target, ok := browseTarget(bc); ok {
		_, _ = st.Repo().InsertLike(ctx, botID, bc.UserID, target, "pass")
	}
	finishCard(bc, "➡️ رد شد")
	return showNextCard(bc, st)
}

// finishCard edits the on-screen browse card to remove its buttons. Prefers
// the tracked card message id (reply-keyboard exits arrive as text messages
// whose MsgID points at the user's message, not the card). Photo cards must
// be edited via caption — editMessageText is rejected for media.
func finishCard(bc *engine.BotContext, text string) {
	data := dataMap(bc)
	msgID := stateInt(data["browse_card_msg"])
	if msgID == 0 {
		msgID = int(bc.MsgID)
	}
	isPhoto, _ := data["browse_card_photo"].(bool)
	delete(data, "browse_card_msg")
	delete(data, "browse_card_photo")
	if msgID <= 0 {
		return
	}
	if isPhoto {
		if ce, ok := bc.Sender.(engine.CaptionEditor); ok {
			_ = ce.EditCaption(engineBG(), bc.ChatID, int64(msgID), text, nil)
			return
		}
		// no caption support — leave the photo card as-is
		return
	}
	_ = bc.Sender.Edit(engineBG(), bc.ChatID, int64(msgID), text, nil)
}

func browseReact(bc *engine.BotContext, st *Store, kind string) bool {
	ctx := engineBG()
	botID := botUUID(bc)
	target, ok := browseTarget(bc)
	if !ok {
		goHome(bc, "کارت منقضی شد. دوباره مرور کن.")
		return true
	}

	var cost int
	if kind == "super" {
		cap := bc.PriceInt("superlike_daily_cap", 3)
		supers, _ := st.Repo().CountSuperSince(ctx, botID, bc.UserID, dayStart())
		if supers >= cap {
			finishCard(bc, "⚠️ امروز سقف سوپرلایک ("+strconv.Itoa(cap)+") تموم شد.")
			return showNextCard(bc, st)
		}
		cost = bc.PriceInt("superlike_cost", 3) // superlike always costs (kpkchat parity)
	} else {
		likesToday, _ := st.Repo().CountLikesSince(ctx, botID, bc.UserID, dayStart())
		cost = browseLikeCost(bc, hasSubscription(bc, botID), likesToday)
	}
	if cost > 0 {
		ok, err := st.Repo().SpendCoins(ctx, botID, bc.UserID, cost)
		if err != nil || !ok {
			bc.Reply(fmt.Sprintf("💰 سکه کافی نیست! (هزینه: %d سکه)\nاز بخش «💰 سکه» سکه بخر.", cost))
			return true
		}
	}

	inserted, err := st.Repo().InsertLike(ctx, botID, bc.UserID, target, kind)
	if err != nil {
		if cost > 0 {
			_, _ = st.Repo().AddCoins(ctx, botID, bc.UserID, cost)
		}
		bc.Reply("خطا در ثبت واکنش. دوباره تلاش کن.")
		return true
	}
	if !inserted {
		// already reacted to this card earlier
		if cost > 0 {
			_, _ = st.Repo().AddCoins(ctx, botID, bc.UserID, cost)
		}
		finishCard(bc, "⚠️ قبلاً به این پروفایل واکنش دادی.")
		return showNextCard(bc, st)
	}

	mutual, _ := st.Repo().HasLiked(ctx, botID, target, bc.UserID)
	if mutual {
		via := "لایک دوطرفه ❤️"
		if kind == "super" {
			via = "سوپرلایک ⭐"
		}
		return connectBrowse(bc, st, target, via)
	}
	if kind == "super" {
		finishCard(bc, "⭐ سوپرلایک ثبت شد! اگر طرف هم تو رو لایک کنه وصل می‌شید.")
		// one-sided superlike: ping the recipient (kpkchat parity)
		if tp, err := st.Profile(ctx, botID, target); err == nil && !tp.IsGhost {
			_, _ = bc.SendTo(tp.TelegramID, "⭐ کسی به پروفایلت سوپرلایک زد!\nبا مرور پروفایل‌ها می‌تونی پیداش کنی.", 0)
		}
	} else {
		finishCard(bc, "✅ لایک ثبت شد.")
	}
	return showNextCard(bc, st)
}

func browseDirectChat(bc *engine.BotContext, st *Store) bool {
	ctx := engineBG()
	botID := botUUID(bc)
	target, ok := browseTarget(bc)
	if !ok {
		goHome(bc, "کارت منقضی شد. دوباره مرور کن.")
		return true
	}
	cost := bc.PriceInt("direct_chat_cost", 5)
	if cost > 0 && !hasSubscription(bc, botID) {
		ok, err := st.Repo().SpendCoins(ctx, botID, bc.UserID, cost)
		if err != nil || !ok {
			bc.Reply(fmt.Sprintf("💰 سکه کافی نیست! (چت مستقیم: %d سکه)\nاز بخش «💰 سکه» سکه بخر.", cost))
			return true
		}
	}
	ok, err := st.ConnectUsers(ctx, botID, bc.UserID, target)
	if err != nil {
		if cost > 0 {
			_, _ = st.Repo().AddCoins(ctx, botID, bc.UserID, cost)
		}
		bc.Reply("خطا در وصل کردن. دوباره تلاش کن.")
		return true
	}
	if !ok {
		if cost > 0 {
			_, _ = st.Repo().AddCoins(ctx, botID, bc.UserID, cost)
		}
		finishCard(bc, "⚠️ این نفر الان به کسی وصل شده. بعداً دوباره ببینش.")
		return showNextCard(bc, st)
	}
	// record so the card never reappears
	_, _ = st.Repo().InsertLike(ctx, botID, bc.UserID, target, "like")
	return notifyBrowseConnect(bc, st, target, "چت مستقیم 💬")
}

// connectBrowse completes a mutual like/superlike into a chat.
func connectBrowse(bc *engine.BotContext, st *Store, target int64, via string) bool {
	ctx := engineBG()
	botID := botUUID(bc)
	ok, err := st.ConnectUsers(ctx, botID, bc.UserID, target)
	if err != nil {
		bc.Reply("خطا در وصل کردن. دوباره تلاش کن.")
		return true
	}
	if !ok {
		finishCard(bc, "⚠️ این نفر الان به کسی وصل شده.")
		return showNextCard(bc, st)
	}
	return notifyBrowseConnect(bc, st, target, via)
}

// notifyBrowseConnect puts both sides into the chat scene and notifies them.
func notifyBrowseConnect(bc *engine.BotContext, st *Store, target int64, via string) bool {
	ctx := engineBG()
	botID := botUUID(bc)
	finishCard(bc, "🎉 وصل شدید!")
	text := ttext(bc, "match.browse", "وصل شدید! 🎉") + " (" + via + ")" + humanRules

	targetProfile, err := st.Profile(ctx, botID, target)
	if err == nil {
		_ = st.SetScene(ctx, botID.String(), target, "chat")
		if _, sendErr := bc.SendKBDTo(targetProfile.TelegramID, text, chatRows); sendErr != nil {
			log.Printf("[dating] browse notify partner %d: %v", targetProfile.TelegramID, sendErr)
		}
	}
	bc.State["scene"] = "chat"
	bc.State["step"] = ""
	data := dataMap(bc)
	delete(data, "browse_target")
	bc.SaveState()
	bc.ReplyKBD(text, chatRows)
	return true
}
