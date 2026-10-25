// Package duel is the rock-paper-scissors duel game template (duel-bot-bale port).
package duel

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/payment"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

type Template struct{}

func (t *Template) Key() string { return domain.TemplateDuel }

func (t *Template) DefaultTexts() []byte {
	return []byte(`{
		"welcome": "🎮 به بازی سنگ‌کاغذ‌قیچی خوش اومدی!\n\n⚡ انرژی: {energy}/{max_energy}\n🏆 امتیاز: {score}\n💎 اوپال سیاه: {opal}",
		"main_menu": "🎮 منوی اصلی",
		"play_choose": "✊ سنگ  ✋ کاغذ  ✌️ قیچی\n\nحریف: {rival_name} (امتیاز: {rival_score})",
		"play_win": "🏆 بردی!\n+{score_delta} امتیاز\n{rewards}",
		"play_lose": "😢 باختی!\n{score_delta} امتیاز",
		"play_draw": "🤝 مساوی!\nانرژی مصرف شد ولی امتیازی تغییر نکرد.",
		"no_energy": "⚡ انرژی کافی نیست!\n\nبرای ادامه بازی انرژی بخر.",
		"profile": "👤 پروفایل شما:\n\n🏆 امتیاز: {score}\n⚡ انرژی: {energy}/{max_energy}\n💎 اوپال سیاه: {opal}\n🍬 آبنبات: {candy}\n🍫 شکلات: {chocolate}\n🥤 نوشابه: {soda}\n\n💰 ارزش جوایز: {value_toman} تومان\n💳 مجموع برداشت: {withdrawn} تومان",
		"rewards": "🎁 جوایز شما:\n\n💎 اوپال سیاه: {opal}\n🍬 آبنبات: {candy}\n🍫 شکلات: {chocolate}\n🥤 نوشابه: {soda}\n\n💰 ارزش تقریبی: {value_toman} تومان\n\n🏆 هر ۲۰ اوپال = ۱ میلی‌گرم طلا = ۲۰,۰۰۰ تومان",
		"convert_menu": "🔄 تبدیل اقلام:\n\n⚡ به انرژی:\n• ۳ نوشابه → ۱ انرژی (۸۰٪)\n• ۵ شکلات → ۱ انرژی (۷۰٪)\n• ۱۰ آبنبات → ۱ انرژی (۶۰٪)\n\n💎 به اوپال:\n• ۲۰ نوشابه → ۱ اوپال (۴۰٪)\n• ۳۰ شکلات → ۱ اوپال (۳۰٪)\n• ۵۰ آبنبات → ۱ اوپال (۲۰٪)\n\n🏆 به امتیاز:\n• هر ۵ آبنبات → ۱۰ امتیاز\n• هر ۳ شکلات → ۲۰ امتیاز\n• هر ۲ نوشابه → ۳۰ امتیاز",
		"convert_success": "✅ تبدیل موفق!\n{result}",
		"convert_fail": "❌ تبدیل ناموفق!\nاقلام مصرف شدند ولی نتیجه‌ای حاصل نشد.",
		"leaderboard": "🏆 لیدربورد:\n\n{entries}",
		"energy_shop": "⚡ خرید انرژی:\n\n{packages}\n\n💳 درگاه پرداخت امن",
		"payment_link": "💳 برای خرید انرژی:\n{link}\n\n⚠️ VPN خود را قبل از پرداخت خاموش کنید.",
		"payment_ok": "✅ پرداخت تایید شد!\n⚡ +{energy} انرژی اضافه شد.",
		"withdraw_cash": "💰 برداشت نقدی:\n\n💎 حداقل: {min_opal} اوپال ({min_mg} میلی‌گرم طلا)\n\n💎 اوپال شما: {opal}",
		"withdraw_physical": "🥇 طلای فیزیکی:\n\n💎 حداقل: {min_opal} اوپال ({min_g} گرم)\n\n💎 اوپال شما: {opal}",
		"withdraw_confirm": "✅ درخواست برداشت ثبت شد!\n\n⏳ در انتظار تایید ادمین.\n💎 {amount_mg} میلی‌گرم طلا ({amount_toman} تومان)",
		"withdraw_insufficient": "❌ اوپال کافی نیست!\n💎 شما: {opal}\n💎 مورد نیاز: {needed}",
		"withdraw_done": "✅ برداشت با موفقیت انجام شد!\n💰 {amount} تومان به حساب شما واریز شد.",
		"unknown": "❓ دستور ناشناخته!\n/start - شروع بازی\n/menu - منوی اصلی"
	}`)
}

