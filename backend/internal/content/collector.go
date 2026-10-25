package content

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"sadbot/internal/domain"
	"sadbot/internal/llm"
	"sadbot/internal/tgsidecar"
)

// PostStore persists the dedup ledger (content_posts).
type PostStore interface {
	LastMessageID(ctx context.Context, botID uuid.UUID, chatID int64) (int64, error)
	InsertPost(ctx context.Context, p domain.ContentPost) error
}

// BotLister yields running bots of a template (implemented by postgres.BotRepo).
type BotLister interface {
	ListRunningByTemplate(ctx context.Context, templateKey string) ([]uuid.UUID, error)
}

// SidecarAPI is the MTProto surface the collector needs (implemented by
// *tgsidecar.Client; fakeable in tests).
type SidecarAPI interface {
	ListFolders(ctx context.Context, accountID string) ([]tgsidecar.Folder, error)
	ListChats(ctx context.Context, accountID string) ([]tgsidecar.Chat, error)
	ListMessages(ctx context.Context, accountID, chatID string, limit, offsetID int) ([]tgsidecar.Message, int, error)
	GetPhoto(ctx context.Context, accountID, chatID string, msgID int) (io.ReadCloser, string, error)
	SendPhotoMeta(ctx context.Context, accountID, chatID, imageB64, filename, caption string) (*tgsidecar.Message, error)
}

// LLMClient is the chat-completions surface (implemented by *llm.Client).
type LLMClient interface {
	Chat(ctx context.Context, system string, history []llm.Message) (string, error)
}

// Publisher sends rendered PNGs (or a text fallback) to the destination
// channel on behalf of the bot. Implemented in main.go over runtime.ClientFor.
type Publisher func(ctx context.Context, botID uuid.UUID, channel, caption string, photos [][]byte, filenameBase string) error

// PublisherViaMeta publishes through the MTProto sidecar account of a content
// bot (sadcontent's telethon path) instead of the running bot's Bot API —
// for destination channels the bot is not admin of.
type PublisherViaMeta func(ctx context.Context, botID uuid.UUID, accountID, channel, caption string, photos [][]byte, filenameBase string) error

// Collector runs the global content loop: for every running "content" bot,
// poll the channels of its configured Telegram folder, clean with the LLM,
// render cards and publish (sadcontent's process_message pipeline).
type Collector struct {
	store      PostStore
	bots       BotLister
	sidecar    SidecarAPI
	llm        LLMClient
	renderer   RendererAPI
	publish    Publisher
	publishVia PublisherViaMeta // MTProto path when content_publish_via=sidecar
	pricesOf   func(uuid.UUID) map[string]any
	textsOf    func(uuid.UUID) map[string]any

	interval time.Duration
	pacing   time.Duration // gap between processed messages
	lastPoll map[uuid.UUID]time.Time
}

func NewCollector(store PostStore, bots BotLister, sidecar SidecarAPI, llmClient LLMClient, renderer RendererAPI, publish Publisher) *Collector {
	return &Collector{
		store: store, bots: bots, sidecar: sidecar, llm: llmClient,
		renderer: renderer, publish: publish,
		interval: 30 * time.Second,
		pacing:   1500 * time.Millisecond,
		lastPoll: map[uuid.UUID]time.Time{},
	}
}

// SetPricesFunc injects per-bot merged prices (content config).
func (c *Collector) SetPricesFunc(fn func(uuid.UUID) map[string]any) { c.pricesOf = fn }

// SetTextsFunc injects per-bot merged texts (editable prompts).
func (c *Collector) SetTextsFunc(fn func(uuid.UUID) map[string]any) { c.textsOf = fn }

// SetPublisherViaMeta injects the MTProto (sidecar account) publish path —
// used when content_publish_via = "sidecar".
func (c *Collector) SetPublisherViaMeta(fn PublisherViaMeta) { c.publishVia = fn }

func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Tick(ctx)
		}
	}
}

func (c *Collector) Tick(ctx context.Context) {
	botIDs, err := c.bots.ListRunningByTemplate(ctx, domain.TemplateContent)
	if err != nil {
		log.Printf("[content] list bots: %v", err)
		return
	}
	for _, botID := range botIDs {
		if !c.due(botID) {
			continue
		}
		c.lastPoll[botID] = time.Now()
		c.tickBot(ctx, botID)
	}
}

// due respects the per-bot poll interval (content_poll_minutes, default 2).
func (c *Collector) due(botID uuid.UUID) bool {
	last, ok := c.lastPoll[botID]
	if !ok {
		return true
	}
	minutes := intOf(c.prices(botID)["content_poll_minutes"], 2)
	if minutes < 1 {
		minutes = 1
	}
	return time.Since(last) >= time.Duration(minutes)*time.Minute
}

