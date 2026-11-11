package dating

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/runtime"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// fakeSender records outgoing messages per chat.
type fakeSender struct {
	sent     []sentMsg
	edits    []sentMsg
	forwards []forwardedMedia
	deletes  []forwardedMedia
	// forwardErr, when set, makes ForwardMedia fail (e.g. Kat's missing
	// forwardMessage → telegram.ErrUnsupported).
	forwardErr error
	// sendErr / photoErr make the matching send fail (platform outage, or a
	// Telegram file_id the Kat side cannot render).
	sendErr  error
	photoErr error
}

// forwardedMedia records a media relay so tests can assert who received what.
type forwardedMedia struct {
	FromChatID int64
	ToChatID   int64
	MessageID  int64
}

type sentMsg struct {
	ChatID    int64
	Text      string
	ReplyTo   int64
	ReplyMark json.RawMessage
}

func (f *fakeSender) Send(ctx context.Context, chatID int64, text string, kbd json.RawMessage, replyTo int64) (*telegram.Message, error) {
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	f.sent = append(f.sent, sentMsg{ChatID: chatID, Text: text, ReplyTo: replyTo, ReplyMark: kbd})
	return &telegram.Message{MessageID: int64(len(f.sent)), Chat: telegram.Chat{ID: chatID}}, nil
}

// SendPhoto satisfies engine.PhotoSender so the browse-card photo path is
// testable (the text fallback previously hid every photo failure).
func (f *fakeSender) SendPhoto(ctx context.Context, chatID int64, fileID, caption string, kbd json.RawMessage) (*telegram.Message, error) {
	if f.photoErr != nil {
		return nil, f.photoErr
	}
	f.sent = append(f.sent, sentMsg{ChatID: chatID, Text: caption, ReplyMark: kbd})
	return &telegram.Message{
		MessageID: int64(len(f.sent)),
		Chat:      telegram.Chat{ID: chatID},
		Photo:     []telegram.PhotoSize{{FileID: fileID}},
	}, nil
}

func (f *fakeSender) Edit(ctx context.Context, chatID, msgID int64, text string, kbd json.RawMessage) error {
	f.edits = append(f.edits, sentMsg{ChatID: chatID, Text: text, ReplyMark: kbd})
	return nil
}

// EditCaption satisfies engine.CaptionEditor so photo-card close paths
// (media rejects editMessageText) behave as they do on the real platforms.
func (f *fakeSender) EditCaption(ctx context.Context, chatID, msgID int64, text string, kbd json.RawMessage) error {
	f.edits = append(f.edits, sentMsg{ChatID: chatID, Text: text, ReplyMark: kbd})
	return nil
}

// Delete satisfies engine.MessageDeleter so browse cards can be removed.
func (f *fakeSender) Delete(ctx context.Context, chatID, msgID int64) error {
	f.deletes = append(f.deletes, forwardedMedia{FromChatID: chatID, ToChatID: chatID, MessageID: msgID})
	return nil
}

// ForwardMedia satisfies engine.MediaForwarder so the chat relay is testable.
func (f *fakeSender) ForwardMedia(ctx context.Context, fromChatID, toChatID, messageID int64) (*telegram.Message, error) {
	if f.forwardErr != nil {
		return nil, f.forwardErr
	}
	f.forwards = append(f.forwards, forwardedMedia{FromChatID: fromChatID, ToChatID: toChatID, MessageID: messageID})
	return &telegram.Message{MessageID: messageID, Chat: telegram.Chat{ID: toChatID}}, nil
}

func (f *fakeSender) AnswerCallback(ctx context.Context, callbackID, text string) error { return nil }

func (f *fakeSender) lastTo(chatID int64) *sentMsg {
	for i := len(f.sent) - 1; i >= 0; i-- {
		if f.sent[i].ChatID == chatID {
			return &f.sent[i]
		}
	}
	return nil
}

func (f *fakeSender) lastEditTo(chatID int64) *sentMsg {
	for i := len(f.edits) - 1; i >= 0; i-- {
		if f.edits[i].ChatID == chatID {
			return &f.edits[i]
		}
	}
	return nil
}

// memUsers is an in-memory UserStore for engine tests.
type memUsers struct {
	states map[int64]json.RawMessage
	next   int64
}

func newMemUsers() *memUsers { return &memUsers{states: map[int64]json.RawMessage{}, next: 1000} }

