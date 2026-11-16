package dating

// Durable retry queue for cross-platform deliveries.
//
// The in-process queue (outbound.go) is fast and enough for a blip, but it dies
// with the process — and a redeploy is exactly when a platform side is down.
// This runner drains outbound_retries: every parked text/photo/match notice is
// re-attempted from the database until it lands, the recipient blocked us
// (permanent), or the attempt cap is reached.

import (
	"context"
	"errors"
	"log"
	"time"

	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// DurableSender is what the runner needs from the runtime: deliver by value on
// a platform, and resolve a photo's bytes on the side that owns the file_id.
type DurableSender interface {
	SendPlatform(ctx context.Context, botID uuid.UUID, platform string, chatID int64, text string, kbd []byte, replyTo int64) (*telegram.Message, error)
	SendPhotoPlatform(ctx context.Context, botID uuid.UUID, platform string, chatID int64, fileID, caption string, image []byte) (*telegram.Message, error)
	FetchPhoto(ctx context.Context, botID uuid.UUID, platform, fileID string) ([]byte, error)
}

var durableRetrySchedule = []time.Duration{
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
}

// DurableQueue is the runner. nil repo disables it (tests without a database).
type DurableQueue struct {
	repo   *postgres.OutboundRetryRepo
	sender DurableSender
}

func NewDurableQueue(repo *postgres.OutboundRetryRepo, sender DurableSender) *DurableQueue {
	return &DurableQueue{repo: repo, sender: sender}
}

// Run drains the queue on a timer until ctx is cancelled.
func (q *DurableQueue) Run(ctx context.Context) {
	if q == nil || q.repo == nil {
		return
	}
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			q.flush(ctx)
		}
	}
}

// Enqueue parks one delivery. A nil queue is a no-op so tests stay simple.
func (q *DurableQueue) Enqueue(ctx context.Context, botID uuid.UUID, kind, platform string, chatID int64, payload, sourcePlatform string) {
	if q == nil || q.repo == nil {
		return
	}
	if err := q.repo.Enqueue(ctx, botID, kind, platform, chatID, payload, sourcePlatform); err != nil {
		log.Printf("[dating] durable enqueue (%s %s→%d): %v", kind, platform, chatID, err)
	}
}

// flush delivers everything due now.
func (q *DurableQueue) flush(ctx context.Context) {
	items, err := q.repo.Due(ctx, 50)
	if err != nil {
		log.Printf("[dating] durable due: %v", err)
		return
	}
	for _, it := range items {
		err := q.deliver(ctx, it)
		switch {
		case err == nil:
			_ = q.repo.Done(ctx, it.ID)
		case isPermanentRetryErr(err):
			log.Printf("[dating] durable #%d dropped (permanent): %v", it.ID, err)
			_ = q.repo.Done(ctx, it.ID)
		default:
			wait := durableRetrySchedule[min(it.Attempts, len(durableRetrySchedule)-1)]
			gaveUp, ferr := q.repo.Fail(ctx, it.ID, wait, err.Error())
			if ferr != nil {
				log.Printf("[dating] durable fail record #%d: %v", it.ID, ferr)
			}
			if gaveUp {
				log.Printf("[dating] durable #%d given up (%s→%d): %v",
					it.ID, it.Platform, it.ChatID, err)
			}
		}
	}
}

func (q *DurableQueue) deliver(ctx context.Context, it postgres.OutboundRetry) error {
	switch it.Kind {
	case "text":
		_, err := q.sender.SendPlatform(ctx, it.BotID, it.Platform, it.ChatID, it.Payload, nil, 0)
		return err
	case "match":
		_, err := q.sender.SendPlatform(ctx, it.BotID, it.Platform, it.ChatID, it.Payload, nil, 0)
		return err
	case "photo":
		// the bytes are fetched fresh each attempt: the source endpoint may have
		// only just published the file the update announced
		img, err := q.sender.FetchPhoto(ctx, it.BotID, it.SourcePlatform, it.Payload)
		if err != nil {
			return err
		}
		if len(img) == 0 {
			return errors.New("photo bytes unavailable")
		}
		_, err = q.sender.SendPhotoPlatform(ctx, it.BotID, it.Platform, it.ChatID, "", "", img)
		return err
	}
	return nil
}

// isPermanentRetryErr reports failures no retry can fix (recipient blocked the
// bot). Everything else stays in the queue.
func isPermanentRetryErr(err error) bool {
	var ae *telegram.APIError
	return errors.As(err, &ae) && ae.IsForbidden()
}

var _ = engine.ErrPlatformDown
