// Package tapearn is the tap-to-earn game template.
package tapearn

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

type Template struct{}

func (t *Template) Key() string { return domain.TemplateTapEarn }

func (t *Template) DefaultTexts() []byte {
	return []byte(`{
		"welcome": "👆 به بازی تپ خوش اومدی!\n\n💰 پول: {money}\n⚡ انرژی: {energy}/{max_energy}\n\nهر تپ = {tap_power} سکه!\nبرای تپ زدن دکمه زیر رو بزن.",
		"tap_result": "👆 +{earned} سکه!\n💰 پول: {money}\n⚡ انرژی: {energy}/{max_energy}",
		"no_energy": "⚡ انرژیت تموم شد!\n\n⏳ کمی صبر کن تا انرژیت شارژ بشه.",
		"stats": "📊 آمار شما:\n💰 پول: {money}\n⚡ انرژی: {energy}/{max_energy}\n👆 قدرت تپ: {tap_power}",
		"upgrade": "👆 قدرت تپ: {tap_power}\n💰 هزینه ارتقا: {upgrade_cost} سکه\n💰 پول شما: {money}",
		"upgrade_ok": "✅ ارتقا موفق!\n👆 قدرت جدید: {tap_power}",
		"upgrade_fail": "❌ پول کافی نیست!\n💰 نیاز: {upgrade_cost}"
	}`)
}

func (t *Template) DefaultMenus() []byte { return []byte(`{}`) }

func (t *Template) DefaultPrices() []byte {
	return []byte(`{
		"tap_power_base": 1,
		"max_energy_base": 100,
		"recharge_per_minute": 5,
		"upgrade_base_cost": 1000,
		"upgrade_cost_multiplier": 2
	}`)
}

func tapearnRepoOf(bc *engine.BotContext) *postgres.TapEarnRepo {
	r, _ := bc.Data["tapearn"].(*postgres.TapEarnRepo)
	return r
}

func (t *Template) Start(bc *engine.BotContext, payload string) bool {
	repo := tapearnRepoOf(bc)
	if repo == nil {
		bc.Reply("خطای سیستمی.")
		return true
	}
	botID, _ := uuid.Parse(bc.BotID)
	if _, err := repo.EnsureUser(context.Background(), botID, bc.UserID, ""); err != nil {
		bc.Reply("خطا. /start بزن.")
		return true
	}
	t.doShow(bc, repo, botID)
	return true
}

func (t *Template) HandleMessage(bc *engine.BotContext, text string) bool {
	repo := tapearnRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)
	switch text {
	case "/start", "/menu":
		t.doShow(bc, repo, botID)
	case "/tap", "👆 تپ":
		t.doTap(bc, repo, botID)
	case "/upgrade":
		t.doUpgrade(bc, repo, botID)
	default:
		// Any other text = tap!
		t.doTap(bc, repo, botID)
	}
	return true
}

func (t *Template) HandleAction(bc *engine.BotContext, action string) bool {
	repo := tapearnRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)
	switch action {
	case "act:tap":
		t.doTap(bc, repo, botID)
	case "act:upgrade":
		t.doUpgrade(bc, repo, botID)
	case "act:show":
		t.doShow(bc, repo, botID)
	}
	return true
}

func (t *Template) doShow(bc *engine.BotContext, repo *postgres.TapEarnRepo, botID uuid.UUID) {
	recharge := bc.PriceInt("recharge_per_minute", 5)
	energy, _ := repo.RechargeEnergy(context.Background(), botID, bc.UserID, recharge)
	u, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	if u == nil {
		bc.Reply("خطا.")
		return
	}
	msg := bc.Text("welcome")
	msg = strings.ReplaceAll(msg, "{money}", strconv.FormatInt(u.Money, 10))
	msg = strings.ReplaceAll(msg, "{energy}", strconv.Itoa(energy))
	msg = strings.ReplaceAll(msg, "{max_energy}", strconv.Itoa(u.MaxEnergy))
	msg = strings.ReplaceAll(msg, "{tap_power}", strconv.Itoa(u.TapPower))
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "👆 تپ!", Callback: "act:tap"}},
		{{Label: "⬆️ ارتقا", Callback: "act:upgrade"}, {Label: "📊 آمار", Callback: "act:show"}},
	})
}

func (t *Template) doTap(bc *engine.BotContext, repo *postgres.TapEarnRepo, botID uuid.UUID) {
	recharge := bc.PriceInt("recharge_per_minute", 5)
	energy, _ := repo.RechargeEnergy(context.Background(), botID, bc.UserID, recharge)
	if energy <= 0 {
		bc.ReplyInline(bc.Text("no_energy"), [][]engine.InlineButton{
			{{Label: "📊 آمار", Callback: "act:show"}},
		})
		return
	}

	earned, err := repo.Tap(context.Background(), botID, bc.UserID)
	if err != nil {
		bc.Reply(bc.Text("no_energy"))
		return
	}

	u, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	msg := bc.Text("tap_result")
	msg = strings.ReplaceAll(msg, "{earned}", strconv.FormatInt(earned, 10))
	msg = strings.ReplaceAll(msg, "{money}", strconv.FormatInt(u.Money, 10))
	msg = strings.ReplaceAll(msg, "{energy}", strconv.Itoa(u.Energy))
	msg = strings.ReplaceAll(msg, "{max_energy}", strconv.Itoa(u.MaxEnergy))
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "👆 تپ!", Callback: "act:tap"}},
		{{Label: "⬆️ ارتقا", Callback: "act:upgrade"}},
	})
}

func (t *Template) doUpgrade(bc *engine.BotContext, repo *postgres.TapEarnRepo, botID uuid.UUID) {
	u, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	if u == nil {
		bc.Reply("خطا.")
		return
	}
	baseCost := bc.PriceInt("upgrade_base_cost", 1000)
	multiplier := bc.PriceInt("upgrade_cost_multiplier", 2)
	cost := int64(baseCost) * int64(u.TapPower) * int64(multiplier)

	if u.Money < cost {
		msg := bc.Text("upgrade_fail")
		msg = strings.ReplaceAll(msg, "{upgrade_cost}", strconv.FormatInt(cost, 10))
		bc.Reply(msg)
		return
	}

	// Upgrade
	ok, err := repo.UpgradePower(context.Background(), botID, bc.UserID, cost)
	if err != nil || !ok {
		bc.Reply("خطا.")
		return
	}

	u2, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	msg := bc.Text("upgrade_ok")
	msg = strings.ReplaceAll(msg, "{tap_power}", strconv.Itoa(u2.TapPower))
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "👆 تپ!", Callback: "act:tap"}},
		{{Label: "⬆️ ارتقا بیشتر", Callback: "act:upgrade"}},
	})
}

var _ = fmt.Sprintf
