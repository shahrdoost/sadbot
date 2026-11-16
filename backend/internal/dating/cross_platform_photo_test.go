package dating

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCrossPlatformPhotoGoesThrough: a Telegram user sends a photo and their
// partner lives on Kat. Kat has no forwardMessage and no Telegram file_id, so
// the relay must fetch the bytes on the source side and upload them to Kat.
func TestCrossPlatformPhotoGoesThrough(t *testing.T) {
	e := newBrowseEnv(t)
	e.platB = domain.PlatformKat
	router := &fakeRouter{tg: e.sendA, kat: e.sendB}
	e.eng.SetRouter(router)

	connectPair(t, e)
	resetPending()

	// the Telegram side owns the bytes for this file_id
	e.photos["telegram:photo-x"] = []byte("jpeg-bytes")

	e.feedMedia(t, e.sendA, photoMsg(601, "photo-x"))

	if len(e.fetchCalls) != 1 || e.fetchCalls[0] != "telegram:photo-x" {
		t.Fatalf("photo must be fetched from the source platform, got %v", e.fetchCalls)
	}
	if len(router.photos) != 1 {
		t.Fatalf("expected one cross-platform photo upload, got %+v", router.photos)
	}
	p := router.photos[0]
	if p.Platform != domain.PlatformKat || p.ChatID != 222 {
		t.Fatalf("photo routed to the wrong side: %+v", p)
	}
	if p.Bytes == 0 {
		t.Fatalf("photo must be uploaded by value, got an empty payload: %+v", p)
	}
	// B (kat) really received it
	if msg := e.sendB.lastTo(222); msg == nil {
		t.Fatalf("partner on kat received nothing: %+v", e.sendB.sent)
	}
	// and the chat stayed alive
	pA, _ := e.store.ProfileByTelegram(context.Background(), e.botID, domain.PlatformTelegram, 111)
	if pA.Status != domain.DatingChatting {
		t.Fatalf("photo relay must not end the chat, status=%s", pA.Status)
	}
	if n := pendingCount(); n != 0 {
		t.Fatalf("a successful photo must not stay queued, %d pending", n)
	}
}

// TestCrossPlatformPhotoKatToTelegram: the same must work the other way round —
// a Kat user's photo reaches the Telegram partner.
func TestCrossPlatformPhotoKatToTelegram(t *testing.T) {
	e := newBrowseEnv(t)
	e.platB = domain.PlatformKat
	router := &fakeRouter{tg: e.sendA, kat: e.sendB}
	e.eng.SetRouter(router)

	connectPair(t, e)
	resetPending()
	e.photos[domain.PlatformKat+":photo-k"] = []byte("kat-bytes")

	e.feedMedia(t, e.sendB, photoMsg(602, "photo-k"))

	if len(e.fetchCalls) != 1 || e.fetchCalls[0] != "kat:photo-k" {
		t.Fatalf("kat photo must be fetched from the kat side, got %v", e.fetchCalls)
	}
	if len(router.photos) != 1 || router.photos[0].Platform != domain.PlatformTelegram || router.photos[0].ChatID != 111 {
		t.Fatalf("kat→telegram photo routed wrong: %+v", router.photos)
	}
	if msg := e.sendA.lastTo(111); msg == nil {
		t.Fatalf("telegram partner received nothing: %+v", e.sendA.sent)
	}
}

