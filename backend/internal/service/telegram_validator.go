package service

import (
	"context"
	"errors"

	"sadbot/internal/domain"
	"sadbot/internal/telegram"
)

// TelegramValidator validates bot tokens via getMe on their platform endpoint.
type TelegramValidator struct{}

func NewTelegramValidator() *TelegramValidator { return &TelegramValidator{} }

func (v *TelegramValidator) ValidateToken(ctx context.Context, platform, token string) (string, error) {
	base := telegram.BaseTelegram
	if platform == domain.PlatformKat {
		base = telegram.BaseKat
	}
	me, err := telegram.NewClientWithBase(base, token).GetMe(ctx)
	if err != nil {
		return "", err
	}
	if me.Username == "" {
		return "", errors.New("bot has no username")
	}
	return "@" + me.Username, nil
}