func (m *memUsers) Upsert(ctx context.Context, botID, platform string, telegramID int64, username, firstName, lastName *string) (int64, error) {
	for id, raw := range m.states {
		var st map[string]any
		_ = json.Unmarshal(raw, &st)
		if tid, _ := st["__tid"].(float64); int64(tid) == telegramID {
			if plat, _ := st["__plat"].(string); plat == platform {
				return id, nil
			}
		}
	}
	m.next++
	id := m.next
	raw, _ := json.Marshal(map[string]any{"scene": "home", "data": map[string]any{}, "__tid": telegramID, "__plat": platform})
	m.states[id] = raw
	return id, nil
}

func (m *memUsers) GetState(ctx context.Context, botID string, userID int64) (json.RawMessage, error) {
	return m.states[userID], nil
}

func (m *memUsers) SetState(ctx context.Context, botID string, userID int64, state json.RawMessage) error {
	m.states[userID] = state
	return nil
}

type memEvents struct{}

func (memEvents) Insert(ctx context.Context, botID string, userID int64, kind string, text *string, meta []byte) error {
	return nil
}

type noopConfigs struct{}

func (noopConfigs) GetMerged(ctx context.Context, botID string, tpl engine.Template) (map[string]any, map[string]any, map[string]any, error) {
	return map[string]any{}, map[string]any{}, map[string]any{}, nil
}

// dynConfigs feeds the env's mutable prices map into BotContext.Prices so
// economy tests can tune prices per scenario.
type dynConfigs struct{ prices *map[string]any }

func (d dynConfigs) GetMerged(ctx context.Context, botID string, tpl engine.Template) (map[string]any, map[string]any, map[string]any, error) {
	if d.prices == nil {
		return map[string]any{}, map[string]any{}, map[string]any{}, nil
	}
	return map[string]any{}, map[string]any{}, *d.prices, nil
}

