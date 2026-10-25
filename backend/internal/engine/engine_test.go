package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"sadbot/internal/domain"
	"sadbot/internal/engine/templates"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

type recSender struct {
	sent      []recMsg
	callbacks []string
}

type recMsg struct {
	ChatID  int64
	Text    string
	ReplyTo int64
	KBD     json.RawMessage
}

func (r *recSender) Send(ctx context.Context, chatID int64, text string, kbd json.RawMessage, replyTo int64) (*telegram.Message, error) {
	r.sent = append(r.sent, recMsg{ChatID: chatID, Text: text, ReplyTo: replyTo, KBD: kbd})
	return &telegram.Message{MessageID: int64(len(r.sent)), Chat: telegram.Chat{ID: chatID}}, nil
}

func (r *recSender) Edit(ctx context.Context, chatID, msgID int64, text string, kbd json.RawMessage) error {
	return nil
}

func (r *recSender) AnswerCallback(ctx context.Context, callbackID, text string) error {
	r.callbacks = append(r.callbacks, callbackID)
	return nil
}

func (r *recSender) last() recMsg { return r.sent[len(r.sent)-1] }

type memUsers struct {
	states map[int64]json.RawMessage
	next   int64
}

func newMemUsers() *memUsers { return &memUsers{states: map[int64]json.RawMessage{}, next: 100} }

func (m *memUsers) Upsert(ctx context.Context, botID string, telegramID int64, username, firstName, lastName *string) (int64, error) {
	for id, raw := range m.states {
		var st map[string]any
		_ = json.Unmarshal(raw, &st)
		if tid, _ := st["__tid"].(float64); int64(tid) == telegramID {
			return id, nil
		}
	}
	m.next++
	id := m.next
	raw, _ := json.Marshal(map[string]any{"scene": "home", "data": map[string]any{}, "__tid": telegramID})
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

type memEvents struct {
	kinds []string
}

func (m *memEvents) Insert(ctx context.Context, botID string, userID int64, kind string, text *string, meta []byte) error {
	m.kinds = append(m.kinds, kind)
	return nil
}

// memConfigs serves the template's own defaults (no DB override).
type memConfigs struct{}

func (memConfigs) GetMerged(ctx context.Context, botID string, tpl Template) (map[string]any, map[string]any, map[string]any, error) {
	var texts map[string]any
	if err := json.Unmarshal(tpl.DefaultTexts(), &texts); err != nil {
		return nil, nil, nil, err
	}
	var menus struct {
		Scenes map[string]any `json:"scenes"`
	}
	if err := json.Unmarshal(tpl.DefaultMenus(), &menus); err != nil {
		return nil, nil, nil, err
	}
	prices := map[string]any{}
	if priced, ok := tpl.(Priced); ok {
		_ = json.Unmarshal(priced.DefaultPrices(), &prices)
	}
	return texts, menus.Scenes, prices, nil
}

func newEngineFixture(t *testing.T) (*Engine, *recSender, *memEvents, *memUsers, *domain.Bot) {
	t.Helper()
	reg := NewRegistry()
	reg.Register(&templates.WelcomeMenu{})
	users := newMemUsers()
	events := &memEvents{}
	eng := New(reg, users, events, memConfigs{})
	sender := &recSender{}
	bot := &domain.Bot{ID: uuid.New(), TemplateKey: domain.TemplateWelcomeMenu}
	return eng, sender, events, users, bot
}

func msgUpdate(text string) *telegram.Update {
	return &telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 100,
			Text:      text,
			Chat:      telegram.Chat{ID: 500, Type: "private"},
			From:      &telegram.User{ID: 700, FirstName: "کاربر"},
		},
	}
}

func cbUpdate(data string) *telegram.Update {
	return &telegram.Update{
		UpdateID: 2,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb-1",
			Data: data,
			From: telegram.User{ID: 700, FirstName: "کاربر"},
			Message: &telegram.Message{
				MessageID: 50,
				Chat:      telegram.Chat{ID: 500, Type: "private"},
			},
		},
	}
}

func TestHandleUpdate_StartRendersHome(t *testing.T) {
	eng, sender, events, users, bot := newEngineFixture(t)

	eng.HandleUpdate(context.Background(), bot, sender, msgUpdate("/start"))
	if len(sender.sent) != 1 {
		t.Fatalf("sent = %d, want 1 scene render", len(sender.sent))
	}
	if !strings.Contains(sender.sent[0].Text, "خوش اومدی") {
		t.Fatalf("welcome text = %q", sender.sent[0].Text)
	}
	if len(sender.sent[0].KBD) == 0 {
		t.Fatal("home keyboard missing")
	}
	// events: command + outbound
	if len(events.kinds) < 2 || events.kinds[0] != domain.EventCommand {
		t.Fatalf("events = %v", events.kinds)
	}
	_ = users
}

