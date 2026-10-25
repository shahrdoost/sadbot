package service

import (
	"context"
	"errors"
	"io"
	"strconv"

	"sadbot/internal/domain"
	"sadbot/internal/tgsidecar"

	"github.com/google/uuid"
)

type TelegramAccountRepository interface {
	Create(ctx context.Context, adminID uuid.UUID, label string, tgUserID int64, phone, sidecarID string) (*domain.TelegramAccount, error)
	List(ctx context.Context, adminID *uuid.UUID) ([]domain.TelegramAccount, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.TelegramAccount, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// TelegramAccountService fronts the MTProto sidecar with ownership checks.
type TelegramAccountService struct {
	accounts TelegramAccountRepository
	sidecar  *tgsidecar.Client
	audit    AuditLogger
	bots     *BotService
}

func NewTelegramAccountService(accounts TelegramAccountRepository, sidecar *tgsidecar.Client, audit AuditLogger, bots *BotService) *TelegramAccountService {
	return &TelegramAccountService{accounts: accounts, sidecar: sidecar, audit: audit, bots: bots}
}

var ErrSidecarUnavailable = errors.New("سرویس اکانت تلگرام پیکربندی نشده است")

func (s *TelegramAccountService) sidecarOrErr() (*tgsidecar.Client, error) {
	if s.sidecar == nil {
		return nil, ErrSidecarUnavailable
	}
	return s.sidecar, nil
}

func (s *TelegramAccountService) StartLogin(ctx context.Context, phone string) (string, error) {
	c, err := s.sidecarOrErr()
	if err != nil {
		return "", err
	}
	return c.LoginStart(ctx, phone)
}

type CompleteLoginInput struct {
	PendingID string `json:"pending_id"`
	Phone     string `json:"phone"`
	Code      string `json:"code"`
	Password  string `json:"password"`
	Label     string `json:"label"`
}

func (s *TelegramAccountService) CompleteLogin(ctx context.Context, actor *domain.Admin, in CompleteLoginInput, ip *string) (*domain.TelegramAccount, error) {
	c, err := s.sidecarOrErr()
	if err != nil {
		return nil, err
	}
	acc, err := c.LoginComplete(ctx, in.PendingID, in.Phone, in.Code, in.Password)
	if err != nil {
		return nil, err
	}
	tgID, _ := strconv.ParseInt(acc.User.ID, 10, 64)
	saved, err := s.accounts.Create(ctx, actor.ID, in.Label, tgID, in.Phone, acc.AccountID)
	if err != nil {
		return nil, err
	}
	saved.DisplayName = joinName(acc.User.FirstName, acc.User.LastName)
	saved.Username = acc.User.Username
	_ = s.audit.Insert(ctx, &actor.ID, "telegram_account.add", "telegram_account", saved.ID.String(), nil, map[string]any{"phone": in.Phone}, ip)
	return saved, nil
}

func (s *TelegramAccountService) List(ctx context.Context, actor *domain.Admin) ([]domain.TelegramAccount, error) {
	var scope *uuid.UUID
	if !actor.IsSuperAdmin() {
		scope = &actor.ID
	}
	accounts, err := s.accounts.List(ctx, scope)
	if err != nil {
		return nil, err
	}
	// enrich display names from the sidecar (best-effort)
	if c, err := s.sidecarOrErr(); err == nil {
		if live, lerr := c.ListAccounts(ctx); lerr == nil {
			byID := map[string]tgsidecar.Account{}
			for _, a := range live {
				byID[a.AccountID] = a
			}
			for i := range accounts {
				if a, ok := byID[accounts[i].SidecarAccountID]; ok {
					accounts[i].DisplayName = joinName(a.User.FirstName, a.User.LastName)
					accounts[i].Username = a.User.Username
				}
			}
		}
	}
	return accounts, nil
}

func (s *TelegramAccountService) owned(ctx context.Context, actor *domain.Admin, id uuid.UUID) (*domain.TelegramAccount, error) {
	acc, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, errors.New("اکانت یافت نشد")
	}
	if !actor.IsSuperAdmin() && acc.AdminID != actor.ID {
		return nil, errors.New("دسترسی ندارید")
	}
	return acc, nil
}

func (s *TelegramAccountService) Delete(ctx context.Context, actor *domain.Admin, id uuid.UUID, ip *string) error {
	acc, err := s.owned(ctx, actor, id)
	if err != nil {
		return err
	}
	if c, err := s.sidecarOrErr(); err == nil {
		_ = c.DeleteAccount(ctx, acc.SidecarAccountID)
	}
	if err := s.accounts.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "telegram_account.delete", "telegram_account", id.String(), nil, nil, ip)
	return nil
}