// TestFullChatFlow runs the definition-of-done scenario against real Postgres:
// two users onboard, both search, matcher pairs them, chat messages flow with
// reply mapping, one ends the chat, both land in rating.
func TestFullChatFlow(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer pool.Close()

	botRepo := postgres.NewBotRepo(pool)
	userRepo := postgres.NewBotUserRepo(pool)
	datingRepo := postgres.NewDatingRepo(pool)

	adminRepo := postgres.NewAdminRepo(pool)
	admin, err := adminRepo.Create(ctx, fmt.Sprintf("test-%d@example.com", time.Now().UnixNano()), "x", "test", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	bot, err := botRepo.Create(ctx, "test-dating-bot", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	store := NewStore(datingRepo, userRepo)
	registry := engine.NewRegistry()
	registry.Register(&Template{})
	tpl, _ := registry.Get(domain.TemplateDating)

	users := runtime.NewUserStoreAdapter(userRepo)
	eng := engine.New(registry, users, memEvents{}, noopConfigs{})
	eng.SetData("dating", store)

	sendA := &fakeSender{}
	sendB := &fakeSender{}

	feed := func(sender *fakeSender, chatID int64, text string) {
		tid := int64(0)
		switch sender {
		case sendA:
			tid = 111
		case sendB:
			tid = 222
		}
		userID, err := users.Upsert(ctx, bot.ID.String(), "telegram", tid, nil, nil, nil)
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}
		_, _ = store.EnsureProfile(ctx, bot.ID, userID, "telegram", tid)
		upd := &telegram.Update{
			Message: &telegram.Message{
				MessageID: time.Now().UnixNano() % 100000,
				From:      &telegram.User{ID: tid, FirstName: "t"},
				Chat:      telegram.Chat{ID: chatID, Type: "private"},
				Text:      text,
			},
		}
		eng.HandleUpdate(ctx, "telegram", &domain.Bot{ID: bot.ID, TemplateKey: domain.TemplateDating}, sender, upd)
	}

	// --- onboarding user A (male, 25, Tehran) ---
	feed(sendA, 111, "/start")
	feed(sendA, 111, "امیر")
	feed(sendA, 111, "25")
	feed(sendA, 111, "تهران")
	feed(sendA, 111, "تهران")
	feed(sendA, 111, "آقام")
	feed(sendA, 111, "⏭ رد شد") // skip optional avatar step

	// --- onboarding user B (female, 22, Tehran) ---
	feed(sendB, 222, "/start")
	feed(sendB, 222, "سارا")
	feed(sendB, 222, "22")
	feed(sendB, 222, "تهران")
	feed(sendB, 222, "تهران")
	feed(sendB, 222, "خانومم")
	feed(sendB, 222, "⏭ رد شد") // skip optional avatar step

	pA, err := store.ProfileByTelegram(ctx, bot.ID, "", 111)
	if err != nil || !IsProfileComplete(pA) {
		t.Fatalf("profile A incomplete: %+v err=%v", pA, err)
	}

	// --- both search ---
	feed(sendA, 111, "🔗 به یک ناشناس وصلم کن")
	feed(sendA, 111, "🔗 غریبه")
	feed(sendB, 222, "🔗 به یک ناشناس وصلم کن")
	feed(sendB, 222, "🔗 غریبه")

	m := NewMatcher(store, staticBots{bot.ID}, func(botID uuid.UUID, platform string, chatID int64, text string, kbd json.RawMessage) error {
		if chatID == 111 {
			sendA.sent = append(sendA.sent, sentMsg{ChatID: chatID, Text: text, ReplyMark: kbd})
		} else {
			sendB.sent = append(sendB.sent, sentMsg{ChatID: chatID, Text: text, ReplyMark: kbd})
		}
		return nil
	})
	m.matchOnce(ctx, bot.ID)

	pA, _ = store.ProfileByTelegram(ctx, bot.ID, "", 111)
	pB, _ := store.ProfileByTelegram(ctx, bot.ID, "", 222)
	if pA.Status != domain.DatingChatting || pB.Status != domain.DatingChatting {
		t.Fatalf("expected both chatting: A=%s B=%s", pA.Status, pB.Status)
	}
	if pA.PartnerUserID == nil || *pA.PartnerUserID != pB.UserID {
		t.Fatalf("A partner wrong: %+v", pA.PartnerUserID)
	}
	if msg := sendA.lastTo(111); msg == nil || !contains(msg.Text, "وصلت انجام شد") {
		t.Fatalf("A did not get match message: %+v", sendA.sent)
	}

	// --- chat: A sends, B replies to it ---
	feed(sendA, 111, "سلام!")
	// B's reply references the forwarded message id in B's chat
	sent := lastForChat(sendB, 222)
	_ = sent
	feed(sendB, 222, "سلام! حالت خوبه؟")

	// A should have received B's text (recorded on B's sender, chat=111)
	if msg := sendB.lastTo(111); msg == nil || !contains(msg.Text, "حالت خوبه") {
		t.Fatalf("A did not receive B's reply: %+v", sendB.sent)
	}

	// --- A ends the chat → pending rate both ---
	feed(sendA, 111, "🛑 قطع مکالمه")
	pA, _ = store.ProfileByTelegram(ctx, bot.ID, "", 111)
	pB, _ = store.ProfileByTelegram(ctx, bot.ID, "", 222)
	if pA.Status != domain.DatingJoined || pB.Status != domain.DatingJoined {
		t.Fatalf("expected joined after end: A=%s B=%s", pA.Status, pB.Status)
	}
	if msg := sendA.lastTo(222); msg == nil || !contains(msg.Text, "امتیاز") {
		t.Fatalf("B did not get rating prompt: %+v", sendA.sent)
	}

	// --- B rates 5 stars ---
	feed(sendB, 222, "⭐⭐⭐⭐⭐")
	pA, _ = store.ProfileByTelegram(ctx, bot.ID, "", 111)
	if pA.TotalRatings != 1 || pA.RatingSum != 5 {
		t.Fatalf("rating not applied: %+v", pA)
	}
	_ = tpl
}

type staticBots struct{ id uuid.UUID }

func (s staticBots) ListRunningByTemplate(ctx context.Context, key string) ([]uuid.UUID, error) {
	return []uuid.UUID{s.id}, nil
}

// --- phase-2 integration tests (browse likes + avatar bonus) ---

// browseEnv is the shared setup for phase-2 integration tests.
type browseEnv struct {
	botID  uuid.UUID
	store  *Store
	eng    *engine.Engine
	users  engine.UserStore
	sendA  *fakeSender
	sendB  *fakeSender
	mod    *postgres.ModerationRepo
	prices map[string]any
	// platform per sender; defaults to "telegram", cross-platform tests set kat.
	platA, platB string
}

func newBrowseEnv(t *testing.T) *browseEnv {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(pool.Close)

	botRepo := postgres.NewBotRepo(pool)
	userRepo := postgres.NewBotUserRepo(pool)
	datingRepo := postgres.NewDatingRepo(pool)
	adminRepo := postgres.NewAdminRepo(pool)
	admin, err := adminRepo.Create(ctx, fmt.Sprintf("test-%d@example.com", time.Now().UnixNano()), "x", "test", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	bot, err := botRepo.Create(ctx, "test-browse-bot", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	store := NewStore(datingRepo, userRepo)
	registry := engine.NewRegistry()
	registry.Register(&Template{})
	users := runtime.NewUserStoreAdapter(userRepo)
	mod := postgres.NewModerationRepo(pool)
	env := &browseEnv{botID: bot.ID, store: store, users: users, sendA: &fakeSender{}, sendB: &fakeSender{}, mod: mod, prices: map[string]any{}, platA: "telegram", platB: "telegram"}
	// real event store (not the in-memory stub) so surveillance tests can read
	// the chat-scene messages back out of the database
	adminEv := runtime.NewEventStoreAdapter(postgres.NewEventRepo(pool))
	eng := engine.New(registry, users, adminEv, dynConfigs{prices: &env.prices})
	eng.SetData("dating", store)
	eng.SetData("moderationRepo", mod)
	env.eng = eng
	eng.SetData("pricesof", func(string) map[string]any { return env.prices })
	return env
}

func (e *browseEnv) tid(sender *fakeSender) int64 {
	if sender == e.sendA {
		return 111
	}
	return 222
}

// plat returns the platform the given sender's user lives on.
func (e *browseEnv) plat(sender *fakeSender) string {
	if sender == e.sendA {
		return e.platA
	}
	return e.platB
}

func (e *browseEnv) feed(t *testing.T, sender *fakeSender, text string) {
	t.Helper()
	ctx := context.Background()
	tid := e.tid(sender)
	plat := e.plat(sender)
	userID, err := e.users.Upsert(ctx, e.botID.String(), plat, tid, nil, nil, nil)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	_, _ = e.store.EnsureProfile(ctx, e.botID, userID, plat, tid)
	e.eng.HandleUpdate(ctx, plat, &domain.Bot{ID: e.botID, TemplateKey: domain.TemplateDating}, sender, &telegram.Update{
		Message: &telegram.Message{
			MessageID: time.Now().UnixNano() % 100000,
			From:      &telegram.User{ID: tid, FirstName: "t"},
			Chat:      telegram.Chat{ID: tid, Type: "private"},
			Text:      text,
		},
	})
}

func (e *browseEnv) feedCB(t *testing.T, sender *fakeSender, data string) {
	t.Helper()
	ctx := context.Background()
	tid := e.tid(sender)
	e.eng.HandleUpdate(ctx, e.plat(sender), &domain.Bot{ID: e.botID, TemplateKey: domain.TemplateDating}, sender, &telegram.Update{
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb",
			From: telegram.User{ID: tid},
			Data: data,
			Message: &telegram.Message{
				MessageID: 99,
				Chat:      telegram.Chat{ID: tid, Type: "private"},
			},
		},
	})
}

func (e *browseEnv) feedPhoto(t *testing.T, sender *fakeSender, fileID string) {
	t.Helper()
	ctx := context.Background()
	tid := e.tid(sender)
	plat := e.plat(sender)
	userID, err := e.users.Upsert(ctx, e.botID.String(), plat, tid, nil, nil, nil)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	_, _ = e.store.EnsureProfile(ctx, e.botID, userID, plat, tid)
	e.eng.HandleUpdate(ctx, plat, &domain.Bot{ID: e.botID, TemplateKey: domain.TemplateDating}, sender, &telegram.Update{
		Message: &telegram.Message{
			MessageID: time.Now().UnixNano() % 100000,
			From:      &telegram.User{ID: tid},
			Chat:      telegram.Chat{ID: tid, Type: "private"},
			Photo:     []telegram.PhotoSize{{FileID: fileID, Width: 100, Height: 100}},
		},
	})
}

func (e *browseEnv) onboard(t *testing.T, sender *fakeSender, name, age, gender string) {
	t.Helper()
	e.feed(t, sender, "/start")
	e.feed(t, sender, name)
	e.feed(t, sender, age)
	e.feed(t, sender, "تهران")
	e.feed(t, sender, "تهران")
	e.feed(t, sender, gender)
}

// TestDeadButtonFallsHome: platform placeholder buttons (Kat shows
// «دکمه یک»/«دکمه دو» when no commands are registered) must land on the
// complete home menu, never on the bare "متوجه نشدم" reply with no keyboard.
func TestDeadButtonFallsHome(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	for _, dead := range []string{"دکمه یک", "دکمه دو", "کلا یه چیز بی‌ربط"} {
		e.sendA.sent = nil
		e.feed(t, e.sendA, dead)
		msg := e.sendA.lastTo(111)
		if msg == nil {
			t.Fatalf("%q: no reply", dead)
		}
		if !strings.Contains(msg.Text, "چیکار برات انجام بدم") {
			t.Fatalf("%q: not the home screen: %q", dead, msg.Text)
		}
		for _, want := range []string{"مرور پروفایل‌ها", "سکه", "پروفایل من"} {
			if !strings.Contains(string(msg.ReplyMark), want) {
				t.Fatalf("%q: home keyboard missing %q: %s", dead, want, msg.ReplyMark)
			}
		}
	}

	// a bot-menu command must navigate like its button
	e.sendA.sent = nil
	e.feed(t, e.sendA, "/menu@ashna_bot")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "چیکار برات انجام بدم") {
		t.Fatalf("/menu did not land home: %#v", e.sendA.sent)
	}

	// an unknown act:* callback lands home too
	e.sendA.sent = nil
	e.feedCB(t, e.sendA, "act:کلا-چیز-ناموجود")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "چیکار برات انجام بدم") {
		t.Fatalf("unknown action did not land home: %#v", e.sendA.sent)
	}
}

