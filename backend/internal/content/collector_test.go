package content

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"sadbot/internal/domain"
	"sadbot/internal/llm"
	"sadbot/internal/tgsidecar"
)

// ---- fakes ----

type fakeStore struct {
	lastID  map[int64]int64
	posts   []domain.ContentPost
	seenKey map[string]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{lastID: map[int64]int64{}, seenKey: map[string]bool{}}
}

func (f *fakeStore) LastMessageID(_ context.Context, _ uuid.UUID, chatID int64) (int64, error) {
	return f.lastID[chatID], nil
}

func (f *fakeStore) InsertPost(_ context.Context, p domain.ContentPost) error {
	key := fmt.Sprintf("%d_%d", p.SourceChatID, p.MessageID)
	if !f.seenKey[key] {
		f.seenKey[key] = true
		f.posts = append(f.posts, p)
	}
	return nil
}

type fakeBots struct{ ids []uuid.UUID }

func (f *fakeBots) ListRunningByTemplate(_ context.Context, _ string) ([]uuid.UUID, error) {
	return f.ids, nil
}

type fakeSidecar struct {
	folders   []tgsidecar.Folder
	messages  map[string][]tgsidecar.Message // chatID → msgs (oldest→newest)
	photoData []byte
	folderErr error
	sentMeta  []publishCall // SendPhotoMeta captures (MTProto publish path)
}

func (f *fakeSidecar) ListFolders(_ context.Context, _ string) ([]tgsidecar.Folder, error) {
	return f.folders, f.folderErr
}

func (f *fakeSidecar) ListChats(_ context.Context, _ string) ([]tgsidecar.Chat, error) {
	return nil, nil
}

func (f *fakeSidecar) ListMessages(_ context.Context, _, chatID string, _, _ int) ([]tgsidecar.Message, int, error) {
	return f.messages[chatID], 0, nil
}

func (f *fakeSidecar) GetPhoto(_ context.Context, _, _ string, _ int) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader(string(f.photoData))), "image/jpeg", nil
}

// SendPhotoMeta is only wired in sidecar-publish tests; each call here is one
// photo (the collector ships exactly one post card).
func (f *fakeSidecar) SendPhotoMeta(_ context.Context, _, chatID, _, _, caption string) (*tgsidecar.Message, error) {
	f.sentMeta = append(f.sentMeta, publishCall{channel: chatID, caption: caption, numPngs: 1})
	return &tgsidecar.Message{ID: 99}, nil
}

type fakeLLM struct {
	replies  []string
	calls    int
	replyErr error
}

func (f *fakeLLM) Chat(_ context.Context, _ string, _ []llm.Message) (string, error) {
	if f.replyErr != nil {
		return "", f.replyErr
	}
	if f.calls < len(f.replies) {
		r := f.replies[f.calls]
		f.calls++
		return r, nil
	}
	return "متن پاک‌سازی‌شدهٔ آزمایشی از خبر بازار.", nil
}

type fakeRenderer struct {
	calls []RenderRequest
	err   error
}

func (f *fakeRenderer) Render(_ context.Context, req RenderRequest) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.calls = append(f.calls, req)
	return []byte("png-" + req.Fmt), nil
}

type publishCall struct {
	channel string
	caption string
	numPngs int
}

type fakePublisher struct {
	calls []publishCall
	err   error
}

func (f *fakePublisher) publish(_ context.Context, _ uuid.UUID, channel, caption string, photos [][]byte, _ string) error {
	f.calls = append(f.calls, publishCall{channel: channel, caption: caption, numPngs: len(photos)})
	return f.err
}

// ---- helpers ----

func longPersianNews() string {
	return "بیت‌کوین امروز با رشد ۴ درصدی به ۶۸۵۰۰ دلار رسید. تحلیل‌گران مقاومت ۷۰ هزار دلار را هدف بعدی می‌دانند. حجم معاملات نسبت به دیروز ۲۰ درصد افزایش یافته است."
}

func newTestCollector(t *testing.T) (*Collector, *fakeStore, *fakeSidecar, *fakeLLM, *fakeRenderer, *fakePublisher, uuid.UUID) {
	t.Helper()
	botID := uuid.New()
	store := newFakeStore()
	side := &fakeSidecar{
		folders: []tgsidecar.Folder{
			{Name: "Crypto", Peers: []tgsidecar.FolderPeer{
				{ChatID: "-100111", Title: "اخبار کریپتو", Username: "cryptonews", Type: "channel"},
				{ChatID: "-100999", Title: "Crypto-Content", Username: "Crypto-Content", Type: "channel"}, // destination — excluded
				{ChatID: "42", Title: "Somebody", Username: "user", Type: "user"},                         // not a channel
			}},
		},
		messages: map[string][]tgsidecar.Message{
			"-100111": {{ID: 10, Text: longPersianNews()}},
		},
	}
	l := &fakeLLM{}
	r := &fakeRenderer{}
	p := &fakePublisher{}
	c := NewCollector(store, &fakeBots{ids: []uuid.UUID{botID}}, side, l, r, p.publish)
	c.pacing = 0 // no sleep in tests
	prices := map[string]any{
		"content_account_id":  "acc1",
		"content_channel":     "@Crypto-Content",
		"content_folder":      "Crypto",
		"content_min_length":  40,
		"render_card_summary": true,
		"brand_key":           "trader",
		"brand_name_en":       "mr.trade.ai",
		"brand_accent":        "#FFA070",
	}
	texts := map[string]any{}
	c.SetPricesFunc(func(id uuid.UUID) map[string]any {
		if id == botID {
			return prices
		}
		return map[string]any{}
	})
	c.SetTextsFunc(func(uuid.UUID) map[string]any { return texts })
	return c, store, side, l, r, p, botID
}

