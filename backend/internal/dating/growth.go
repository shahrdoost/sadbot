package dating

import (
	"fmt"
	"log"
	"time"

	"sadbot/internal/engine"

	"github.com/google/uuid"
)

// growth: daily streak rewards, profile boost, paid profile edit — kpkchat
// streak.ts / boost.ts parity, per-bot via prices.

// recordStreak advances the daily streak and toasts milestone rewards
// (streak_rewards: {"3":5,"7":15,"14":30,"30":75} in prices).
func recordStreak(bc *engine.BotContext, st *Store, botID uuid.UUID) {
	ctx := engineBG()
	p, err := st.Profile(ctx, botID, bc.UserID)
	if err != nil || p.StreakLastDate != nil && sameDay(*p.StreakLastDate, time.Now()) {
		return // already counted today (or no profile)
	}
	count, err := st.Repo().RecordStreak(ctx, botID, bc.UserID)
	if err != nil {
		log.Printf("[dating] streak: %v", err)
		return
	}
	// milestone reward?
	rewards, _ := bc.Prices["streak_rewards"].(map[string]any)
	if rewards == nil {
		return
	}
	if raw, ok := rewards[fmt.Sprintf("%d", count)]; ok {
		bonus := toInt(raw)
		if bonus > 0 {
			if _, err := st.Repo().AddCoins(ctx, botID, bc.UserID, bonus); err != nil {
				log.Printf("[dating] streak reward: %v", err)
				return
			}
			bc.Reply(fmt.Sprintf("🔥 استریک %d روزه!\n🎁 پاداش: +%d سکه", count, bonus))
		}
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// --- boost ---

func showBoost(bc *engine.BotContext, st *Store, botID uuid.UUID) {
	ctx := engineBG()
	p, _ := st.Profile(ctx, botID, bc.UserID)
	active := ""
	if p != nil && p.BoostedUntil != nil && p.BoostedUntil.After(time.Now()) {
		active = "\n⭐ بوست فعال تا " + p.BoostedUntil.Format("15:04")
	}
	price := func(key string, def int) int {
		cost := bc.PriceInt(key, def)
		if hasSubscription(bc, botID) {
			cost = cost * (100 - bc.PriceInt("sub_discount_pct", 30)) / 100
		}
		return cost
	}
	bc.ReplyInline(
		"🚀 بوست پروفایل"+active+"\n━━━━━━━━━━━━━━\n\n"+
			"پروفایلت بالای فید مرور دیگران قرار می‌گیره.\n\n"+
			fmt.Sprintf("• ۱ ساعت: %d سکه\n", price("boost_1h_coins", 2))+
			fmt.Sprintf("• ۶ ساعت: %d سکه\n", price("boost_6h_coins", 5))+
			fmt.Sprintf("• ۲۴ ساعت: %d سکه", price("boost_24h_coins", 10)),
		[][]engine.InlineButton{
			{{Label: "🚀 ۱ ساعت", Callback: "act:boost:1"}, {Label: "🚀 ۶ ساعت", Callback: "act:boost:6"}},
			{{Label: "🚀 ۲۴ ساعت", Callback: "act:boost:24"}, {Label: "🏠 بازگشت", Callback: "act:wallet"}},
		},
	)
}

// boostAction activates a boost by spending coins (kpkchat charges coins for
// boost; price in coins via boost_*_coins, fall back to toman-style numbers).
func boostAction(bc *engine.BotContext, st *Store, botID uuid.UUID, hours string) bool {
	ctx := engineBG()
	coinsKey := "boost_" + hours + "h_coins"
	def := map[string]int{"1": 2, "6": 5, "24": 10}[hours]
	if def == 0 {
		def = 10
	}
	cost := bc.PriceInt(coinsKey, def)
	if hasSubscription(bc, botID) {
		cost = cost * (100 - bc.PriceInt("sub_discount_pct", 30)) / 100
	}
	ok, err := st.Repo().SpendCoins(ctx, botID, bc.UserID, cost)
	if err != nil || !ok {
		bc.Reply(fmt.Sprintf("❌ سکه کافی نیست (هزینه: %d سکه).", cost))
		return true
	}
	dur := time.Duration(toInt(hours)) * time.Hour
	until := time.Now().Add(dur)
	if err := st.Repo().SetBoost(ctx, botID, bc.UserID, until); err != nil {
		bc.Reply("خطا در فعال‌سازی بوست.")
		return true
	}
	bc.Reply(fmt.Sprintf("🚀 بوست فعال شد!\nتا %s پروفایلت بالای فید مروره.", until.Format("15:04")))
	return true
}

// --- paid profile edit (moved to profile.go) ---

// finishPaidEdit stamps the cooldown timestamp after a successful setup edit.
func finishPaidEdit(bc *engine.BotContext, st *Store, botID uuid.UUID) {
	ctx := engineBG()
	data := dataMap(bc)
	if _, ok := data["edit_profile"]; ok {
		delete(data, "edit_profile")
		bc.SaveState()
		_ = st.Repo().MarkProfileEdited(ctx, botID, bc.UserID)
		// land back home instead of finishing onboarding again
		goHome(bc, ttext(bc, "home.welcome", "خوش اومدی! 👋\nچیکار برات انجام بدم؟"))
		return
	}
	// name edit path: setname in setup scene for a complete profile
	_ = st.Repo().MarkNameEdited(ctx, botID, bc.UserID)
}
