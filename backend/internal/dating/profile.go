package dating

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

// profile page + self-service edits — kpkchat «حساب من» parity.

// availableInterests mirrors kpkchat's core list (15 items, max 5 selected).
var availableInterests = []string{
	"موسیقی", "فیلم", "کتاب", "ورزش", "بازی",
	"برنامه‌نویسی", "هنر", "آشپزی", "سفر", "عکاسی",
	"علم", "تکنولوژی", "فلسفه", "تاریخ", "طبیعت",
}

const maxInterests = 5

// showProfilePage renders the rich «👤 پروفایل من» screen with the full edit
// keyboard (name, age, gender, location, photo, interests).
func showProfilePage(bc *engine.BotContext, st *Store, botID uuid.UUID) {
	ctx := engineBG()
	p, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		bc.ReplyKBDStyled("پروفایل پیدا نشد. /start بزن.", homeKB)
		return
	}
	genderFA := "نامشخص"
	if p.Gender != nil && *p.Gender == "female" {
		genderFA = "👩🏻 دختر"
	} else if p.Gender != nil {
		genderFA = "👨🏻 پسر"
	}
	lines := []string{
		"👤 حساب شما:",
		"━━━━━━━━━━━━━━",
		"📝 نام: " + derefStr(p.Name),
		"⚥ جنسیت: " + genderFA,
		"🎂 سن: " + derefInt(p.Age),
		"📍 استان: " + derefStr(p.Province),
		"🏙 شهر: " + derefStr(p.City),
	}
	if len(p.Interests) > 0 {
		lines = append(lines, "🎯 علاقه‌مندی‌ها: "+strings.Join(p.Interests, "، "))
	} else {
		lines = append(lines, "🎯 علاقه‌مندی‌ها: ندارد")
	}
	lines = append(lines,
		"⭐ امتیاز: "+formatRating(p.Rating, p.TotalRatings),
		"📷 عکس: "+photoStatus(p),
		fmt.Sprintf("💰 سکه: %d", p.Coins),
	)
	if r := payRepoOf(bc); r != nil {
		if subEnd, ok, _ := r.ActiveSubscriptionEnd(ctx, botID, bc.UserID); ok {
			lines = append(lines, "⭐ اشتراک: ویژه فعال تا "+subEnd.Format("2006-01-02"))
		} else {
			lines = append(lines, "⭐ اشتراک: ندارید")
		}
	}
	bc.ReplyInline(strings.Join(lines, "\n"), [][]engine.InlineButton{
		{{Label: "✏️ تغییر نام", Callback: "act:edit:name"}, {Label: "✏️ تغییر سن", Callback: "act:edit:age"}},
		{{Label: "✏️ تغییر جنسیت", Callback: "act:edit:gender"}, {Label: "✏️ تغییر محل سکونت", Callback: "act:edit:location"}},
		{{Label: "📷 تغییر عکس پروفایل", Callback: "act:edit:photo"}, {Label: "🎯 تغییر علاقه‌مندی‌ها", Callback: "act:edit:interests"}},
		{{Label: "💰 سکه", Callback: "act:wallet"}, {Label: "🏠 منوی اصلی", Callback: "act:wallet:home"}},
	})
}

func photoStatus(p *domain.DatingProfile) string {
	if p.PhotoFileID != nil && *p.PhotoFileID != "" {
		return "📷 دارد"
	}
	return "📷 ندارد"
}

func derefInt(p *int) string {
	if p == nil {
		return "—"
	}
	return strconv.Itoa(*p)
}

// inCooldown reports whether t is newer than days days ago.
func inCooldown(t time.Time, days int) bool {
	return time.Since(t) < time.Duration(days)*24*time.Hour
}

// profileEditAction opens one edit flow. Cost model (kpkchat parity):
// name — edit_name_cost + name cooldown; every other field — edit_cost with
// the shared weekly profile cooldown (first profile edit is free); interests
// are free with no cooldown.
func profileEditAction(bc *engine.BotContext, st *Store, botID uuid.UUID, what string) bool {
	ctx := engineBG()
	p, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا. /start بزن.")
		return true
	}
	switch what {
	case "name":
		cost := bc.PriceInt("edit_name_cost", 100)
		if p.NameEditedAt == nil {
			cost = 0
		} else if days := bc.PriceInt("name_edit_cooldown_days", 14); inCooldown(*p.NameEditedAt, days) {
			bc.Reply(fmt.Sprintf("🔒 تغییر نام هر %d روز یک‌بار مجازه.", days))
			return true
		}
		if !spendOrRefuse(bc, st, botID, cost) {
			return true
		}
		bc.State["scene"] = "setup"
		setStep(bc, "setname")
		bc.Reply("✏️ نام جدید رو بنویس (حداقل ۲ حرف):")
		return true

	case "interests":
		return openInterestsPicker(bc, p)

	case "photo":
		cost := bc.PriceInt("edit_cost", 50)
		if p.ProfileEditedAt != nil && inCooldown(*p.ProfileEditedAt, bc.PriceInt("edit_cooldown_days", 7)) {
			bc.Reply(fmt.Sprintf("🔒 تغییر عکس هر %d روز یک‌بار مجازه.", bc.PriceInt("edit_cooldown_days", 7)))
			return true
		}
		if p.ProfileEditedAt == nil {
			cost = 0
		}
		if !spendOrRefuse(bc, st, botID, cost) {
			return true
		}
		dataMap(bc)["edit_profile"] = true
		bc.SaveState()
		bc.State["scene"] = "setup"
		setStep(bc, "editphoto")
		bc.ReplyKBD("📷 عکس جدید پروفایل رو بفرست:\n\n(فقط عکس — jpg، png و…)\nویدیو و صدا پذیرفته نمی‌شود.", [][]string{{"🏠 منوی اصلی"}})
		return true

	default: // age | gender | location — shared weekly gate
		cost := bc.PriceInt("edit_cost", 50)
		if p.ProfileEditedAt != nil && inCooldown(*p.ProfileEditedAt, bc.PriceInt("edit_cooldown_days", 7)) {
			bc.Reply(fmt.Sprintf("🔒 تغییر این بخش هر %d روز یک‌بار مجازه.", bc.PriceInt("edit_cooldown_days", 7)))
			return true
		}
		if p.ProfileEditedAt == nil {
			cost = 0
		}
		if !spendOrRefuse(bc, st, botID, cost) {
			return true
		}
		dataMap(bc)["edit_profile"] = true
		bc.SaveState()
		bc.State["scene"] = "setup"
		switch what {
		case "age":
			setStep(bc, "setage")
			bc.ReplyKBD("🎂 سن جدید رو انتخاب کن (۱۸ تا ۷۰):", ageRows())
		case "gender":
			setStep(bc, "setgender")
			bc.ReplyKBD("جنسیت خود را انتخاب کن:", genderRows(pw(p.Age)))
		default: // location
			setStep(bc, "setprovince")
			bc.ReplyKBD("📍 استان جدید رو انتخاب کن:\n(بعد شهر رو هم می‌پرسم)", provinceRows())
		}
		return true
	}
}