// ---- tests ----

func TestCollectorHappyPathPublishesRenderedCards(t *testing.T) {
	c, store, _, l, r, p, botID := newTestCollector(t)
	c.Tick(context.Background())

	if len(p.calls) != 1 {
		t.Fatalf("want 1 publish, got %d (%+v)", len(p.calls), p.calls)
	}
	call := p.calls[0]
	if call.channel != "@Crypto-Content" {
		t.Errorf("channel = %q", call.channel)
	}
	if call.numPngs != 1 { // one post card per post (no story duplicate)
		t.Errorf("want 1 png, got %d", call.numPngs)
	}
	if len(r.calls) != 1 {
		t.Errorf("want 1 render call, got %d", len(r.calls))
	}
	// caption = cleaned text (from LLM), card = summary (LLM second call returns default)
	if call.caption == "" {
		t.Error("empty caption")
	}
	if len(store.posts) != 1 || store.posts[0].Status != domain.ContentStatusPublished {
		t.Errorf("posts = %+v", store.posts)
	}
	if store.posts[0].BotID != botID {
		t.Errorf("wrong bot id on post")
	}
	_ = l
}

func TestCollectorSkipsDestinationChannel(t *testing.T) {
	c, store, side, _, _, p, _ := newTestCollector(t)
	// only the destination has messages — must NOT be collected
	side.messages["-100111"] = nil
	side.messages["-100999"] = []tgsidecar.Message{{ID: 5, Text: longPersianNews()}}
	c.Tick(context.Background())

	if len(p.calls) != 0 {
		t.Fatalf("destination channel must be excluded, got %d publishes", len(p.calls))
	}
	if len(store.posts) != 0 {
		t.Fatalf("no posts expected, got %d", len(store.posts))
	}
}

func TestCollectorNoContentDropsAd(t *testing.T) {
	c, store, _, l, _, p, _ := newTestCollector(t)
	l.replies = []string{"NO_CONTENT"}
	c.Tick(context.Background())

	if len(p.calls) != 0 {
		t.Fatalf("ad must not be published")
	}
	if len(store.posts) != 1 || store.posts[0].Status != domain.ContentStatusNoContent {
		t.Errorf("want no_content record, got %+v", store.posts)
	}
}

func TestCollectorLLMErrorFallsBackToRawText(t *testing.T) {
	c, store, _, l, r, p, _ := newTestCollector(t)
	l.replyErr = context.DeadlineExceeded
	c.Tick(context.Background())

	if len(p.calls) != 1 {
		t.Fatalf("raw text should still be published, got %d publishes", len(p.calls))
	}
	if p.calls[0].caption != longPersianNews() {
		t.Errorf("caption should be raw text, got %q", p.calls[0].caption)
	}
	if store.posts[0].Status != domain.ContentStatusPublished {
		t.Errorf("status = %s", store.posts[0].Status)
	}
	_ = r
}

func TestCollectorConversationalReplyDropped(t *testing.T) {
	c, store, _, l, _, p, _ := newTestCollector(t)
	l.replies = []string{"لطفاً متنی که می‌خواهید را برایم بفرستید"}
	c.Tick(context.Background())

	if len(p.calls) != 0 {
		t.Fatal("conversational reply must be dropped")
	}
	if store.posts[0].Status != domain.ContentStatusNoContent {
		t.Errorf("status = %s", store.posts[0].Status)
	}
}

func TestCollectorRenderFailureFallsBackToText(t *testing.T) {
	c, store, _, _, r, p, _ := newTestCollector(t)
	r.err = io.ErrUnexpectedEOF
	c.Tick(context.Background())

	if len(p.calls) != 1 {
		t.Fatalf("text fallback expected, got %d", len(p.calls))
	}
	if p.calls[0].numPngs != 0 {
		t.Errorf("fallback must send no photos, got %d", p.calls[0].numPngs)
	}
	if store.posts[0].Status != domain.ContentStatusRenderFailed {
		t.Errorf("status = %s", store.posts[0].Status)
	}
}

func TestCollectorDedupSkipsSeenMessages(t *testing.T) {
	c, store, _, _, _, p, botID := newTestCollector(t)
	c.Tick(context.Background())
	if len(p.calls) != 1 {
		t.Fatalf("setup: want 1 publish, got %d", len(p.calls))
	}

	// message already recorded → next tick must not re-publish
	store.lastID[-100111] = 10
	c.Tick(context.Background())
	if len(p.calls) != 1 {
		t.Errorf("dedup failed: %d publishes", len(p.calls))
	}
	_ = botID
}