// TestChatTextIsNotSwallowed: inside an active chat free text must reach the
// partner — the dead-input fallback may never hijack it.
func TestChatTextIsNotSwallowed(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")
	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendA, "act:browse:like")
	e.feed(t, e.sendB, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendB, "act:browse:like")

	// outbound of an update rides that update's sender, so A's chat text to B
	// (chat 222) is recorded on sendA
	e.sendA.sent = nil
	e.feed(t, e.sendA, "دکمه یک")
	if msg := e.sendA.lastTo(222); msg == nil || msg.Text != "دکمه یک" {
		t.Fatalf("chat text must reach the partner verbatim: %#v", e.sendA.sent)
	}
}

// TestReplaceViewNavigation: menu screens replace each other — opening a
// screen deletes the previous one instead of stacking («الان هی پیام جدید
// میشه»).
func TestReplaceViewNavigation(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	// fresh numbering: the next screen sent becomes message 1
	e.sendA.sent = nil
	e.sendA.deletes = nil

	// wallet opens on top of the onboarding home screen → old screen goes
	e.feed(t, e.sendA, "💰 سکه")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "کیف پول") {
		t.Fatalf("wallet screen not shown: %#v", e.sendA.sent)
	}
	if len(e.sendA.deletes) == 0 {
		t.Fatalf("opening wallet must delete the previous screen")
	}
	walletID := int64(len(e.sendA.sent))

	// profile replaces wallet
	e.sendA.deletes = nil
	e.feed(t, e.sendA, "👤 پروفایل من")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(string(msg.ReplyMark), "حذف حساب") {
		t.Fatalf("profile screen not shown: %#v", e.sendA.sent)
	}
	if len(e.sendA.deletes) != 1 || e.sendA.deletes[0].MessageID != walletID {
		t.Fatalf("opening profile must delete wallet message %d: %#v", walletID, e.sendA.deletes)
	}
	profileID := int64(len(e.sendA.sent))

	// home replaces profile
	e.sendA.deletes = nil
	e.feed(t, e.sendA, "🏠 منوی اصلی")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "چیکار برات انجام بدم") {
		t.Fatalf("home screen not shown: %#v", e.sendA.sent)
	}
	if len(e.sendA.deletes) != 1 || e.sendA.deletes[0].MessageID != profileID {
		t.Fatalf("returning home must delete profile message %d: %#v", profileID, e.sendA.deletes)
	}
}

