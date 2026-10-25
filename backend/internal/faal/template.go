// Package faal is the daily fortune-telling bot template.
package faal

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

type Template struct{}

func (t *Template) Key() string { return domain.TemplateFaal }

func (t *Template) DefaultTexts() []byte {
	return []byte(`{
		"welcome": "🔮 به فال روزانه خوش اومدی!\n\nهر روز یه فال بگیر و روزت رو بساز.\n\n🔮 فال امروز:",
		"fortune_result": "🔮 فال امروز شما:\n\n{fortune}\n\n✨ روز خوبی داشته باشی!",
		"fortune_cooldown": "⏳ فال امروز رو قبلاً گرفتی!\nفردا دوباره بیا.\n\n📅 تعداد فال‌های شما: {count}",
		"stats": "📊 آمار شما:\n🔮 تعداد فال‌ها: {count}\n⏰ آخرین فال: {last}",
		"unknown": "🔮 برای گرفتن فال /start بزن."
	}`)
}

func (t *Template) DefaultMenus() []byte { return []byte(`{}`) }

func (t *Template) DefaultPrices() []byte {
	return []byte(`{
		"fortune_cooldown_hours": 20,
		"fortunes": [
			"🌟 امروز روز شانس توئه! یه فرصت جدید داره میاد.",
			"💰 پول از جایی که انتظارش رو نداری میاد.",
			"❤️ عشق در نزدیکی توئه، چشماتو باز کن.",
				"📚 یه خبر خوب درباره تحصیل یا کار داری.",
			"🍀 شانس با توئه، ریسک کن!",
			"🌈 بعد از هر سختی، آسونی هست. صبور باش.",
			"🎯 هدفی که داری نزدیک‌تر از اونیه که فکر می‌کنی.",
			"🤝 یه دوست قدیمی بهت خبر خوبی میده.",
			"✨ امروز روز خلاقیت توئه، ایده‌هات رو عملی کن.",
			"🏠 آرامش خانواده بهت انرژی میده.",
			"🌟 یه تغییر مثبت در زندگیت داره اتفاق میفته.",
			"💪 قدرت درونیت امروز بیشتر از همیشه‌ست.",
			"🎁 یه هدیه غیرمنتظره در راهه.",
			"🌻 انرژی مثبت امروز رو به بقیه هم منتقل کن.",
			"💫 یه آرزوی قدیمی داره برآورده میشه."
		]
	}`)
}

func faalRepoOf(bc *engine.BotContext) *postgres.FaalRepo {
	r, _ := bc.Data["faal"].(*postgres.FaalRepo)
	return r
}

func (t *Template) Start(bc *engine.BotContext, payload string) bool {
	repo := faalRepoOf(bc)
	if repo == nil {
		bc.Reply("خطای سیستمی.")
		return true
	}
	botID, _ := uuid.Parse(bc.BotID)
	username := ""
	if bc.Sender != nil {
		// try to get username from state
	}
	_, err := repo.EnsureUser(context.Background(), botID, bc.UserID, username)
	if err != nil {
		bc.Reply("خطا. /start بزن.")
		return true
	}
	t.doFortune(bc, repo, botID)
	return true
}

func (t *Template) HandleMessage(bc *engine.BotContext, text string) bool {
	repo := faalRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)
	switch text {
	case "/start", "/fortune", "🔮 فال":
		t.doFortune(bc, repo, botID)
	case "/stats":
		t.showStats(bc, repo, botID)
	default:
		bc.Reply(bc.Text("unknown"))
	}
	return true
}

func (t *Template) HandleAction(bc *engine.BotContext, action string) bool {
	repo := faalRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)
	switch action {
	case "act:fortune":
		t.doFortune(bc, repo, botID)
	case "act:stats":
		t.showStats(bc, repo, botID)
	}
	return true
}

func (t *Template) doFortune(bc *engine.BotContext, repo *postgres.FaalRepo, botID uuid.UUID) {
	// Check cooldown
	lastTime, err := repo.GetLastFortuneTime(context.Background(), botID, bc.UserID)
	if err == nil && lastTime != nil {
		cooldownHours := bc.PriceInt("fortune_cooldown_hours", 20)
		if timeSince := timeSince(lastTime); timeSince < cooldownHours*3600 {
			msg := bc.Text("fortune_cooldown")
			msg = strings.ReplaceAll(msg, "{count}", fmt.Sprintf("%d", fortuneCount(bc, repo, botID)))
			bc.Reply(msg)
			return
		}
	}

	// Pick random fortune from prices
	fortunes := bc.Prices["fortunes"].([]any)
	if len(fortunes) == 0 {
		bc.Reply("فالی موجود نیست.")
		return
	}
	fortune := fortunes[rand.Intn(len(fortunes))].(string)

	_ = repo.SaveFortune(context.Background(), botID, bc.UserID, fortune, "daily")

	msg := bc.Text("fortune_result")
	msg = strings.ReplaceAll(msg, "{fortune}", fortune)
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "🔮 فال دیگه", Callback: "act:fortune"}},
		{{Label: "📊 آمار", Callback: "act:stats"}},
	})
}

func (t *Template) showStats(bc *engine.BotContext, repo *postgres.FaalRepo, botID uuid.UUID) {
	msg := bc.Text("stats")
	msg = strings.ReplaceAll(msg, "{count}", fmt.Sprintf("%d", fortuneCount(bc, repo, botID)))
	msg = strings.ReplaceAll(msg, "{last}", "—")
	bc.ReplyInline(msg, [][]engine.InlineButton{{{Label: "🔮 فال", Callback: "act:fortune"}}})
}

func fortuneCount(bc *engine.BotContext, repo *postgres.FaalRepo, botID uuid.UUID) int {
	u, err := repo.EnsureUser(context.Background(), botID, bc.UserID, "")
	if err != nil {
		return 0
	}
	return u.FortuneCount
}

func timeSince(t *time.Time) int {
	if t == nil {
		return 999999
	}
	return int(time.Since(*t).Seconds())
}
