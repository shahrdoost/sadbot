package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// --- store adapters: repos -> engine interfaces ---

// NewUserStoreAdapter exposes BotUserRepo as an engine.UserStore.
func NewUserStoreAdapter(repo *postgres.BotUserRepo) engine.UserStore { return &userStore{repo: repo} }

// NewEventStoreAdapter exposes EventRepo as an engine.EventStore.
func NewEventStoreAdapter(repo *postgres.EventRepo) engine.EventStore { return &eventStore{repo: repo} }

// NewConfigStoreAdapter exposes ConfigRepo as an engine.ConfigStore.
func NewConfigStoreAdapter(repo *postgres.ConfigRepo) engine.ConfigStore {
	return &configStore{repo: repo}
}

type userStore struct{ repo *postgres.BotUserRepo }

func (s *userStore) Upsert(ctx context.Context, botID, platform string, telegramID int64, username, firstName, lastName *string) (int64, error) {
	id, err := uuid.Parse(botID)
	if err != nil {
		return 0, err
	}
	u, err := s.repo.Upsert(ctx, id, platform, telegramID, username, firstName, lastName)
	if err != nil {
		return 0, err
	}
	return u.ID, nil
}

func (s *userStore) GetState(ctx context.Context, botID string, userID int64) (json.RawMessage, error) {
	id, err := uuid.Parse(botID)
	if err != nil {
		return nil, err
	}
	u, err := s.repo.GetByID(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	return u.State, nil
}

func (s *userStore) SetState(ctx context.Context, botID string, userID int64, state json.RawMessage) error {
	id, err := uuid.Parse(botID)
	if err != nil {
		return err
	}
	return s.repo.SetState(ctx, id, userID, state)
}

type eventStore struct{ repo *postgres.EventRepo }

func (s *eventStore) Insert(ctx context.Context, botID string, userID int64, kind string, text *string, meta []byte) error {
	id, err := uuid.Parse(botID)
	if err != nil {
		return err
	}
	return s.repo.Insert(ctx, id, userID, kind, text, meta)
}

type configStore struct{ repo *postgres.ConfigRepo }

func (s *configStore) GetMerged(ctx context.Context, botID string, tpl engine.Template) (map[string]any, map[string]any, map[string]any, error) {
	id, err := uuid.Parse(botID)
	if err != nil {
		return nil, nil, nil, err
	}
	var cfgTexts, cfgMenus, cfgPrices json.RawMessage
	cfg, err := s.repo.Get(ctx, id)
	if err == nil {
		cfgTexts, cfgMenus, cfgPrices = cfg.Texts, cfg.Menus, cfg.Prices
	}
	texts := mergeJSON(tpl.DefaultTexts(), cfgTexts)
	scenesRoot := mergeJSON(tpl.DefaultMenus(), cfgMenus)
	scenes, _ := scenesRoot["scenes"].(map[string]any)
	if scenes == nil {
		scenes = map[string]any{}
	}
	var defaultPrices []byte
	if priced, ok := tpl.(engine.Priced); ok {
		defaultPrices = priced.DefaultPrices()
	}
	prices := mergeJSON(defaultPrices, cfgPrices)
	return texts, scenes, prices, nil
}

// mergeJSON deep-merges override on top of base (both raw JSON objects).
func mergeJSON(base, override []byte) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(base, &out)
	if len(override) > 0 {
		var ov map[string]any
		if json.Unmarshal(override, &ov) == nil {
			deepMerge(out, ov)
		}
	}
	return out
}

func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k].(map[string]any); ok {
				deepMerge(dv, sv)
				continue
			}
			dst[k] = sv
			continue
		}
		dst[k] = v
	}
}

// --- Telegram sender ---

type TelegramSender struct{ client *telegram.Client }

// Keep the sender's optional capabilities honest: if one of these assertions
// breaks, the matching feature silently degrades to a text fallback in the bot.
var (
	_ engine.Sender         = (*TelegramSender)(nil)
	_ engine.PhotoSender    = (*TelegramSender)(nil)
	_ engine.MediaForwarder = (*TelegramSender)(nil)
)

