package dating

import (
	"context"
	"strings"
	"testing"

	"sadbot/internal/domain"
)

// TestSupportTicketRejectsLabelPress: a stale screen's button must never become
// a support ticket. On Kat every reply-keyboard press arrives as a callback
// whose data IS the label, so the label routed through the typed-text path and
// was stored verbatim — live tickets read «📞 پشتیبانی» and «👤 مشخصات من».
func TestSupportTicketRejectsLabelPress(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	e.feed(t, e.sendA, "📩 پشتیبانی")

	// stale NON-home labels from other screens, arriving as Kat label
	// callbacks (home labels are navigated by the central guard and never
	// reach the support handler)
	e.feedCB(t, e.sendA, "👤 مشخصات من")
	e.feedCB(t, e.sendA, "❤️ لایک (۲ سکه)")
	e.feedCB(t, e.sendA, "❌ لغو جستجو")

	pA, _ := e.store.ProfileByTelegram(context.Background(), e.botID, "", 111)
	tickets, _, err := e.mod.ListTickets(context.Background(), e.botID, domain.TicketOpen, 1, 50)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	for _, tk := range tickets {
		if tk.UserID != pA.UserID {
			continue
		}
		t.Fatalf("a button label became a ticket: %q", tk.Message)
	}

	// the last reply must be the prompt asking for a real message
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "تایپ کن") {
		t.Fatalf("label press must be refused with a prompt, got %#v", msg)
	}

	// real typed text still files a ticket
	e.feed(t, e.sendA, "پیام واقعی پشتیبانی من")
	tickets, _, err = e.mod.ListTickets(context.Background(), e.botID, domain.TicketOpen, 1, 50)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	found := false
	for _, tk := range tickets {
		if tk.UserID == pA.UserID && strings.Contains(tk.Message, "پیام واقعی") {
			found = true
		}
	}
	if !found {
		t.Fatalf("typed message must still be filed: %+v", tickets)
	}
}
