package dating

import (
	"context"
	"testing"
)

// TestChatSurveillance: the engine tags inbound events with the scene they
// arrived in, so the panel can reconstruct a conversation from chat-scene
// messages alone and ignore menu/setup text.
func TestChatSurveillance(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	// A and B both onboard and mutual-like into a chat
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendA, "act:browse:like")
	e.feed(t, e.sendB, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendB, "act:browse:like")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pA.Status != "chatting" || pB.Status != "chatting" {
		t.Fatalf("precondition: both must be chatting, got %s / %s", pA.Status, pB.Status)
	}

	// one conversation message each way, plus a third typed line — the engine
	// logs the scene the message *arrived* in, so chat text is part of the
	// chat-scene record (test 2 proves messages from other scenes stay out).
	// (The home keyboard label is no longer usable here: it now leaves the
	// chat instead of relaying.)
	e.feed(t, e.sendA, "سلام، حالت چطوره؟")
	e.feed(t, e.sendB, "سلام! ممنون، عالی")
	e.feed(t, e.sendA, "فعلاً خداحافظ")

	// --- active chats must show exactly one deduplicated pair ---
	chats, err := e.store.Repo().ActiveChats(ctx, e.botID)
	if err != nil {
		t.Fatalf("ActiveChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("expected exactly 1 active chat (deduplicated), got %d: %+v", len(chats), chats)
	}
	c := chats[0]
	// lower id is user A
	if c.UserAID != pA.UserID || c.UserBID != pB.UserID {
		t.Fatalf("pair not normalized to lower-id-first: A=%d B=%d (pA=%d pB=%d)",
			c.UserAID, c.UserBID, pA.UserID, pB.UserID)
	}

	// --- transcript: both chat messages, oldest first, correct speakers ---
	rows, err := e.store.Repo().ChatTranscript(ctx, e.botID, pA.UserID, pB.UserID)
	if err != nil {
		t.Fatalf("ChatTranscript: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 chat-scene messages, got %d: %+v", len(rows), rows)
	}
	var texts []string
	for _, m := range rows {
		if m.Text != nil {
			texts = append(texts, *m.Text)
		}
	}
	if !contains(texts[0], "حالت") || !contains(texts[1], "ممنون") {
		t.Fatalf("transcript out of order or wrong: %v", texts)
	}

	// direction flags must identify who is speaking
	if !rows[0].IsUserA {
		t.Fatalf("first message must come from A: %+v", rows[0])
	}
	if rows[1].IsUserA {
		t.Fatalf("second message must come from B: %+v", rows[1])
	}

	// swapping the ids must be tolerated (service normalizes, repo accepts any order)
	rowsSwapped, err := e.store.Repo().ChatTranscript(ctx, e.botID, pB.UserID, pA.UserID)
	if err != nil {
		t.Fatalf("ChatTranscript swapped: %v", err)
	}
	if len(rowsSwapped) != len(rows) {
		t.Fatalf("swapped order must return the same messages: %d vs %d", len(rowsSwapped), len(rows))
	}

	// End the chat first: production reads the feed after conversations have
	// finished, and ending clears partner_user_id on both profiles. The feed
	// must still name the recipient of every past message.
	if _, err := e.store.Repo().EndChat(ctx, e.botID, pA.UserID); err != nil {
		t.Fatalf("EndChat A: %v", err)
	}
	if _, err := e.store.Repo().EndChat(ctx, e.botID, pB.UserID); err != nil {
		t.Fatalf("EndChat B: %v", err)
	}

	// --- global recent feed ---
	// one row per typed message: the sender's inbound event, with the partner
	// resolved from their profile (the mirrored outbound rows stay out of the
	// feed so each message is listed once, not twice).
	feed, err := e.store.Repo().RecentChatMessages(ctx, e.botID, 50)
	if err != nil {
		t.Fatalf("RecentChatMessages: %v", err)
	}
	if len(feed) != 3 {
		t.Fatalf("global feed must hold each of the 3 chat messages once, got %d", len(feed))
	}
	for _, m := range feed {
		if m.Kind != "inbound_message" {
			t.Fatalf("feed must only carry inbound rows, got kind %q", m.Kind)
		}
	}
	// newest first: the last thing said was A's farewell, the first was A's line
	if !contains(feed[0].Text, "خداحافظ") {
		t.Fatalf("feed must be newest-first: %+v", feed[0])
	}
	if !contains(feed[len(feed)-1].Text, "حالت") {
		t.Fatalf("feed must end with the oldest message: %+v", feed[len(feed)-1])
	}
	// The recipient must resolve on every row: the panel renders
	// "دریافتی: <sender> - ارسالی: <recipient>", and an empty recipient would
	// blank half the line. Profiles do not help here — ending a chat clears
	// partner_user_id on both — so this must come from the outbound mirror.
	for _, m := range feed {
		if m.PartnerID == nil {
			t.Fatalf("feed row %d (%q) has no recipient", m.EventID, m.Text)
		}
		if m.PartnerName == nil || *m.PartnerName == "" {
			t.Fatalf("feed row %d (%q) has recipient id but no name", m.EventID, m.Text)
		}
	}
	// each row names the other party as recipient, and never itself
	oldest := feed[len(feed)-1]
	if *oldest.PartnerName != "سارا" {
		t.Fatalf("A's message must name B as recipient, got %v", *oldest.PartnerName)
	}
	if *feed[1].PartnerName != "امیر" {
		t.Fatalf("B's reply must name A as recipient, got %v", *feed[1].PartnerName)
	}
	for _, m := range feed {
		if *m.PartnerID == m.UserID {
			t.Fatalf("feed row %d names its own sender as recipient", m.EventID)
		}
	}
}

// TestChatTranscriptIgnoresOtherScenes: messages typed in the setup scene or
// as commands must never leak into a chat transcript.
func TestChatTranscriptIgnoresOtherScenes(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)

	// user never chats; only talks from home + setup
	e.feed(t, e.sendA, "📖 راهنما")
	e.feed(t, e.sendA, "💌 اعتراف ناشناس")

	rows, err := e.store.Repo().ChatTranscript(ctx, e.botID, pA.UserID, pA.UserID)
	if err != nil {
		t.Fatalf("ChatTranscript: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("non-chat messages must not appear in a transcript: %+v", rows)
	}
	feed, err := e.store.Repo().RecentChatMessages(ctx, e.botID, 50)
	if err != nil {
		t.Fatalf("RecentChatMessages: %v", err)
	}
	if len(feed) != 0 {
		t.Fatalf("global feed must be empty when nothing was said in chat: %+v", feed)
	}
}
