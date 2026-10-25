// Package bombland is the idle-factory game template (BombLandBot port).
package bombland

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/payment"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

type Template struct{}

func (t *Template) Key() string { return domain.TemplateBombLand }

func (t *Template) DefaultTexts() []byte {
	return []byte(`{
		"welcome": "🏭 به کارخانه بومبلند خوش اومدی!\n\nهر بمبی که بسازی رو می‌تونی بفروشی و ماشین‌هات رو ارتقا بدی.\n\n💰 پول: {money}\n💣 بمب‌ها: {bombs}\n⚙️ سطح ماشین: {auto_lvl}",
		"status": "📊 وضعیت کارخانه:\n\n💣 بمب‌ها: {bombs}\n💰 پول: {money}\n⚙️ سطح ماشین: {auto_lvl}\n⚡ تولید خودکار: {auto_rate} بمب/دقیقه\n👥 زیرمجموعه‌ها: {referrals}",
		"produce": "💣 یه بمب ساخته شد!\n💣 بمب‌ها: {bombs}",
		"sell_ok": "✅ فروخته شد!\n💣 {sold} بمب → 💰 {earned} تومان\n💰 پول جدید: {money}",
		"sell_empty": "❌ بمبی برای فروش نداری!",
		"upgrade_ok": "✅ ماشین ارتقا یافت!\n⚙️ سطح جدید: {auto_lvl}\n💰 پول باقی‌مانده: {money}",
		"upgrade_fail": "❌ پول کافی نیست!\n💰 پول شما: {money}\n💰 هزینه ارتقا: {cost}",
		"invite": "🔗 لینک دعوت شما:\n{link}\n\n🎁 با هر دعوت: +{bonus} بمب رایگان\n👥 تعداد زیرمجموعه‌ها: {referrals}",
		"invite_reward": "🎉 {name} با لینک شما عضو شد!\n+{bonus} بمب به شما اضافه شد.",
		"battle_teaser": "⚔️ نبرد به‌زودی فعال می‌شه!\nجایزه‌های ویژه در انتظارته.",
		"unknown": "❓ دستور ناشناخته!\n\nدستورات:\n/start - شروع\n/produce - ساخت بمب\n/sell - فروش بمب‌ها\n/upgrade - ارتقا ماشین\n/status - وضعیت\n/invite - دعوت دوستان",
		"payment_link": "💳 برای خرید پول، روی لینک زیر بزن:\n{link}\n\n⚠️ قبل از پرداخت VPN خود را خاموش کنید.",
		"payment_ok": "✅ پرداخت تایید شد!\n💰 +{amount} تومان به حساب شما اضافه شد.",
		"energy_buy": "💰 خرید پول کارخانه:\n\n{packages}\n\n💳 درگاه پرداخت امن",
		"stats": "📊 آمار کلی:\n👥 کل بازیکنان: {total}"
	}`)
}

func (t *Template) DefaultMenus() []byte { return []byte(`{}`) }

func (t *Template) DefaultPrices() []byte {
	return []byte(`{
		"sell_price_per_bomb": 5,
		"upgrade_base_cost": 50,
		"auto_rate_per_level": 2,
		"manual_produce_amount": 1,
		"referral_bonus_bombs": 100,
		"packages": [
			{"id": "money_10k", "label": "💰 ۱۰,۰۰۰ تومان", "amount": 10000, "price": 10000},
			{"id": "money_50k", "label": "💰 ۵۰,۰۰۰ تومان", "amount": 50000, "price": 50000},
			{"id": "money_100k", "label": "💰 ۱۰۰,۰۰۰ تومان", "amount": 100000, "price": 100000}
		]
	}`)
}

// paySvcOf retrieves the shared payment service from DI.
func paySvcOf(bc *engine.BotContext) *payment.Service {
	s, _ := bc.Data["payments"].(*payment.Service)
	return s
}

func blRepoOf(bc *engine.BotContext) *postgres.BombLandRepo {
	r, _ := bc.Data["bombland"].(*postgres.BombLandRepo)
	return r
}