func (t *Template) DefaultMenus() []byte { return []byte(`{}`) }

func (t *Template) DefaultPrices() []byte {
	return []byte(`{
		"max_energy": 10,
		"recharge_seconds": 600,
		"score_win": 15,
		"score_lose": -10,
		"drop_candy_chance": 0.60,
		"drop_candy_min": 1,
		"drop_candy_max": 3,
		"drop_chocolate_chance": 0.30,
		"drop_chocolate_min": 1,
		"drop_chocolate_max": 2,
		"drop_soda_chance": 0.15,
		"drop_soda_min": 1,
		"drop_soda_max": 1,
		"opal_jackpot_threshold": 10000,
		"opal_jackpot_chance": 0.05,
		"opal_jackpot_min": 1,
		"opal_jackpot_max": 3,
		"pool_share_pct": 70,
		"opal_value_toman": 1000,
		"gold_1mg_toman": 20000,
		"gold_1mg_opal": 20,
		"withdraw_min_mg_cash": 100,
		"withdraw_min_g_physical": 1,
		"convert_energy_candy": 10,
		"convert_energy_chocolate": 5,
		"convert_energy_soda": 3,
		"convert_opal_candy": 50,
		"convert_opal_chocolate": 30,
		"convert_opal_soda": 20,
		"convert_score_candy_ratio": 5,
		"convert_score_candy_points": 10,
		"convert_score_chocolate_ratio": 3,
		"convert_score_chocolate_points": 20,
		"convert_score_soda_ratio": 2,
		"convert_score_soda_points": 30,
		"packages": [
			{"id": "energy_5", "label": "⚡ ۵ انرژی", "energy": 5, "price": 5000},
			{"id": "energy_10", "label": "⚡ ۱۰ انرژی", "energy": 10, "price": 9000},
			{"id": "energy_100", "label": "⚡ ۱۰۰ انرژی", "energy": 100, "price": 80000}
		]
	}`)
}

func paySvcOf(bc *engine.BotContext) *payment.Service {
	s, _ := bc.Data["payments"].(*payment.Service)
	return s
}

func duelRepoOf(bc *engine.BotContext) *postgres.DuelRepo {
	r, _ := bc.Data["duel"].(*postgres.DuelRepo)
	return r
}

func (t *Template) Start(bc *engine.BotContext, payload string) bool {
	repo := duelRepoOf(bc)
	if repo == nil {
		bc.Reply("خطای سیستمی.")
		return true
	}
	botID, err := uuid.Parse(bc.BotID)
	if err != nil {
		bc.Reply("خطا.")
		return true
	}

	if _, err := repo.EnsurePlayer(context.Background(), botID, bc.UserID); err != nil {
		log.Printf("[duel] ensure player: %v", err)
		bc.Reply("خطا. /start بزن.")
		return true
	}

	maxEnergy := bc.PriceInt("max_energy", 10)
	rechargeSec := bc.PriceInt("recharge_seconds", 600)
	energy, _ := repo.RechargeEnergy(context.Background(), botID, bc.UserID, maxEnergy, rechargeSec)

	p, _ := repo.GetPlayer(context.Background(), botID, bc.UserID)

	welcome := bc.Text("welcome")
	welcome = strings.ReplaceAll(welcome, "{energy}", strconv.Itoa(energy))
	welcome = strings.ReplaceAll(welcome, "{max_energy}", strconv.Itoa(maxEnergy))
	welcome = strings.ReplaceAll(welcome, "{score}", strconv.Itoa(p.Score))
	welcome = strings.ReplaceAll(welcome, "{opal}", strconv.Itoa(p.BlackOpal))

	t.showMainMenu(bc, welcome)
	return true
}