func TestCollectorBackfillLimit(t *testing.T) {
	c, store, side, _, _, p, _ := newTestCollector(t)
	// 30 old messages, default backfill limit 20 → only newest 20 processed
	var msgs []tgsidecar.Message
	for i := 1; i <= 30; i++ {
		msgs = append(msgs, tgsidecar.Message{ID: i, Text: longPersianNews()})
	}
	side.messages["-100111"] = msgs
	c.Tick(context.Background())

	if len(p.calls) != 20 {
		t.Errorf("backfill should process 20, got %d", len(p.calls))
	}
	if len(store.posts) != 20 {
		t.Errorf("want 20 records, got %d", len(store.posts))
	}
	// oldest processed first
	if store.posts[0].MessageID != 11 {
		t.Errorf("first processed id = %d, want 11", store.posts[0].MessageID)
	}
}

func TestCollectorFolderMissingFailClosed(t *testing.T) {
	c, store, side, _, _, p, _ := newTestCollector(t)
	side.folders = []tgsidecar.Folder{{Name: "Something else", Peers: nil}}
	c.Tick(context.Background())

	if len(p.calls) != 0 || len(store.posts) != 0 {
		t.Error("missing folder must fail closed — nothing collected")
	}
}

func TestCollectorFilterShortText(t *testing.T) {
	c, store, side, _, _, p, _ := newTestCollector(t)
	side.messages["-100111"] = []tgsidecar.Message{{ID: 10, Text: "کوتاه"}}
	c.Tick(context.Background())

	if len(p.calls) != 0 {
		t.Error("short text must be filtered (min_length=40)")
	}
	if len(store.posts) != 0 {
		t.Error("filtered messages are not recorded")
	}
}

func TestCollectorFilterForwarded(t *testing.T) {
	c, _, side, _, _, p, _ := newTestCollector(t)
	c.SetPricesFunc(func(uuid.UUID) map[string]any {
		return map[string]any{
			"content_account_id":     "acc1",
			"content_channel":        "@Crypto-Content",
			"content_folder":         "Crypto",
			"content_min_length":     40,
			"content_skip_forwarded": true,
		}
	})
	side.messages["-100111"] = []tgsidecar.Message{{ID: 10, Text: longPersianNews(), Forwarded: true}}
	c.Tick(context.Background())

	if len(p.calls) != 0 {
		t.Error("forwarded messages must be skipped when skip_forwarded=true")
	}
}

func TestPassesFilterKeywords(t *testing.T) {
	prices := map[string]any{
		"content_min_length": 10,
		"content_keywords":   []any{"بیت‌کوین", "طلا"},
	}
	m := tgsidecar.Message{ID: 1}
	if !passesFilter(m, "تحلیل طلا و سکه امروز", prices) {
		t.Error("matching keyword should pass")
	}
	if passesFilter(m, "خبر ورزشی امروز", prices) {
		t.Error("non-matching keyword should fail")
	}
}

func TestAiCleanGuards(t *testing.T) {
	c, _, _, l, _, _, _ := newTestCollector(t)

	// short text: never hits the model
	out, drop := c.aiClean(context.Background(), uuid.New(), "کوتاه", map[string]any{})
	if drop || out != "کوتاه" {
		t.Errorf("short text: out=%q drop=%v", out, drop)
	}

	// < 30 chars also bypasses the model (never send short text)
	out, drop = c.aiClean(context.Background(), uuid.New(), strings.Repeat("x", 29), map[string]any{})
	if drop {
		t.Error("29-char text must not be dropped")
	}
	if l.calls != 0 {
		t.Error("short text must not reach the model")
	}

	// ai_enabled=false → raw passthrough
	l.calls = 0
	out, _ = c.aiClean(context.Background(), uuid.New(), longPersianNews(), map[string]any{"ai_enabled": false})
	if out != longPersianNews() || l.calls != 0 {
		t.Error("ai_enabled=false must skip the model")
	}

	// NO_CONTENT followed by an explanation is still a refusal
	l.calls = 0
	l.replies = []string{"NO_CONTENT\n\n(این متن یک یادداشت فنی/توسعه است — برای انتشار مناسب نیست.)"}
	out, drop = c.aiClean(context.Background(), uuid.New(), longPersianNews(), map[string]any{})
	if !drop || out != "" {
		t.Errorf("NO_CONTENT+explanation: out=%q drop=%v", out, drop)
	}

	// markdown-wrapped refusal (**NO_CONTENT**) is still a refusal
	l.calls = 0
	l.replies = []string{"**NO_CONTENT**\n\nاین پیام یک مکالمهٔ فنی خصوصی هست که برای انتشار مناسب نیست."}
	out, drop = c.aiClean(context.Background(), uuid.New(), longPersianNews(), map[string]any{})
	if !drop || out != "" {
		t.Errorf("bold NO_CONTENT: out=%q drop=%v", out, drop)
	}

	// meta commentary about the task is not cleaned content
	l.calls = 0
	l.replies = []string{"متوجه شدم — این یک باگ در پردازش خروجی بوده. برای رفع مشکل، لایه پردازش را اصلاح کنید."}
	out, drop = c.aiClean(context.Background(), uuid.New(), longPersianNews(), map[string]any{})
	if !drop {
		t.Errorf("meta commentary must be dropped, got %q", out)
	}
}

