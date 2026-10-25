package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/telegram"
)

// Sender abstracts Telegram sending so the engine is testable.
type Sender interface {
	Send(ctx context.Context, chatID int64, text string, kbd json.RawMessage, replyTo int64) (*telegram.Message, error)
	Edit(ctx context.Context, chatID, msgID int64, text string, kbd json.RawMessage) error
	AnswerCallback(ctx context.Context, callbackID, text string) error
}

// PhotoSender is an optional Sender capability (browse cards / avatar display).
// Templates fall back to text when the sender doesn't implement it.
type PhotoSender interface {
	SendPhoto(ctx context.Context, chatID int64, fileID, caption string, kbd json.RawMessage) (*telegram.Message, error)
}

// CaptionEditor is an optional Sender capability: photo/media messages must
// be edited via caption — editMessageText fails on them. Browse uses it to
// close photo cards.
type CaptionEditor interface {
	EditCaption(ctx context.Context, chatID, msgID int64, text string, kbd json.RawMessage) error
}

// MediaForwarder relays any media a user sent to their chat partner. Optional
// capability: senders that can't copy media return the "not supported" error
// and the template tells the user it only carries text right now.
type MediaForwarder interface {
	ForwardMedia(ctx context.Context, fromChatID, toChatID, messageID int64) (*telegram.Message, error)
}

// Stores the engine depends on (implemented by repos / runtime glue).
type UserStore interface {
	Upsert(ctx context.Context, botID string, telegramID int64, username, firstName, lastName *string) (userID int64, err error)
	GetState(ctx context.Context, botID string, userID int64) (json.RawMessage, error)
	SetState(ctx context.Context, botID string, userID int64, state json.RawMessage) error
}

type EventStore interface {
	Insert(ctx context.Context, botID string, userID int64, kind string, text *string, meta []byte) error
}

type ConfigStore interface {
	GetMerged(ctx context.Context, botID string, tpl Template) (texts map[string]any, scenes map[string]any, prices map[string]any, err error)
}

// BotContext is everything a template needs to handle one update.
type BotContext struct {
	BotID         string
	UserID        int64
	TelegramID    int64
	ChatID        int64
	MsgID         int64 // message_id of the inbound message (0 for callbacks)
	Sender        Sender
	State         map[string]any // decoded state; mutate then SaveState
	Data          map[string]any // DI bag for template stores (e.g. "dating")
	texts         map[string]any
	scenes        map[string]any
	Prices        map[string]any // coin costs / packages (merged bot config)
	users         UserStore
	events        EventStore
	actionHandler ActionHandler
}

// PriceInt reads an int price/cost from Prices with a fallback default.
func (bc *BotContext) PriceInt(key string, def int) int {
	if v, ok := bc.Prices[key].(float64); ok {
		return int(v)
	}
	if v, ok := bc.Prices[key].(int); ok {
		return v
	}
	return def
}