func (t *Template) showMainMenu(bc *engine.BotContext, msg string) {
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "🎮 بازی جدید", Callback: "act:play"}, {Label: "👤 پروفایل", Callback: "act:profile"}},
		{{Label: "🎁 جوایز", Callback: "act:rewards"}, {Label: "🔄 تبدیل", Callback: "act:convert"}},
		{{Label: "🏆 لیدربورد", Callback: "act:leaderboard"}, {Label: "⚡ خرید انرژی", Callback: "act:energy"}},
	})
}

func (t *Template) HandleMessage(bc *engine.BotContext, text string) bool {
	switch text {
	case "/menu":
		t.showMainMenu(bc, bc.Text("main_menu"))
		return true
	}
	return false
}

func (t *Template) HandleAction(bc *engine.BotContext, action string) bool {
	repo := duelRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, err := uuid.Parse(bc.BotID)
	if err != nil {
		return false
	}

	switch {
	case action == "act:play":
		t.doPlay(bc, repo, botID, "")
	case strings.HasPrefix(action, "act:play:"):
		move := strings.TrimPrefix(action, "act:play:")
		t.doPlay(bc, repo, botID, move)
	case action == "act:profile":
		t.showProfile(bc, repo, botID)
	case action == "act:rewards":
		t.showRewards(bc, repo, botID)
	case action == "act:convert":
		t.showConvertMenu(bc)
	case strings.HasPrefix(action, "act:convert:"):
		itemType := strings.TrimPrefix(action, "act:convert:")
		t.doConvert(bc, repo, botID, itemType)
	case action == "act:leaderboard":
		t.showLeaderboard(bc, repo, botID)
	case action == "act:energy":
		t.showEnergyShop(bc)
	case strings.HasPrefix(action, "act:buy:"):
		pkgID := strings.TrimPrefix(action, "act:buy:")
		t.doBuyEnergy(bc, repo, botID, pkgID)
	case action == "act:withdraw":
		t.showWithdraw(bc, repo, botID, "")
	case strings.HasPrefix(action, "act:withdraw:"):
		kind := strings.TrimPrefix(action, "act:withdraw:")
		t.doWithdraw(bc, repo, botID, kind)
	case action == "act:menu":
		t.showMainMenu(bc, bc.Text("main_menu"))
	default:
		return false
	}
	return true
}

func (t *Template) doPlay(bc *engine.BotContext, repo *postgres.DuelRepo, botID uuid.UUID, move string) {
	maxEnergy := bc.PriceInt("max_energy", 10)
	rechargeSec := bc.PriceInt("recharge_seconds", 600)
	energy, _ := repo.RechargeEnergy(context.Background(), botID, bc.UserID, maxEnergy, rechargeSec)

	if energy <= 0 {
		bc.ReplyInline(bc.Text("no_energy"), [][]engine.InlineButton{
			{{Label: "⚡ خرید انرژی", Callback: "act:energy"}},
			{{Label: "🏠 منو", Callback: "act:menu"}},
		})
		return
	}

	if move == "" {
		// Show rival + move buttons
		rival, err := repo.FindRival(context.Background(), botID, bc.UserID)
		rivalName := "ربات بازی"
		rivalScore := 1000
		if err == nil {
			rivalName = fmt.Sprintf("بازیکن %d", rival.UserID%10000)
			rivalScore = rival.Score
		}
		msg := bc.Text("play_choose")
		msg = strings.ReplaceAll(msg, "{rival_name}", rivalName)
		msg = strings.ReplaceAll(msg, "{rival_score}", strconv.Itoa(rivalScore))
		bc.ReplyInline(msg, [][]engine.InlineButton{
			{{Label: "✊ سنگ", Callback: "act:play:rock"}, {Label: "✋ کاغذ", Callback: "act:play:paper"}, {Label: "✌️ قیچی", Callback: "act:play:scissors"}},
			{{Label: "🏠 منو", Callback: "act:menu"}},
		})
		return
	}

	// Spend energy
	ok, _ := repo.SpendEnergy(context.Background(), botID, bc.UserID, 1)
	if !ok {
		bc.Reply(bc.Text("no_energy"))
		return
	}

	// Determine outcome (opponent move is random)
	moves := []string{"rock", "paper", "scissors"}
	oppMove := moves[rand.Intn(3)]
	result := determineWinner(move, oppMove)

	scoreWin := bc.PriceInt("score_win", 15)
	scoreLose := bc.PriceInt("score_lose", -10)

	var msg string
	switch result {
	case "win":
		_ = repo.UpdateScore(context.Background(), botID, bc.UserID, scoreWin)
		rewards := t.rollRewards(bc, repo, botID)
		msg = bc.Text("play_win")
		msg = strings.ReplaceAll(msg, "{score_delta}", strconv.Itoa(scoreWin))
		msg = strings.ReplaceAll(msg, "{rewards}", rewards)
	case "lose":
		_ = repo.UpdateScore(context.Background(), botID, bc.UserID, scoreLose)
		msg = bc.Text("play_lose")
		msg = strings.ReplaceAll(msg, "{score_delta}", strconv.Itoa(scoreLose))
	default:
		msg = bc.Text("play_draw")
	}

	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "🔄 بازی بعدی", Callback: "act:play"}},
		{{Label: "🏠 منو", Callback: "act:menu"}},
	})
}