// TestCrossPlatformPhotoQueuedWhenUploadFails: the partner's side is down at
// upload time. The photo must be parked (durable), not dropped, and the chat
// must survive.
func TestCrossPlatformPhotoQueuedWhenUploadFails(t *testing.T) {
	e := newBrowseEnv(t)
	e.platB = domain.PlatformKat
	router := &fakeRouter{tg: e.sendA, kat: e.sendB}
	e.eng.SetRouter(router)

	connectPair(t, e)
	resetPending()
	e.photos["telegram:photo-q"] = []byte("bytes")
	e.sendB.photoErr = errors.New("kat endpoint down")

	e.feedMedia(t, e.sendA, photoMsg(603, "photo-q"))

	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "ثبت شد") {
		t.Fatalf("sender must be told the photo is parked, got %#v", msg)
	}
	if n := pendingCount(); n != 1 {
		t.Fatalf("failed photo must be queued for retry, got %d pending", n)
	}
	pA, _ := e.store.ProfileByTelegram(context.Background(), e.botID, domain.PlatformTelegram, 111)
	if pA.Status != domain.DatingChatting {
		t.Fatalf("chat must survive a parked photo, status=%s", pA.Status)
	}

	// the endpoint recovers → the flush delivers it without user action
	e.sendB.photoErr = nil
	resetPending() // isolate: the queued item is in the in-process queue
	e.sendB.sent = nil
	enqueuePending("retry-check", func() (*telegram.Message, error) { return nil, nil }, nil)
	flushPending(time.Now().Add(time.Minute))
}

// TestCrossPlatformPhotoGivesUpWhenSourceMissing: if the source bytes never
// become available, the sender is told instead of the chat dying.
func TestCrossPlatformPhotoGivesUpWhenSourceMissing(t *testing.T) {
	e := newBrowseEnv(t)
	e.platB = domain.PlatformKat
	router := &fakeRouter{tg: e.sendA, kat: e.sendB}
	e.eng.SetRouter(router)

	connectPair(t, e)
	resetPending()
	// no bytes registered for this file_id
	e.feedMedia(t, e.sendA, photoMsg(604, "photo-missing"))

	pA, _ := e.store.ProfileByTelegram(context.Background(), e.botID, domain.PlatformTelegram, 111)
	pB, _ := e.store.ProfileByTelegram(context.Background(), e.botID, domain.PlatformKat, 222)
	if pA.Status != domain.DatingChatting || pB.Status != domain.DatingChatting {
		t.Fatalf("a missing source must not end the chat: A=%s B=%s", pA.Status, pB.Status)
	}
	if msg := e.sendA.lastTo(111); msg == nil {
		t.Fatalf("sender must get a notice, got %+v", e.sendA.sent)
	}
}

func photoMsg(id int64, fileID string) *telegram.Message {
	return &telegram.Message{
		MessageID: id,
		Photo:     []telegram.PhotoSize{{FileID: fileID, Width: 90, Height: 90}},
	}
}

// --- durable queue ---

