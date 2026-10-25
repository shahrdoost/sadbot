// Package favaani is the money-mindset (abundance game) bot template.
package favaani

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

func (t *Template) Key() string { return domain.TemplateFavaani }

func (t *Template) DefaultTexts() []byte {
	return []byte(`{
		"welcome": "💰 به بازی فراوانی خوش اومدی!\n\n🎯 هر روز یه چالش مالی انجام بده.\nثبت خرج روزانه = امتیاز!\n\n📊 آمار شما:\n🏆 امتیاز: {score}\n🔥 استریک: {streak} روز\n📅 روز: {day}",
		"challenge": "🎯 چالش امروز:\n\n{challenge_text}\n\n💰 مبلغ خرج امروزت رو وارد کن (تومان):",
		"challenge_logged": "✅ ثبت شد!\n💰 مبلغ: {amount} تومان\n🏆 امتیاز: {score}\n🔥 استریک: {streak} روز",
		"stats": "📊 آمار شما:\n🏆 امتیاز: {score}\n🔥 استریک: {streak} روز\n📅 روز: {day}\n💸 کل خرج: {total_spent} تومان",
		"invite": "🔗 لینک دعوت شما:\n{link}\n\n🎁 با هر دعوت: +{bonus} امتیاز",
		"leaderboard": "🏆 لیدربورد:\n\n{entries}",
		"unknown": "❓ دستور ناشناخته!\n/start - شروع\n/stats - آمار\n/invite - دعوت دوستان"
	}`)
}

func (t *Template) DefaultMenus() []byte { return []byte(`{}`) }

func (t *Template) DefaultPrices() []byte {
	return []byte(`{
		"referral_bonus_score": 5,
		"daily_challenge_amount": 1000000,
		"challenges": [
			"🛍 امروز خریدی انجام بده و مبلغش رو ثبت کن!",
			"☕ یه قهوه بخر و هزینه‌اش رو ثبت کن.",
			"📚 یه کتاب بخر و هزینه‌اش رو ثبت کن.",
			"🎁 به یکی هدیه بده و هزینه‌اش رو ثبت کن.",
			"💡 یه سرمایه‌گذاری کوچیک انجام بده.",
			"🧹 یه چیز اضافی رو بفروش و درآمدش رو ثبت کن.",
			"📝 بودجه فردا رو برنامه‌ریزی کن.",
			"💵 یه قلک پس‌انداز شروع کن."
		]
	}`)
}

func favaaniRepoOf(bc *engine.BotContext) *postgres.FavaaniRepo {
	r, _ := bc.Data["favaani"].(*postgres.FavaaniRepo)
	return r
}

func (t *Template) Start(bc *engine.BotContext, payload string) bool {
	repo := favaaniRepoOf(bc)
	if repo == nil {
		bc.Reply("خطای سیستمی.")
		return true
	}
	botID, _ := uuid.Parse(bc.BotID)

	// Handle referral
	if payload != "" {
		inviterID, err := strconv.ParseInt(payload, 10, 64)
		if err == nil && inviterID != bc.UserID {
			_, _ = repo.EnsureUser(context.Background(), botID, bc.UserID, "")
			// Give bonus to inviter
			_ = repo.SubmitChallenge(context.Background(), botID, inviterID, 0, 0, "referral")
		}
	}

	u, err := repo.EnsureUser(context.Background(), botID, bc.UserID, "")
	if err != nil {
		bc.Reply("خطا.")
		return true
	}

	welcome := bc.Text("welcome")
	welcome = strings.ReplaceAll(welcome, "{score}", strconv.Itoa(u.Score))
	welcome = strings.ReplaceAll(welcome, "{streak}", strconv.Itoa(u.Streak))
	welcome = strings.ReplaceAll(welcome, "{day}", strconv.Itoa(u.Day))

	bc.ReplyInline(welcome, [][]engine.InlineButton{
		{{Label: "🎯 چالش امروز", Callback: "act:challenge"}},
		{{Label: "📊 آمار", Callback: "act:stats"}},
		{{Label: "🔗 دعوت دوستان", Callback: "act:invite"}},
	})
	return true
}

