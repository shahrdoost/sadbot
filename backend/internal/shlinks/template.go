// Package shlinks is the link shortener with earnings template.
package shlinks

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

type Template struct{}

func (t *Template) Key() string { return domain.TemplateShlinks }

func (t *Template) DefaultTexts() []byte {
	return []byte(`{
		"welcome": "🔗 به لینک‌ساز خوش اومدی!\n\n💰 موجودی: {balance} تومان\n📊 کل درآمد: {total_earned} تومان\n\nبا هر کلیک معتبر روی لینکت درآمد کسب کن!",
		"create_link": "🔗 لینک جدید بساز:\n\nآدرس URL مورد نظرت رو بفرست:",
		"link_created": "✅ لینک شما ساخته شد!\n\n🔗 کد کوتاه: {code}\n📍 مقصد: {url}\n\nکلیک‌ها: 0\n💰 درآمد: 0 تومان",
		"my_links": "📋 لینک‌های شما:\n\n{links}\n\n💰 موجودی: {balance} تومان",
		"no_links": "📋 هنوز لینکی نساختی!\n\n🔗 یه لینک جدید بساز.",
		"click_recorded": "📊 کلیک ثبت شد!\n💰 +{earn} تومان\n💰 موجودی: {balance} تومان",
		"withdraw": "💰 برداشت درآمد:\n\n💰 موجودی: {balance} تومان\n💵 حداقل برداشت: {min_withdraw} تومان",
		"withdraw_ok": "✅ درخواست برداشت ثبت شد!\n⏳ در انتظار تایید ادمین.",
		"withdraw_fail": "❌ موجودی کافی نیست!\n💰 نیاز: {min_withdraw} تومان",
		"invalid_url": "❌ لینک نامعتبره! لطفاً یه URL کامل بفرست (شامل https://)",
		"unknown": "❓ دستور ناشناخته!\n/start - شروع\n/mylinks - لینک‌های من\n/withdraw - برداشت"
	}`)
}

func (t *Template) DefaultMenus() []byte { return []byte(`{}`) }

func (t *Template) DefaultPrices() []byte {
	return []byte(`{
		"earn_per_click": 100,
		"min_withdraw": 50000,
		"base_url": "https://shlink.example.com"
	}`)
}

func shlinksRepoOf(bc *engine.BotContext) *postgres.ShlinksRepo {
	r, _ := bc.Data["shlinks"].(*postgres.ShlinksRepo)
	return r
}

func (t *Template) Start(bc *engine.BotContext, payload string) bool {
	repo := shlinksRepoOf(bc)
	if repo == nil {
		bc.Reply("خطای سیستمی.")
		return true
	}
	botID, _ := uuid.Parse(bc.BotID)
	u, err := repo.EnsureUser(context.Background(), botID, bc.UserID, "")
	if err != nil {
		bc.Reply("خطا.")
		return true
	}

	welcome := bc.Text("welcome")
	welcome = strings.ReplaceAll(welcome, "{balance}", fmt.Sprintf("%.0f", u.Balance))
	welcome = strings.ReplaceAll(welcome, "{total_earned}", fmt.Sprintf("%.0f", u.TotalEarned))

	bc.ReplyInline(welcome, [][]engine.InlineButton{
		{{Label: "🔗 ساخت لینک", Callback: "act:create"}},
		{{Label: "📋 لینک‌های من", Callback: "act:mylinks"}},
		{{Label: "💰 برداشت", Callback: "act:withdraw"}},
	})
	return true
}

func (t *Template) HandleMessage(bc *engine.BotContext, text string) bool {
	repo := shlinksRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)

	// Check if this is a URL
	if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
		t.doCreateLink(bc, repo, botID, text)
		return true
	}

	switch text {
	case "/start":
		return t.Start(bc, "")
	case "/mylinks":
		t.showMyLinks(bc, repo, botID)
	case "/withdraw":
		t.showWithdraw(bc, repo, botID)
	default:
		bc.Reply(bc.Text("unknown"))
	}
	return true
}