func TestBrandOf(t *testing.T) {
	b := brandOf(map[string]any{
		"brand_key": "trader", "brand_name_en": "mr.trade.ai",
		"brand_accent": "#FFA070",
	})
	if b.Key != "trader" || b.NameEN != "mr.trade.ai" || b.Accent != "#FFA070" {
		t.Errorf("brand = %+v", b)
	}
	if brandOf(map[string]any{}).Key != "brand" {
		t.Error("default key")
	}
}

func TestSourceLink(t *testing.T) {
	pub := sourceLink(sourceChannel{Username: "cryptonews"}, 42)
	if pub != "https://t.me/cryptonews/42" {
		t.Errorf("public link = %s", pub)
	}
	priv := sourceLink(sourceChannel{ChatID: "-100123456"}, 7)
	if priv != "https://t.me/c/123456/7" {
		t.Errorf("private link = %s", priv)
	}
}

func TestDueRespectsPollInterval(t *testing.T) {
	c := &Collector{
		lastPoll: map[uuid.UUID]time.Time{},
		pricesOf: func(uuid.UUID) map[string]any {
			return map[string]any{"content_poll_minutes": 2}
		},
	}
	id := uuid.New()
	if !c.due(id) {
		t.Error("first poll must be due")
	}
	c.lastPoll[id] = time.Now()
	if c.due(id) {
		t.Error("must not poll again before the interval")
	}
	c.lastPoll[id] = time.Now().Add(-3 * time.Minute)
	if !c.due(id) {
		t.Error("must poll after the interval")
	}
}

func TestCollectorSidecarPublishViaAccount(t *testing.T) {
	// content_publish_via = sidecar → cards go out through the MTProto
	// account's SendPhotoMeta, never the running bot's Publisher.
	c, store, side, _, _, p, _ := newTestCollector(t)
	c.SetPublisherViaMeta(func(_ context.Context, _ uuid.UUID, accountID, channel, caption string, photos [][]byte, _ string) error {
		// the fake sidecar records one call per photo; emulate that shape here
		// only for the channel/account assertions
		if accountID != "acc1" {
			t.Errorf("account = %q, want acc1", accountID)
		}
		for range photos {
			side.sentMeta = append(side.sentMeta, publishCall{channel: channel, caption: caption, numPngs: 1})
		}
		return nil
	})
	c.SetPricesFunc(func(uuid.UUID) map[string]any {
		return map[string]any{
			"content_account_id":  "acc1",
			"content_channel":     "@Crypto-Content",
			"content_folder":      "Crypto",
			"content_publish_via": "sidecar",
			"content_min_length":  40,
			"render_card_summary": true,
			"brand_key":           "trader",
		}
	})
	c.Tick(context.Background())

	if len(p.calls) != 0 {
		t.Fatalf("bot publisher must not be called on sidecar path, got %d", len(p.calls))
	}
	if len(side.sentMeta) != 1 { // one post card per post (no story duplicate)
		t.Fatalf("want 1 SendPhotoMeta call, got %d (%+v)", len(side.sentMeta), side.sentMeta)
	}
	for _, call := range side.sentMeta {
		if call.channel != "@Crypto-Content" {
			t.Errorf("channel = %q", call.channel)
		}
	}
	if len(store.posts) != 1 || store.posts[0].Status != domain.ContentStatusPublished {
		t.Errorf("posts = %+v", store.posts)
	}
}

func TestCollectorBotPublishDefaultPath(t *testing.T) {
	// no content_publish_via key (old config) → Bot API path, SendPhotoMeta unused
	c, store, side, _, _, p, _ := newTestCollector(t)
	c.SetPricesFunc(func(uuid.UUID) map[string]any {
		return map[string]any{
			"content_account_id":  "acc1",
			"content_channel":     "@Crypto-Content",
			"content_folder":      "Crypto",
			"content_min_length":  40,
			"render_card_summary": true,
			"brand_key":           "trader",
		}
	})
	c.Tick(context.Background())
	if len(p.calls) != 1 {
		t.Fatalf("bot path must publish via Publisher, got %d", len(p.calls))
	}
	if len(side.sentMeta) != 0 {
		t.Fatalf("SendPhotoMeta must not be called on bot path, got %d", len(side.sentMeta))
	}
	_ = store
}