func TestHandleUpdate_ButtonLabelRoutes(t *testing.T) {
	eng, sender, _, _, bot := newEngineFixture(t)

	// /start first so the user is in home
	eng.HandleUpdate(context.Background(), bot, sender, msgUpdate("/start"))
	// tap the "about" button by label
	eng.HandleUpdate(context.Background(), bot, sender, msgUpdate("📖 درباره ما"))

	if len(sender.sent) != 2 {
		t.Fatalf("sent = %d, want 2", len(sender.sent))
	}
	if !strings.Contains(sender.sent[1].Text, "نمونه") {
		t.Fatalf("about text = %q", sender.sent[1].Text)
	}
}

func TestHandleUpdate_UnknownText(t *testing.T) {
	eng, sender, _, _, bot := newEngineFixture(t)

	eng.HandleUpdate(context.Background(), bot, sender, msgUpdate("هر چی بی‌ربط"))
	if len(sender.sent) != 1 {
		t.Fatalf("sent = %d", len(sender.sent))
	}
	if !strings.Contains(sender.sent[0].Text, "متوجه نشدم") {
		t.Fatalf("unknown text = %q", sender.sent[0].Text)
	}
}

func TestHandleUpdate_CallbackSceneDispatch(t *testing.T) {
	eng, sender, events, _, bot := newEngineFixture(t)

	eng.HandleUpdate(context.Background(), bot, sender, cbUpdate("scene:about"))
	if len(sender.callbacks) != 1 {
		t.Fatalf("callback not answered")
	}
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].Text, "نمونه") {
		t.Fatalf("sent = %+v", sender.sent)
	}
	found := false
	for _, k := range events.kinds {
		if k == domain.EventCallback {
			found = true
		}
	}
	if !found {
		t.Fatalf("callback event not logged: %v", events.kinds)
	}
}

func TestHandleUpdate_SettingsTogglePersists(t *testing.T) {
	eng, sender, _, users, bot := newEngineFixture(t)

	eng.HandleUpdate(context.Background(), bot, sender, msgUpdate("/start"))
	eng.HandleUpdate(context.Background(), bot, sender, cbUpdate("scene:settings"))
	// first render: notify off
	if !strings.Contains(sender.sent[len(sender.sent)-1].Text, "خاموش") {
		t.Fatalf("initial settings text = %q", sender.last().Text)
	}
	// toggle
	eng.HandleUpdate(context.Background(), bot, sender, cbUpdate("act:settings.notify"))
	if !strings.Contains(sender.last().Text, "روشن") {
		t.Fatalf("after toggle text = %q", sender.last().Text)
	}

	// state persisted: reload and go to settings again — still on
	eng.HandleUpdate(context.Background(), bot, sender, cbUpdate("scene:settings"))
	if !strings.Contains(sender.last().Text, "روشن") {
		t.Fatalf("persisted toggle lost: %q", sender.last().Text)
	}
	_ = users
}

func TestHandleUpdate_IgnoresBots(t *testing.T) {
	eng, sender, _, _, bot := newEngineFixture(t)
	upd := msgUpdate("/start")
	upd.Message.From.IsBot = true

	eng.HandleUpdate(context.Background(), bot, sender, upd)
	if len(sender.sent) != 0 {
		t.Fatalf("bot updates must be ignored, got %+v", sender.sent)
	}
}

func TestHandleUpdate_UnknownMediaGetsFallback(t *testing.T) {
	eng, sender, _, _, bot := newEngineFixture(t)
	upd := msgUpdate("")
	upd.Message.Photo = []telegram.PhotoSize{{FileID: "abc"}}

	eng.HandleUpdate(context.Background(), bot, sender, upd)
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].Text, "متوجه نشدم") {
		t.Fatalf("sent = %+v", sender.sent)
	}
}

func TestHandleUpdate_UnknownTemplateIsNoop(t *testing.T) {
	reg := NewRegistry()
	eng := New(reg, newMemUsers(), &memEvents{}, memConfigs{})
	sender := &recSender{}
	bot := &domain.Bot{ID: uuid.New(), TemplateKey: "missing-template"}

	eng.HandleUpdate(context.Background(), bot, sender, msgUpdate("/start"))
	if len(sender.sent) != 0 {
		t.Fatalf("unknown template must not send, got %+v", sender.sent)
	}
}

func TestLookupLabel_UnknownLabel(t *testing.T) {
	eng, _, _, _, bot := newEngineFixture(t)
	reg := eng.registry
	tpl, _ := reg.Get(bot.TemplateKey)
	_ = tpl

	users := newMemUsers()
	_ = users
	bc := &BotContext{scenes: map[string]any{}}
	if _, found := bc.LookupLabel("نیست"); found {
		t.Fatal("unknown label must not be found")
	}
}

func TestText_MissingKeyReturnsKey(t *testing.T) {
	bc := &BotContext{texts: map[string]any{"welcome": "hi"}}
	if bc.Text("missing") != "missing" {
		t.Fatalf("Text(missing) = %q", bc.Text("missing"))
	}
}