func (c *Collector) prices(botID uuid.UUID) map[string]any {
	if c.pricesOf == nil {
		return map[string]any{}
	}
	return c.pricesOf(botID)
}

func (c *Collector) text(botID uuid.UUID, key, def string) string {
	if c.textsOf == nil {
		return def
	}
	if v, ok := c.textsOf(botID)[key].(string); ok && v != "" {
		return v
	}
	return def
}

// tickBot runs one collection pass for one bot.
func (c *Collector) tickBot(ctx context.Context, botID uuid.UUID) {
	if c.sidecar == nil || c.publish == nil {
		return
	}
	prices := c.prices(botID)
	accountID := strOf(prices["content_account_id"])
	channel := strOf(prices["content_channel"])
	folderName := strOf(prices["content_folder"])
	if accountID == "" || channel == "" {
		return // not configured yet
	}
	if folderName == "" {
		log.Printf("[content] bot %s: no content_folder configured — fail-closed", botID)
		return
	}

	sources, err := c.resolveSources(ctx, accountID, folderName, channel, strSliceOf(prices["content_exclude"]))
	if err != nil {
		log.Printf("[content] bot %s: resolve sources: %v", botID, err)
		return
	}
	if len(sources) == 0 {
		log.Printf("[content] bot %s: folder %q not found or empty — fail-closed", botID, folderName)
		return
	}

	backfillLimit := intOf(prices["content_backfill_limit"], 20)
	for _, src := range sources {
		if err := c.pollSource(ctx, botID, accountID, channel, src, backfillLimit); err != nil {
			log.Printf("[content] bot %s: poll %s: %v", botID, src.ChatID, err)
		}
	}
}

// sourceChannel is one resolved channel of the folder.
type sourceChannel struct {
	ChatID   string
	Title    string
	Username string
}

// resolveSources finds the folder's channels (fail-closed when missing) and
// excludes the destination channel plus content_exclude entries.
func (c *Collector) resolveSources(ctx context.Context, accountID, folderName, channel string, exclude []string) ([]sourceChannel, error) {
	folders, err := c.sidecar.ListFolders(ctx, accountID)
	if err != nil {
		return nil, err
	}
	want := normName(folderName)
	var peers []tgsidecar.FolderPeer
	found := false
	for _, f := range folders {
		if normName(f.Name) == want {
			peers = f.Peers
			found = true
			break
		}
	}
	if !found {
		return nil, nil // fail-closed: folder missing → nothing collected
	}
	excl := map[string]bool{normName(strings.TrimPrefix(channel, "@")): true}
	for _, e := range exclude {
		excl[normName(e)] = true
	}
	out := make([]sourceChannel, 0, len(peers))
	for _, p := range peers {
		if p.Type != "channel" && p.Type != "group" {
			continue
		}
		// destination may be configured by @username, title, or numeric chat_id
		if excl[normName(p.Username)] || excl[normName(p.Title)] || excl[p.ChatID] {
			continue
		}
		out = append(out, sourceChannel{ChatID: p.ChatID, Title: p.Title, Username: p.Username})
	}
	return out, nil
}

// pollSource fetches new messages of one source channel and processes them.
func (c *Collector) pollSource(ctx context.Context, botID uuid.UUID, accountID, channel string, src sourceChannel, backfillLimit int) error {
	msgs, _, err := c.sidecar.ListMessages(ctx, accountID, src.ChatID, 50, 0)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}
	// sidecar returns oldest→newest
	lastID, err := c.store.LastMessageID(ctx, botID, int64Of(src.ChatID))
	if err != nil {
		return err
	}
	newMsgs := msgs
	if lastID == 0 {
		// first run for this source: only the most recent backfillLimit, oldest-first
		if len(msgs) > backfillLimit {
			newMsgs = msgs[len(msgs)-backfillLimit:]
		}
	} else {
		var filtered []tgsidecar.Message
		for _, m := range msgs {
			if int64(m.ID) > lastID {
				filtered = append(filtered, m)
			}
		}
		newMsgs = filtered
	}
	for _, m := range newMsgs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.processMessage(ctx, botID, accountID, channel, src, m)
		time.Sleep(c.pacing) // pacing, faithful to sadcontent backfill
	}
	return nil
}