func determineWinner(player, opponent string) string {
	if player == opponent {
		return "draw"
	}
	wins := map[string]string{"rock": "scissors", "paper": "rock", "scissors": "paper"}
	if wins[player] == opponent {
		return "win"
	}
	return "lose"
}

func (t *Template) rollRewards(bc *engine.BotContext, repo *postgres.DuelRepo, botID uuid.UUID) string {
	r := rand.Float64()
	var candy, chocolate, soda int

	if r < priceFloat(bc, "drop_candy_chance", 0.60) {
		candy = bc.PriceInt("drop_candy_min", 1) + rand.Intn(bc.PriceInt("drop_candy_max", 3)-bc.PriceInt("drop_candy_min", 1)+1)
	}
	if r < priceFloat(bc, "drop_chocolate_chance", 0.30) {
		chocolate = bc.PriceInt("drop_chocolate_min", 1) + rand.Intn(bc.PriceInt("drop_chocolate_max", 2)-bc.PriceInt("drop_chocolate_min", 1)+1)
	}
	if r < priceFloat(bc, "drop_soda_chance", 0.15) {
		soda = bc.PriceInt("drop_soda_min", 1) + rand.Intn(bc.PriceInt("drop_soda_max", 1)-bc.PriceInt("drop_soda_min", 1)+1)
	}

	if candy > 0 || chocolate > 0 || soda > 0 {
		_ = repo.AddItems(context.Background(), botID, bc.UserID, candy, chocolate, soda)
	}

	var parts []string
	if candy > 0 {
		parts = append(parts, fmt.Sprintf("🍬 آبنبات: %d", candy))
	}
	if chocolate > 0 {
		parts = append(parts, fmt.Sprintf("🍫 شکلات: %d", chocolate))
	}
	if soda > 0 {
		parts = append(parts, fmt.Sprintf("🥤 نوشابه: %d", soda))
	}

	// Opal jackpot
	threshold := bc.PriceInt("opal_jackpot_threshold", 10000)
	if threshold > 0 {
		pool, _ := repo.GetPool(context.Background(), botID)
		if pool != nil && pool.TotalToman > int64(threshold) {
			chance := priceFloat(bc, "opal_jackpot_chance", 0.05)
			if poolChance := float64(pool.TotalToman) / 2000000.0; poolChance < chance {
				chance = poolChance
			}
			if rand.Float64() < chance {
				opalMin := bc.PriceInt("opal_jackpot_min", 1)
				opalMax := bc.PriceInt("opal_jackpot_max", 3)
				opals := opalMin + rand.Intn(opalMax-opalMin+1)
				_ = repo.AddOpals(context.Background(), botID, bc.UserID, opals)
				_, _ = repo.ReducePool(context.Background(), botID, int64(opals)*int64(bc.PriceInt("opal_value_toman", 1000)))
				parts = append(parts, fmt.Sprintf("💎 اوپال سیاه: %d (جکپات!)", opals))
			}
		}
	}

	if len(parts) == 0 {
		return "🎁 جایزه‌ای در این دست نبود."
	}
	return "🎁 جوایز:\n" + strings.Join(parts, "\n")
}