func TestCollectorTextOnlySendsPlainText(t *testing.T) {
	// content_text_only=true (no cards): publish cleaned text only —
	// renderer unused, no photos, status=published
	c, store, side, _, r, p, _ := newTestCollector(t)
	c.SetPublisherViaMeta(func(_ context.Context, _ uuid.UUID, _, channel, caption string, photos [][]byte, _ string) error {
		side.sentMeta = append(side.sentMeta, publishCall{channel: channel, caption: caption, numPngs: len(photos)})
		return nil
	})
	c.SetPricesFunc(func(uuid.UUID) map[string]any {
		return map[string]any{
			"content_account_id":  "acc1",
			"content_channel":     "@mr_vpn_here",
			"content_folder":      "Crypto",
			"content_publish_via": "sidecar",
			"content_text_only":   true,
			"content_min_length":  40,
			"render_card_summary": true,
			"brand_handle":        "@mr_vpn_here",
		}
	})
	c.Tick(context.Background())

	if len(r.calls) != 0 {
		t.Fatalf("renderer must not be called in text-only mode, got %d", len(r.calls))
	}
	// the brand handle must survive whatever the LLM produced
	if len(side.sentMeta) == 1 && !strings.Contains(side.sentMeta[0].caption, "@mr_vpn_here") {
		t.Errorf("brand handle missing from text post: %q", side.sentMeta[0].caption)
	}
	if len(side.sentMeta) != 1 {
		t.Fatalf("want 1 text publish, got %d (%+v)", len(side.sentMeta), side.sentMeta)
	}
	if side.sentMeta[0].numPngs != 0 {
		t.Errorf("must send no photos, got %d", side.sentMeta[0].numPngs)
	}
	if side.sentMeta[0].channel != "@mr_vpn_here" {
		t.Errorf("channel = %q", side.sentMeta[0].channel)
	}
	if len(store.posts) != 1 || store.posts[0].Status != domain.ContentStatusPublished {
		t.Errorf("posts = %+v", store.posts)
	}
	_ = p
}

func TestStripSourceCredits(t *testing.T) {
	in := "خبر مهم اینترنت.\n🔗 مرجع این خبر: @vmess_v2ray\n\nمنبع: زومان\n\nادامه متن."
	out := stripSourceCredits(in)
	for _, bad := range []string{"مرجع این خبر", "منبع:", "@vmess_v2ray"} {
		if strings.Contains(out, bad) {
			t.Errorf("still contains %q in %q", bad, out)
		}
	}
	if !strings.Contains(out, "خبر مهم اینترنت") || !strings.Contains(out, "ادامه متن") {
		t.Errorf("body text lost: %q", out)
	}
}

func TestLooksLikePromo(t *testing.T) {
	promo := []string{
		"⚽ Barcelona 🆚 PSG ⚽\nپیش‌بینی بازی امشب رو بفرست تا از بین بهترین‌ها ۵ کانفیگ هدیه بگیری",
		"🔑 بروزرسانی مهم برای کاربران VIP – Vr-V2ray",
		"پلن یک‌ماهه ۱۵۰/۰۰۰ تومان\nبرای خرید پیام بدید",
		"کانفیگ‌های رایگان فقط تا تکمیل ظرفیت توزیع میشن",
		// reported 2026-11-11: these three shipped to the channel
		"Server: — YOUR PROXY HERE —\nPort: 443\nSecret: reserve your spot ↓\n\n👆 This is where your proxy could sit — a dedicated post at the time you pick.\n🎁 First slot 50% off.",
		"🚀 وقتی شرط ‌هاتون رو توی ریتزوبت ثبت کنین ، علاوه از ضرایب بالا ، هفتگی با کد های هدیه کسب درآمد میکنید 🤑\n#شرطبندی\n♦️ آموزش شارژ حساب با کریپتو\n♦️ آموزش شارژ حساب  ریالی در ریتزوبت",
		"♦️ آموزش شارژ حساب با کریپتو\n♦️ آموزش شارژ حساب  ریالی در ریتزوبت",
	}
	for _, s := range promo {
		if !looksLikePromo(s) {
			t.Errorf("want promo for %q", s)
		}
	}
	keep := []string{
		"🖥 سرور جدید V2rayNG\n```\nvless://uuid@host:443?security=tls\n```",
		"⚠️ قطع فوری اینترنت در صورت آغاز جنگ جدید",
		"بیت‌کوین امروز با رشد ۴ درصدی به ۶۸۵۰۰ دلار رسید",
		"🖥 سرور جدید\nServer: 1.2.3.4\nPort: 443\nSecret: aBcDeFgHiJkL",
		"شرط لازم برای اجرای این توافق هنوز اعلام نشده است",
		"اپلیکیشن تلگرام نسخهٔ جدیدش رو منتشر کرد",
	}
	for _, s := range keep {
		if looksLikePromo(s) {
			t.Errorf("must not flag promo for %q", s)
		}
	}
}

