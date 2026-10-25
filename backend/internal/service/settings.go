package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"sadbot/internal/domain"
)

// SettingField describes one editable numeric/boolean price key: its label,
// bounds, and where it applies. The panel renders a form from this list.
type SettingField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Group string `json:"group"`
	Kind  string `json:"kind"` // "int" | "bool"
	Min   int    `json:"min"`
	Max   int    `json:"max"`
	Value int    `json:"value"`
}

// settingSpec is the authoritative field list. Keys not listed here (channels,
// packages, streak_rewards, channel_id…) are preserved untouched on save.
var settingSpec = []SettingField{
	{Key: "signup_bonus", Label: "هدیهٔ ثبت‌نام (سکه)", Group: "bonuses", Kind: "int", Min: 0, Max: 1000},
	{Key: "referral_bonus", Label: "هدیهٔ دعوت دوستان (سکه)", Group: "bonuses", Kind: "int", Min: 0, Max: 1000},
	{Key: "channel_bonus", Label: "هدیهٔ عضویت کانال (سکه)", Group: "bonuses", Kind: "int", Min: 0, Max: 1000},
	{Key: "avatar_bonus", Label: "هدیهٔ بارگذاری عکس (سکه)", Group: "bonuses", Kind: "int", Min: 0, Max: 1000},
	{Key: "streak_enabled", Label: "استریک روزانه فعال", Group: "bonuses", Kind: "bool", Min: 0, Max: 1},

	{Key: "search_stranger_male", Label: "هزینهٔ جستجوی ناشناس (آقا)", Group: "search", Kind: "int", Min: 0, Max: 100},
	{Key: "search_stranger_female", Label: "هزینهٔ جستجوی ناشناس (خانم)", Group: "search", Kind: "int", Min: 0, Max: 100},
	{Key: "search_gender_male", Label: "هزینهٔ جستجوی جنسیت‌دار (آقا)", Group: "search", Kind: "int", Min: 0, Max: 100},
	{Key: "search_gender_female", Label: "هزینهٔ جستجوی جنسیت‌دار (خانم)", Group: "search", Kind: "int", Min: 0, Max: 100},
	{Key: "search_advanced_any", Label: "هزینهٔ جستجوی پیشرفته (بدون فیلتر)", Group: "search", Kind: "int", Min: 0, Max: 100},
	{Key: "search_advanced_gender", Label: "هزینهٔ جستجوی پیشرفته (با فیلتر)", Group: "search", Kind: "int", Min: 0, Max: 100},
	{Key: "search_advanced_timeout", Label: "مهلت جستجوی پیشرفته (ثانیه)", Group: "search", Kind: "int", Min: 30, Max: 900},

	{Key: "browse_free_likes", Label: "لایک رایگان روزانه", Group: "browse", Kind: "int", Min: 0, Max: 500},
	{Key: "browse_like_cost", Label: "هزینهٔ لایک اضافه (سکه)", Group: "browse", Kind: "int", Min: 0, Max: 100},
	{Key: "superlike_cost", Label: "هزینهٔ سوپرلایک (سکه)", Group: "browse", Kind: "int", Min: 0, Max: 100},
	{Key: "superlike_daily_cap", Label: "سقف سوپرلایک روزانه", Group: "browse", Kind: "int", Min: 0, Max: 100},
	{Key: "direct_chat_cost", Label: "هزینهٔ چت مستقیم (سکه)", Group: "browse", Kind: "int", Min: 0, Max: 100},

	{Key: "confess_cost", Label: "هزینهٔ اعتراف ناشناس (سکه)", Group: "content", Kind: "int", Min: 0, Max: 100},
	{Key: "ai_free_messages", Label: "پیام رایگان دستیار هوشمند", Group: "content", Kind: "int", Min: 0, Max: 500},
	{Key: "ai_enabled", Label: "دستیار هوشمند فعال", Group: "content", Kind: "bool", Min: 0, Max: 1},
	{Key: "ai_message_cost", Label: "هزینهٔ پیام دستیار (سکه)", Group: "content", Kind: "int", Min: 0, Max: 100},

	{Key: "boost_1h_coins", Label: "هزینهٔ بوست ۱ ساعته", Group: "growth", Kind: "int", Min: 0, Max: 1000},
	{Key: "boost_6h_coins", Label: "هزینهٔ بوست ۶ ساعته", Group: "growth", Kind: "int", Min: 0, Max: 1000},
	{Key: "boost_24h_coins", Label: "هزینهٔ بوست ۲۴ ساعته", Group: "growth", Kind: "int", Min: 0, Max: 1000},
	{Key: "edit_cost", Label: "هزینهٔ ویرایش پروفایل (سکه)", Group: "growth", Kind: "int", Min: 0, Max: 1000},
	{Key: "edit_name_cost", Label: "هزینهٔ ویرایش نام (سکه)", Group: "growth", Kind: "int", Min: 0, Max: 1000},
	{Key: "edit_cooldown_days", Label: "فاصلهٔ ویرایش پروفایل (روز)", Group: "growth", Kind: "int", Min: 0, Max: 365},
	{Key: "name_edit_cooldown_days", Label: "فاصلهٔ ویرایش نام (روز)", Group: "growth", Kind: "int", Min: 0, Max: 365},

	{Key: "ghost_enabled", Label: "کاربران جعلی فعال", Group: "coldstart", Kind: "bool", Min: 0, Max: 1},
	{Key: "ghost_pool_size", Label: "اندازهٔ استخر کاربران جعلی", Group: "coldstart", Kind: "int", Min: 0, Max: 100},
	{Key: "ghost_max_messages", Label: "سقف پیام چت جعلی", Group: "coldstart", Kind: "int", Min: 1, Max: 200},
	{Key: "ghost_delay_seconds", Label: "تأخیر تزریق کاربر جعلی (ثانیه)", Group: "coldstart", Kind: "int", Min: 1, Max: 600},

	{Key: "notify_enabled", Label: "اعلان‌های بازگشت فعال", Group: "notify", Kind: "bool", Min: 0, Max: 1},
	{Key: "notify_daily_cap", Label: "سقف اعلان روزانه", Group: "notify", Kind: "int", Min: 0, Max: 20},
	{Key: "notify_min_gap_hours", Label: "حداقل فاصلهٔ اعلان‌ها (ساعت)", Group: "notify", Kind: "int", Min: 0, Max: 72},
	{Key: "notify_quiet_start", Label: "شروع سکوت شبانه (ساعت)", Group: "notify", Kind: "int", Min: 0, Max: 23},
	{Key: "notify_quiet_end", Label: "پایان سکوت شبانه (ساعت)", Group: "notify", Kind: "int", Min: 0, Max: 23},

	{Key: "sub_price", Label: "قیمت اشتراک (ریال)", Group: "payment", Kind: "int", Min: 0, Max: 100000000},
	{Key: "sub_discount_pct", Label: "درصد تخفیف اشتراک", Group: "payment", Kind: "int", Min: 0, Max: 100},
}