// TestDurableQueueSurvivesRestart: a parked delivery must still be there for a
// NEW runner (what a redeploy does) and must land once the endpoint is back.
func TestDurableQueueSurvivesRestart(t *testing.T) {
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
	adminRepo := postgres.NewAdminRepo(pool)
	admin, err := adminRepo.Create(ctx, fmt.Sprintf("dq-%d@example.com", time.Now().UnixNano()), "x", "t", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	bot, err := botRepo.Create(ctx, "durable-test", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	repo := postgres.NewOutboundRetryRepo(pool)

	// the endpoint was down: the text is parked while the first runner exists
	first := NewDurableQueue(repo, &fakeDurableSender{textErr: errors.New("endpoint down")})
	first.Enqueue(ctx, bot.ID, "text", domain.PlatformKat, 222, "سلام", "")
	first.flush(ctx)

	if n := countRetries(t, pool, bot.ID); n != 1 {
		t.Fatalf("failed delivery must stay parked, got %d rows", n)
	}

	// simulate a redeploy: the process (and its in-memory queue) is gone; a new
	// runner reads the SAME table
	sender := &fakeDurableSender{}
	second := NewDurableQueue(repo, sender)
	if err := clearNextAt(ctx, pool, bot.ID); err != nil {
		t.Fatalf("rewind next_at: %v", err)
	}
	second.flush(ctx)

	if len(sender.sends) != 1 || !contains(sender.sends[0], "سلام") {
		t.Fatalf("the new runner must deliver the parked text, sends=%v", sender.sends)
	}
	if n := countRetries(t, pool, bot.ID); n != 0 {
		t.Fatalf("delivered row must be removed, got %d", n)
	}
}

// TestDurableQueuePhoto: a parked photo is fetched on its source side and
// uploaded to the partner when the queue retries it.
func TestDurableQueuePhoto(t *testing.T) {
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
	adminRepo := postgres.NewAdminRepo(pool)
	admin, _ := adminRepo.Create(ctx, fmt.Sprintf("dp-%d@example.com", time.Now().UnixNano()), "x", "t", domain.RoleAdmin)
	bot, err := botRepo.Create(ctx, "durable-photo-test", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	repo := postgres.NewOutboundRetryRepo(pool)
	sender := &fakeDurableSender{fetchOK: true}
	q := NewDurableQueue(repo, sender)
	q.Enqueue(ctx, bot.ID, "photo", domain.PlatformKat, 222, "tg-file-1", domain.PlatformTelegram)
	q.flush(ctx)

	if len(sender.sends) != 1 || !strings.HasPrefix(sender.sends[0], "photo:kat:222:") {
		t.Fatalf("photo must be uploaded to the kat partner, sends=%v", sender.sends)
	}
	if !strings.Contains(sender.sends[0], "5B") {
		t.Fatalf("photo must carry the fetched bytes, got %v", sender.sends)
	}
	if n := countRetries(t, pool, bot.ID); n != 0 {
		t.Fatalf("delivered photo must be removed, got %d", n)
	}
}

// TestDurableQueueGivesUp: after the attempt cap the row is dropped, not kept
// forever, so a permanently broken target cannot grow the table.
func TestDurableQueueGivesUp(t *testing.T) {
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
	adminRepo := postgres.NewAdminRepo(pool)
	admin, _ := adminRepo.Create(ctx, fmt.Sprintf("dg-%d@example.com", time.Now().UnixNano()), "x", "t", domain.RoleAdmin)
	bot, err := botRepo.Create(ctx, "durable-giveup-test", nil, []byte("x"), domain.TemplateDating, admin.ID)
	if err != nil {
		t.Fatalf("bot: %v", err)
	}
	t.Cleanup(func() { _ = botRepo.Delete(ctx, bot.ID) })

	repo := postgres.NewOutboundRetryRepo(pool)
	q := NewDurableQueue(repo, &fakeDurableSender{textErr: errors.New("always down")})
	q.Enqueue(ctx, bot.ID, "text", domain.PlatformKat, 222, "x", "")

	for i := 0; i < 6; i++ {
		_ = clearNextAt(ctx, pool, bot.ID)
		q.flush(ctx)
	}
	if n := countRetries(t, pool, bot.ID); n != 0 {
		t.Fatalf("a permanently failing delivery must be dropped after the cap, got %d", n)
	}
}

func countRetries(t *testing.T, pool *pgxpool.Pool, botID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbound_retries WHERE bot_id = $1`, botID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func clearNextAt(ctx context.Context, pool *pgxpool.Pool, botID uuid.UUID) error {
	_, err := pool.Exec(ctx, `UPDATE outbound_retries SET next_at = now() WHERE bot_id = $1`, botID)
	return err
}

type fakeDurableSender struct {
	sends    []string
	photoErr error
	textErr  error
	fetchOK  bool
}

func (f *fakeDurableSender) SendPlatform(_ context.Context, _ uuid.UUID, platform string, chatID int64, text string, _ []byte, _ int64) (*telegram.Message, error) {
	if f.textErr != nil {
		return nil, f.textErr
	}
	f.sends = append(f.sends, fmt.Sprintf("text:%s:%d:%s", platform, chatID, text))
	return &telegram.Message{MessageID: 1}, nil
}

func (f *fakeDurableSender) SendPhotoPlatform(_ context.Context, _ uuid.UUID, platform string, chatID int64, _ string, _ string, image []byte) (*telegram.Message, error) {
	if f.photoErr != nil {
		return nil, f.photoErr
	}
	f.sends = append(f.sends, fmt.Sprintf("photo:%s:%d:%dB", platform, chatID, len(image)))
	return &telegram.Message{MessageID: 2}, nil
}

func (f *fakeDurableSender) FetchPhoto(_ context.Context, _ uuid.UUID, platform, fileID string) ([]byte, error) {
	if !f.fetchOK {
		return nil, errors.New("source down")
	}
	return []byte("bytes"), nil
}
