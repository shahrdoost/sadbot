package dating

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/service"

	"github.com/google/uuid"
)

// TestGhostChatFlow: a lone searcher is paired with a scripted ghost when
// injection is forced; ghost replies replace forwarding; the chat auto-ends
// after the message budget without a rating step.
func TestGhostChatFlow(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.prices = map[string]any{
		"ghost_force":           float64(1),
		"ghost_delay_seconds":   float64(0),
		"ghost_pool_size":       float64(2),
		"ghost_max_messages":    float64(3),
		"ghost_reply_delay_ms":  float64(1),
		"ghost_reply_jitter_ms": float64(1),
	}

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.feed(t, e.sendA, "🔗 به یک ناشناس وصلم کن")
	e.feed(t, e.sendA, "🔗 غریبه")

	m := NewMatcher(e.store, staticBots{e.botID}, func(botID uuid.UUID, chatID int64, text string, kbd json.RawMessage) error {
		if chatID == 111 {
			e.sendA.sent = append(e.sendA.sent, sentMsg{ChatID: chatID, Text: text, ReplyMark: kbd})
		}
		return nil
	})
	m.SetPricesFunc(func(uuid.UUID) map[string]any { return e.prices })
	m.injectGhosts(ctx, e.botID)

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Status != domain.DatingChatting || pA.PartnerUserID == nil {
		t.Fatalf("lone searcher must be paired with a ghost: %+v", pA)
	}
	gp, _ := e.store.Profile(ctx, e.botID, *pA.PartnerUserID)
	if !gp.IsGhost {
		t.Fatal("partner must be a ghost")
	}
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "وصلت انجام شد") {
		t.Fatalf("human did not get match message: %+v", e.sendA.sent)
	}

	// human messages get scripted ghost replies (not forwarded anywhere)
	for i := 0; i < 3; i++ {
		e.feed(t, e.sendA, "سلام، حالت چطوره؟")
		time.Sleep(50 * time.Millisecond) // async ghost reply + home transition
	}
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Status != domain.DatingJoined {
		t.Fatalf("ghost chat must end after budget: %s", pA.Status)
	}
	if pA.PartnerUserID != nil {
		t.Fatal("partner must be cleared")
	}
	// no rating burden for ghost chats
	pending, _ := e.store.Repo().GetPendingRate(ctx, e.botID, pA.UserID)
	if pending != nil {
		t.Fatalf("ghost chat must not register pending rate: %+v", pending)
	}
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "منوی اصلی") {
		t.Fatalf("human must land home after ghost chat: %+v", e.sendA.sent)
	}
}

// TestConfessionFlow: submit with confirm spends coins and lands pending;
// admin approve notifies the user; reject notifies too.
func TestConfessionFlow(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	e.feed(t, e.sendA, "💌 اعتراف ناشناس")
	e.feed(t, e.sendA, "یه اعتراف آزمایشی برای تست سیستم")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "پیش‌نمایش") {
		t.Fatalf("preview expected: %+v", e.sendA.sent)
	}
	e.feed(t, e.sendA, "✅ ثبت اعتراف")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Coins != 3 { // 5 signup - 2 confess
		t.Fatalf("confession must cost 2 coins, got %d", pA.Coins)
	}
	confessions, total, err := e.mod.ListConfessions(ctx, e.botID, domain.ConfessionPending, 1, 20)
	if err != nil || total != 1 || len(confessions) != 1 {
		t.Fatalf("pending confession expected: total=%d err=%v", total, err)
	}
	if confessions[0].Text != "یه اعتراف آزمایشی برای تست سیستم" {
		t.Fatalf("confession text = %q", confessions[0].Text)
	}

	// admin service: approve with fake channel + user notify
	adminID := uuid.New()
	var notified []string
	svc := service.NewModerationService(e.mod,
		func(ctx context.Context, botID uuid.UUID, userID int64, text string) error {
			notified = append(notified, text)
			return nil
		},
		func(ctx context.Context, botID uuid.UUID, text string) (int64, error) { return 0, nil },
	)
	if err := svc.ApproveConfession(ctx, e.botID, confessions[0].ID, adminID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if len(notified) != 1 || !contains(notified[0], "تایید شد") {
		t.Fatalf("user must be notified of approval: %v", notified)
	}
	_, totalPending, _ := e.mod.ListConfessions(ctx, e.botID, domain.ConfessionPending, 1, 20)
	if totalPending != 0 {
		t.Fatalf("pending must be empty after approve, got %d", totalPending)
	}
}

// TestSupportTicketFlow: user message creates a ticket; admin reply marks it
// answered and notifies the user.
func TestSupportTicketFlow(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	e.feed(t, e.sendA, "📩 پشتیبانی")
	e.feed(t, e.sendA, "سلام، یه مشکل دارم با خرید سکه")
	tickets, total, err := e.mod.ListTickets(ctx, e.botID, domain.TicketOpen, 1, 20)
	if err != nil || total != 1 {
		t.Fatalf("open ticket expected: total=%d err=%v", total, err)
	}

	var notified []string
	svc := service.NewModerationService(e.mod,
		func(ctx context.Context, botID uuid.UUID, userID int64, text string) error {
			notified = append(notified, text)
			return nil
		}, nil)
	if err := svc.ReplyTicket(ctx, e.botID, tickets[0].ID, uuid.New(), "سلام؛ بررسی می‌کنیم."); err != nil {
		t.Fatalf("reply: %v", err)
	}
	if len(notified) != 1 || !contains(notified[0], "پشتیبانی") {
		t.Fatalf("user must get the reply: %v", notified)
	}
	answered, _, _ := e.mod.ListTickets(ctx, e.botID, domain.TicketAnswered, 1, 20)
	if len(answered) != 1 {
		t.Fatalf("ticket must move to answered: %+v", answered)
	}
}