func (t *Template) showProfile(bc *engine.BotContext, repo *postgres.DuelRepo, botID uuid.UUID) {
	maxEnergy := bc.PriceInt("max_energy", 10)
	rechargeSec := bc.PriceInt("recharge_seconds", 600)
	energy, _ := repo.RechargeEnergy(context.Background(), botID, bc.UserID, maxEnergy, rechargeSec)
	p, _ := repo.GetPlayer(context.Background(), botID, bc.UserID)

	opalValue := bc.PriceInt("opal_value_toman", 1000)
	valueToman := p.BlackOpal * opalValue

	msg := bc.Text("profile")
	msg = strings.ReplaceAll(msg, "{score}", strconv.Itoa(p.Score))
	msg = strings.ReplaceAll(msg, "{energy}", strconv.Itoa(energy))
	msg = strings.ReplaceAll(msg, "{max_energy}", strconv.Itoa(maxEnergy))
	msg = strings.ReplaceAll(msg, "{opal}", strconv.Itoa(p.BlackOpal))
	msg = strings.ReplaceAll(msg, "{candy}", strconv.Itoa(p.Candy))
	msg = strings.ReplaceAll(msg, "{chocolate}", strconv.Itoa(p.Chocolate))
	msg = strings.ReplaceAll(msg, "{soda}", strconv.Itoa(p.Soda))
	msg = strings.ReplaceAll(msg, "{value_toman}", strconv.Itoa(valueToman))
	msg = strings.ReplaceAll(msg, "{withdrawn}", strconv.FormatInt(p.TotalWithdrawnToman, 10))

	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "🏠 منو", Callback: "act:menu"}},
	})
}

func (t *Template) showRewards(bc *engine.BotContext, repo *postgres.DuelRepo, botID uuid.UUID) {
	p, _ := repo.GetPlayer(context.Background(), botID, bc.UserID)
	opalValue := bc.PriceInt("opal_value_toman", 1000)
	valueToman := p.BlackOpal * opalValue

	msg := bc.Text("rewards")
	msg = strings.ReplaceAll(msg, "{opal}", strconv.Itoa(p.BlackOpal))
	msg = strings.ReplaceAll(msg, "{candy}", strconv.Itoa(p.Candy))
	msg = strings.ReplaceAll(msg, "{chocolate}", strconv.Itoa(p.Chocolate))
	msg = strings.ReplaceAll(msg, "{soda}", strconv.Itoa(p.Soda))
	msg = strings.ReplaceAll(msg, "{value_toman}", strconv.Itoa(valueToman))

	minOpalCash := bc.PriceInt("withdraw_min_mg_cash", 100) * bc.PriceInt("gold_1mg_opal", 20)
	minOpalPhysical := bc.PriceInt("withdraw_min_g_physical", 1) * 1000 * bc.PriceInt("gold_1mg_opal", 20)

	var buttons [][]engine.InlineButton
	if p.BlackOpal >= minOpalCash {
		buttons = append(buttons, []engine.InlineButton{{Label: "💰 برداشت نقدی", Callback: "act:withdraw:cash"}})
	}
	if p.BlackOpal >= minOpalPhysical {
		buttons = append(buttons, []engine.InlineButton{{Label: "🥇 طلای فیزیکی", Callback: "act:withdraw:physical"}})
	}
	buttons = append(buttons, []engine.InlineButton{{Label: "🏠 منو", Callback: "act:menu"}})
	bc.ReplyInline(msg, buttons)
}

func (t *Template) showConvertMenu(bc *engine.BotContext) {
	bc.ReplyInline(bc.Text("convert_menu"), [][]engine.InlineButton{
		{{Label: "⚡ به انرژی", Callback: "act:convert:energy"}, {Label: "💎 به اوپال", Callback: "act:convert:opal"}},
		{{Label: "🏆 به امتیاز", Callback: "act:convert:score"}},
		{{Label: "🏠 منو", Callback: "act:menu"}},
	})
}