func TestCollectorTextOnlyDropsPromo(t *testing.T) {
	c, store, side, l, _, _, _ := newTestCollector(t)
	l.replies = []string{
		"⚽ Barcelona 🆚 PSG ⚽\nپیش‌بینی بازی امشب رو بفرست تا ۵ کانفیگ هدیه بگیری. نحوه شرکت: نتیجه رو بنویس.",
		"🔑 بروزرسانی مهم برای کاربران VIP – Vr-V2ray\nلینک جایگزین: it.v2v.uno",
		"🖥 سرور جدید\nvless://1234-5678@host.example.com:443?security=tls&type=ws#cfg",
	}
	c.SetPublisherViaMeta(func(_ context.Context, _ uuid.UUID, _, channel, caption string, photos [][]byte, _ string) error {
		side.sentMeta = append(side.sentMeta, publishCall{channel: channel, caption: caption, numPngs: len(photos)})
		return nil
	})
	c.SetPricesFunc(func(uuid.UUID) map[string]any {
		return map[string]any{
			"content_account_id":  "acc1",
			"content_channel":     "@mr_vpn_here",
			"content_folder":      "Crypto",
			"content_publish_via": "sidecar",
			"content_text_only":   true,
			"content_min_length":  10,
			"brand_handle":        "@mr_vpn_here",
		}
	})
	side.messages["-100111"] = []tgsidecar.Message{
		{ID: 20, Text: "⚽ Barcelona 🆚 PSG ⚽\nپیش‌بینی بازی امشب رو بفرست تا ۵ کانفیگ هدیه بگیری. نحوه شرکت: نتیجه رو بنویس."},
		{ID: 21, Text: "🔑 بروزرسانی مهم برای کاربران VIP – Vr-V2ray\nلینک جایگزین: it.v2v.uno"},
		{ID: 22, Text: "🖥 سرور جدید\nvless://1234-5678@host.example.com:443?security=tls&type=ws#cfg"},
	}
	c.Tick(context.Background())

	if len(side.sentMeta) != 1 {
		t.Fatalf("want only the config post published, got %d (%+v)", len(side.sentMeta), side.sentMeta)
	}
	if !strings.Contains(side.sentMeta[0].caption, "vless://") {
		t.Errorf("published post must carry the config URI: %q", side.sentMeta[0].caption)
	}
	if len(store.posts) != 3 {
		t.Fatalf("want 3 ledger rows (2 no_content + 1 published), got %d", len(store.posts))
	}
	var noContent, published int
	for _, p := range store.posts {
		switch p.Status {
		case domain.ContentStatusNoContent:
			noContent++
		case domain.ContentStatusPublished:
			published++
		}
	}
	if noContent != 2 || published != 1 {
		t.Errorf("no_content=%d published=%d, want 2/1", noContent, published)
	}
}

// TestCollectorDropsReportedAds: the exact texts the user reported on
// @mr_vpn_here (2026-11-11) — a sold ad slot and two betting promos — must
// never publish, on the text path or the card path.
func TestCollectorDropsReportedAds(t *testing.T) {
	ads := []string{
		"Server: — YOUR PROXY HERE —\nPort: 443\nSecret: reserve your spot ↓\n\n👆 This is where your proxy could sit — a dedicated post at the time you pick.\n🎁 First slot 50% off.",
		"🚀 وقتی شرط ‌هاتون رو توی ریتزوبت ثبت کنین ، علاوه از ضرایب بالا ، هفتگی با کد های هدیه کسب درآمد میکنید 🤑\n#شرطبندی\n♦️ آموزش شارژ حساب با کریپتو\n♦️ آموزش شارژ حساب  ریالی در ریتزوبت",
		"♦️ آموزش شارژ حساب با کریپتو\n♦️ آموزش شارژ حساب  ریالی در ریتزوبت",
	}

	// text-only path (the @mr_vpn_here shape)
	c, store, side, l, _, _, _ := newTestCollector(t)
	l.replies = []string{ads[0], ads[1], ads[2]}
	c.SetPublisherViaMeta(func(_ context.Context, _ uuid.UUID, _, channel, caption string, photos [][]byte, _ string) error {
		side.sentMeta = append(side.sentMeta, publishCall{channel: channel, caption: caption, numPngs: len(photos)})
		return nil
	})
	c.SetPricesFunc(func(uuid.UUID) map[string]any {
		return map[string]any{
			"content_account_id":  "acc1",
			"content_channel":     "@mr_vpn_here",
			"content_folder":      "Crypto",
			"content_publish_via": "sidecar",
			"content_text_only":   true,
			"content_min_length":  10,
			"brand_handle":        "@mr_vpn_here",
		}
	})
	msgs := make([]tgsidecar.Message, 0, len(ads))
	for i, ad := range ads {
		msgs = append(msgs, tgsidecar.Message{ID: 30 + i, Text: ad})
	}
	side.messages["-100111"] = msgs
	c.Tick(context.Background())
	if len(side.sentMeta) != 0 {
		t.Fatalf("text path published ads: %+v", side.sentMeta)
	}
	for _, p := range store.posts {
		if p.Status != domain.ContentStatusNoContent {
			t.Fatalf("ad must land as no_content in the ledger, got %s (%+v)", p.Status, p)
		}
	}
	if len(store.posts) != len(ads) {
		t.Fatalf("want %d ledger rows, got %d", len(ads), len(store.posts))
	}

	// card path must gate identically
	c2, _, side2, l2, _, p2, _ := newTestCollector(t)
	l2.replies = []string{ads[1]}
	side2.messages["-100111"] = []tgsidecar.Message{{ID: 40, Text: ads[1]}}
	c2.Tick(context.Background())
	if len(p2.calls) != 0 {
		t.Fatalf("card path published an ad: %+v", p2.calls)
	}
}