// TestFailedSendKeepsPreviousView: the old screen is deleted only after its
// replacement is on screen. A send that fails must leave the current menu
// untouched — never «message deleted and nothing in its place» (Kat bug).
func TestFailedSendKeepsPreviousView(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.sendA.sent = nil
	e.sendA.deletes = nil

	// first screen: wallet becomes the tracked view (message 1)
	e.feed(t, e.sendA, "💰 سکه")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "کیف پول") {
		t.Fatalf("wallet screen not shown: %#v", e.sendA.sent)
	}
	// the wallet replaced the onboarding screen (its delete is expected);
	// from here on nothing may be deleted until a replacement lands
	e.sendA.deletes = nil

	// platform outage: the next screen's send fails → nothing may be deleted
	e.sendA.sendErr = errors.New("platform down")
	e.sendA.sent = nil
	e.feed(t, e.sendA, "👤 پروفایل من")
	if len(e.sendA.deletes) != 0 {
		t.Fatalf("failed send deleted the previous screen: %#v", e.sendA.deletes)
	}
	if len(e.sendA.sent) != 0 {
		t.Fatalf("failed send produced messages: %#v", e.sendA.sent)
	}

	// platform back: the same tap now replaces the wallet screen
	e.sendA.sendErr = nil
	e.feed(t, e.sendA, "👤 پروفایل من")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(string(msg.ReplyMark), "حذف حساب") {
		t.Fatalf("profile screen not shown after recovery: %#v", e.sendA.sent)
	}
	if len(e.sendA.deletes) != 1 || e.sendA.deletes[0].MessageID != 1 {
		t.Fatalf("recovery tap must delete the wallet screen (msg 1): %#v", e.sendA.deletes)
	}
}

