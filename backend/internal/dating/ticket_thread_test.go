package dating

import (
	"context"
	"strings"
	"testing"

	"sadbot/internal/domain"
)

// TestTicketThreadLoop: an admin reply must not be a dead end. The user
// continues the same ticket from the reply's button, the ticket returns to the
// admins' queue, and the thread keeps every turn in order. «بستن تیکت» closes
// it and the next message starts a fresh ticket instead of reviving the old one.
func TestTicketThreadLoop(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)

	// 1. the user opens a ticket
	e.feed(t, e.sendA, "📩 پشتیبانی")
	e.feed(t, e.sendA, "مشکل اول من")
	tickets, _, err := e.mod.ListTickets(ctx, e.botID, domain.TicketOpen, 1, 50)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 1 || tickets[0].UserID != pA.UserID {
		t.Fatalf("expected one open ticket for A, got %+v", tickets)
	}
	ticketID := tickets[0].ID

	// 2. the admin answers — the repo records it as a thread turn
	if _, err := e.mod.ReplyTicket(ctx, e.botID, ticketID, "بررسی کردیم؛ لطفاً بیشتر توضیح بده."); err != nil {
		t.Fatalf("reply: %v", err)
	}
	thread, err := e.mod.TicketMessages(ctx, ticketID)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if len(thread) != 1 || thread[0].Sender != "admin" || !strings.Contains(thread[0].Body, "بررسی کردیم") {
		t.Fatalf("admin reply must be the first thread turn, got %+v", thread)
	}

	// 3. the reply's «ادامهٔ گفتگو» button opens the same ticket
	e.feedCB(t, e.sendA, "act:support:continue:"+itoa(ticketID))
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "ادامهٔ گفتگو") {
		t.Fatalf("continue button must prompt for an answer, got %#v", msg)
	}

	// 4. the user's answer lands in the same ticket, which goes back to open
	e.feed(t, e.sendA, "جزئیات بیشتر دربارهٔ مشکل")
	thread, err = e.mod.TicketMessages(ctx, ticketID)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if len(thread) != 2 {
		t.Fatalf("user answer must append to the thread, got %+v", thread)
	}
	if thread[1].Sender != "user" || !strings.Contains(thread[1].Body, "جزئیات بیشتر") {
		t.Fatalf("second turn must be the user's, got %+v", thread[1])
	}
	open, _, err := e.mod.ListTickets(ctx, e.botID, domain.TicketOpen, 1, 50)
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 1 || open[0].ID != ticketID {
		t.Fatalf("ticket must return to the admins' queue, got %+v", open)
	}
	// the reply's buttons are still functional against a re-opened ticket

	// 5. «بستن تیکت» closes it
	e.feedCB(t, e.sendA, "act:support:close:"+itoa(ticketID))
	closed, _, err := e.mod.ListTickets(ctx, e.botID, domain.TicketClosed, 1, 50)
	if err != nil {
		t.Fatalf("list closed: %v", err)
	}
	if len(closed) != 1 || closed[0].ID != ticketID {
		t.Fatalf("ticket must be closed, got %+v", closed)
	}

	// 6. a message after closing opens a NEW ticket, not the closed one
	e.feed(t, e.sendA, "📩 پشتیبانی")
	e.feed(t, e.sendA, "موضوع تازه")
	open, _, err = e.mod.ListTickets(ctx, e.botID, domain.TicketOpen, 1, 50)
	if err != nil {
		t.Fatalf("list open after close: %v", err)
	}
	if len(open) != 1 || open[0].ID == ticketID {
		t.Fatalf("a closed ticket must not be revived, got %+v", open)
	}
	if !strings.Contains(open[0].Message, "موضوع تازه") {
		t.Fatalf("the new ticket must carry the new message, got %+v", open[0])
	}
}

// TestSupportSceneContinuesOpenTicket: pressing «📩 پشتیبانی» while a ticket is
// open must offer to continue it instead of silently opening a second one.
func TestSupportSceneContinuesOpenTicket(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	e.feed(t, e.sendB, "📩 پشتیبانی")
	e.feed(t, e.sendB, "اولین پیام")
	if msg := e.sendB.lastTo(222); msg == nil || !strings.Contains(msg.Text, "ثبت شد") {
		t.Fatalf("first message must open a ticket, got %#v", msg)
	}

	// visiting support again finds the open ticket and says so
	e.feed(t, e.sendB, "📩 پشتیبانی")
	if msg := e.sendB.lastTo(222); msg == nil || !strings.Contains(msg.Text, "تیکت باز") {
		t.Fatalf("open ticket must be announced, got %#v", msg)
	}

	// typing still goes into that ticket, not a new one
	e.feed(t, e.sendB, "دومین پیام")
	tickets, total, err := e.mod.ListTickets(ctx, e.botID, domain.TicketOpen, 1, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 {
		t.Fatalf("must stay one ticket, got %d: %+v", total, tickets)
	}
	thread, err := e.mod.TicketMessages(ctx, tickets[0].ID)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if len(thread) != 1 || !strings.Contains(thread[0].Body, "دومین پیام") {
		t.Fatalf("second message must append to the open ticket, got %+v", thread)
	}

	// «تیکت جدید» opts into a separate ticket
	e.feedCB(t, e.sendB, "act:support:new")
	e.feed(t, e.sendB, "موضوع جدا")
	_, total, err = e.mod.ListTickets(ctx, e.botID, domain.TicketOpen, 1, 50)
	if err != nil {
		t.Fatalf("list after new: %v", err)
	}
	if total != 2 {
		t.Fatalf("«تیکت جدید» must open a second ticket, got %d", total)
	}
}

// TestTicketContinueRejectsClosed: a stale «ادامه» button on a closed ticket
// must not revive it.
func TestTicketContinueRejectsClosed(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	e.feed(t, e.sendA, "📩 پشتیبانی")
	e.feed(t, e.sendA, "تیکت که بسته می‌شود")
	tickets, _, _ := e.mod.ListTickets(ctx, e.botID, domain.TicketOpen, 1, 50)
	id := tickets[0].ID
	if ok, err := e.mod.CloseTicket(ctx, e.botID, id); err != nil || !ok {
		t.Fatalf("close: ok=%v err=%v", ok, err)
	}

	e.feedCB(t, e.sendA, "act:support:continue:"+itoa(id))
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "بسته شده") {
		t.Fatalf("closed ticket must be refused, got %#v", msg)
	}
	e.feed(t, e.sendA, "این نباید به تیکت بسته اضافه شود")
	thread, _ := e.mod.TicketMessages(ctx, id)
	for _, m := range thread {
		if strings.Contains(m.Body, "نباید") {
			t.Fatalf("closed ticket accepted a message: %+v", thread)
		}
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
