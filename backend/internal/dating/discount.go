package dating

import (
	"context"
	"fmt"
	"strings"

	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"
)

// discountRepoOf reads the injected per-bot discount repo from BotContext.
func discountRepoOf(bc *engine.BotContext) *postgres.DiscountRepo {
	r, _ := bc.Data["discountRepo"].(*postgres.DiscountRepo)
	return r
}

// appliedCode returns the discount code the user entered but hasn't spent yet.
func appliedCode(bc *engine.BotContext) string {
	data, _ := bc.State["data"].(map[string]any)
	if data == nil {
		return ""
	}
	s, _ := data["discount_code"].(string)
	return s
}

func setAppliedCode(bc *engine.BotContext, code string) {
	data, _ := bc.State["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
		bc.State["data"] = data
	}
	data["discount_code"] = code
}

func clearAppliedCode(bc *engine.BotContext) {
	if data, ok := bc.State["data"].(map[string]any); ok {
		delete(data, "discount_code")
	}
}

// showDiscountPrompt starts the "enter discount code" scene.
func showDiscountPrompt(bc *engine.BotContext) {
	bc.State["scene"] = "discount"
	bc.SaveState()
	bc.ReplyKBD("🎫 کد تخفیف\n\nکد تخفیف رو بفرست تا بررسی کنم.\n(برای انصراف «لغو» رو بزن.)",
		[][]string{{"لغو"}})
}

// handleDiscountText validates the entered code and stores it for the next
// purchase (percent / flat_toman applied at checkout, coin_bonus granted now).
func handleDiscountText(bc *engine.BotContext, st *Store, text string) bool {
	repo := discountRepoOf(bc)
	if repo == nil {
		goHome(bc, "سرویس تخفیف در دسترس نیست.")
		return true
	}
	raw := strings.TrimSpace(text)
	if raw == "لغو" || raw == "❌ لغو" {
		showWallet(bc, st, botUUID(bc))
		return true
	}
	code := strings.ToUpper(raw)
	dc, err := repo.GetByCode(context.Background(), botUUID(bc), code)
	if err != nil {
		bc.Reply("خطا در بررسی کد. دوباره تلاش کن.")
		return true
	}
	if dc == nil {
		bc.ReplyKBD("❌ کد تخفیف نامعتبر یا منقضی‌ست.\nدوباره بفرست یا «لغو» رو بزن.", [][]string{{"لغو"}})
		return true
	}
	if dc.Kind == "coin_bonus" {
		ok, err := repo.IncrementUse(context.Background(), botUUID(bc), code)
		if err != nil || !ok {
			bc.Reply("❌ این کد قابل استفاده نیست (منقضی یا تمام‌شده).")
			return true
		}
		if _, err := st.Repo().AddCoins(context.Background(), botUUID(bc), bc.UserID, dc.Value); err != nil {
			bc.Reply("خطا در افزودن سکه.")
			return true
		}
		bc.Reply(fmt.Sprintf("🎉 کد اعمال شد! +%d سکه به حسابت اضافه شد.", dc.Value))
		showWallet(bc, st, botUUID(bc))
		return true
	}
	setAppliedCode(bc, code)
	bc.SaveState()
	bc.Reply(fmt.Sprintf("✅ کد «%s» ثبت شد.\nهنگام خرید اعمال می‌شه.", code))
	showWallet(bc, st, botUUID(bc))
	return true
}

// applyDiscount computes the final price after the user's stored code.
// Returns the effective price and a label suffix (may be empty).
func applyDiscount(bc *engine.BotContext, base int) (int, string) {
	repo := discountRepoOf(bc)
	code := appliedCode(bc)
	if repo == nil || code == "" {
		return base, ""
	}
	dc, err := repo.GetByCode(context.Background(), botUUID(bc), code)
	if err != nil || dc == nil {
		clearAppliedCode(bc)
		bc.SaveState()
		return base, ""
	}
	switch dc.Kind {
	case "percent":
		pct := dc.Value
		if pct > 100 {
			pct = 100
		}
		return base * (100 - pct) / 100, fmt.Sprintf(" 🎫 (−%d%%)", pct)
	case "flat_toman":
		final := base - dc.Value
		if final < 0 {
			final = 0
		}
		return final, " 🎫"
	}
	return base, ""
}

// consumeDiscount marks the stored code used after a successful purchase.
func consumeDiscount(bc *engine.BotContext) {
	repo := discountRepoOf(bc)
	code := appliedCode(bc)
	if repo == nil || code == "" {
		return
	}
	_, _ = repo.IncrementUse(context.Background(), botUUID(bc), code)
	clearAppliedCode(bc)
	bc.SaveState()
}