func (t *Template) doConvert(bc *engine.BotContext, repo *postgres.DuelRepo, botID uuid.UUID, convertType string) {
	ctx := context.Background()
	p, err := repo.GetPlayer(ctx, botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا.")
		return
	}

	switch convertType {
	case "energy":
		// Try soda first, then chocolate, then candy
		if p.Soda >= 3 {
			ok, _ := repo.ConsumeItems(ctx, botID, bc.UserID, "soda", 3)
			if ok && rand.Float64() < 0.80 {
				_ = repo.AddEnergy(ctx, botID, bc.UserID, 1)
				bc.Reply(bc.Text("convert_success") + "\n⚡ +1 انرژی (از نوشابه)")
				return
			}
		}
		if p.Chocolate >= 5 {
			ok, _ := repo.ConsumeItems(ctx, botID, bc.UserID, "chocolate", 5)
			if ok && rand.Float64() < 0.70 {
				_ = repo.AddEnergy(ctx, botID, bc.UserID, 1)
				bc.Reply(bc.Text("convert_success") + "\n⚡ +1 انرژی (از شکلات)")
				return
			}
		}
		if p.Candy >= 10 {
			ok, _ := repo.ConsumeItems(ctx, botID, bc.UserID, "candy", 10)
			if ok && rand.Float64() < 0.60 {
				_ = repo.AddEnergy(ctx, botID, bc.UserID, 1)
				bc.Reply(bc.Text("convert_success") + "\n⚡ +1 انرژی (از آبنبات)")
				return
			}
		}
		bc.Reply(bc.Text("convert_fail"))

	case "opal":
		if p.Soda >= 20 {
			ok, _ := repo.ConsumeItems(ctx, botID, bc.UserID, "soda", 20)
			if ok && rand.Float64() < 0.40 {
				_ = repo.AddOpals(ctx, botID, bc.UserID, 1)
				bc.Reply(bc.Text("convert_success") + "\n💎 +1 اوپال (از نوشابه)")
				return
			}
		}
		if p.Chocolate >= 30 {
			ok, _ := repo.ConsumeItems(ctx, botID, bc.UserID, "chocolate", 30)
			if ok && rand.Float64() < 0.30 {
				_ = repo.AddOpals(ctx, botID, bc.UserID, 1)
				bc.Reply(bc.Text("convert_success") + "\n💎 +1 اوپال (از شکلات)")
				return
			}
		}
		if p.Candy >= 50 {
			ok, _ := repo.ConsumeItems(ctx, botID, bc.UserID, "candy", 50)
			if ok && rand.Float64() < 0.20 {
				_ = repo.AddOpals(ctx, botID, bc.UserID, 1)
				bc.Reply(bc.Text("convert_success") + "\n💎 +1 اوپال (از آبنبات)")
				return
			}
		}
		bc.Reply(bc.Text("convert_fail"))

	case "score":
		// Bulk convert all items to score
		totalScore := 0
		if p.Candy >= 5 {
			n := p.Candy / 5
			_, _ = repo.ConsumeItems(ctx, botID, bc.UserID, "candy", n*5)
			totalScore += n * 10
		}
		if p.Chocolate >= 3 {
			n := p.Chocolate / 3
			_, _ = repo.ConsumeItems(ctx, botID, bc.UserID, "chocolate", n*3)
			totalScore += n * 20
		}
		if p.Soda >= 2 {
			n := p.Soda / 2
			_, _ = repo.ConsumeItems(ctx, botID, bc.UserID, "soda", n*2)
			totalScore += n * 30
		}
		if totalScore > 0 {
			_ = repo.UpdateScore(ctx, botID, bc.UserID, totalScore)
			bc.Reply(fmt.Sprintf("✅ %d امتیاز اضافه شد!", totalScore))
		} else {
			bc.Reply("❌ آیتم کافی برای تبدیل نداری.")
		}
	}
}

func (t *Template) showLeaderboard(bc *engine.BotContext, repo *postgres.DuelRepo, botID uuid.UUID) {
	players, _ := repo.TopPlayers(context.Background(), botID, 20)
	medals := []string{"🥇", "🥈", "🥉"}
	var entries []string
	for i, p := range players {
		prefix := fmt.Sprintf("%d.", i+1)
		if i < 3 {
			prefix = medals[i]
		}
		name := fmt.Sprintf("بازیکن %d", p.UserID%10000)
		if p.IsBot {
			name = fmt.Sprintf("🤖 %s", name)
		}
		entries = append(entries, fmt.Sprintf("%s %s — %d امتیاز", prefix, name, p.Score))
	}
	msg := bc.Text("leaderboard")
	msg = strings.ReplaceAll(msg, "{entries}", strings.Join(entries, "\n"))
	bc.ReplyInline(msg, [][]engine.InlineButton{{{Label: "🏠 منو", Callback: "act:menu"}}})
}