// TestPhotoFailureFallsBackToTextCard: the browse card is a photo (a file_id
// from another platform — Telegram id shown on Kat — cannot be sent there).
// The card must fall back to the text inline card; the previous menu may only
// disappear together with that replacement.
func TestPhotoFailureFallsBackToTextCard(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feedPhoto(t, e.sendB, "tg-photo-1")

	e.sendA.sent = nil
	e.sendA.deletes = nil

	e.sendA.photoErr = errors.New("bad file_id")
	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "سارا") {
		t.Fatalf("photo failure must fall back to the text card: %#v", e.sendA.sent)
	}
	if len(e.sendA.deletes) != 1 {
		t.Fatalf("the previous screen must be replaced exactly once: %#v", e.sendA.deletes)
	}
}

// TestBrowseAdvanceKeepsCardOnSendFailure: advancing the deck (like/pass/text)
// must not remove the current card unless the next one actually landed —
// a failed send leaves the card (with live buttons) instead of an empty chat.
func TestBrowseAdvanceKeepsCardOnSendFailure(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "ساله") {
		t.Fatalf("first card not shown: %#v", e.sendA.sent)
	}
	e.sendA.sent = nil
	e.sendA.deletes = nil

	// the next advance's send fails → the card must stay untouched
	e.sendA.sendErr = errors.New("platform down")
	e.feedCB(t, e.sendA, "act:browse:next")
	if len(e.sendA.deletes) != 0 {
		t.Fatalf("failed advance deleted the card: %#v", e.sendA.deletes)
	}

	// platform back: the deck is now empty (the pass took) and the home
	// screen replaces the card exactly when its send lands
	e.sendA.sendErr = nil
	e.feedCB(t, e.sendA, "act:browse:next")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "باقی نمونده") {
		t.Fatalf("empty-deck home not shown after recovery: %#v", e.sendA.sent)
	}
	if len(e.sendA.deletes) == 0 {
		t.Fatalf("recovery screen must replace the old card: %#v", e.sendA.deletes)
	}
}

// TestChatHomeButtonLeavesChat: the chat keyboard must offer a way back to the
// main menu (Kat showed only قطع مکالمه/بلاک — no escape). The «🏠 منوی اصلی»
// tap ends the chat like قطع مکالمه and must never reach the partner as text.
func TestChatHomeButtonLeavesChat(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

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
	if pA.Status != domain.DatingChatting || pB.Status != domain.DatingChatting {
		t.Fatalf("precondition: both must be chatting, got %s / %s", pA.Status, pB.Status)
	}

	e.sendB.sent = nil
	e.feed(t, e.sendA, "🏠 منوی اصلی")

	// the tap ends the chat into the rating screen (same as قطع مکالمه)
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "امتیاز") {
		t.Fatalf("home escape must end the chat into rating: %+v", e.sendA.sent)
	}
	// the label must never be relayed to the partner
	for _, s := range e.sendB.sent {
		if contains(s.Text, "منوی اصلی") {
			t.Fatalf("home label leaked to the partner: %+v", s)
		}
	}
}