func TestStripForeignMentions(t *testing.T) {
	in := "🌐 کانفیگ ملی‌شکن\n@SERVERNETT\n\nServer: x.test\nSecret: abc\n@ProxyMTProto\n\n@mr_vpn_here ✅\n\nلینک: https://t.me/VlessConfig/12"
	out := stripForeignMentions(in, "@mr_vpn_here")
	for _, bad := range []string{"@SERVERNETT", "@ProxyMTProto", "t.me/VlessConfig"} {
		if strings.Contains(out, bad) {
			t.Errorf("still contains %q in %q", bad, out)
		}
	}
	if !strings.Contains(out, "@mr_vpn_here") {
		t.Errorf("brand handle lost: %q", out)
	}
	if !strings.Contains(out, "Server: x.test") {
		t.Errorf("proxy body lost: %q", out)
	}

	// URI @host must survive
	uri := "vless://uuid-1@dl.example.com:443?security=tls#tag"
	out2 := stripForeignMentions("کانفیگ:\n"+uri+"\n@SomeSource", "@mr_vpn_here")
	if !strings.Contains(out2, "uuid-1@dl.example.com") {
		t.Errorf("URI damaged: %q", out2)
	}
	if strings.Contains(out2, "@SomeSource") {
		t.Errorf("source handle survived: %q", out2)
	}
}

func TestAttachProxyLinks(t *testing.T) {
	u1 := "https://t.me/proxy?server=1.2.3.4&port=443&secret=dd00"
	u2 := "https://t.me/proxy?server=5.6.7.8&port=8443&secret=ee11"

	t.Run("block replaced by numbered links", func(t *testing.T) {
		in := "🔒 پروکسی MTProto\n\nServer: Unknown\nPort: 7443\nSecret: AAAAAAAAAAAAAAAAAAAAABQ=\n\nبرای اتصال، سکرت را در تنظیمات پروکسی تلگرام وارد کنید.\n\nادامه."
		out := attachProxyLinks(in, []string{u1, u2})
		for _, bad := range []string{"Server: Unknown", "Port: 7443", "وارد کنید"} {
			if strings.Contains(out, bad) {
				t.Errorf("still contains %q in %q", bad, out)
			}
		}
		if !strings.Contains(out, "[🔒 اتصال به پروکسی ۱]("+u1+")") || !strings.Contains(out, "[🔒 اتصال به پروکسی ۲]("+u2+")") {
			t.Errorf("numbered connect links missing: %q", out)
		}
		if !strings.Contains(out, "🔒 پروکسی MTProto") || !strings.Contains(out, "ادامه.") {
			t.Errorf("surrounding text lost: %q", out)
		}
	})

	t.Run("single link keeps the direct label", func(t *testing.T) {
		out := attachProxyLinks("Server: Unknown\nPort: 443\nSecret: EERighJJvXrFGRMCIMjdCQ", []string{u1})
		if !strings.Contains(out, "[🔒 اتصال مستقیم به پروکسی — روی همین دکمه بزن]("+u1+")") {
			t.Errorf("direct label missing: %q", out)
		}
	})

	t.Run("bold block replaced", func(t *testing.T) {
		in := "📡 پروکسی MTProto\n\n**Server:** Unknown\n**Port:** 853\n**Secret:** ee54ce330e4690cc297d2b031ff3f288b0\n\nفالو کنید."
		out := attachProxyLinks(in, []string{u1})
		for _, bad := range []string{"Unknown", "Port", "Secret"} {
			if strings.Contains(out, bad) {
				t.Errorf("still contains %q in %q", bad, out)
			}
		}
		if !strings.Contains(out, "[🔒 اتصال مستقیم به پروکسی — روی همین دکمه بزن]("+u1+")") {
			t.Errorf("direct label missing: %q", out)
		}
	})

	t.Run("no urls unchanged", func(t *testing.T) {
		in := "Server: Unknown\nPort: 443\nSecret: EERighJJvXrFGRMCIMjdCQ"
		if out := attachProxyLinks(in, nil); out != in {
			t.Errorf("got %q", out)
		}
	})

	t.Run("no block appends links", func(t *testing.T) {
		out := attachProxyLinks("پروکسی جدید رسید.", []string{u1})
		if !strings.HasSuffix(out, "[🔒 اتصال مستقیم به پروکسی — روی همین دکمه بزن]("+u1+")") {
			t.Errorf("link not appended: %q", out)
		}
	})
}