// SettingsOf merges the current prices onto the field spec, returning the
// merged view. Missing keys fall back to the spec's zero value.
func SettingsOf(defaults []byte, current json.RawMessage) ([]SettingField, error) {
	base := map[string]any{}
	if len(defaults) > 0 {
		if err := json.Unmarshal(defaults, &base); err != nil {
			return nil, err
		}
	}
	if len(current) > 0 {
		var cur map[string]any
		if err := json.Unmarshal(current, &cur); err == nil {
			for k, v := range cur {
				base[k] = v
			}
		}
	}
	out := make([]SettingField, len(settingSpec))
	for i, f := range settingSpec {
		out[i] = f
		// zero is the natural fallback for most keys, but a field whose range
		// starts above zero (e.g. a timeout) must fall back to its minimum
		out[i].Value = f.Min
		if v, ok := base[f.Key]; ok {
			if n, ok := toNumber(v); ok {
				out[i].Value = n
			}
		}
	}
	return out, nil
}

// ApplySettings validates the submitted values against the spec's bounds and
// merges them into the existing prices, leaving unknown keys untouched.
func ApplySettings(current json.RawMessage, incoming map[string]int) (json.RawMessage, error) {
	prices := map[string]any{}
	if len(current) > 0 {
		if err := json.Unmarshal(current, &prices); err != nil {
			return nil, fmt.Errorf("تنظیمات فعلی نامعتبر است: %w", err)
		}
	}
	known := map[string]SettingField{}
	for _, f := range settingSpec {
		known[f.Key] = f
	}
	applied := 0
	for key, val := range incoming {
		f, ok := known[key]
		if !ok {
			return nil, fmt.Errorf("کلید ناشناخته: %s", key)
		}
		if val < f.Min || val > f.Max {
			return nil, fmt.Errorf("%s باید بین %d و %d باشد", f.Label, f.Min, f.Max)
		}
		prices[key] = val
		applied++
	}
	if applied == 0 {
		return nil, errors.New("هیچ مقداری ارسال نشد")
	}
	return json.Marshal(prices)
}

// GroupedSettings returns the field list bucketed by group for the panel form.
func GroupedSettings(fields []SettingField) map[string][]SettingField {
	out := map[string][]SettingField{}
	for _, f := range fields {
		out[f.Group] = append(out[f.Group], f)
	}
	return out
}

// SettingGroups returns the stable display order of the setting groups.
func SettingGroups() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range settingSpec {
		if !seen[f.Group] {
			seen[f.Group] = true
			out = append(out, f.Group)
		}
	}
	sort.Strings(out)
	return out
}