// PricePackages reads the coin package list; each item is {id, coins, price}.
func (bc *BotContext) PricePackages() []map[string]any {
	raw, _ := bc.Prices["packages"].([]any)
	var out []map[string]any
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

type Engine struct {
	registry *Registry
	users    UserStore
	events   EventStore
	configs  ConfigStore
	data     map[string]any
}

func New(registry *Registry, users UserStore, events EventStore, configs ConfigStore) *Engine {
	return &Engine{registry: registry, users: users, events: events, configs: configs, data: map[string]any{}}
}

// SetData injects dependencies exposed to templates via BotContext.Data.
func (e *Engine) SetData(key string, v any) { e.data[key] = v }

// HandleUpdate processes one Telegram update for a bot.
func (e *Engine) HandleUpdate(ctx context.Context, bot *domain.Bot, sender Sender, upd *telegram.Update) {
	tgUser := upd.EffectiveUser()
	chatID := upd.EffectiveChatID()
	if tgUser == nil || chatID == 0 {
		return
	}
	if tgUser.IsBot {
		return
	}

	tpl, ok := e.registry.Get(bot.TemplateKey)
	if !ok {
		log.Printf("[engine] bot %s: unknown template %q", bot.ID, bot.TemplateKey)
		return
	}

	userID, err := e.users.Upsert(ctx, bot.ID.String(), tgUser.ID, ptr(tgUser.Username), ptr(tgUser.FirstName), ptr(tgUser.LastName))
	if err != nil {
		log.Printf("[engine] upsert user: %v", err)
		return
	}

	rawState, err := e.users.GetState(ctx, bot.ID.String(), userID)
	if err != nil {
		rawState = json.RawMessage(`{"scene":"home","data":{}}`)
	}
	state := decodeState(rawState)

	texts, scenes, prices, err := e.configs.GetMerged(ctx, bot.ID.String(), tpl)
	if err != nil {
		log.Printf("[engine] config: %v", err)
		return
	}

	bc := &BotContext{
		BotID:      bot.ID.String(),
		UserID:     userID,
		TelegramID: tgUser.ID,
		ChatID:     chatID,
		Sender:     sender,
		State:      state,
		Data:       e.data,
		texts:      texts,
		scenes:     scenes,
		Prices:     prices,
		users:      e.users,
		events:     e.events,
	}
	if upd.Message != nil {
		bc.MsgID = upd.Message.MessageID
	}
	if upd.CallbackQuery != nil && upd.CallbackQuery.Message != nil {
		// so templates can edit the message the callback button belongs to
		bc.MsgID = upd.CallbackQuery.Message.MessageID
	}
	if ah, ok := tpl.(ActionHandler); ok {
		bc.actionHandler = ah
	}

	// Log inbound. The scene is recorded so the panel can reconstruct a chat
	// transcript from chat-scene messages alone, and tell them apart from menu
	// taps or onboarding text.
	if upd.Message != nil && upd.Message.Text != "" {
		kind := domain.EventInboundMessage
		if strings.HasPrefix(upd.Message.Text, "/") {
			kind = domain.EventCommand
		}
		scene, _ := bc.State["scene"].(string)
		var meta []byte
		if scene != "" {
			meta = mustMeta(map[string]any{"scene": scene})
		}
		_ = e.events.Insert(ctx, bc.BotID, userID, kind, &upd.Message.Text, meta)
	}
	if upd.CallbackQuery != nil {
		_ = e.events.Insert(ctx, bc.BotID, userID, domain.EventCallback, &upd.CallbackQuery.Data, nil)
	}

	if starter, ok := tpl.(Starter); ok {
		if upd.Message != nil && strings.HasPrefix(upd.Message.Text, "/start") {
			payload := strings.TrimSpace(strings.TrimPrefix(upd.Message.Text, "/start"))
			if starter.Start(bc, payload) {
				return
			}
		}
	}

	switch {
	case upd.Message != nil && strings.HasPrefix(upd.Message.Text, "/start"):
		payload := strings.TrimSpace(strings.TrimPrefix(upd.Message.Text, "/start"))
		bc.OnStart(payload)
	case upd.CallbackQuery != nil:
		_ = sender.AnswerCallback(ctx, upd.CallbackQuery.ID, "")
		bc.DispatchAction(upd.CallbackQuery.Data)
	case upd.Message != nil && upd.Message.Text != "":
		if mh, ok := tpl.(MessageHandler); ok && mh.HandleMessage(bc, upd.Message.Text) {
			return
		}
		if action, found := bc.LookupLabel(upd.Message.Text); found {
			bc.DispatchAction(action)
		} else {
			bc.Reply(bc.Text("unknown"))
		}
	default:
		// templates may consume media (e.g. dating avatar upload)
		if mh, ok := tpl.(MediaHandler); ok && upd.Message != nil && mh.HandleMedia(bc, upd.Message) {
			return
		}
		bc.Reply(bc.Text("unknown"))
	}
}

// --- BotContext methods ---

func (bc *BotContext) OnStart(payload string) {
	bc.SetScene("home")
}

func (bc *BotContext) Text(key string) string {
	if v, ok := bc.texts[key].(string); ok {
		return v
	}
	// templates that ship empty texts (e.g. dating) still need sane fallbacks
	if key == "unknown" {
		return "متوجه نشدم 🤔\n\nاز دکمه‌های منو استفاده کن."
	}
	return key
}

func (bc *BotContext) SetScene(name string) {
	if _, ok := bc.scenes[name]; !ok {
		name = "home"
	}
	bc.State["scene"] = name
	bc.SaveState()
	bc.RenderScene(name)
}

func (bc *BotContext) DispatchAction(action string) {
	switch {
	case strings.HasPrefix(action, "scene:"):
		bc.SetScene(strings.TrimPrefix(action, "scene:"))
	case strings.HasPrefix(action, "act:"):
		bc.HandleAction(strings.TrimPrefix(action, "act:"))
	case strings.HasPrefix(action, "url:"):
		// URL buttons open in Telegram client; nothing to do server-side.
	default:
		bc.Reply(bc.Text("unknown"))
	}
}

// HandleAction is template-specific behavior. Templates implementing
// ActionHandler take precedence; otherwise the built-in handlers run.
func (bc *BotContext) HandleAction(action string) {
	if bc.actionHandler != nil && bc.actionHandler.HandleAction(bc, action) {
		return
	}
	switch action {
	case "settings.notify":
		data := bc.dataMap()
		cur, _ := data["notify"].(bool)
		data["notify"] = !cur
		bc.State["data"] = data
		bc.SaveState()
		// stay on settings scene, re-render
		scene, _ := bc.State["scene"].(string)
		if scene == "" {
			scene = "settings"
			bc.State["scene"] = scene
		}
		bc.RenderScene(scene)
	default:
		bc.Reply(bc.Text("unknown"))
	}
}

// LookupLabel resolves a user-typed keyboard label to an action.
func (bc *BotContext) LookupLabel(label string) (string, bool) {
	for _, raw := range bc.scenes {
		scene, _ := raw.(map[string]any)
		rows, _ := scene["rows"].([]any)
		for _, row := range rows {
			btns, _ := row.([]any)
			for _, b := range btns {
				btn, _ := b.(map[string]any)
				if btn["label"] == label {
					action, _ := btn["action"].(string)
					return action, action != ""
				}
			}
		}
	}
	return "", false
}

// RenderScene sends the scene text + reply keyboard.
func (bc *BotContext) RenderScene(name string) {
	scene, ok := bc.scenes[name].(map[string]any)
	if !ok {
		return
	}
	textKey, _ := scene["text_key"].(string)
	text := bc.Text(textKey)

	// dynamic_text: per-scene computed suffix (welcome_menu settings toggle)
	if dyn, _ := scene["dynamic_text"].(string); dyn == "notify" {
		data := bc.dataMap()
		if on, _ := data["notify"].(bool); on {
			text += "\n\n" + bc.Text("settings.notify_on")
		} else {
			text += "\n\n" + bc.Text("settings.notify_off")
		}
	}

	kbd := bc.buildKeyboard(scene)
	msg, err := bc.Sender.Send(context.Background(), bc.ChatID, text, kbd, 0)
	if err != nil {
		log.Printf("[engine] send scene %s: %v", name, err)
		return
	}
	_ = bc.events.Insert(context.Background(), bc.BotID, bc.UserID, domain.EventOutboundMessage, &text, mustMeta(map[string]any{"scene": name, "msg_id": msg.MessageID}))
}

func (bc *BotContext) Reply(text string) {
	msg, err := bc.Sender.Send(context.Background(), bc.ChatID, text, nil, 0)
	if err != nil {
		return
	}
	_ = bc.events.Insert(context.Background(), bc.BotID, bc.UserID, domain.EventOutboundMessage, &text, mustMeta(map[string]any{"msg_id": msg.MessageID}))
}

// KBButton is a reply-keyboard button with an optional color style
// ("primary" / "success", Bot API 7.11+).
type KBButton struct {
	Text  string
	Style string
}

// ReplyKBD sends text with a custom reply keyboard (rows of button labels).
func (bc *BotContext) ReplyKBD(text string, rows [][]string) *telegram.Message {
	styled := make([][]KBButton, 0, len(rows))
	for _, row := range rows {
		srow := make([]KBButton, 0, len(row))
		for _, label := range row {
			srow = append(srow, KBButton{Text: label})
		}
		styled = append(styled, srow)
	}
	return bc.ReplyKBDStyled(text, styled)
}

// ReplyKBDStyled sends text with a colored reply keyboard (kpkchat-style
// primary/success buttons).
func (bc *BotContext) ReplyKBDStyled(text string, rows [][]KBButton) *telegram.Message {
	var kbd telegram.ReplyKeyboardMarkup
	kbd.ResizeKeyboard = true
	kbd.IsPersistent = true
	for _, row := range rows {
		kRow := make([]telegram.ReplyKeyboardButton, 0, len(row))
		for _, b := range row {
			if b.Text != "" {
				kRow = append(kRow, telegram.ReplyKeyboardButton{Text: b.Text, Style: b.Style})
			}
		}
		if len(kRow) > 0 {
			kbd.Keyboard = append(kbd.Keyboard, kRow)
		}
	}
	raw, _ := json.Marshal(kbd)
	msg, err := bc.Sender.Send(context.Background(), bc.ChatID, text, raw, 0)
	if err != nil {
		return nil
	}
	_ = bc.events.Insert(context.Background(), bc.BotID, bc.UserID, domain.EventOutboundMessage, &text, mustMeta(map[string]any{"msg_id": msg.MessageID}))
	return msg
}

// SendTo sends a message to an arbitrary chat (e.g. the chat partner).
func (bc *BotContext) SendTo(chatID int64, text string, replyTo int64) (*telegram.Message, error) {
	return bc.Sender.Send(context.Background(), chatID, text, nil, replyTo)
}

// ForwardMediaTo relays a message this bot received (from the sender's chat)
// into chatID. Returns ErrMediaUnsupported when the sender can't copy media,
// so callers can tell the user it wasn't carried.
func (bc *BotContext) ForwardMediaTo(chatID, fromChatID, messageID int64) (*telegram.Message, error) {
	f, ok := bc.Sender.(MediaForwarder)
	if !ok {
		return nil, ErrMediaUnsupported
	}
	return f.ForwardMedia(context.Background(), fromChatID, chatID, messageID)
}

// ErrMediaUnsupported means the configured sender cannot relay media.
var ErrMediaUnsupported = errors.New("media forwarding is not supported")

// RecordInbound stores a synthetic inbound message for the current scene.
// The engine never logs media updates (it only logs text), so templates that
// relay a photo/sticker/voice use this to keep the transcript complete.
func (bc *BotContext) RecordInbound(text string, extraMeta map[string]any) {
	meta := map[string]any{}
	if scene, ok := bc.State["scene"].(string); ok && scene != "" {
		meta["scene"] = scene
	}
	for k, v := range extraMeta {
		meta[k] = v
	}
	_ = bc.events.Insert(context.Background(), bc.BotID, bc.UserID,
		domain.EventInboundMessage, &text, mustMeta(meta))
}

// ReplyInline sends text with an inline keyboard. Each button is {label, callbackData}.
type InlineButton struct {
	Label    string
	Callback string
}

func (bc *BotContext) ReplyInline(text string, rows [][]InlineButton) *telegram.Message {
	var kbd telegram.InlineKeyboardMarkup
	for _, row := range rows {
		var kRow []telegram.InlineKeyboardButton
		for _, b := range row {
			kRow = append(kRow, telegram.InlineKeyboardButton{Text: b.Label, CallbackData: b.Callback})
		}
		if len(kRow) > 0 {
			kbd.InlineKeyboard = append(kbd.InlineKeyboard, kRow)
		}
	}
	raw, _ := json.Marshal(kbd)
	msg, err := bc.Sender.Send(context.Background(), bc.ChatID, text, raw, 0)
	if err != nil {
		return nil
	}
	_ = bc.events.Insert(context.Background(), bc.BotID, bc.UserID, domain.EventOutboundMessage, &text, mustMeta(map[string]any{"msg_id": msg.MessageID}))
	return msg
}

// SendPhotoKBD sends a photo (Telegram file_id) with caption + inline keyboard.
// Falls back to a text inline message when the sender can't send photos.
func (bc *BotContext) SendPhotoKBD(chatID int64, fileID, caption string, rows [][]InlineButton) *telegram.Message {
	ps, ok := bc.Sender.(PhotoSender)
	if !ok || fileID == "" {
		return bc.ReplyInline(caption, rows)
	}
	var kbd telegram.InlineKeyboardMarkup
	for _, row := range rows {
		var kRow []telegram.InlineKeyboardButton
		for _, b := range row {
			kRow = append(kRow, telegram.InlineKeyboardButton{Text: b.Label, CallbackData: b.Callback})
		}
		if len(kRow) > 0 {
			kbd.InlineKeyboard = append(kbd.InlineKeyboard, kRow)
		}
	}
	raw, _ := json.Marshal(kbd)
	msg, err := ps.SendPhoto(context.Background(), chatID, fileID, caption, raw)
	if err != nil {
		log.Printf("[engine] send photo: %v", err)
		return nil
	}
	_ = bc.events.Insert(context.Background(), bc.BotID, bc.UserID, domain.EventOutboundMessage, &caption, mustMeta(map[string]any{"msg_id": msg.MessageID, "photo": true}))
	return msg
}

// SendKBDTo sends text + reply keyboard to an arbitrary chat.
func (bc *BotContext) SendKBDTo(chatID int64, text string, rows [][]string) (*telegram.Message, error) {
	var kbd telegram.ReplyKeyboardMarkup
	kbd.ResizeKeyboard = true
	for _, row := range rows {
		kRow := make([]telegram.ReplyKeyboardButton, 0, len(row))
		for _, label := range row {
			kRow = append(kRow, telegram.ReplyKeyboardButton{Text: label})
		}
		if len(kRow) > 0 {
			kbd.Keyboard = append(kbd.Keyboard, kRow)
		}
	}
	raw, _ := json.Marshal(kbd)
	return bc.Sender.Send(context.Background(), chatID, text, raw, 0)
}

func (bc *BotContext) SaveState() {
	raw, err := json.Marshal(bc.State)
	if err != nil {
		return
	}
	if err := bc.users.SetState(context.Background(), bc.BotID, bc.UserID, raw); err != nil {
		log.Printf("[engine] save state: %v", err)
	}
}

func (bc *BotContext) buildKeyboard(scene map[string]any) json.RawMessage {
	rows, _ := scene["rows"].([]any)
	if len(rows) == 0 {
		return nil
	}
	var kbd telegram.ReplyKeyboardMarkup
	kbd.ResizeKeyboard = true
	kbd.IsPersistent = true
	for _, row := range rows {
		btns, _ := row.([]any)
		var kRow []telegram.ReplyKeyboardButton
		for _, b := range btns {
			btn, _ := b.(map[string]any)
			label, _ := btn["label"].(string)
			if label != "" {
				kRow = append(kRow, telegram.ReplyKeyboardButton{Text: label})
			}
		}
		if len(kRow) > 0 {
			kbd.Keyboard = append(kbd.Keyboard, kRow)
		}
	}
	raw, err := json.Marshal(kbd)
	if err != nil {
		return nil
	}
	return raw
}

// --- helpers ---

func (bc *BotContext) dataMap() map[string]any {
	data, _ := bc.State["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	return data
}

func decodeState(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	if _, ok := m["scene"]; !ok {
		m["scene"] = "home"
	}
	if _, ok := m["data"]; !ok {
		m["data"] = map[string]any{}
	}
	return m
}

func mustMeta(m map[string]any) []byte {
	raw, err := json.Marshal(m)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

var _ = fmt.Sprintf