func TestCollectorTextOnlyInjectsProxyLinks(t *testing.T) {
	// @ProxyMTProto-style posts: placeholder "Server: Unknown" block in text,
	// real endpoints on the URL buttons — the published post must carry the
	// connect links, not the dead block.
	c, store, side, l, _, _, _ := newTestCollector(t)
	c.SetPublisherViaMeta(func(_ context.Context, _ uuid.UUID, _, channel, caption string, photos [][]byte, _ string) error {
		side.sentMeta = append(side.sentMeta, publishCall{channel: channel, caption: caption, numPngs: len(photos)})
		return nil
	})
	c.SetPricesFunc(func(uuid.UUID) map[string]any {
		return map[string]any{
			"content_account_id":  "acc1",
			"content_channel":     "@mr_vpn_here",
			"content_folder":      "Crypto",
			"content_publish_via": "sidecar",
			"content_text_only":   true,
			"content_min_length":  10,
			"brand_handle":        "@mr_vpn_here",
		}
	})
	l.replies = []string{"Server: Unknown\nPort: 7443\nSecret: AAAAAAAAAAAAAAAAAAAAABQ="}
	side.messages["-100111"] = []tgsidecar.Message{{
		ID:   30,
		Text: "Server: Unknown\nPort: 7443\nSecret: AAAAAAAAAAAAAAAAAAAAABQ=",
		ProxyURLs: []string{
			"https://t.me/proxy?server=1.2.3.4&port=443&secret=dd00",
			"https://t.me/webproxy?server=zov.example&secret=ee11",
		},
	}}
	c.Tick(context.Background())

	if len(side.sentMeta) != 1 {
		t.Fatalf("want 1 publish, got %d (%+v)", len(side.sentMeta), side.sentMeta)
	}
	cap := side.sentMeta[0].caption
	if strings.Contains(cap, "Server: Unknown") {
		t.Errorf("dead block must be replaced: %q", cap)
	}
	if !strings.Contains(cap, "[🔒 اتصال به پروکسی ۱](https://t.me/proxy?server=1.2.3.4&port=443&secret=dd00)") ||
		!strings.Contains(cap, "[🔒 اتصال به پروکسی ۲](https://t.me/webproxy?server=zov.example&secret=ee11)") {
		t.Errorf("connect links missing: %q", cap)
	}
	if len(store.posts) != 1 || store.posts[0].Status != domain.ContentStatusPublished {
		t.Errorf("posts = %+v", store.posts)
	}
}

func TestCollectorButtonOnlySurvivesNoContentDrop(t *testing.T) {
	// the model may discard a "Server: Unknown" text block as junk; when the
	// real endpoints ride on URL buttons the post must still ship as links
	c, store, side, l, _, _, _ := newTestCollector(t)
	c.SetPublisherViaMeta(func(_ context.Context, _ uuid.UUID, _, channel, caption string, photos [][]byte, _ string) error {
		side.sentMeta = append(side.sentMeta, publishCall{channel: channel, caption: caption, numPngs: len(photos)})
		return nil
	})
	c.SetPricesFunc(func(uuid.UUID) map[string]any {
		return map[string]any{
			"content_account_id":  "acc1",
			"content_channel":     "@mr_vpn_here",
			"content_folder":      "Crypto",
			"content_publish_via": "sidecar",
			"content_text_only":   true,
			"content_min_length":  10,
			"brand_handle":        "@mr_vpn_here",
		}
	})
	l.replies = []string{"NO_CONTENT"}
	side.messages["-100111"] = []tgsidecar.Message{{
		ID:   31,
		Text: "Server: Unknown\nPort: 7443\nSecret: AAAAAAAAAAAAAAAAAAAAABQ=",
		ProxyURLs: []string{
			"https://t.me/proxy?server=1.2.3.4&port=443&secret=dd00",
			"https://t.me/proxy?server=5.6.7.8&port=443&secret=dd00",
		},
	}}
	c.Tick(context.Background())

	if len(side.sentMeta) != 1 {
		t.Fatalf("want 1 publish, got %d (%+v)", len(side.sentMeta), side.sentMeta)
	}
	cap := side.sentMeta[0].caption
	if !strings.Contains(cap, "[🔒 اتصال به پروکسی ۱](https://t.me/proxy?server=1.2.3.4&port=443&secret=dd00)") {
		t.Errorf("connect links missing: %q", cap)
	}
	if strings.Contains(cap, "Secret: AAAA") {
		t.Errorf("dead block must not leak: %q", cap)
	}
	if len(store.posts) != 1 || store.posts[0].Status != domain.ContentStatusPublished {
		t.Errorf("posts = %+v", store.posts)
	}
}

// TestStripForeignMentionsDropsWebLinks: posts carry no links at all (user:
// «هیچ لینکی نباید باشه») — web URLs and bare domains go, markdown labels and
// the brand handle stay, VPN config URIs survive.
func TestStripForeignMentionsDropsWebLinks(t *testing.T) {
	in := "طلا ریزش خواهد کرد به چند دلیل مهم فاندامنتال\n\nhttps://www.instagram.com/reel/DfmoXS9ISeF/?psln=abc\n\nجزئیات [ویدیو](https://youtube.com/watch?v=x1)\ninstagram.com/reel/other\n@mrtradeaibot #طلا"
	out := stripForeignMentions(in, "@mrtradeaibot")
	if strings.Contains(out, "instagram.com") || strings.Contains(out, "youtube.com") {
		t.Fatalf("web links must be stripped: %q", out)
	}
	if !strings.Contains(out, "طلا ریزش خواهد کرد") || !strings.Contains(out, "جزئیات ویدیو") {
		t.Fatalf("text and markdown labels must survive: %q", out)
	}
	if !strings.Contains(out, "@mrtradeaibot") || !strings.Contains(out, "#طلا") {
		t.Fatalf("brand handle and hashtags must survive: %q", out)
	}
	uri := stripForeignMentions("کانفیگ:\nvless://uuid@host:443?type=tcp", "@brand")
	if !strings.Contains(uri, "vless://uuid@host:443?type=tcp") {
		t.Fatalf("config URI must survive: %q", uri)
	}
}