func spendOrRefuse(bc *engine.BotContext, st *Store, botID uuid.UUID, cost int) bool {
	if cost <= 0 {
		return true
	}
	if ok, _ := st.Repo().SpendCoins(engineBG(), botID, bc.UserID, cost); !ok {
		bc.Reply(fmt.Sprintf("❌ سکه کافی نیست (هزینه: %d سکه).", cost))
		return false
	}
	return true
}

// --- interests picker (inline toggle, max 5) ---

func openInterestsPicker(bc *engine.BotContext, p *domain.DatingProfile) bool {
	data := dataMap(bc)
	cur := p.Interests
	if cur == nil {
		cur = []string{}
	}
	data["int_pick"] = cur
	bc.SaveState()
	bc.ReplyInline(interestsIntro(cur), interestsKeyboard(cur))
	return true
}

func interestsIntro(sel []string) string {
	head := "🎯 علاقه‌مندی‌ها\n━━━━━━━━━━━━━━\nاز دکمه‌های زیر علاقه‌مندی‌هات رو انتخاب کن.\nحداکثر ۵ تا."
	if len(sel) > 0 {
		return head + "\n\nانتخاب فعلی: " + strings.Join(sel, "، ")
	}
	return head + "\n\nفعلاً چیزی انتخاب نشده."
}

func interestsKeyboard(sel []string) [][]engine.InlineButton {
	isSel := map[string]bool{}
	for _, s := range sel {
		isSel[s] = true
	}
	var rows [][]engine.InlineButton
	for i := 0; i < len(availableInterests); i += 3 {
		var row []engine.InlineButton
		for _, interest := range availableInterests[i:min(i+3, len(availableInterests))] {
			label := "☐ " + interest
			if isSel[interest] {
				label = "✅ " + interest
			}
			row = append(row, engine.InlineButton{Label: label, Callback: "act:int:toggle:" + strconv.Itoa(indexOfInterest(interest))})
		}
		rows = append(rows, row)
	}
	rows = append(rows, []engine.InlineButton{{Label: "✔️ تایید و ثبت", Callback: "act:int:save"}})
	return rows
}

func indexOfInterest(name string) int {
	for i, s := range availableInterests {
		if s == name {
			return i
		}
	}
	return -1
}

// handleInterestToggle flips one interest in the pending selection and
// re-renders the picker.
func handleInterestToggle(bc *engine.BotContext, idx int) bool {
	if idx < 0 || idx >= len(availableInterests) {
		return true
	}
	data := dataMap(bc)
	sel := toStringSlice(data["int_pick"])
	name := availableInterests[idx]
	out := sel[:0:0]
	found := false
	for _, s := range sel {
		if s == name {
			found = true
			continue
		}
		out = append(out, s)
	}
	if !found {
		if len(sel) >= maxInterests {
			bc.Reply(fmt.Sprintf("حداکثر %d علاقه‌مندی می‌تونی انتخاب کنی.", maxInterests))
			return true
		}
		out = append(out, name)
	}
	data["int_pick"] = out
	bc.SaveState()
	bc.ReplyInline(interestsIntro(out), interestsKeyboard(out))
	return true
}

// handleInterestSave persists the pending selection.
func handleInterestSave(bc *engine.BotContext, st *Store, botID uuid.UUID) bool {
	data := dataMap(bc)
	sel := toStringSlice(data["int_pick"])
	delete(data, "int_pick")
	bc.SaveState()
	if len(sel) > maxInterests {
		sel = sel[:maxInterests]
	}
	if _, err := st.Repo().UpdateProfile(engineBG(), botID, bc.UserID, postgres.ProfileUpdate{Interests: sel}); err != nil {
		bc.Reply("خطا در ذخیره. دوباره تلاش کن.")
		return true
	}
	if len(sel) == 0 {
		bc.Reply("✅ علاقه‌مندی‌ها پاک شد.")
		return true
	}
	bc.Reply("✅ علاقه‌مندی‌ها ثبت شد: " + strings.Join(sel, "، "))
	return true
}

func toStringSlice(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	// also accept []string (in-memory state before JSON round-trip)
	if ss, ok := v.([]string); ok {
		return ss
	}
	return out
}
