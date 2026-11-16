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
	// sendTicketDM delivers a support reply with its inline buttons, so the
	// user can answer in the same ticket or close it from the message itself.
	sendTicketDM func(ctx context.Context, botID uuid.UUID, userID int64, text string, ticketID int64) error
}

// SetTicketNotifier wires the bot-facing delivery for support replies. It is
// set after construction because it needs the same runtime closure as the
// other admin actions.
func (s *ModerationService) SetTicketNotifier(fn func(ctx context.Context, botID uuid.UUID, userID int64, text string, ticketID int64) error) {
	s.sendTicketDM = fn
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
	// the reply is a conversation turn, not a dead end: it carries «ادامه» and
	// «بستن» buttons so the user answers inside this ticket
	if s.sendTicketDM != nil {
		_ = s.sendTicketDM(ctx, botID, t.UserID, "📩 پاسخ پشتیبانی:\n\n"+reply, t.ID)
	} else if s.sendTo != nil {
		_ = s.sendTo(ctx, botID, t.UserID, "📩 پاسخ پشتیبانی:\n\n"+reply)
	}
	return nil
}

// ContinueTicket appends the user's answer to an open ticket and returns it to
// the admins' queue. Reports ok=false when the ticket is closed or unknown.
func (s *ModerationService) ContinueTicket(ctx context.Context, botID uuid.UUID, id int64, body string) (bool, error) {
	if body == "" {
		return false, fmt.Errorf("متن پیام خالی است")
	}
	return s.repo.AddUserMessage(ctx, botID, id, body)
}

// CloseTicket closes a ticket (any non-closed state).
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