func (t *Template) Start(bc *engine.BotContext, payload string) bool {
	repo := blRepoOf(bc)
	if repo == nil {
		bc.Reply("خطای سیستمی.")
		return true
	}
	botID, err := uuid.Parse(bc.BotID)
	if err != nil {
		bc.Reply("خطا.")
		return true
	}

	p, err := repo.EnsurePlayer(context.Background(), botID, bc.UserID)
	if err != nil {
		log.Printf("[bombland] ensure player: %v", err)
		bc.Reply("خطا. /start بزن.")
		return true
	}

	// Handle referral
	if payload != "" {
		inviterID, err := strconv.ParseInt(payload, 10, 64)
		if err == nil && inviterID != bc.UserID {
			ok, _ := repo.SetInvitedBy(context.Background(), botID, bc.UserID, inviterID)
			if ok {
				bonus := bc.PriceInt("referral_bonus_bombs", 100)
				_ = repo.AddBombs(context.Background(), botID, inviterID, bonus)
				bc.Reply(fmt.Sprintf("🎉 با لینک دعوت عضو شدی! +%d بمب به دعوت‌کننده اضافه شد.", bonus))
			}
		}
	}

	// Catch up auto production
	rate := bc.PriceInt("auto_rate_per_level", 2)
	_, _ = repo.CatchUpAuto(context.Background(), botID, bc.UserID, rate)

	// Reload player after catch-up
	p, _ = repo.GetPlayer(context.Background(), botID, bc.UserID)
	refCount, _ := repo.ReferralCount(context.Background(), botID, bc.UserID)

	welcome := bc.Text("welcome")
	welcome = strings.ReplaceAll(welcome, "{money}", strconv.Itoa(p.Money))
	welcome = strings.ReplaceAll(welcome, "{bombs}", strconv.Itoa(p.Bombs))
	welcome = strings.ReplaceAll(welcome, "{auto_lvl}", strconv.Itoa(p.AutoLvl))

	bc.ReplyInline(welcome, [][]engine.InlineButton{
		{{Label: "💣 ساخت بمب", Callback: "act:produce"}, {Label: "💰 فروش", Callback: "act:sell"}},
		{{Label: "⚙️ ارتقا", Callback: "act:upgrade"}, {Label: "📊 وضعیت", Callback: "act:status"}},
		{{Label: "🔗 دعوت دوستان", Callback: "act:invite"}, {Label: "💳 خرید پول", Callback: "act:buy"}},
	})
	_ = refCount
	return true
}

func (t *Template) HandleMessage(bc *engine.BotContext, text string) bool {
	repo := blRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, err := uuid.Parse(bc.BotID)
	if err != nil {
		return false
	}

	switch text {
	case "/produce":
		t.doProduce(bc, repo, botID)
	case "/sell":
		t.doSell(bc, repo, botID)
	case "/upgrade":
		t.doUpgrade(bc, repo, botID)
	case "/status":
		t.doStatus(bc, repo, botID)
	case "/invite":
		t.doInvite(bc, repo, botID)
	case "/battle":
		bc.Reply(bc.Text("battle_teaser"))
	case "/xstats":
		n, _ := repo.TotalPlayers(context.Background(), botID)
		bc.Reply(fmt.Sprintf("👥 کل بازیکنان: %d", n))
	default:
		bc.Reply(bc.Text("unknown"))
	}
	return true
}

func (t *Template) HandleAction(bc *engine.BotContext, action string) bool {
	repo := blRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, err := uuid.Parse(bc.BotID)
	if err != nil {
		return false
	}

	switch action {
	case "act:produce":
		t.doProduce(bc, repo, botID)
	case "act:sell":
		t.doSell(bc, repo, botID)
	case "act:upgrade":
		t.doUpgrade(bc, repo, botID)
	case "act:status":
		t.doStatus(bc, repo, botID)
	case "act:invite":
		t.doInvite(bc, repo, botID)
	case "act:buy":
		t.showBuyMenu(bc, botID)
	case "act:buy:pkg":
		// handled below with param
		t.showBuyMenu(bc, botID)
	default:
		if strings.HasPrefix(action, "act:buy:") {
			pkgID := strings.TrimPrefix(action, "act:buy:")
			t.doBuy(bc, botID, pkgID)
		}
	}
	return true
}

func (t *Template) doProduce(bc *engine.BotContext, repo *postgres.BombLandRepo, botID uuid.UUID) {
	amount := bc.PriceInt("manual_produce_amount", 1)
	if err := repo.AddBombs(context.Background(), botID, bc.UserID, amount); err != nil {
		bc.Reply("خطا.")
		return
	}
	p, _ := repo.GetPlayer(context.Background(), botID, bc.UserID)
	msg := bc.Text("produce")
	msg = strings.ReplaceAll(msg, "{bombs}", strconv.Itoa(p.Bombs))
	bc.Reply(msg)
}

func (t *Template) doSell(bc *engine.BotContext, repo *postgres.BombLandRepo, botID uuid.UUID) {
	price := bc.PriceInt("sell_price_per_bomb", 5)
	sold, earned, err := repo.SellAll(context.Background(), botID, bc.UserID, price)
	if err != nil || sold == 0 {
		bc.Reply(bc.Text("sell_empty"))
		return
	}
	p, _ := repo.GetPlayer(context.Background(), botID, bc.UserID)
	msg := bc.Text("sell_ok")
	msg = strings.ReplaceAll(msg, "{sold}", strconv.Itoa(sold))
	msg = strings.ReplaceAll(msg, "{earned}", strconv.Itoa(earned))
	msg = strings.ReplaceAll(msg, "{money}", strconv.Itoa(p.Money))
	bc.Reply(msg)
}

