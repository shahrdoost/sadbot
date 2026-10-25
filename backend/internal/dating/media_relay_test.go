package dating

import (
	"context"
	"encoding/json"
	"testing"

	"sadbot/internal/domain"
	"sadbot/internal/telegram"
)

// feedMedia sends an arbitrary message (photo/sticker/voice…) from a user,
// the way Telegram would deliver it to the bot.
func (e *browseEnv) feedMedia(t *testing.T, sender *fakeSender, msg *telegram.Message) {
	t.Helper()
	tid := e.tid(sender)
	ctx := context.Background()
	userID, err := e.users.Upsert(ctx, e.botID.String(), tid, nil, nil, nil)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	_, _ = e.store.EnsureProfile(ctx, e.botID, userID, tid)
	msg.From = &telegram.User{ID: tid}
	msg.Chat = telegram.Chat{ID: tid, Type: "private"}
	e.eng.HandleUpdate(ctx, &domain.Bot{ID: e.botID, TemplateKey: domain.TemplateDating}, sender,
		&telegram.Update{Message: msg})
}

// connectPair runs both users through onboarding and mutual-likes so they end
// up in the chat scene together.
func connectPair(t *testing.T, e *browseEnv) {
	t.Helper()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendA, "act:browse:like")
	e.feed(t, e.sendB, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendB, "act:browse:like")

	ctx := context.Background()
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, 222)
	if pA.Status != domain.DatingChatting || pB.Status != domain.DatingChatting {
		t.Fatalf("precondition: both must be chatting, got %s / %s", pA.Status, pB.Status)
	}
}

// TestChatMediaRelay: a photo sent inside the chat scene is copied to the
// partner and shows up in the panel transcript as a media entry.
func TestChatMediaRelay(t *testing.T) {
	e := newBrowseEnv(t)
	connectPair(t, e)
	ctx := context.Background()

	e.feedMedia(t, e.sendA, &telegram.Message{
		MessageID: 777,
		Photo:     []telegram.PhotoSize{{FileID: "photo-1", Width: 90, Height: 90}},
	})

	if len(e.sendA.forwards) != 1 {
		t.Fatalf("expected exactly one forwarded media, got %d: %+v", len(e.sendA.forwards), e.sendA.forwards)
	}
	fwd := e.sendA.forwards[0]
	if fwd.ToChatID != 222 {
		t.Fatalf("media must go to the partner's chat (222), got %d", fwd.ToChatID)
	}
	if fwd.FromChatID != 111 {
		t.Fatalf("media must be forwarded from the sender's chat (111), got %d", fwd.FromChatID)
	}
	if fwd.MessageID != 777 {
		t.Fatalf("forward must carry the original message id, got %d", fwd.MessageID)
	}

	// the chat must still be alive (media doesn't end it)
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Status != domain.DatingChatting {
		t.Fatalf("media relay must not end the chat, status=%s", pA.Status)
	}

	// transcript must record the media for the panel
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, 222)
	rows, err := e.store.Repo().ChatTranscript(ctx, e.botID, pA.UserID, pB.UserID)
	if err != nil {
		t.Fatalf("transcript: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 transcript row for the photo, got %d: %+v", len(rows), rows)
	}
	if rows[0].Text == nil || !contains(*rows[0].Text, "عکس") {
		t.Fatalf("transcript must label the photo, got %+v", rows[0])
	}
	if !rows[0].IsUserA {
		t.Fatal("media must be attributed to the sender")
	}
}

// TestChatMediaRelay_StickerAndVoice: every media kind rides the same forward.
func TestChatMediaRelay_StickerAndVoice(t *testing.T) {
	e := newBrowseEnv(t)
	connectPair(t, e)
	ctx := context.Background()

	e.feedMedia(t, e.sendA, &telegram.Message{
		MessageID: 10,
		Sticker:   json.RawMessage(`{"file_id":"st-1","file_unique_id":"u1"}`),
	})
	e.feedMedia(t, e.sendB, &telegram.Message{
		MessageID: 11,
		Voice:     json.RawMessage(`{"file_id":"vo-1","file_unique_id":"u2"}`),
	})

	if len(e.sendA.forwards) != 1 || len(e.sendB.forwards) != 1 {
		t.Fatalf("both media must be forwarded (one per sender): A=%d B=%d",
			len(e.sendA.forwards), len(e.sendB.forwards))
	}
	// A's sticker → B (222); B's voice → A (111)
	if e.sendA.forwards[0].ToChatID != 222 {
		t.Fatalf("A's sticker must reach 222, got %d", e.sendA.forwards[0].ToChatID)
	}
	if e.sendB.forwards[0].ToChatID != 111 {
		t.Fatalf("B's voice must reach 111, got %d", e.sendB.forwards[0].ToChatID)
	}

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, 222)
	rows, _ := e.store.Repo().ChatTranscript(ctx, e.botID, pA.UserID, pB.UserID)
	if len(rows) != 2 {
		t.Fatalf("both media must land in the transcript, got %d", len(rows))
	}
	if !contains(*rows[0].Text, "استیکر") {
		t.Fatalf("first media must be the sticker: %s", *rows[0].Text)
	}
	if !contains(*rows[1].Text, "صوتی") {
		t.Fatalf("second media must be the voice note: %s", *rows[1].Text)
	}
}

// TestChatMediaRelay_RequiresActiveChat: media sent outside a live chat must
// not be forwarded anywhere.
func TestChatMediaRelay_RequiresActiveChat(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد") // ends onboarding; user is home, not chatting

	e.feedMedia(t, e.sendA, &telegram.Message{
		MessageID: 5,
		Photo:     []telegram.PhotoSize{{FileID: "p"}},
	})

	if len(e.sendA.forwards) != 0 {
		t.Fatalf("media outside a chat must not be forwarded: %+v", e.sendA.forwards)
	}
}

// TestMediaTypeDetection: the kind used for the transcript label must match
// the payload Telegram sent.
func TestMediaTypeDetection(t *testing.T) {
	cases := []struct {
		name string
		msg  telegram.Message
		want string
	}{
		{"photo", telegram.Message{Photo: []telegram.PhotoSize{{FileID: "x"}}}, "photo"},
		{"sticker", telegram.Message{Sticker: json.RawMessage(`{}`)}, "sticker"},
		{"voice", telegram.Message{Voice: json.RawMessage(`{}`)}, "voice"},
		{"video_note", telegram.Message{VideoNote: json.RawMessage(`{}`)}, "video_note"},
		{"video", telegram.Message{Video: json.RawMessage(`{}`)}, "video"},
		{"animation", telegram.Message{Animation: json.RawMessage(`{}`)}, "animation"},
		{"audio", telegram.Message{Audio: json.RawMessage(`{}`)}, "audio"},
		{"document", telegram.Message{Document: json.RawMessage(`{}`)}, "document"},
		{"text", telegram.Message{Text: "سلام"}, ""},
	}
	for _, c := range cases {
		if got := c.msg.MediaType(); got != c.want {
			t.Fatalf("MediaType(%s) = %q, want %q", c.name, got, c.want)
		}
		if c.want != "" && c.msg.MediaTypeLabel() == "" {
			t.Fatalf("MediaTypeLabel(%s) must not be empty", c.name)
		}
	}
}
