package service

import (
	"context"
	"errors"

	"sadbot/internal/telegram"
)

// TelegramValidator validates bot tokens via getMe.
type TelegramValidator struct{}

func NewTelegramValidator() *TelegramValidator { return &TelegramValidator{} }

func (v *TelegramValidator) ValidateToken(ctx context.Context, token string) (string, error) {
	me, err := telegram.NewClient(token).GetMe(ctx)
	if err != nil {
		return "", err
	}
	if me.Username == "" {
		return "", errors.New("bot has no username")
	}
	return "@" + me.Username, nil
}