func (s *TelegramSender) Send(ctx context.Context, chatID int64, text string, kbd json.RawMessage, replyTo int64) (*telegram.Message, error) {
	return s.client.SendMessage(ctx, telegram.SendMessagePayload{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: kbd,
		ReplyTo:     replyTo,
	})
}

func (s *TelegramSender) SendPhoto(ctx context.Context, chatID int64, fileID, caption string, kbd json.RawMessage) (*telegram.Message, error) {
	return s.client.SendPhoto(ctx, telegram.SendPhotoPayload{
		ChatID:      chatID,
		Photo:       fileID,
		Caption:     caption,
		ReplyMarkup: kbd,
	})
}

func (s *TelegramSender) Edit(ctx context.Context, chatID, msgID int64, text string, kbd json.RawMessage) error {
	_, err := s.client.EditMessageText(ctx, telegram.EditMessageTextPayload{
		ChatID: chatID, MessageID: msgID, Text: text, ReplyMarkup: kbd,
	})
	return err
}

// EditCaption satisfies engine.CaptionEditor so photo browse cards can be
// closed in place (editMessageText is rejected for media messages).
func (s *TelegramSender) EditCaption(ctx context.Context, chatID, msgID int64, text string, kbd json.RawMessage) error {
	_, err := s.client.EditMessageCaption(ctx, telegram.EditMessageCaptionPayload{
		ChatID: chatID, MessageID: msgID, Caption: text, ReplyMarkup: kbd,
	})
	return err
}

// ForwardMedia satisfies engine.MediaForwarder so the dating chat can relay
// photos, stickers, voice notes, videos and files between matched users.
func (s *TelegramSender) ForwardMedia(ctx context.Context, fromChatID, toChatID, messageID int64) (*telegram.Message, error) {
	return s.client.ForwardMedia(ctx, fromChatID, toChatID, messageID)
}

// SendChatAction shows a typing/chat-action indicator (best-effort; the Kat
// subset lacks the method and simply errors).
func (s *TelegramSender) SendChatAction(ctx context.Context, chatID int64, action string) error {
	return s.client.SendChatAction(ctx, chatID, action)
}

func (s *TelegramSender) AnswerCallback(ctx context.Context, callbackID, text string) error {
	return s.client.AnswerCallbackQuery(ctx, telegram.AnswerCallbackPayload{
		CallbackQueryID: callbackID, Text: text,
	})
}

// --- bot runtime manager ---

type StatusReporter interface {
	SetStatus(ctx context.Context, botID uuid.UUID, status string, lastError *string) error
}

// botKey identifies one platform runtime of a bot — a bot serves Telegram
// and Kat simultaneously, each with its own poller and client.
type botKey struct {
	botID    uuid.UUID
	platform string
}

// Manager owns every platform runtime of every bot in one process.
//
// Degradation contract:
//   - a failing side tears only itself down; siblings keep serving,
//   - a bot's DB status goes to error only when nothing serves it, and it
//     stays repairable (dead sides retry via Reconcile), and
//   - bots stay resumable: since a deploy never rewrites status while a bot
//     is merely degraded, StartRunningBots picks it up on the next boot.
//
// Run(ctx) periodically calls Reconcile, which retries dead sides.
type Manager struct {
	mu       sync.Mutex
	runtimes map[botKey]*botRuntime
	// failed holds sides held down because only a new token can fix them
	// (401/decrypt). Set at start/teardown, cleared only by Start() — which
	// is how a panel-driven retry opts back in. Transient failures live here
	// only as absence: the poller's backoff or Reconcile retries them.
	failed map[botKey]struct{}
	// booting counts in-flight Start() calls per bot; deferred carries a
	// pending "everything is down" verdict whose flip waits until the last
	// Start() returns (see finishBoot). Both guard m.mu.
	booting  map[uuid.UUID]int
	deferred map[uuid.UUID]string
	engine   *engine.Engine
	bots     *postgres.BotRepo
	users    *postgres.BotUserRepo
	status   StatusReporter
}

