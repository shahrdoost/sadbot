package dating

import (
	"context"
	"encoding/json"
	"testing"

	"sadbot/internal/domain"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// fakeRouter implements engine.Router: records cross-platform sends and
// delivers them to the fake sender of the destination platform.
type fakeRouter struct {
	tg, kat  *fakeSender
	sends    []routedSend
	forwards []routedForward
}

type routedSend struct {
	Platform string
	ChatID   int64
	Text     string
}

type routedForward struct {
	Platform  string
	ToChatID  int64
	FromChat  int64
	MessageID int64
}

func (r *fakeRouter) SendPlatform(ctx context.Context, botID uuid.UUID, platform string, chatID int64, text string, kbd json.RawMessage, replyTo int64) (*telegram.Message, error) {
	r.sends = append(r.sends, routedSend{Platform: platform, ChatID: chatID, Text: text})
	dest := r.tg
	if platform == domain.PlatformKat {
		dest = r.kat
	}
	return dest.Send(ctx, chatID, text, kbd, replyTo)
}

func (r *fakeRouter) ForwardMediaPlatform(ctx context.Context, botID uuid.UUID, platform string, fromChatID, toChatID, messageID int64) (*telegram.Message, error) {
	r.forwards = append(r.forwards, routedForward{Platform: platform, ToChatID: toChatID, FromChat: fromChatID, MessageID: messageID})
	dest := r.tg
	if platform == domain.PlatformKat {
		dest = r.kat
	}
	return dest.ForwardMedia(ctx, fromChatID, toChatID, messageID)
}

func (r *fakeRouter) lastTo(platform string, chatID int64) *routedSend {
	for i := len(r.sends) - 1; i >= 0; i-- {
		if r.sends[i].Platform == platform && r.sends[i].ChatID == chatID {
			return &r.sends[i]
		}
	}
	return nil
}

// TestCrossPlatformChatRelay: user A lives on Telegram, user B on Kat. The bot,
// templates and data are shared; every delivery to the partner must leave
// through the router and land on the partner's platform, never on the sender's.
func TestCrossPlatformChatRelay(t *testing.T) {
	e := newBrowseEnv(t)
	e.platB = domain.PlatformKat
	router := &fakeRouter{tg: e.sendA, kat: e.sendB}
	e.eng.SetRouter(router)
	ctx := context.Background()

	connectPair(t, e)

	// separate users: same numeric tid on two platforms would be two rows
	pA, err := e.store.ProfileByTelegram(ctx, e.botID, domain.PlatformTelegram, 111)
	if err != nil || pA == nil {
		t.Fatalf("profile A (telegram): %v", err)
	}
	pB, err := e.store.ProfileByTelegram(ctx, e.botID, domain.PlatformKat, 222)
	if err != nil || pB == nil {
		t.Fatalf("profile B (kat): %v", err)
	}
	if pA.Platform != domain.PlatformTelegram || pB.Platform != domain.PlatformKat {
		t.Fatalf("platforms not denormalized: A=%q B=%q", pA.Platform, pB.Platform)
	}
	if pA.PartnerUserID == nil || *pA.PartnerUserID != pB.UserID {
		t.Fatalf("A partner wrong: %+v", pA.PartnerUserID)
	}

	// the router is the only channel between platforms
	router.sends = nil
	router.forwards = nil
	sentA, sentB := len(e.sendA.sent), len(e.sendB.sent)

	// --- A (telegram) chats → B must get it on kat ---
	e.feed(t, e.sendA, "سلام!")
	if got := router.lastTo(domain.PlatformKat, 222); got == nil || !contains(got.Text, "سلام") {
		t.Fatalf("telegram→kat relay missing from router: %+v", router.sends)
	}
	if msg := e.sendB.lastTo(222); msg == nil || !contains(msg.Text, "سلام") {
		t.Fatalf("B (kat) did not receive A's message: %+v", e.sendB.sent[sentB:])
	}
	// nothing may leak onto the sender's own platform-side sender
	for _, m := range e.sendA.sent[sentA:] {
		if m.ChatID == 222 {
			t.Fatalf("relay leaked to telegram sender: %+v", m)
		}
	}

	// --- B (kat) replies → A must get it on telegram ---
	router.sends = nil
	e.feed(t, e.sendB, "حالت خوبه؟")
	if got := router.lastTo(domain.PlatformTelegram, 111); got == nil || !contains(got.Text, "حالت خوبه") {
		t.Fatalf("kat→telegram relay missing from router: %+v", router.sends)
	}
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "حالت خوبه") {
		t.Fatalf("A (telegram) did not receive B's reply: %+v", e.sendA.sent)
	}

	// --- media rides the same cross-platform path ---
	router.forwards = nil
	e.feedMedia(t, e.sendA, &telegram.Message{
		MessageID: 777,
		Photo:     []telegram.PhotoSize{{FileID: "photo-x", Width: 90, Height: 90}},
	})
	if len(router.forwards) != 1 {
		t.Fatalf("expected 1 routed forward, got %+v", router.forwards)
	}
	f := router.forwards[0]
	if f.Platform != domain.PlatformKat || f.ToChatID != 222 || f.FromChat != 111 || f.MessageID != 777 {
		t.Fatalf("forward routed wrong: %+v", f)
	}

	// --- A ends the chat → rating prompt must reach B on kat ---
	router.sends = nil
	e.feed(t, e.sendA, "🛑 قطع مکالمه")
	if got := router.lastTo(domain.PlatformKat, 222); got == nil || !contains(got.Text, "امتیاز") {
		t.Fatalf("rating prompt not routed to kat: %+v", router.sends)
	}

	// --- B (kat) rates → applied to A's profile ---
	e.feed(t, e.sendB, "⭐⭐⭐⭐⭐")
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, domain.PlatformTelegram, 111)
	if pA.TotalRatings != 1 || pA.RatingSum != 5 {
		t.Fatalf("rating not applied: %+v", pA)
	}
}