func (t *Template) HandleMessage(bc *engine.BotContext, text string) bool {
	repo := favaaniRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)

	// If the text is a number, treat it as challenge submission
	if amount, err := strconv.ParseInt(strings.ReplaceAll(text, ",", ""), 10, 64); err == nil && amount > 0 {
		t.doLogSpend(bc, repo, botID, amount)
		return true
	}

	switch text {
	case "/start":
		return t.Start(bc, "")
	case "/stats":
		t.showStats(bc, repo, botID)
	case "/invite":
		t.showInvite(bc)
	default:
		bc.Reply(bc.Text("unknown"))
	}
	return true
}

func (t *Template) HandleAction(bc *engine.BotContext, action string) bool {
	repo := favaaniRepoOf(bc)
	if repo == nil {
		return false
	}
	botID, _ := uuid.Parse(bc.BotID)
	switch action {
	case "act:challenge":
		t.showChallenge(bc, repo, botID)
	case "act:stats":
		t.showStats(bc, repo, botID)
	case "act:invite":
		t.showInvite(bc)
	}
	return true
}

func (t *Template) showChallenge(bc *engine.BotContext, repo *postgres.FavaaniRepo, botID uuid.UUID) {
	challenges := bc.Prices["challenges"].([]any)
	if len(challenges) == 0 {
		bc.Reply("چالشی موجود نیست.")
		return
	}
	challengeText := challenges[0].(string)
	msg := bc.Text("challenge")
	msg = strings.ReplaceAll(msg, "{challenge_text}", challengeText)
	bc.Reply(msg)
}

func (t *Template) doLogSpend(bc *engine.BotContext, repo *postgres.FavaaniRepo, botID uuid.UUID, amount int64) {
	u, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	if u == nil {
		bc.Reply("خطا.")
		return
	}
	if err := repo.SubmitChallenge(context.Background(), botID, bc.UserID, u.Day, amount, fmt.Sprintf("%d toman", amount)); err != nil {
		bc.Reply("خطا.")
		return
	}
	u2, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	msg := bc.Text("challenge_logged")
	msg = strings.ReplaceAll(msg, "{amount}", strconv.FormatInt(amount, 10))
	msg = strings.ReplaceAll(msg, "{score}", strconv.Itoa(u2.Score))
	msg = strings.ReplaceAll(msg, "{streak}", strconv.Itoa(u2.Streak))
	bc.ReplyInline(msg, [][]engine.InlineButton{
		{{Label: "🎯 چالش بعدی", Callback: "act:challenge"}},
		{{Label: "📊 آمار", Callback: "act:stats"}},
	})
}

func (t *Template) showStats(bc *engine.BotContext, repo *postgres.FavaaniRepo, botID uuid.UUID) {
	u, _ := repo.GetUser(context.Background(), botID, bc.UserID)
	if u == nil {
		bc.Reply("خطا.")
		return
	}
	msg := bc.Text("stats")
	msg = strings.ReplaceAll(msg, "{score}", strconv.Itoa(u.Score))
	msg = strings.ReplaceAll(msg, "{streak}", strconv.Itoa(u.Streak))
	msg = strings.ReplaceAll(msg, "{day}", strconv.Itoa(u.Day))
	msg = strings.ReplaceAll(msg, "{total_spent}", strconv.FormatInt(u.TotalSpent, 10))
	bc.Reply(msg)
}

func (t *Template) showInvite(bc *engine.BotContext) {
	bonus := bc.PriceInt("referral_bonus_score", 5)
	link := fmt.Sprintf("https://t.me/bot?start=%d", bc.UserID)
	msg := bc.Text("invite")
	msg = strings.ReplaceAll(msg, "{link}", link)
	msg = strings.ReplaceAll(msg, "{bonus}", strconv.Itoa(bonus))
	bc.Reply(msg)
}
