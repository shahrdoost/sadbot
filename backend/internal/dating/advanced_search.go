package dating

import (
	"fmt"
	"strconv"
	"strings"

	"sadbot/internal/engine"
)

// handleAdvSearch is a small persistent wizard in the home scene. Every step,
// cost and timeout is scoped to bot prices/state so cloned bots configure it independently.

// startAdvWizard opens the filter wizard. browseMode=true is the kpkchat
// «جستجوی ویژه» / «جستجوی کاربران» path: same filters, but the result is a
// free filtered browse deck instead of a paid timed search.
func startAdvWizard(bc *engine.BotContext, browseMode bool) {
	data := dataMap(bc)
	if browseMode {
		data["adv_mode"] = "browse"
	} else {
		delete(data, "adv_mode")
	}
	bc.State["step"] = "adv_gender"
	bc.SaveState()
	if browseMode {
		bc.ReplyKBD(
			"🔍 جستجوی کاربران\n\nچه کسی رو می‌خوای پیدا کنی؟\n(مرور پروفایل‌های فیلترشده رایگان است)",
			[][]string{{"همه", "🚺 دختر", "🚹 پسر"}, {"🏠 منوی اصلی"}},
		)
		return
	}
	bc.ReplyKBD(
		"🔍 جستجوی پیشرفته\n\nچه کسی رو می‌خوای پیدا کنی؟",
		[][]string{{"همه", "🚺 دختر", "🚹 پسر"}, {"🏠 منوی اصلی"}},
	)
}

// provinceKeyboardRows lays out province names 2-per-row (kpkchat kb.province()).
func provinceKeyboardRows() [][]string {
	names := ProvinceNames()
	var rows [][]string
	for i := 0; i < len(names); i += 2 {
		row := []string{names[i]}
		if i+1 < len(names) {
			row = append(row, names[i+1])
		}
		rows = append(rows, row)
	}
	rows = append(rows, []string{"همه استان‌ها"}, []string{"🏠 منوی اصلی"})
	return rows
}

// cityKeyboardRows lays out cities of a province 2-per-row plus «همه شهرها».
func cityKeyboardRows(province string) [][]string {
	cities := Cities(province)
	var rows [][]string
	for i := 0; i < len(cities); i += 2 {
		row := []string{cities[i]}
		if i+1 < len(cities) {
			row = append(row, cities[i+1])
		}
		rows = append(rows, row)
	}
	rows = append(rows, []string{"همه شهرها"}, []string{"🏠 منوی اصلی"})
	return rows
}

var ageRanges = map[string][2]int{
	"۱۸-۲۲": {18, 22},
	"۲۳-۲۷": {23, 27},
	"۲۸-۳۲": {28, 32},
	"۳۳-۳۷": {33, 37},
	"۳۸-۴۲": {38, 42},
	"۴۳-۵۰": {43, 50},
	"۵۰-۷۰": {50, 70},
}

var ageRangeKB = [][]string{
	{"۱۸-۲۲", "۲۳-۲۷", "۲۸-۳۲"},
	{"۳۳-۳۷", "۳۸-۴۲", "۴۳-۵۰"},
	{"۵۰-۷۰", "بدون محدودیت"},
	{"🏠 منوی اصلی"},
}

// interestsFilterKB builds the toggle keyboard exactly like kpkchat
// (advancedSearchInterests): 15 interests 3-per-row with ✅/☐ prefix,
// plus a single confirm row. Tapping toggles; empty selection = no filter.
func interestsFilterKB(selected []string) [][]string {
	isSel := map[string]bool{}
	for _, s := range selected {
		isSel[s] = true
	}
	var rows [][]string
	for i := 0; i < len(availableInterests); i += 3 {
		var row []string
		for _, interest := range availableInterests[i:min(i+3, len(availableInterests))] {
			if isSel[interest] {
				row = append(row, "✅ "+interest)
			} else {
				row = append(row, "☐ "+interest)
			}
		}
		rows = append(rows, row)
	}
	rows = append(rows, []string{"✔️ تایید"}, []string{"🏠 منوی اصلی"})
	return rows
}

// interestsFilterText mirrors kpkchat's post-toggle message: always shows
// the current selection so the user sees the state after each tap.
func interestsFilterText(selected []string) string {
	if len(selected) == 0 {
		return "انتخاب شده: هیچ\n\nحداکثر ۵ تا انتخاب کن."
	}
	return "انتخاب شده: " + strings.Join(selected, "، ") + "\n\nحداکثر ۵ تا انتخاب کن."
}