func (t *Template) doUpgrade(bc *engine.BotContext, repo *postgres.BombLandRepo, botID uuid.UUID) {
	p, err := repo.GetPlayer(context.Background(), botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا.")
		return
	}
	baseCost := bc.PriceInt("upgrade_base_cost", 50)
	cost := (p.AutoLvl + 1) * baseCost
	ok, err := repo.TryUpgrade(context.Background(), botID, bc.UserID, cost)
	if err != nil || !ok {
		msg := bc.Text("upgrade_fail")
		msg = strings.ReplaceAll(msg, "{money}", strconv.Itoa(p.Money))
		msg = strings.ReplaceAll(msg, "{cost}", strconv.Itoa(cost))
		bc.Reply(msg)
		return
	}
	p2, _ := repo.GetPlayer(context.Background(), botID, bc.UserID)
	msg := bc.Text("upgrade_ok")
	msg = strings.ReplaceAll(msg, "{auto_lvl}", strconv.Itoa(p2.AutoLvl))
	msg = strings.ReplaceAll(msg, "{money}", strconv.Itoa(p2.Money))
	bc.Reply(msg)
}

func (t *Template) doStatus(bc *engine.BotContext, repo *postgres.BombLandRepo, botID uuid.UUID) {
	rate := bc.PriceInt("auto_rate_per_level", 2)
	_, _ = repo.CatchUpAuto(context.Background(), botID, bc.UserID, rate)
	p, err := repo.GetPlayer(context.Background(), botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا.")
		return
	}
	refCount, _ := repo.ReferralCount(context.Background(), botID, bc.UserID)
	msg := bc.Text("status")
	msg = strings.ReplaceAll(msg, "{bombs}", strconv.Itoa(p.Bombs))
	msg = strings.ReplaceAll(msg, "{money}", strconv.Itoa(p.Money))
	msg = strings.ReplaceAll(msg, "{auto_lvl}", strconv.Itoa(p.AutoLvl))
	msg = strings.ReplaceAll(msg, "{auto_rate}", strconv.Itoa(p.AutoLvl*rate))
	msg = strings.ReplaceAll(msg, "{referrals}", strconv.Itoa(refCount))
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "💣 ساخت بمب", Callback: "act:produce"}, {Label: "💰 فروش", Callback: "act:sell"}},
		{{Label: "⚙️ ارتقا", Callback: "act:upgrade"}, {Label: "🏠 منو", Callback: "act:status"}},
	})
}

func (t *Template) doInvite(bc *engine.BotContext, repo *postgres.BombLandRepo, botID uuid.UUID) {
	refCount, _ := repo.ReferralCount(context.Background(), botID, bc.UserID)
	bonus := bc.PriceInt("referral_bonus_bombs", 100)
	// Build deep link from bot username if available
	link := fmt.Sprintf("https://t.me/%s?start=%d", "bot", bc.UserID)
	msg := bc.Text("invite")
	msg = strings.ReplaceAll(msg, "{link}", link)
	msg = strings.ReplaceAll(msg, "{bonus}", strconv.Itoa(bonus))
	msg = strings.ReplaceAll(msg, "{referrals}", strconv.Itoa(refCount))
	bc.Reply(msg)
}

func (t *Template) showBuyMenu(bc *engine.BotContext, botID uuid.UUID) {
	pkgs := bc.PricePackages()
	if len(pkgs) == 0 {
		bc.Reply("پکیجی موجود نیست.")
		return
	}
	var lines []string
	lines = append(lines, bc.Text("energy_buy"))
	var buttons [][]engine.InlineButton
	for _, pkg := range pkgs {
		label, _ := pkg["label"].(string)
		id, _ := pkg["id"].(string)
		if label == "" || id == "" {
			continue
		}
		buttons = append(buttons, []engine.InlineButton{{Label: label, Callback: "act:buy:" + id}})
	}
	buttons = append(buttons, []engine.InlineButton{{Label: "🏠 منوی اصلی", Callback: "act:status"}})
	bc.ReplyInline(strings.Join(lines, "\n"), buttons)
}

func (t *Template) doBuy(bc *engine.BotContext, botID uuid.UUID, pkgID string) {
	pkgs := bc.PricePackages()
	for _, pkg := range pkgs {
		id, _ := pkg["id"].(string)
		if id != pkgID {
			continue
		}
		amount := int64(0)
		if v, ok := pkg["amount"].(float64); ok {
			amount = int64(v)
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
		payURL, err := svc.BuyPackage(context.Background(), botID, bc.UserID, bc.TelegramID, pkgID, amount, price)
		if err != nil {
			log.Printf("[bombland] buy: %v", err)
			bc.Reply("خطا در ایجاد پرداخت.")
			return
		}
		if payURL == "dev://payment-success" {
			bc.Reply(fmt.Sprintf("✅ پرداخت تستی تایید شد! +%d تومان", amount))
			return
		}
		msg := bc.Text("payment_link")
		msg = strings.ReplaceAll(msg, "{link}", payURL)
		bc.ReplyInline(msg, [][]engine.InlineButton{
			{{Label: "✅ بررسی پرداخت", Callback: "act:pay:verify"}},
			{{Label: "🏠 منوی اصلی", Callback: "act:status"}},
		})
		return
	}
	bc.Reply("پکیج نامعتبر.")
}
