package dating

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"time"

	"sadbot/internal/engine"

	"github.com/google/uuid"
)

// Notification sweep: every 30 minutes, send safe capped re-engagement
// prompts. No LLM; every constraint is per-bot configurable and additionally
// has a conservative hard maximum (2/day, 8h gap) that cannot be relaxed.

type NotificationService struct {
	store    *Store
	bots     BotLister
	notify   Notifier
	pricesOf func(uuid.UUID) map[string]any
	interval time.Duration
}

func NewNotificationService(store *Store, bots BotLister, notify Notifier, pricesOf func(uuid.UUID) map[string]any) *NotificationService {
	return &NotificationService{store: store, bots: bots, notify: notify, pricesOf: pricesOf, interval: 30 * time.Minute}
}

func (n *NotificationService) Run(ctx context.Context) {
	n.sweep(ctx)
	ticker := time.NewTicker(n.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.sweep(ctx)
		}
	}
}

func (n *NotificationService) sweep(ctx context.Context) {
	botIDs, err := n.bots.ListRunningByTemplate(ctx, "dating")
	if err != nil {
		log.Printf("[notifications] list bots: %v", err)
		return
	}
	for _, botID := range botIDs {
		n.sendBot(ctx, botID)
	}
}

func (n *NotificationService) sendBot(ctx context.Context, botID uuid.UUID) {
	prices := map[string]any(nil)
	if n.pricesOf != nil {
		prices = n.pricesOf(botID)
	}
	if priceInt(prices, "notify_enabled", 1) == 0 || inQuietHours(time.Now(), prices) {
		return
	}
	cap := priceInt(prices, "notify_daily_cap", 2)
	if cap > 2 {
		cap = 2
	}
	gap := priceInt(prices, "notify_min_gap_hours", 8)
	if gap < 8 {
		gap = 8
	}
	rows, err := n.store.Repo().NotifyCandidates(ctx, botID, cap, gap, 20)
	if err != nil {
		log.Printf("[notifications] candidates %s: %v", botID, err)
		return
	}
	var sent []int64
	for _, user := range rows {
		text := notificationText()
		if err := n.notify(botID, user.TelegramID, text, notificationKeyboard()); err != nil {
			log.Printf("[notifications] send %d: %v", user.TelegramID, err)
			continue
		}
		sent = append(sent, user.UserID)
	}
	if len(sent) > 0 {
		if err := n.store.Repo().MarkNotified(ctx, botID, sent); err != nil {
			log.Printf("[notifications] mark %s: %v", botID, err)
		}
	}
}

func inQuietHours(now time.Time, prices map[string]any) bool {
	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		loc = time.FixedZone("IRST", 3*3600+30*60)
	}
	hour := now.In(loc).Hour()
	start := priceInt(prices, "notify_quiet_start", 22)
	end := priceInt(prices, "notify_quiet_end", 9)
	if start == end {
		return false
	}
	if start < end {
		return hour >= start && hour < end
	}
	return hour >= start || hour < end
}

var notificationMessages = []string{
	"💌 یه سر به ربات بزن؛ شاید آدم جدیدی منتظر آشنایی باشه!",
	"👋 دلت برای گپ تنگ نشده؟ آدم‌های جدید منتظرن.",
	"✨ هنوز فرصت آشنایی با آدم‌های تازه هست. بیا ببین چه خبره!",
}

func notificationText() string { return notificationMessages[rand.Intn(len(notificationMessages))] }

func notificationKeyboard() json.RawMessage {
	var kbd struct {
		InlineKeyboard [][]struct {
			Text         string `json:"text"`
			CallbackData string `json:"callback_data"`
		} `json:"inline_keyboard"`
	}
	kbd.InlineKeyboard = [][]struct {
		Text         string `json:"text"`
		CallbackData string `json:"callback_data"`
	}{
		{{Text: "💘 مرور پروفایل‌ها", CallbackData: "act:notify:browse"}},
		{{Text: "🔗 جستجوی ناشناس", CallbackData: "act:notify:search"}},
		{{Text: "🔕 دیگه یادآوری نکن", CallbackData: "act:notify:dismiss"}},
	}
	raw, _ := json.Marshal(kbd)
	return raw
}

// handleNotificationAction records dismiss/mute or returns to home.
func handleNotificationAction(bc *engine.BotContext, st *Store, action string) bool {
	ctx := context.Background()
	botID := botUUID(bc)
	switch action {
	case "notify:dismiss", "notify:mute":
		_ = st.Repo().SetNotifyDismissed(ctx, botID, bc.UserID)
		bc.Reply("باشه، دیگه یادآوری نمی‌کنم. برای برگشت به منو /start رو بزن.")
		return true
	case "notify:browse":
		return startBrowse(bc, st)
	case "notify:search":
		bc.ReplyKBDStyled("جستجوی ناشناس\n\nیکی از گزینه‌ها رو بزن:", searchKB)
		return true
	}
	return false
}