func handleAdvSearch(bc *engine.BotContext, st *Store, text string) bool {
	if text == "🏠 منوی اصلی" {
		delete(dataMap(bc), "adv_mode")
		goHome(bc, "🏠 منوی اصلی")
		return true
	}
	data := dataMap(bc)
	switch step(bc) {
	case "adv_gender":
		gender := map[string]string{"همه": "any", "🚺 دختر": "female", "🚹 پسر": "male"}[text]
		if gender == "" {
			bc.ReplyKBD("یکی از گزینه‌ها رو انتخاب کن.", [][]string{{"همه", "🚺 دختر", "🚹 پسر"}, {"🏠 منوی اصلی"}})
			return true
		}
		data["adv_gender"] = gender
		bc.State["step"] = "adv_province"
		bc.SaveState()
		bc.ReplyKBD("استان مخاطب رو انتخاب کن:", provinceKeyboardRows())
		return true
	case "adv_province":
		if text == "همه استان‌ها" {
			delete(data, "adv_province")
			delete(data, "adv_city")
			bc.State["step"] = "adv_age"
			bc.SaveState()
			bc.ReplyKBD("بازه سنی مخاطب رو انتخاب کن:", ageRangeKB)
			return true
		}
		// validate province
		valid := false
		for _, p := range ProvinceNames() {
			if p == text {
				valid = true
				break
			}
		}
		if !valid {
			bc.ReplyKBD("یکی از استان‌ها رو انتخاب کن.", provinceKeyboardRows())
			return true
		}
		data["adv_province"] = text
		delete(data, "adv_city")
		bc.State["step"] = "adv_city"
		bc.SaveState()
		bc.ReplyKBD(fmt.Sprintf("شهرهای %s:", text), cityKeyboardRows(text))
		return true
	case "adv_city":
		province := stateString(data["adv_province"])
		if text == "همه شهرها" {
			delete(data, "adv_city")
			bc.State["step"] = "adv_age"
			bc.SaveState()
			bc.ReplyKBD("بازه سنی مخاطب رو انتخاب کن:", ageRangeKB)
			return true
		}
		// validate city is in the chosen province
		valid := false
		for _, c := range Cities(province) {
			if c == text {
				valid = true
				break
			}
		}
		if !valid {
			bc.ReplyKBD("یکی از شهرها رو انتخاب کن.", cityKeyboardRows(province))
			return true
		}
		data["adv_city"] = text
		bc.State["step"] = "adv_age"
		bc.SaveState()
		bc.ReplyKBD("بازه سنی مخاطب رو انتخاب کن:", ageRangeKB)
		return true
	case "adv_age":
		if text == "بدون محدودیت" {
			data["adv_age_min"] = 0
			data["adv_age_max"] = 0
		} else if rng, ok := ageRanges[text]; ok {
			data["adv_age_min"] = rng[0]
			data["adv_age_max"] = rng[1]
		} else {
			bc.ReplyKBD("یکی از بازه‌ها رو انتخاب کن.", ageRangeKB)
			return true
		}
		data["adv_interests"] = []string{}
		bc.State["step"] = "adv_interests"
		bc.SaveState()
		bc.ReplyKBD(interestsFilterText(nil), interestsFilterKB(nil))
		return true
	case "adv_interests":
		if text == "🏠 منوی اصلی" {
			delete(dataMap(bc), "adv_mode")
			goHome(bc, "🏠 منوی اصلی")
			return true
		}
		if text != "✔️ تایید" {
			// toggle an interest (kpkchat: strip prefix, add/remove, re-render)
			clean := strings.TrimPrefix(strings.TrimPrefix(text, "✅ "), "☐ ")
			found := false
			for _, i := range availableInterests {
				if i == clean {
					found = true
					break
				}
			}
			if !found {
				// unknown text: re-render current state (kpkchat returns null → no-op)
				bc.ReplyKBD(interestsFilterText(toStringSlice(data["adv_interests"])), interestsFilterKB(toStringSlice(data["adv_interests"])))
				return true
			}
			sel := toStringSlice(data["adv_interests"])
			if idx := indexOfStr(sel, clean); idx >= 0 {
				sel = append(sel[:idx], sel[idx+1:]...)
			} else {
				if len(sel) >= maxInterests {
					bc.ReplyKBD("حداکثر ۵ تا!", interestsFilterKB(sel))
					return true
				}
				sel = append(sel, clean)
			}
			data["adv_interests"] = sel
			bc.SaveState()
			bc.ReplyKBD(interestsFilterText(sel), interestsFilterKB(sel))
			return true
		}
		// "✔️ تایید" → advance to confirm
		data["adv_interests"] = toStringSlice(data["adv_interests"])
		min := stateInt(data["adv_age_min"])
		max := stateInt(data["adv_age_max"])
		data["adv_cost"] = bc.PriceInt("search_advanced_any", 1)
		bc.State["step"] = "adv_confirm"
		bc.SaveState()
		city, _ := data["adv_city"].(string)
		cityLabel := city
		if cityLabel == "" {
			prov, _ := data["adv_province"].(string)
			if prov != "" {
				cityLabel = prov + " (همه شهرها)"
			} else {
				cityLabel = "همه"
			}
		}
		genderLabel := map[string]string{"any": "همه", "female": "دختر", "male": "پسر"}[stateString(data["adv_gender"])]
		intSel := toStringSlice(data["adv_interests"])
		intLabel := "همه"
		if len(intSel) > 0 {
			intLabel = strings.Join(intSel, "، ")
		}
		if stateString(data["adv_mode"]) == "browse" {
			bc.ReplyKBD(fmt.Sprintf(
				"✅ فیلترها آماده است:\nجنسیت: %s\nشهر: %s\nسن: %s تا %s\nعلاقه‌مندی: %s\n🔎 مرور پروفایل‌های فیلترشده\n💰 رایگان",
				genderLabel, cityLabel, ageLabel(min), ageLabel(max), intLabel),
				[][]string{{"🔎 نمایش پروفایل‌ها"}, {"🏠 منوی اصلی"}},
			)
			return true
		}
		bc.ReplyKBD(fmt.Sprintf(
			"✅ فیلترها آماده است:\nجنسیت: %s\nشهر: %s\nسن: %s تا %s\nعلاقه‌مندی: %s\n⏱ جستجوی زمان‌دار (%d ثانیه)\n💰 هزینه: ۱ تا ۳ سکه (طبق تنظیم این بات)",
			genderLabel, cityLabel, ageLabel(min), ageLabel(max), intLabel, bc.PriceInt("search_advanced_timeout", 180)),
			[][]string{{"🚀 شروع جستجوی پیشرفته"}, {"🏠 منوی اصلی"}},
		)
		return true
	case "adv_confirm":
		if text != "🚀 شروع جستجوی پیشرفته" && text != "🔎 نمایش پروفایل‌ها" {
			bc.ReplyKBD("برای شروع دکمه رو بزن یا به منوی اصلی برگرد.", [][]string{{"🚀 شروع جستجوی پیشرفته"}, {"🏠 منوی اصلی"}})
			return true
		}
		return startAdvancedSearch(bc, st)
	}
	return false
}