// TestRatingViaKatCallback: on Kat the star rows arrive as label callbacks
// (inlineized keyboard — callback_data is the button label). The star press
// must register the rating exactly like a typed star.
func TestRatingViaKatCallback(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")
	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendA, "act:browse:like")
	e.feed(t, e.sendB, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendB, "act:browse:like")

	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pB.Status != domain.DatingChatting {
		t.Fatalf("precondition: B must be chatting, got %s", pB.Status)
	}

	// A ends the chat into the rating scene
	e.feed(t, e.sendA, "🛑 قطع مکالمه")

	// the star arrives as a callback (Kat's inlineized rating keyboard)
	e.feedCB(t, e.sendA, "⭐⭐⭐")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "امتیازت ثبت شد") {
		t.Fatalf("rating via label callback must register: %+v", e.sendA.sent)
	}
}

// TestBrowseMutualLikeFlow: A (male) likes B (female) from the deck, then B
// likes A back → mutual → both land in the chat scene.
func TestBrowseMutualLikeFlow(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	// A opens the deck and sees B's card
	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "سارا") {
		t.Fatalf("A did not see B's card: %+v", e.sendA.sent)
	}

	// A likes B — no mutual yet
	e.feedCB(t, e.sendA, "act:browse:like")
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pA.Status != domain.DatingJoined || pB.Status != domain.DatingJoined {
		t.Fatalf("one-sided like must not connect: A=%s B=%s", pA.Status, pB.Status)
	}
	if liked, _ := e.store.Repo().HasLiked(ctx, e.botID, pA.UserID, pB.UserID); !liked {
		t.Fatal("A's like not recorded")
	}

	// B opens the deck, sees A, likes → mutual → both chatting
	e.feed(t, e.sendB, "💘 مرور پروفایل‌ها")
	if msg := e.sendB.lastTo(222); msg == nil || !contains(msg.Text, "امیر") {
		t.Fatalf("B did not see A's card: %+v", e.sendB.sent)
	}
	e.feedCB(t, e.sendB, "act:browse:like")

	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	pB, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pA.Status != domain.DatingChatting || pB.Status != domain.DatingChatting {
		t.Fatalf("mutual like must connect: A=%s B=%s", pA.Status, pB.Status)
	}
	if msg := e.sendB.lastTo(222); msg == nil || !contains(msg.Text, "وصل شدید") {
		t.Fatalf("B did not get connect message: %+v", e.sendB.sent)
	}
	if msg := e.sendB.lastTo(111); msg == nil || !contains(msg.Text, "وصل شدید") {
		t.Fatalf("A did not get partner connect message: %+v", e.sendB.sent)
	}
}

// TestBrowsePassExcludesCard: passing a card records kind=pass so it never
// shows again.
func TestBrowsePassExcludesCard(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendA, "act:browse:next")

	// pool for A is now empty → home
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "فعلاً") {
		t.Fatalf("A should see empty pool after pass: %+v", e.sendA.sent)
	}
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if liked, _ := e.store.Repo().HasLiked(ctx, e.botID, pA.UserID, pB.UserID); liked {
		t.Fatal("pass must not count as like")
	}
}

// TestAvatarBonusOnce: photo during the avatar step stores file_id and pays
// the one-time bonus; a second photo pays nothing.
func TestAvatarBonusOnce(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	if pA.Coins != 5 {
		t.Fatalf("signup bonus should be 5, got %d", pA.Coins)
	}

	e.feedPhoto(t, e.sendA, "file-avatar-1")
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	if pA.PhotoFileID == nil || *pA.PhotoFileID != "file-avatar-1" {
		t.Fatalf("photo not saved: %+v", pA.PhotoFileID)
	}
	if pA.AvatarBonusAt == nil {
		t.Fatal("avatar_bonus_at not set")
	}
	if pA.Coins != 15 {
		t.Fatalf("avatar bonus should add 10, got %d", pA.Coins)
	}
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "ثبت‌نامت کامل شد") {
		t.Fatalf("onboarding should complete after photo: %+v", e.sendA.sent)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func lastForChat(f *fakeSender, chatID int64) *sentMsg { return f.lastTo(chatID) }