func NewManager(eng *engine.Engine, bots *postgres.BotRepo, users *postgres.BotUserRepo, status StatusReporter) *Manager {
	return &Manager{
		runtimes: map[botKey]*botRuntime{},
		failed:   map[botKey]struct{}{},
		booting:  map[uuid.UUID]int{},
		deferred: map[uuid.UUID]string{},
		engine:   eng,
		bots:     bots,
		users:    users,
		status:   status,
	}
}

func (m *Manager) StartRunningBots(ctx context.Context) error {
	bots, err := m.bots.List(ctx)
	if err != nil {
		return err
	}
	for i := range bots {
		b := bots[i]
		if b.Status == domain.BotStatusRunning || b.Status == domain.BotStatusStarting {
			if err := m.Start(ctx, &b); err != nil {
				log.Printf("[runtime] start bot %s: %v", b.Name, err)
			}
		}
	}
	return nil
}

// Start brings up both sides of a bot and is strictly best-effort per side:
// a broken half must not abort the healthy half — one side failing logs
// instead of returning an error, while an all-broken Start reports it to the
// caller (the panel button). m.booting brackets the whole call so an
// "all sides down" verdict racing a concurrent Start cannot flip status
// out from under a sibling that is seconds from landing (finishBoot settles
// it exactly once).
func (m *Manager) Start(ctx context.Context, bot *domain.Bot) error {
	m.mu.Lock()
	m.booting[bot.ID]++
	delete(m.deferred, bot.ID) // a verdict from an earlier boot is superseded
	m.mu.Unlock()

	err := m.startLocked(ctx, bot)
	m.finishBoot(bot)
	return err
}

