// Package whoseeme is the "who saw my link" bait-link tracker template.
package whoseeme

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

func (t *Template) Key() string { return domain.TemplateWhoseeme }

func (t *Template) DefaultTexts() []byte {
	return []byte(`{
		"welcome": "🕵️ به «کی منو دیده» خوش اومدی!\n\n🔗 لینک طعمه بساز و ببین کی کلیک می‌کنه!\n💰 سکه: {coins}\n💰 موجودی: {balance} تومان",
		"create_link": "🔗 لینک طعمه بساز:\n\n📝 متن طعمه رو بنویس (مثلاً: «عکس پروفایلت رو دیدم!»):",
		"link_created": "✅ لینک طعمه ساخته شد!\n\n🔗 لینک: {link}\n📝 متن: {bait_text}\n\nهرکی روش کلیک کنه، بهت خبر میدم!\n👁 بازدیدها: 0",
		"my_links": "📋 لینک‌های طعمه شما:\n\n{links}",
		"no_links": "📋 هنوز لینکی نساختی!\n\n🔗 یه لینک طعمه بساز.",
		"click_notification": "👁 کسی روی لینک طعمه‌ات کلیک کرد!\n\n👤 بازدیدکننده: {viewer}\n🔗 لینک: {link}",
		"leaderboard": "🏆 لیدربورد:\n\n{entries}",
		"stats": "📊 آمار شما:\n🔗 لینک‌ها: {links}\n👁 کل بازدیدها: {clicks}\n💰 سکه: {coins}",
		"unknown": "❓ دستور ناشناخته!\n/start - شروع\n/mylinks - لینک‌های من"
	}`)
}

func (t *Template) DefaultMenus() []byte { return []byte(`{}`) }

func (t *Template) DefaultPrices() []byte {
	return []byte(`{
		"coins_per_click": 5,
		"base_url": "https://whos.example.com",
		"invite_bonus_coins": 50
	}`)
}

func whoseemeRepoOf(bc *engine.BotContext) *postgres.WhoseemeRepo {
	r, _ := bc.Data["whoseeme"].(*postgres.WhoseemeRepo)
	return r
}

func (t *Template) Start(bc *engine.BotContext, payload string) bool {
	repo := whoseemeRepoOf(bc)
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
	welcome = strings.ReplaceAll(welcome, "{coins}", strconv.Itoa(u.Coins))
	welcome = strings.ReplaceAll(welcome, "{balance}", fmt.Sprintf("%.0f", u.Balance))

	bc.ReplyInline(welcome, [][]engine.InlineButton{
		{{Label: "🔗 ساخت لینک طعمه", Callback: "act:create"}},
		{{Label: "📋 لینک‌های من", Callback: "act:mylinks"}},
		{{Label: "📊 آمار", Callback: "act:stats"}},
	})
	return true
}

func (t *Template) HandleMessage(bc *engine.BotContext, text string) bool {
	repo := whoseemeRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)

	// If in create-link state, treat text as bait text
	state := bc.State["creating_link"]
	if state == true {
		delete(bc.State, "creating_link")
		t.doCreateLink(bc, repo, botID, text)
		return true
	}

	switch text {
	case "/start":
		return t.Start(bc, "")
	case "/mylinks":
		t.showMyLinks(bc, repo, botID)
	case "/stats":
		t.showStats(bc, repo, botID)
	default:
		// Default: treat as bait text creation
		t.doCreateLink(bc, repo, botID, text)
	}
	return true
}

func (t *Template) HandleAction(bc *engine.BotContext, action string) bool {
	repo := whoseemeRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)
	switch action {
	case "act:create":
		bc.State["creating_link"] = true
		bc.Reply(bc.Text("create_link"))
	case "act:mylinks":
		t.showMyLinks(bc, repo, botID)
	case "act:stats":
		t.showStats(bc, repo, botID)
	}
	return true
}

func (t *Template) doCreateLink(bc *engine.BotContext, repo *postgres.WhoseemeRepo, botID uuid.UUID, baitText string) {
	token := generateToken()
	_, err := repo.CreateLink(context.Background(), botID, bc.UserID, token, baitText)
	if err != nil {
		bc.Reply("خطا در ساخت لینک.")
		return
	}

	baseURL := "https://t.me/bot?start=" + token
	msg := bc.Text("link_created")
	msg = strings.ReplaceAll(msg, "{link}", baseURL)
	msg = strings.ReplaceAll(msg, "{bait_text}", baitText)
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "🔗 لینک جدید", Callback: "act:create"}},
		{{Label: "📋 لینک‌های من", Callback: "act:mylinks"}},
	})
}

func (t *Template) showMyLinks(bc *engine.BotContext, repo *postgres.WhoseemeRepo, botID uuid.UUID) {
	links, err := repo.ListLinks(context.Background(), botID, bc.UserID)
	if err != nil || len(links) == 0 {
		bc.ReplyInline(bc.Text("no_links"), [][]engine.InlineButton{{{Label: "🔗 ساخت لینک", Callback: "act:create"}}})
		return
	}

	var parts []string
	for _, l := range links {
		clicks, _ := repo.GetClicks(context.Background(), botID, l.ID)
		baitText := ""
		if l.BaitText != nil {
			baitText = *l.BaitText
		}
		parts = append(parts, fmt.Sprintf("🔗 `%s` — %s — 👁 %d بازدید", l.Token[:8], baitText, clicks))
	}
	msg := bc.Text("my_links")
	msg = strings.ReplaceAll(msg, "{links}", strings.Join(parts, "\n"))
	bc.Reply(msg)
}

func (t *Template) showStats(bc *engine.BotContext, repo *postgres.WhoseemeRepo, botID uuid.UUID) {
	links, _ := repo.ListLinks(context.Background(), botID, bc.UserID)
	totalClicks := 0
	for _, l := range links {
		n, _ := repo.GetClicks(context.Background(), botID, l.ID)
		totalClicks += n
	}
	u, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	msg := bc.Text("stats")
	msg = strings.ReplaceAll(msg, "{links}", strconv.Itoa(len(links)))
	msg = strings.ReplaceAll(msg, "{clicks}", strconv.Itoa(totalClicks))
	if u != nil {
		msg = strings.ReplaceAll(msg, "{coins}", strconv.Itoa(u.Coins))
	}
	bc.Reply(msg)
}

func generateToken() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	code := make([]byte, 10)
	for i := range code {
		code[i] = chars[rand.Intn(len(chars))]
	}
	return string(code)
}