func (s *TelegramAccountService) Chats(ctx context.Context, actor *domain.Admin, id uuid.UUID) ([]tgsidecar.Chat, error) {
	c, err := s.sidecarOrErr()
	if err != nil {
		return nil, err
	}
	acc, err := s.owned(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return c.ListChats(ctx, acc.SidecarAccountID)
}

// Folders returns the account's dialog folders (panel source-folder picker).
func (s *TelegramAccountService) Folders(ctx context.Context, actor *domain.Admin, id uuid.UUID) ([]tgsidecar.Folder, error) {
	c, err := s.sidecarOrErr()
	if err != nil {
		return nil, err
	}
	acc, err := s.owned(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return c.ListFolders(ctx, acc.SidecarAccountID)
}

func (s *TelegramAccountService) Messages(ctx context.Context, actor *domain.Admin, id uuid.UUID, chatID string, limit, offsetID int) ([]tgsidecar.Message, int, error) {
	c, err := s.sidecarOrErr()
	if err != nil {
		return nil, 0, err
	}
	acc, err := s.owned(ctx, actor, id)
	if err != nil {
		return nil, 0, err
	}
	return c.ListMessages(ctx, acc.SidecarAccountID, chatID, limit, offsetID)
}

func (s *TelegramAccountService) SendMessage(ctx context.Context, actor *domain.Admin, id uuid.UUID, chatID, text string) (*tgsidecar.Message, error) {
	c, err := s.sidecarOrErr()
	if err != nil {
		return nil, err
	}
	acc, err := s.owned(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	msg, err := c.SendMessage(ctx, acc.SidecarAccountID, chatID, text)
	if err != nil {
		return nil, err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "telegram_account.send_message", "telegram_account", id.String(), nil, map[string]any{"chat_id": chatID}, nil)
	return msg, nil
}

func (s *TelegramAccountService) Photo(ctx context.Context, actor *domain.Admin, id uuid.UUID, chatID string, msgID int) (io.ReadCloser, string, error) {
	c, err := s.sidecarOrErr()
	if err != nil {
		return nil, "", err
	}
	acc, err := s.owned(ctx, actor, id)
	if err != nil {
		return nil, "", err
	}
	return c.GetPhoto(ctx, acc.SidecarAccountID, chatID, msgID)
}

func (s *TelegramAccountService) ChatPhoto(ctx context.Context, actor *domain.Admin, id uuid.UUID, chatID string) (io.ReadCloser, string, error) {
	c, err := s.sidecarOrErr()
	if err != nil {
		return nil, "", err
	}
	acc, err := s.owned(ctx, actor, id)
	if err != nil {
		return nil, "", err
	}
	return c.ChatPhoto(ctx, acc.SidecarAccountID, chatID)
}

func joinName(first, last string) string {
	switch {
	case first == "" && last == "":
		return ""
	case last == "":
		return first
	case first == "":
		return last
	default:
		return first + " " + last
	}
}

type CreateBotViaAccountInput struct {
	Name         string `json:"name"`
	TemplateKey  string `json:"template_key"`
	UsernameHint string `json:"username_hint"`
}

// CreateBot drives BotFather from the chosen MTProto account, then runs the
// normal bot-creation flow with the captured token.
func (s *TelegramAccountService) CreateBot(ctx context.Context, actor *domain.Admin, accountID uuid.UUID, in CreateBotViaAccountInput, ip *string) (*domain.Bot, error) {
	c, err := s.sidecarOrErr()
	if err != nil {
		return nil, err
	}
	acc, err := s.owned(ctx, actor, accountID)
	if err != nil {
		return nil, err
	}
	if s.bots == nil {
		return nil, errors.New("سرویس ساخت بات در دسترس نیست")
	}
	res, err := c.CreateBotViaBotFather(ctx, acc.SidecarAccountID, in.Name, in.UsernameHint)
	if err != nil {
		return nil, err
	}
	bot, err := s.bots.Create(ctx, actor, CreateBotInput{
		Name:        in.Name,
		Token:       res.Token,
		TemplateKey: in.TemplateKey,
	}, ip)
	if err != nil {
		return nil, err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "telegram_account.create_bot", "bot", bot.ID.String(), &bot.ID, map[string]any{
		"account_id": accountID.String(), "bot_username": res.Username,
	}, ip)
	return bot, nil
}