func startAdvancedSearch(bc *engine.BotContext, st *Store) bool {
	ctx := engineBG()
	botID := botUUID(bc)
	profile, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil {
		goHome(bc, "پروفایل پیدا نشد. /start بزن.")
		return true
	}
	data := dataMap(bc)
	gender := stateString(data["adv_gender"])
	province := stateString(data["adv_province"])
	city := stateString(data["adv_city"])
	ageMin := stateInt(data["adv_age_min"])
	ageMax := stateInt(data["adv_age_max"])
	interests := toStringSlice(data["adv_interests"])

	// browse mode (kpkchat «جستجوی کاربران»): free filtered deck, no coins,
	// no timed queue — straight into the browse scene.
	if stateString(data["adv_mode"]) == "browse" {
		delete(data, "adv_gender")
		delete(data, "adv_province")
		delete(data, "adv_city")
		delete(data, "adv_age_min")
		delete(data, "adv_age_max")
		delete(data, "adv_interests")
		delete(data, "adv_cost")
		delete(data, "adv_mode")
		bc.SaveState()
		return startFilteredBrowse(bc, st, gender, province, city, interests, ageMin, ageMax)
	}

	cost := bc.PriceInt("search_advanced_any", 1)
	if gender != "any" {
		cost = bc.PriceInt("search_advanced_gender", 3)
	}
	if !hasSubscription(bc, botID) && cost > 0 {
		ok, err := st.Repo().SpendCoins(ctx, botID, bc.UserID, cost)
		if err != nil || !ok {
			bc.ReplyKBDStyled(fmt.Sprintf("💰 سکه کافی نیست. هزینهٔ جستجو: %d سکه.", cost), homeKB)
			bc.State["step"] = ""
			bc.SaveState()
			return true
		}
	}
	filters := map[string]any{
		"gender": gender, "province": province, "city": city,
		"age_min": ageMin, "age_max": ageMax,
		"interests": interests,
		"cost": cost, "timeout_sec": bc.PriceInt("search_advanced_timeout", 180), "advanced": true,
	}
	if err := st.StartSearchWithFilters(ctx, botID, bc.UserID, filters); err != nil {
		if cost > 0 && !hasSubscription(bc, botID) {
			_, _ = st.Repo().AddCoins(ctx, botID, bc.UserID, cost)
		}
		bc.ReplyKBDStyled("خطا در شروع جستجو.", searchKB)
		bc.State["step"] = ""
		bc.SaveState()
		return true
	}
	_ = profile
	bc.State["scene"] = "search"
	bc.State["step"] = ""
	delete(data, "adv_gender")
	delete(data, "adv_province")
	delete(data, "adv_city")
	delete(data, "adv_age_min")
	delete(data, "adv_age_max")
	delete(data, "adv_interests")
	delete(data, "adv_cost")
	bc.SaveState()
	bc.ReplyKBD(searchingText(bc), [][]string{{"❌ لغو جستجو"}, {"🏠 منوی اصلی"}})
	return true
}

func stateString(v any) string {
	s, _ := v.(string)
	return s
}

func indexOfStr(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func stateInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func ageLabel(n int) string {
	if n == 0 {
		return "بدون محدودیت"
	}
	return strconv.Itoa(n)
}