func (t *Template) showEnergyShop(bc *engine.BotContext) {
	pkgs := bc.PricePackages()
	if len(pkgs) == 0 {
		bc.Reply("پکیجی موجود نیست.")
		return
	}
	var lines []string
	lines = append(lines, bc.Text("energy_shop"))
	var buttons [][]engine.InlineButton
	for _, pkg := range pkgs {
		label, _ := pkg["label"].(string)
		id, _ := pkg["id"].(string)
		if label == "" || id == "" {
			continue
		}
		buttons = append(buttons, []engine.InlineButton{{Label: label, Callback: "act:buy:" + id}})
	}
	buttons = append(buttons, []engine.InlineButton{{Label: "🏠 منو", Callback: "act:menu"}})
	bc.ReplyInline(strings.Join(lines, "\n"), buttons)
}

func (t *Template) doBuyEnergy(bc *engine.BotContext, repo *postgres.DuelRepo, botID uuid.UUID, pkgID string) {
	pkgs := bc.PricePackages()
	for _, pkg := range pkgs {
		id, _ := pkg["id"].(string)
		if id != pkgID {
			continue
		}
		energy := int64(0)
		if v, ok := pkg["energy"].(float64); ok {
			energy = int64(v)
		}
		price := int64(0)
		if v, ok := pkg["price"].(float64); ok {
			price = int64(v)
		}
		svc := paySvcOf(bc)
		if svc == nil {
			bc.Reply("سیستم پرداخت در دسترس نیست.")
			return
		}
		payURL, err := svc.BuyPackage(context.Background(), botID, bc.UserID, bc.TelegramID, pkgID, energy, price)
		if err != nil {
			log.Printf("[duel] buy energy: %v", err)
			bc.Reply("خطا در ایجاد پرداخت.")
			return
		}
		if payURL == "dev://payment-success" {
			_ = repo.AddEnergy(context.Background(), botID, bc.UserID, int(energy))
			_ = repo.RecordSpent(context.Background(), botID, bc.UserID, price)
			poolShare := price * int64(bc.PriceInt("pool_share_pct", 70)) / 100
			_ = repo.AddToPool(context.Background(), botID, poolShare)
			bc.Reply(fmt.Sprintf("✅ پرداخت تستی! +%d انرژی", energy))
			return
		}
		msg := bc.Text("payment_link")
		msg = strings.ReplaceAll(msg, "{link}", payURL)
		bc.ReplyInline(msg, [][]engine.InlineButton{
			{{Label: "✅ بررسی پرداخت", Callback: "act:pay:verify"}},
			{{Label: "🏠 منو", Callback: "act:menu"}},
		})
		return
	}
	bc.Reply("پکیج نامعتبر.")
}

func (t *Template) showWithdraw(bc *engine.BotContext, repo *postgres.DuelRepo, botID uuid.UUID, kind string) {
	if kind == "" {
		// Show withdraw options menu
		bc.ReplyInline("💰 برداشت جوایز:", [][]engine.InlineButton{
			{{Label: "💰 برداشت نقدی", Callback: "act:withdraw:cash"}},
			{{Label: "🥇 طلای فیزیکی", Callback: "act:withdraw:physical"}},
			{{Label: "🏠 منو", Callback: "act:menu"}},
		})
		return
	}

	p, _ := repo.GetPlayer(context.Background(), botID, bc.UserID)

	switch kind {
	case "cash":
		minMg := bc.PriceInt("withdraw_min_mg_cash", 100)
		goldMgOpal := bc.PriceInt("gold_1mg_opal", 20)
		minOpal := minMg * goldMgOpal
		msg := bc.Text("withdraw_cash")
		msg = strings.ReplaceAll(msg, "{min_opal}", strconv.Itoa(minOpal))
		msg = strings.ReplaceAll(msg, "{min_mg}", strconv.Itoa(minMg))
		msg = strings.ReplaceAll(msg, "{opal}", strconv.Itoa(p.BlackOpal))
		bc.ReplyInline(msg, [][]engine.InlineButton{
			{{Label: "✅ تایید برداشت", Callback: "act:withdraw:cash"}},
			{{Label: "🏠 منو", Callback: "act:menu"}},
		})
	case "physical":
		minG := bc.PriceInt("withdraw_min_g_physical", 1)
		goldMgOpal := bc.PriceInt("gold_1mg_opal", 20)
		minOpal := minG * 1000 * goldMgOpal
		msg := bc.Text("withdraw_physical")
		msg = strings.ReplaceAll(msg, "{min_opal}", strconv.Itoa(minOpal))
		msg = strings.ReplaceAll(msg, "{min_g}", strconv.Itoa(minG))
		msg = strings.ReplaceAll(msg, "{opal}", strconv.Itoa(p.BlackOpal))
		bc.ReplyInline(msg, [][]engine.InlineButton{
			{{Label: "✅ تایید برداشت", Callback: "act:withdraw:physical"}},
			{{Label: "🏠 منو", Callback: "act:menu"}},
		})
	}
}

