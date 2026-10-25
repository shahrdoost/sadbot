package service

import (
	"context"
	"errors"
	"fmt"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ModerationService powers the admin panel for confessions and support
// tickets. Channel publishing and user DMs are injected as closures so this
// package stays free of runtime/telegram imports.
type ModerationService struct {
	repo   *postgres.ModerationRepo
	sendTo func(ctx context.Context, botID uuid.UUID, userID int64, text string) error
	post   func(ctx context.Context, botID uuid.UUID, text string) (int64, error) // channel post; (0,nil) when unconfigured
}

func NewModerationService(
	repo *postgres.ModerationRepo,
	sendTo func(ctx context.Context, botID uuid.UUID, userID int64, text string) error,
	post func(ctx context.Context, botID uuid.UUID, text string) (int64, error),
) *ModerationService {
	return &ModerationService{repo: repo, sendTo: sendTo, post: post}
}

// --- confessions ---

func (s *ModerationService) ListConfessions(ctx context.Context, botID uuid.UUID, status string, page int) ([]domain.Confession, int, error) {
	if page < 1 {
		page = 1
	}
	return s.repo.ListConfessions(ctx, botID, status, page, 20)
}

func (s *ModerationService) ApproveConfession(ctx context.Context, botID uuid.UUID, id int64, adminID uuid.UUID) error {
	c, err := s.repo.DecideConfession(ctx, botID, id, adminID, true)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("اعتراف یافت نشد یا قبلاً بررسی شده")
		}
		return err
	}
	if s.post != nil {
		if msgID, err := s.post(ctx, botID, "💌 اعتراف ناشناس:\n\n"+c.Text); err == nil && msgID > 0 {
			_ = s.repo.MarkConfessionPublished(ctx, id, msgID)
		}
	}
	if s.sendTo != nil {
		_ = s.sendTo(ctx, botID, c.UserID, "✅ اعتراف تو تایید شد و در کانال منتشر شد.")
	}
	return nil
}

func (s *ModerationService) RejectConfession(ctx context.Context, botID uuid.UUID, id int64, adminID uuid.UUID) error {
	c, err := s.repo.DecideConfession(ctx, botID, id, adminID, false)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("اعتراف یافت نشد یا قبلاً بررسی شده")
		}
		return err
	}
	if s.sendTo != nil {
		_ = s.sendTo(ctx, botID, c.UserID, "متأسفانه اعتراف تو توسط ادمین تایید نشد.")
	}
	return nil
}

// --- support tickets ---

func (s *ModerationService) ListTickets(ctx context.Context, botID uuid.UUID, status string, page int) ([]domain.SupportTicket, int, error) {
	if page < 1 {
		page = 1
	}
	return s.repo.ListTickets(ctx, botID, status, page, 20)
}

func (s *ModerationService) ReplyTicket(ctx context.Context, botID uuid.UUID, id int64, adminID uuid.UUID, reply string) error {
	if reply == "" {
		return fmt.Errorf("متن پاسخ خالی است")
	}
	t, err := s.repo.ReplyTicket(ctx, botID, id, reply)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("تیکت یافت نشد")
		}
		return err
	}
	if s.sendTo != nil {
		_ = s.sendTo(ctx, botID, t.UserID, "📩 پاسخ پشتیبانی:\n\n"+reply)
	}
	return nil
}

func (s *ModerationService) CloseTicket(ctx context.Context, botID uuid.UUID, id int64) error {
	ok, err := s.repo.CloseTicket(ctx, botID, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("تیکت یافت نشد")
	}
	return nil
}