// startLocked attempts both configured sides; the caller must already count
// this Start in m.booting.
func (m *Manager) startLocked(ctx context.Context, bot *domain.Bot) error {
	var errs []string
	if err := m.startPlatform(ctx, bot, domain.PlatformTelegram); err != nil {
		errs = append(errs, err.Error())
	}
	if len(bot.KatTokenEncrypted) > 0 {
		if err := m.startPlatform(ctx, bot, domain.PlatformKat); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	if len(errs) < len(desiredPlatforms(bot)) {
		// some side is serving; keep the "running" key (reconcile keeps going)
		log.Printf("[runtime] bot %s: partial start: %s", bot.Name, strings.Join(errs, "; "))
		return nil
	}
	return errors.New(strings.Join(errs, "; "))
}

// finishBoot retires this Start's counter and settles any verdict deferred by
// handlePlatformDown: only when no Start of this bot remains and nothing
// serves it does the parked error text become the DB status — decided under
// m.mu, applied outside it.
func (m *Manager) finishBoot(bot *domain.Bot) {
	m.mu.Lock()
	if m.booting[bot.ID] > 0 {
		m.booting[bot.ID]--
	}
	verdict := ""
	if msg, ok := m.deferred[bot.ID]; ok && m.booting[bot.ID] == 0 {
		down := true
		for _, p := range desiredPlatforms(bot) {
			if _, exists := m.runtimes[botKey{botID: bot.ID, platform: p}]; exists {
				down = false
				break
			}
		}
		if down {
			verdict = msg
		}
		delete(m.deferred, bot.ID)
	}
	m.mu.Unlock()

	if verdict != "" {
		_ = m.status.SetStatus(context.Background(), bot.ID, domain.BotStatusError, &verdict)
		log.Printf("[runtime] bot %s: all sides down after boot (%s)", bot.Name, verdict)
	}
}

func (m *Manager) startPlatform(ctx context.Context, bot *domain.Bot, platform string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := botKey{botID: bot.ID, platform: platform}
	if _, exists := m.runtimes[key]; exists {
		return nil
	}
	if _, held := m.failed[key]; held {
		return fmt.Errorf("start %s: token held down (needs a new token via panel start)", platform)
	}
	token, err := decryptToken(bot, platform)
	if err != nil {
		// a key the process cannot read at all — not worth hammering
		m.failed[key] = struct{}{}
		return fmt.Errorf("start %s: %w", platform, err)
	}
	base := telegram.BaseTelegram
	if platform == domain.PlatformKat {
		base = telegram.BaseKat
	}
	client := telegram.NewClientWithBase(base, token)
	if _, err := client.GetMe(ctx); err != nil {
		if isPermanentTokenErr(err) {
			m.failed[key] = struct{}{} // only an interactive Start() clears this
		}
		return fmt.Errorf("start %s: %w", platform, err)
	}

	rtCtx, cancel := context.WithCancel(context.Background())
	rt := &botRuntime{
		bot:      *bot,
		platform: platform,
		client:   client,
		cancel:   cancel,
		out:      make(chan outgoing, 256),
		updates:  make(chan telegram.Update, 64),
	}
	rt.wg.Add(3)
	go m.pollLoop(rtCtx, rt)
	go m.workLoop(rtCtx, rt)
	go m.outLoop(rtCtx, rt)

	m.runtimes[key] = rt
	_ = m.status.SetStatus(ctx, bot.ID, domain.BotStatusRunning, nil)
	log.Printf("[runtime] bot %s (@%s) running on %s", bot.Name, deref(bot.TelegramUsername), platform)
	return nil
}

func (m *Manager) Stop(ctx context.Context, botID uuid.UUID) error {
	m.mu.Lock()
	stopped := false
	for key, rt := range m.runtimes {
		if key.botID == botID {
			delete(m.runtimes, key)
			rt.cancel()
			stopped = true
		}
	}
	m.mu.Unlock()
	_ = m.status.SetStatus(ctx, botID, domain.BotStatusStopped, nil)
	if stopped {
		log.Printf("[runtime] bot %s stopped", botID)
	}
	return nil
}

func (m *Manager) StopAll(ctx context.Context) {
	m.mu.Lock()
	rts := make([]*botRuntime, 0, len(m.runtimes))
	for key := range m.runtimes {
		rts = append(rts, m.runtimes[key])
		delete(m.runtimes, key)
	}
	m.mu.Unlock()
	for _, rt := range rts {
		// halt the pollers without touching DB status: a process restart
		// (deploy) must not mark bots stopped, or the next boot's
		// StartRunningBots skips them and the whole fleet silently dies.
		rt.cancel()
	}
}

// PlatformRunning reports whether a bot's side is currently serving — the
// matcher uses it to gate cross-platform claims, and Outbound uses it to pick
// a deliverable path.
func (m *Manager) PlatformRunning(botID uuid.UUID, platform string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.runtimes[botKey{botID: botID, platform: platform}]
	return ok
}

func (m *Manager) IsRunning(botID uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.runtimes {
		if key.botID == botID {
			return true
		}
	}
	return false
}

// ClientFor exposes a running bot's Telegram client (for matcher notifications).
func (m *Manager) ClientFor(botID uuid.UUID) (*telegram.Client, bool) {
	return m.ClientForPlatform(botID, domain.PlatformTelegram)
}

// ClientForPlatform exposes the client of one platform runtime.
func (m *Manager) ClientForPlatform(botID uuid.UUID, platform string) (*telegram.Client, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, ok := m.runtimes[botKey{botID: botID, platform: platform}]
	if !ok {
		return nil, false
	}
	return rt.client, true
}

// SendPlatform implements engine.Router: deliver to a chat on any platform
// this bot serves (cross-platform chat relay / notifications).
func (m *Manager) SendPlatform(ctx context.Context, botID uuid.UUID, platform string, chatID int64, text string, kbd json.RawMessage, replyTo int64) (*telegram.Message, error) {
	client, ok := m.ClientForPlatform(botID, platform)
	if !ok {
		return nil, fmt.Errorf("%w: bot %s on %s", engine.ErrPlatformDown, botID, platform)
	}
	return client.SendMessage(ctx, telegram.SendMessagePayload{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: kbd,
		ReplyTo:     replyTo,
	})
}

// ForwardMediaPlatform implements engine.Router for media relays.
func (m *Manager) ForwardMediaPlatform(ctx context.Context, botID uuid.UUID, platform string, fromChatID, toChatID, messageID int64) (*telegram.Message, error) {
	client, ok := m.ClientForPlatform(botID, platform)
	if !ok {
		return nil, fmt.Errorf("%w: bot %s on %s", engine.ErrPlatformDown, botID, platform)
	}
	return client.ForwardMedia(ctx, fromChatID, toChatID, messageID)
}

type outgoing struct {
	chatID int64
	text   string
	kbd    json.RawMessage
}

type botRuntime struct {
	bot      domain.Bot
	platform string
	client   *telegram.Client
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	out      chan outgoing
	updates  chan telegram.Update
}

// isPollHold reports whether the error means the endpoint may still be holding
// our previous getUpdates (client-side deadline, or the 409 CONFLICT that a
// second concurrent getUpdates gets).
func isPollHold(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *telegram.APIError
	return errors.As(err, &apiErr) && apiErr.Code == 409
}

// isPermanentTokenErr classifies a start-time failure: a 401 can only be fixed
// by handing in a new token, so it is worth remembering (and worth not
// hammering on every reconcile tick). Everything else — DNS, TLS, timeouts —
// is transient and deserves a retry.
func isPermanentTokenErr(err error) bool {
	var apiErr *telegram.APIError
	return errors.As(err, &apiErr) && apiErr.IsAuthError()
}

// desiredPlatforms lists the platforms a bot is configured to serve.
func desiredPlatforms(bot *domain.Bot) []string {
	ps := []string{domain.PlatformTelegram}
	if len(bot.KatTokenEncrypted) > 0 {
		ps = append(ps, domain.PlatformKat)
	}
	return ps
}

// Reconcile retries sides of service bots that are missing but not held down —
// what recovers a side whose Start failed transiently (endpoint down at boot
// as it settles on its own). Sides in m.failed stay parked: only an operator
// START raises them, and only a NEW token fixes their cause anyway.
func (m *Manager) Reconcile(ctx context.Context) {
	bots, err := m.bots.List(ctx)
	if err != nil {
		log.Printf("[runtime] reconcile list: %v", err)
		return
	}
	for i := range bots {
		b := bots[i]
		if b.Status != domain.BotStatusRunning && b.Status != domain.BotStatusStarting {
			continue
		}
		for _, p := range desiredPlatforms(&b) {
			key := botKey{botID: b.ID, platform: p}
			m.mu.Lock()
			_, running := m.runtimes[key]
			_, held := m.failed[key]
			m.mu.Unlock()
			if running || held {
				continue
			}
			if err := m.startPlatform(ctx, &b, p); err != nil {
				log.Printf("[runtime] reconcile bot %s (%s): %v", b.Name, p, err)
			} else {
				log.Printf("[runtime] reconcile bot %s: %s side recovered", b.Name, p)
			}
		}
	}
}

// Run drives Reconcile for the life of the process. A dead endpoint usually
// settles on minutes — a keyed probe per missing side is cheap enough at 60s.
func (m *Manager) Run(ctx context.Context) {
	m.Reconcile(ctx) // catch anything Start left behind before the first tick
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Reconcile(ctx)
		}
	}
}