func (t *Template) doWithdraw(bc *engine.BotContext, repo *postgres.DuelRepo, botID uuid.UUID, kind string) {
	ctx := context.Background()
	p, _ := repo.GetPlayer(ctx, botID, bc.UserID)

	var amountMg float64
	var amountToman int64
	var minOpal int

	goldMgOpal := bc.PriceInt("gold_1mg_opal", 20)
	goldMgToman := bc.PriceInt("gold_1mg_toman", 20000)

	switch kind {
	case "cash":
		minMg := bc.PriceInt("withdraw_min_mg_cash", 100)
		minOpal = minMg * goldMgOpal
		if p.BlackOpal < minOpal {
			msg := bc.Text("withdraw_insufficient")
			msg = strings.ReplaceAll(msg, "{opal}", strconv.Itoa(p.BlackOpal))
			msg = strings.ReplaceAll(msg, "{needed}", strconv.Itoa(minOpal))
			bc.Reply(msg)
			return
		}
		amountMg = float64(p.BlackOpal / goldMgOpal)
		amountToman = int64(amountMg) * int64(goldMgToman)
		totalOpals := int(amountMg) * goldMgOpal
		ok, _ := repo.ConsumeOpals(ctx, botID, bc.UserID, totalOpals)
		if !ok {
			bc.Reply("خطا در کسر اوپال.")
			return
		}
	case "physical":
		minG := bc.PriceInt("withdraw_min_g_physical", 1)
		minOpal = minG * 1000 * goldMgOpal
		if p.BlackOpal < minOpal {
			msg := bc.Text("withdraw_insufficient")
			msg = strings.ReplaceAll(msg, "{opal}", strconv.Itoa(p.BlackOpal))
			msg = strings.ReplaceAll(msg, "{needed}", strconv.Itoa(minOpal))
			bc.Reply(msg)
			return
		}
		grams := p.BlackOpal / (1000 * goldMgOpal)
		amountMg = float64(grams * 1000)
		amountToman = int64(amountMg) * int64(goldMgToman)
		totalOpals := grams * 1000 * goldMgOpal
		ok, _ := repo.ConsumeOpals(ctx, botID, bc.UserID, totalOpals)
		if !ok {
			bc.Reply("خطا در کسر اوپال.")
			return
		}
	default:
		return
	}

	_, _ = repo.CreateWithdrawal(ctx, &domain.DuelWithdrawal{
		BotID:       botID,
		UserID:      bc.UserID,
		AmountMg:    amountMg,
		AmountToman: amountToman,
		Kind:        kind,
	})

	msg := bc.Text("withdraw_confirm")
	msg = strings.ReplaceAll(msg, "{amount_mg}", strconv.FormatFloat(amountMg, 'f', 0, 64))
	msg = strings.ReplaceAll(msg, "{amount_toman}", strconv.FormatInt(amountToman, 10))
	bc.ReplyInline(msg, [][]engine.InlineButton{{{Label: "🏠 منو", Callback: "act:menu"}}})
}

// PriceFloat reads a float price from Prices with fallback.
func priceFloat(bc *engine.BotContext, key string, def float64) float64 {
	if v, ok := bc.Prices[key].(float64); ok {
		return v
	}
	return def
}