// processMessage is the per-message pipeline: filter → AI clean → summary →
// render → publish → record.
func (c *Collector) processMessage(ctx context.Context, botID uuid.UUID, accountID, channel string, src sourceChannel, m tgsidecar.Message) {
	prices := c.prices(botID)
	post := domain.ContentPost{
		BotID:        botID,
		SourceChatID: int64Of(src.ChatID),
		MessageID:    int64(m.ID),
		SourceTitle:  strPtr(src.Title),
		SourceLink:   strPtr(sourceLink(src, m.ID)),
	}
	raw := strings.TrimSpace(m.Text)
	post.RawText = strPtr(raw)

	if !passesFilter(m, raw, prices) {
		return // filtered: not recorded (re-evaluated if filters change, like sadcontent)
	}

	// AI clean (fallback to raw text on LLM errors; drop on NO_CONTENT)
	cleaned := raw
	if raw != "" {
		out, drop := c.aiClean(ctx, botID, raw, prices)
		if drop {
			post.Status = domain.ContentStatusNoContent
			_ = c.store.InsertPost(ctx, post)
			return
		}
		cleaned = out
	}
	cleaned = stripSourceCredits(cleaned)
	if cleaned == "" && m.Media.Kind == "none" {
		return // nothing to publish
	}
	brand := brandOf(prices)
	cleaned = stripForeignMentions(cleaned, brand.Handle)
	if cleaned == "" && m.Media.Kind == "none" {
		return
	}
	post.CleanedText = strPtr(cleaned)

	caption := cleaned
	if len(caption) > 1000 {
		caption = caption[:1000]
	}
	filenameBase := fmt.Sprintf("%s_%d", brand.Key, m.ID)

	// publish path: MTProto account (sadcontent's telethon flow) or the
	// running bot, selected per content_publish_via from prices
	viaMeta := strOf(prices["content_publish_via"]) == "sidecar" && c.publishVia != nil && accountID != ""
	send := func(photos [][]byte, textCaption string) error {
		if viaMeta {
			return c.publishVia(ctx, botID, accountID, channel, textCaption, photos, filenameBase)
		}
		return c.publish(ctx, botID, channel, textCaption, photos, filenameBase)
	}

	// text-only bots (no cards): plain messages, published status
	if boolOf(prices["content_text_only"], false) {
		// deterministic ad gate: contests / VIP notices / price lists never ship,
		// even when the model keeps them (or rewords them just enough to slip past)
		if looksLikePromo(cleaned) || looksLikePromo(raw) {
			log.Printf("[content] bot %s: promo/ad dropped (msg %d)", botID, m.ID)
			post.Status = domain.ContentStatusNoContent
			_ = c.store.InsertPost(ctx, post)
			return
		}
		text := cleaned
		if len(text) > 4000 {
			text = text[:4000]
		}
		// text posts allow 4096 chars — append the brand handle AFTER the
		// trim so config blocks can't push the ad line off the end
		if h := brand.Handle; h != "" && !strings.Contains(text, h) {
			if text != "" {
				text += "\n\n"
			}
			text += h + " ✅ آپدیت‌های روز را دنبال کنید"
		}
		if text == "" {
			return
		}
		if err := send(nil, text); err != nil {
			post.Status = domain.ContentStatusFailed
			post.Error = strPtr(err.Error())
		} else {
			post.Status = domain.ContentStatusPublished
		}
		_ = c.store.InsertPost(ctx, post)
		return
	}

	// card summary (errors → full cleaned text, like sadcontent)
	cardText := cleaned
	if boolOf(prices["render_card_summary"], true) && cleaned != "" {
		if summary, ok := c.aiSummary(ctx, botID, cleaned); ok {
			cardText = summary
		}
	}

	// source photo (sidecar serves JPEG)
	var imageB64, imageMime string
	if m.Media.Kind == "photo" {
		if b64, mime, err := c.fetchPhoto(ctx, accountID, src.ChatID, m.ID); err == nil {
			imageB64, imageMime = b64, mime
		} else {
			log.Printf("[content] bot %s: photo %d/%s: %v", botID, m.ID, src.ChatID, err)
		}
	}

	// render post + story
	var pngs [][]byte
	renderErr := error(nil)
	if c.renderer != nil && cardText != "" {
		for _, fmtName := range []string{"post", "story"} {
			png, err := c.renderer.Render(ctx, RenderRequest{
				Fmt: fmtName, Text: cardText,
				ImageB64: imageB64, ImageMime: imageMime,
				Brand: brand,
			})
			if err != nil {
				renderErr = err
				break
			}
			pngs = append(pngs, png)
		}
	}

	if renderErr != nil || len(pngs) == 0 {
		// fallback: plain text to the channel (sadcontent render-failure path)
		if caption == "" {
			return
		}
		if err := send(nil, caption); err != nil {
			post.Status = domain.ContentStatusFailed
			post.Error = strPtr(err.Error())
		} else {
			post.Status = domain.ContentStatusRenderFailed // text sent, cards skipped
			if renderErr != nil {
				post.Error = strPtr(renderErr.Error())
			}
		}
		_ = c.store.InsertPost(ctx, post)
		return
	}

	if err := send(pngs, caption); err != nil {
		post.Status = domain.ContentStatusFailed
		post.Error = strPtr(err.Error())
	} else {
		post.Status = domain.ContentStatusPublished
	}
	_ = c.store.InsertPost(ctx, post)
}