// toNumber converts a JSON number (or bool, for flag keys) to an int.
func toNumber(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// Ensure domain import stays used if the file is trimmed later.

// ─── Game template settings ───

// bomblandSpec is the editable economy for the BombLand idle-factory game.
var bomblandSpec = []SettingField{
	{Key: "sell_price_per_bomb", Label: "قیمت فروش هر بمب (تومان)", Group: "economy", Kind: "int", Min: 1, Max: 1000},
	{Key: "upgrade_base_cost", Label: "هزینه پایه ارتقا (تومان)", Group: "economy", Kind: "int", Min: 1, Max: 100000},
	{Key: "auto_rate_per_level", Label: "تولید خودکار هر سطح (بمب/دقیقه)", Group: "economy", Kind: "int", Min: 1, Max: 100},
	{Key: "manual_produce_amount", Label: "تولید دستی هر بار (بمب)", Group: "economy", Kind: "int", Min: 1, Max: 100},
	{Key: "referral_bonus_bombs", Label: "پاداش دعوت (بمب)", Group: "growth", Kind: "int", Min: 0, Max: 10000},
}

// duelSpec is the editable economy for the rock-paper-scissors duel game.
var duelSpec = []SettingField{
	{Key: "max_energy", Label: "حداکثر انرژی", Group: "energy", Kind: "int", Min: 1, Max: 1000},
	{Key: "recharge_seconds", Label: "زمان شارژ هر انرژی (ثانیه)", Group: "energy", Kind: "int", Min: 10, Max: 86400},
	{Key: "score_win", Label: "امتیاز برد", Group: "economy", Kind: "int", Min: 1, Max: 1000},
	{Key: "score_lose", Label: "امتیاز باخت (منفی)", Group: "economy", Kind: "int", Min: -1000, Max: -1},
	{Key: "drop_candy_chance", Label: "شانس آبنبات (٪×100)", Group: "drops", Kind: "int", Min: 0, Max: 100},
	{Key: "drop_chocolate_chance", Label: "شانس شکلات (٪×100)", Group: "drops", Kind: "int", Min: 0, Max: 100},
	{Key: "drop_soda_chance", Label: "شانس نوشابه (٪×100)", Group: "drops", Kind: "int", Min: 0, Max: 100},
	{Key: "opal_value_toman", Label: "ارزش هر اوپال (تومان)", Group: "prizes", Kind: "int", Min: 1, Max: 1000000},
	{Key: "gold_1mg_toman", Label: "ارزش ۱ میلی‌گرم طلا (تومان)", Group: "prizes", Kind: "int", Min: 1, Max: 10000000},
	{Key: "gold_1mg_opal", Label: "هر ۱ میلی‌گرم طلا (اوپال)", Group: "prizes", Kind: "int", Min: 1, Max: 10000},
	{Key: "withdraw_min_mg_cash", Label: "حداقل برداشت نقدی (میلی‌گرم)", Group: "prizes", Kind: "int", Min: 1, Max: 10000},
	{Key: "withdraw_min_g_physical", Label: "حداقل طلای فیزیکی (گرم)", Group: "prizes", Kind: "int", Min: 1, Max: 1000},
	{Key: "pool_share_pct", Label: "سهم جایزه از پرداخت (٪)", Group: "economy", Kind: "int", Min: 0, Max: 100},
}

// SettingsForTemplate returns the setting spec for a template key.
func SettingsForTemplate(templateKey string) []SettingField {
	switch templateKey {
	case "bombland":
		return bomblandSpec
	case "duel":
		return duelSpec
	default:
		return settingSpec
	}
}

// SettingsOfTemplate merges prices onto the template's field spec.
func SettingsOfTemplate(templateKey string, defaults []byte, current json.RawMessage) ([]SettingField, error) {
	spec := SettingsForTemplate(templateKey)
	base := map[string]any{}
	if len(defaults) > 0 {
		if err := json.Unmarshal(defaults, &base); err != nil {
			return nil, err
		}
	}
	if len(current) > 0 {
		var cur map[string]any
		if err := json.Unmarshal(current, &cur); err == nil {
			for k, v := range cur {
				base[k] = v
			}
		}
	}
	out := make([]SettingField, len(spec))
	for i, f := range spec {
		out[i] = f
		out[i].Value = f.Min
		if v, ok := base[f.Key]; ok {
			if n, ok := toNumber(v); ok {
				out[i].Value = n
			}
		}
	}
	return out, nil
}

// ApplySettingsTemplate validates and merges using the template's spec.
func ApplySettingsTemplate(templateKey string, current json.RawMessage, incoming map[string]int) (json.RawMessage, error) {
	spec := SettingsForTemplate(templateKey)
	prices := map[string]any{}
	if len(current) > 0 {
		if err := json.Unmarshal(current, &prices); err != nil {
			return nil, fmt.Errorf("تنظیمات فعلی نامعتبر است: %w", err)
		}
	}
	known := map[string]SettingField{}
	for _, f := range spec {
		known[f.Key] = f
	}
	applied := 0
	for key, val := range incoming {
		f, ok := known[key]
		if !ok {
			return nil, fmt.Errorf("کلید ناشناخته: %s", key)
		}
		if val < f.Min || val > f.Max {
			return nil, fmt.Errorf("%s باید بین %d و %d باشد", f.Label, f.Min, f.Max)
		}
		prices[key] = val
		applied++
	}
	if applied == 0 {
		return nil, errors.New("هیچ مقداری ارسال نشد")
	}
	return json.Marshal(prices)
}
var _ = domain.PermBotConfig