// handlePlatformDown tears down a side that reached "up" and went hard down
// mid-flight (dead token in the poller). Status inheritance: only when no
// side is left does the bot itself become error — a degraded bot keeps its
// running key so a redeploy still resumes it; the dead side parks in m.failed
// (Reconcile skips it; live "پس از خطا" style panel retries hit startPlatform
// which reports "held down" instead of pretending progress).
//
// Lock rule: pollLoop calls this with no lock held; the body's verdict runs
// under one critical section, then status flips outside it.
func (m *Manager) handlePlatformDown(ctx context.Context, rt *botRuntime, msg string) {
	m.mu.Lock()
	m.failed[botKey{botID: rt.bot.ID, platform: rt.platform}] = struct{}{}
	delete(m.runtimes, botKey{botID: rt.bot.ID, platform: rt.platform})
	rt.cancel()
	alive, deferred := false, false
	for _, p := range desiredPlatforms(&rt.bot) {
		if _, ok := m.runtimes[botKey{botID: rt.bot.ID, platform: p}]; ok {
			alive = true
			break
		}
	}
	if !alive {
		if m.booting[rt.bot.ID] > 0 {
			// a Start() is mid-flight here; that caller's finishBoot owes the flip
			m.deferred[rt.bot.ID] = msg
			deferred = true
		}
	}
	m.mu.Unlock()

	switch {
	case alive:
		log.Printf("[runtime] bot %s: %s side down (%s) — bot keeps serving on its peer",
			rt.bot.Name, rt.platform, msg)
	case deferred:
		log.Printf("[runtime] bot %s: every side down (%s) — flip deferred to boot end", rt.bot.Name, msg)
	default:
		_ = m.status.SetStatus(ctx, rt.bot.ID, domain.BotStatusError, &msg)
		log.Printf("[runtime] bot %s: every side down (%s)", rt.bot.Name, msg)
	}
}