// aiClean runs the CLEAN_SYSTEM prompt with the sadcontent guards.
// drop=true means the post is an ad / unusable (NO_CONTENT path).
func (c *Collector) aiClean(ctx context.Context, botID uuid.UUID, text string, prices map[string]any) (string, bool) {
	if c.llm == nil || len(strings.TrimSpace(text)) < 30 {
		return text, false // too short: never send to the model; publish as-is
	}
	if disabled, ok := prices["ai_enabled"].(bool); ok && !disabled {
		return text, false
	}
	prompt := c.text(botID, "clean_prompt", CleanSystem)
	in := text
	if len(in) > 4000 {
		in = in[:4000]
	}
	out, err := c.llm.Chat(ctx, prompt, []llm.Message{{Role: "user", Content: in}})
	if err != nil {
		log.Printf("[content] bot %s: ai clean: %v (publishing raw)", botID, err)
		return text, false // fallback: raw text
	}
	out = strings.TrimSpace(out)
	if out == "" || strings.ToUpper(out) == "NO_CONTENT" {
		return "", true
	}
	if containsAny(out, conversationalMarkers) {
		log.Printf("[content] bot %s: ai returned conversational reply — dropped", botID)
		return "", true
	}
	if len(out) < 20 {
		return "", true
	}
	return out, false
}

// aiSummary runs the CARD_SYSTEM prompt. ok=false → caller uses full text.
func (c *Collector) aiSummary(ctx context.Context, botID uuid.UUID, text string) (string, bool) {
	if c.llm == nil || len(strings.TrimSpace(text)) < 40 {
		return "", false
	}
	prompt := c.text(botID, "card_prompt", CardSystem)
	in := text
	if len(in) > 3000 {
		in = in[:3000]
	}
	out, err := c.llm.Chat(ctx, prompt, []llm.Message{{Role: "user", Content: in}})
	if err != nil {
		return "", false
	}
	out = strings.TrimSpace(out)
	if out == "" || containsAny(out, cardConversationalMarkers) {
		return "", false
	}
	return out, true
}

func (c *Collector) fetchPhoto(ctx context.Context, accountID, chatID string, msgID int) (string, string, error) {
	rc, mime, err := c.sidecar.GetPhoto(ctx, accountID, chatID, msgID)
	if err != nil {
		return "", "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 8<<20))
	if err != nil {
		return "", "", err
	}
	if mime == "" {
		mime = "image/jpeg"
	}
	return base64.StdEncoding.EncodeToString(data), mime, nil
}

// passesFilter ports sadcontent's passes_filter against sidecar message data.
func passesFilter(m tgsidecar.Message, text string, prices map[string]any) bool {
	// service messages have no text and no media — nothing to collect
	if text == "" && m.Media.Kind == "none" {
		return false
	}
	if boolOf(prices["content_skip_forwarded"], false) && m.Forwarded {
		return false
	}
	if boolOf(prices["content_skip_replies"], false) && m.ReplyToID != nil {
		return false
	}
	minLen := intOf(prices["content_min_length"], 40)
	if len([]rune(strings.TrimSpace(text))) < minLen {
		return false
	}
	if kws, ok := prices["content_keywords"].([]any); ok && len(kws) > 0 {
		lower := strings.ToLower(text)
		matched := false
		for _, k := range kws {
			if kw, ok := k.(string); ok && kw != "" && strings.Contains(lower, strings.ToLower(kw)) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func brandOf(prices map[string]any) Brand {
	key := strOf(prices["brand_key"])
	if key == "" {
		key = "brand"
	}
	return Brand{
		Key:    key,
		NameEN: strOf(prices["brand_name_en"]),
		NameFA: strOf(prices["brand_name_fa"]),
		Handle: strOf(prices["brand_handle"]),
		GlowA:  strOf(prices["brand_glow_a"]),
		GlowB:  strOf(prices["brand_glow_b"]),
		GlowC:  strOf(prices["brand_glow_c"]),
		Accent: strOf(prices["brand_accent"]),
	}
}

func sourceLink(src sourceChannel, msgID int) string {
	if src.Username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", src.Username, msgID)
	}
	// private channel: t.me/c/<bare id>/<msg>
	bare := strings.TrimPrefix(src.ChatID, "-100")
	return fmt.Sprintf("https://t.me/c/%s/%d", bare, msgID)
}

// normName mirrors sadcontent's norm(): strip @, lowercase.
func normName(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "@")))
}

func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intOf(v any, def int) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return def
	}
}

func int64Of(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func boolOf(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

// strSliceOf reads a []any of strings from merged JSON (e.g. content_exclude).
func strSliceOf(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