func (t *Template) HandleAction(bc *engine.BotContext, action string) bool {
	repo := shlinksRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)
	switch action {
	case "act:create":
		bc.Reply(bc.Text("create_link"))
	case "act:mylinks":
		t.showMyLinks(bc, repo, botID)
	case "act:withdraw":
		t.showWithdraw(bc, repo, botID)
	case "act:do_withdraw":
		t.doWithdraw(bc, repo, botID)
	}
	return true
}

func (t *Template) doCreateLink(bc *engine.BotContext, repo *postgres.ShlinksRepo, botID uuid.UUID, targetURL string) {
	shortCode := generateShortCode()
	_, err := repo.CreateLink(context.Background(), botID, bc.UserID, shortCode, targetURL)
	if err != nil {
		bc.Reply("خطا در ساخت لینک.")
		return
	}

	baseURL := bc.Prices["base_url"].(string)
	msg := bc.Text("link_created")
	msg = strings.ReplaceAll(msg, "{code}", shortCode)
	msg = strings.ReplaceAll(msg, "{url}", targetURL)
	_ = baseURL
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "🔗 لینک جدید", Callback: "act:create"}},
		{{Label: "📋 لینک‌های من", Callback: "act:mylinks"}},
	})
}

func (t *Template) showMyLinks(bc *engine.BotContext, repo *postgres.ShlinksRepo, botID uuid.UUID) {
	links, err := repo.ListLinks(context.Background(), botID, bc.UserID)
	if err != nil || len(links) == 0 {
		bc.ReplyInline(bc.Text("no_links"), [][]engine.InlineButton{{{Label: "🔗 ساخت لینک", Callback: "act:create"}}})
		return
	}

	u, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	var parts []string
	for _, l := range links {
		parts = append(parts, fmt.Sprintf("🔗 `%s` — %d کلیک — %.0f تومان", l.ShortCode, l.TotalClicks, l.TotalEarned))
	}
	msg := bc.Text("my_links")
	msg = strings.ReplaceAll(msg, "{links}", strings.Join(parts, "\n"))
	msg = strings.ReplaceAll(msg, "{balance}", fmt.Sprintf("%.0f", u.Balance))
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "🔗 لینک جدید", Callback: "act:create"}},
		{{Label: "💰 برداشت", Callback: "act:withdraw"}},
	})
}

func (t *Template) showWithdraw(bc *engine.BotContext, repo *postgres.ShlinksRepo, botID uuid.UUID) {
	u, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	minWithdraw := bc.PriceInt("min_withdraw", 50000)

	if u.Balance < float64(minWithdraw) {
		msg := bc.Text("withdraw_fail")
		msg = strings.ReplaceAll(msg, "{min_withdraw}", strconv.Itoa(minWithdraw))
		bc.Reply(msg)
		return
	}

	msg := bc.Text("withdraw")
	msg = strings.ReplaceAll(msg, "{balance}", fmt.Sprintf("%.0f", u.Balance))
	msg = strings.ReplaceAll(msg, "{min_withdraw}", strconv.Itoa(minWithdraw))
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "✅ تایید برداشت", Callback: "act:do_withdraw"}},
	})
}

func (t *Template) doWithdraw(bc *engine.BotContext, repo *postgres.ShlinksRepo, botID uuid.UUID) {
	u, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	minWithdraw := bc.PriceInt("min_withdraw", 50000)
	if u.Balance < float64(minWithdraw) {
		bc.Reply(bc.Text("withdraw_fail"))
		return
	}
	amount := u.Balance
	if err := repo.RequestWithdrawal(context.Background(), botID, bc.UserID, amount); err != nil {
		bc.Reply("خطا.")
		return
	}
	bc.Reply(bc.Text("withdraw_ok"))
}

func generateShortCode() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, 6)
	for i := range code {
		code[i] = chars[rand.Intn(len(chars))]
	}
	return string(code)
}