func (m *Manager) pollLoop(ctx context.Context, rt *botRuntime) {
	defer rt.wg.Done()
	offset := int64(0)
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		// generous budget: a slow or stalled long-poll must outwait the
		// endpoint's own hold, or the abandoned poll 409s every retry
		pollTimeout := 30
		if rt.platform == domain.PlatformKat {
			// cr0.ir's poll wake lags behind new messages; keep the hold
			// short so a reply never waits out a full long-poll cycle
			pollTimeout = 2
		}
		pollCtx, cancel := context.WithTimeout(ctx, 65*time.Second)
		updates, err := rt.client.GetUpdates(pollCtx, offset, pollTimeout)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var apiErr *telegram.APIError
			if errors.As(err, &apiErr) && apiErr.IsAuthError() {
				// the side is done — dead token. The bot's DB status follows
				// only when every side is down (see handlePlatformDown).
				msg := "توکن ربات نامعتبر شد (401)"
				m.handlePlatformDown(context.Background(), rt, msg)
				return
			}
			wait := backoff
			// deadline/409: the server may still be holding our poll — give it
			// time to expire before retrying, or every retry 409s
			if isPollHold(err) && wait < 15*time.Second {
				wait = 15 * time.Second
			}
			log.Printf("[runtime] poll error bot=%s: %v (backoff %s)", rt.bot.Name, err, wait)
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		for _, u := range updates {
			offset = u.UpdateID + 1
			select {
			case rt.inbound() <- u:
			default:
				// channel full — process inline to avoid losing updates
				m.handleUpdate(ctx, rt, u)
			}
		}
	}
}

// inbound returns the shared work channel (created lazily via outLoop? no — separate).
// For simplicity the poller dispatches to the single worker via a buffered chan stored on rt.
func (rt *botRuntime) inbound() chan telegram.Update {
	return rt.updates
}

func (m *Manager) workLoop(ctx context.Context, rt *botRuntime) {
	defer rt.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case u := <-rt.updates:
			m.handleUpdate(ctx, rt, u)
		}
	}
}

func (m *Manager) handleUpdate(ctx context.Context, rt *botRuntime, u telegram.Update) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[runtime] panic handling update bot=%s: %v", rt.bot.Name, r)
		}
	}()
	sender := &TelegramSender{client: rt.client}
	m.engine.HandleUpdate(ctx, rt.platform, &rt.bot, sender, &u)
}

func (m *Manager) outLoop(ctx context.Context, rt *botRuntime) {
	defer rt.wg.Done()
	limiter := time.NewTicker(50 * time.Millisecond) // 20 msg/s per bot
	defer limiter.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-rt.out:
			<-limiter.C
			_, err := rt.client.SendMessage(context.Background(), telegram.SendMessagePayload{
				ChatID: msg.chatID, Text: msg.text, ReplyMarkup: msg.kbd,
			})
			if err != nil {
				var apiErr *telegram.APIError
				if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
					time.Sleep(time.Duration(apiErr.RetryAfter) * time.Second)
				}
				log.Printf("[runtime] send failed bot=%s chat=%d: %v", rt.bot.Name, msg.chatID, err)
			}
		}
	}
}

// token decryption helper (set by main via package-level func to avoid import cycle)
var decryptToken = func(bot *domain.Bot, platform string) (string, error) {
	return "", errors.New("decryptToken not configured")
}

func ConfigureTokenDecryptor(fn func(bot *domain.Bot, platform string) (string, error)) {
	decryptToken = fn
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
