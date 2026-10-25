// Package anonychat is the anonymous chat bot template.
package anonychat

import (
	"context"
	"log"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

type Template struct{}

func (t *Template) Key() string { return domain.TemplateAnonyChat }

func (t *Template) DefaultTexts() []byte {
	return []byte(`{
		"welcome": "🕶 به چت ناشناس خوش اومدی!\n\nاینجا می‌تونی ناشناس با کسی چت کنی.\n\n🔍 جستجوی هم‌صحبت:",
		"searching": "🔍 در حال جستجوی هم‌صحبت...\n\n⏳ لطفاً صبر کن...",
		"found": "✅ هم‌صحبت پیدا شد!\n\n💬 حالا می‌تونی چت کنی.\nهر پیامی که بفرستی ناشناس ارسال میشه.",
		"chat_ended": "💬 چت تمام شد.\n\n🔍 دوباره جستجو کن.",
		"no_partner": "😔 فعلاً کسی آنلاین نیست.\nکمی بعد دوباره امتحان کن.",
		"partner_left": "😔 هم‌صحبتت چت رو ترک کرد.",
		"invalid": "❓ لطفاً از دکمه‌ها استفاده کن.",
		"report_ok": "✅ گزارش ثبت شد. ممنون.",
		"search_cancel": "❌ جستجو لغو شد."
	}`)
}

func (t *Template) DefaultMenus() []byte { return []byte(`{}`) }

func (t *Template) DefaultPrices() []byte {
	return []byte(`{
		"search_timeout_seconds": 60
	}`)
}

func repoOf(bc *engine.BotContext) *postgres.AnonyChatRepo {
	r, _ := bc.Data["anonychat"].(*postgres.AnonyChatRepo)
	return r
}

func (t *Template) Start(bc *engine.BotContext, payload string) bool {
	repo := repoOf(bc)
	if repo == nil {
		bc.Reply("خطای سیستمی.")
		return true
	}
	botID, _ := uuid.Parse(bc.BotID)
	if _, err := repo.EnsureUser(context.Background(), botID, bc.UserID, ""); err != nil {
		bc.Reply("خطا.")
		return true
	}

	bc.ReplyInline(bc.Text("welcome"), [][]engine.InlineButton{
		{{Label: "🔍 جستجوی هم‌صحبت", Callback: "act:search"}},
		{{Label: "⛔ پایان چت", Callback: "act:end"}},
	})
	return true
}

func (t *Template) HandleMessage(bc *engine.BotContext, text string) bool {
	repo := repoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)

	// If in chat, relay message to partner
	status, partnerID, err := repo.GetStatus(context.Background(), botID, bc.UserID)
	if err != nil {
		return false
	}

	if status == "chatting" && partnerID != nil {
		// Relay the message to partner via sender
		if bc.Sender != nil {
			bc.Sender.Send(context.Background(), *partnerID, "📩 پیام ناشناس:\n\n"+text, nil, 0)
		}
		return true
	}

	switch text {
	case "/start":
		return t.Start(bc, "")
	case "/search":
		t.doSearch(bc, repo, botID)
	case "/end":
		t.doEnd(bc, repo, botID)
	default:
		bc.Reply(bc.Text("invalid"))
	}
	return true
}

func (t *Template) HandleAction(bc *engine.BotContext, action string) bool {
	repo := repoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)
	switch action {
	case "act:search":
		t.doSearch(bc, repo, botID)
	case "act:end":
		t.doEnd(bc, repo, botID)
	case "act:cancel":
		_ = repo.SetStatus(context.Background(), botID, bc.UserID, "idle", nil)
		bc.Reply(bc.Text("search_cancel"))
	case "act:report":
		bc.Reply(bc.Text("report_ok"))
	}
	return true
}

func (t *Template) doSearch(bc *engine.BotContext, repo *postgres.AnonyChatRepo, botID uuid.UUID) {
	// Set status to searching
	_ = repo.SetStatus(context.Background(), botID, bc.UserID, "searching", nil)

	// Try to find a partner
	partnerID, err := repo.FindPartner(context.Background(), botID, bc.UserID)
	if err != nil {
		// No partner found yet — show searching message
		bc.ReplyInline(bc.Text("searching"), [][]engine.InlineButton{
			{{Label: "❌ لغو", Callback: "act:cancel"}},
		})
		return
	}

	// Match!
	if err := repo.MatchUsers(context.Background(), botID, bc.UserID, partnerID); err != nil {
		log.Printf("[anonychat] match error: %v", err)
		bc.Reply("خطا.")
		return
	}

	bc.Reply(bc.Text("found"))
	if bc.Sender != nil {
		bc.Sender.Send(context.Background(), partnerID, bc.Text("found"), nil, 0)
	}
}

func (t *Template) doEnd(bc *engine.BotContext, repo *postgres.AnonyChatRepo, botID uuid.UUID) {
	partnerID, err := repo.EndChat(context.Background(), botID, bc.UserID)
	if err != nil {
		bc.Reply("خطا.")
		return
	}
	bc.ReplyInline(bc.Text("chat_ended"), [][]engine.InlineButton{
		{{Label: "🔍 جستجوی جدید", Callback: "act:search"}},
	})
	if partnerID != nil && bc.Sender != nil {
		_ = repo.SetStatus(context.Background(), botID, *partnerID, "idle", nil)
		bc.Sender.Send(context.Background(), *partnerID, bc.Text("partner_left"), nil, 0)
	}
}

var _ = strings.TrimSpace
