package service

import (
	"context"
	"log"
	"strings"
	"time"

	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

// BroadcastSender delivers one broadcast message to a chat. photo may be a
// Telegram file_id or an https URL (both accepted by the Bot API photo field);
// footer is the text under the photo caption. When photo is empty only text
// is sent and footer is ignored.
type BroadcastSender func(ctx context.Context, botID uuid.UUID, chatID int64, text, photo, footer string) error

// BroadcastService runs panel/telegram broadcasts in batches (kpkchat
// admin-ops parity: batches of 15 with pacing, sent/failed counters).
type BroadcastService struct {
	repo   *postgres.BroadcastRepo
	send   BroadcastSender
	batch  int
	pacing time.Duration
}

func NewBroadcastService(repo *postgres.BroadcastRepo, send BroadcastSender) *BroadcastService {
	return &BroadcastService{repo: repo, send: send, batch: 15, pacing: 400 * time.Millisecond}
}

func (s *BroadcastService) Preview(ctx context.Context, botID uuid.UUID) (int, error) {
	return s.repo.BroadcastAudienceCount(ctx, botID)
}

// Start creates a run record and sends asynchronously; returns the run id.
// photo/footer are optional (empty strings mean text-only broadcast).
func (s *BroadcastService) Start(ctx context.Context, botID, adminID uuid.UUID, text, photo, footer string) (postgres.BroadcastRun, error) {
	audience, err := s.repo.BroadcastAudienceCount(ctx, botID)
	if err != nil {
		return postgres.BroadcastRun{}, err
	}
	var photoPtr, footerPtr *string
	if p := strings.TrimSpace(photo); p != "" {
		photoPtr = &p
		if f := strings.TrimSpace(footer); f != "" {
			footerPtr = &f
		}
	}
	run, err := s.repo.CreateRun(ctx, botID, adminID, text, photoPtr, footerPtr, audience)
	if err != nil {
		return postgres.BroadcastRun{}, err
	}
	go s.execute(run)
	return run, nil
}

func (s *BroadcastService) execute(run postgres.BroadcastRun) {
	ctx := context.Background()
	var sent, failed int
	var afterID int64
	var photo, footer string
	if run.Photo != nil {
		photo = *run.Photo
	}
	if run.Footer != nil {
		footer = *run.Footer
	}
	for {
		page, err := s.repo.BroadcastPage(ctx, run.BotID, afterID, s.batch)
		if err != nil {
			log.Printf("[broadcast] page: %v", err)
			break
		}
		if len(page) == 0 {
			break
		}
		for _, u := range page {
			if err := s.send(ctx, run.BotID, u.TelegramID, run.Text, photo, footer); err != nil {
				failed++
			} else {
				sent++
			}
			afterID = u.UserID
		}
		_ = s.repo.UpdateProgress(ctx, run.ID, sent, failed)
		time.Sleep(s.pacing)
	}
	if err := s.repo.Finish(ctx, run.ID, sent, failed); err != nil {
		log.Printf("[broadcast] finish: %v", err)
	}
	log.Printf("[broadcast] done bot=%s sent=%d failed=%d audience=%d", run.BotID, sent, failed, run.Audience)
}

func (s *BroadcastService) Get(ctx context.Context, botID, runID uuid.UUID) (*postgres.BroadcastRun, error) {
	return s.repo.Get(ctx, botID, runID)
}

func (s *BroadcastService) List(ctx context.Context, botID uuid.UUID) ([]postgres.BroadcastRun, error) {
	return s.repo.ListByBot(ctx, botID, 20)
}