// TestCrossPlatformMediaDegrades: Kat's Bot API subset has no forwardMessage.
// A media relay to a Kat partner must show the degrade notice and keep the
// chat alive instead of force-ending it as "partner unreachable".
func TestCrossPlatformMediaDegrades(t *testing.T) {
	e := newBrowseEnv(t)
	e.platB = domain.PlatformKat
	router := &fakeRouter{tg: e.sendA, kat: e.sendB}
	e.eng.SetRouter(router)
	ctx := context.Background()

	connectPair(t, e)
	e.sendB.forwardErr = telegram.ErrUnsupported

	e.feedMedia(t, e.sendA, &telegram.Message{
		MessageID: 901,
		Photo:     []telegram.PhotoSize{{FileID: "photo-d", Width: 90, Height: 90}},
	})

	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "فقط متن") {
		t.Fatalf("A must get the degrade notice, got: %+v", e.sendA.sent)
	}
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, domain.PlatformTelegram, 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, domain.PlatformKat, 222)
	if pA.Status != domain.DatingChatting || pB.Status != domain.DatingChatting {
		t.Fatalf("chat must survive media degrade: A=%s B=%s", pA.Status, pB.Status)
	}
}

// TestPlatformScopedIdentity: the same numeric telegram_id on Telegram and Kat
// must resolve to two distinct end users.
func TestPlatformScopedIdentity(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	tgID, err := e.users.Upsert(ctx, e.botID.String(), domain.PlatformTelegram, 900, nil, nil, nil)
	if err != nil {
		t.Fatalf("upsert tg: %v", err)
	}
	katID, err := e.users.Upsert(ctx, e.botID.String(), domain.PlatformKat, 900, nil, nil, nil)
	if err != nil {
		t.Fatalf("upsert kat: %v", err)
	}
	if tgID == katID {
		t.Fatalf("same id %d for two platforms", tgID)
	}

	// re-upsert is idempotent per platform
	again, err := e.users.Upsert(ctx, e.botID.String(), domain.PlatformKat, 900, nil, nil, nil)
	if err != nil || again != katID {
		t.Fatalf("kat re-upsert not idempotent: %d vs %d err=%v", again, katID, err)
	}

	if _, err := e.store.EnsureProfile(ctx, e.botID, tgID, domain.PlatformTelegram, 900); err != nil {
		t.Fatalf("ensure tg profile: %v", err)
	}
	if _, err := e.store.EnsureProfile(ctx, e.botID, katID, domain.PlatformKat, 900); err != nil {
		t.Fatalf("ensure kat profile: %v", err)
	}
	pTg, err := e.store.ProfileByTelegram(ctx, e.botID, domain.PlatformTelegram, 900)
	if err != nil || pTg.UserID != tgID {
		t.Fatalf("telegram lookup wrong: %+v err=%v", pTg, err)
	}
	pKat, err := e.store.ProfileByTelegram(ctx, e.botID, domain.PlatformKat, 900)
	if err != nil || pKat.UserID != katID {
		t.Fatalf("kat lookup wrong: %+v err=%v", pKat, err)
	}
	pAny, err := e.store.ProfileByTelegram(ctx, e.botID, "", 900)
	if err != nil || pAny == nil {
		t.Fatalf("any-platform lookup failed: %v", err)
	}
}
