package payment

import (
	"context"
	"fmt"
	"log"
	"time"

	"sadbot/internal/domain"

	"github.com/google/uuid"
)

// PaymentStore is the payments/subscriptions persistence the service needs.
type PaymentStore interface {
	Create(ctx context.Context, p *domain.Payment) (*domain.Payment, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	ListPending(ctx context.Context, maxAge time.Duration) ([]domain.Payment, error)
	LatestPendingForUser(ctx context.Context, botID uuid.UUID, userID int64) (*domain.Payment, error)
	MarkVerified(ctx context.Context, id uuid.UUID) (bool, error)
	MarkFailed(ctx context.Context, id uuid.UUID, status string) (bool, error)
	ActivateSubscription(ctx context.Context, botID uuid.UUID, userID int64, paymentID uuid.UUID, amountRials int64, days int) (*domain.Subscription, error)
	ExpireSubscriptions(ctx context.Context) (int64, error)
}

// CoinCreditor adds coins to a user's wallet (dating template).
type CoinCreditor interface {
	AddCoins(ctx context.Context, botID uuid.UUID, userID int64, n int) (int, error)
}

// Notifier tells the user their payment result via their bot.
type Notifier interface {
	NotifyUser(botID uuid.UUID, userID int64, text string)
}

const (
	KindCoinPackage  = "coin_package"
	KindSubscription = "subscription"
)

// Service ties gateway + store + wallet together for both the bot UX and
// the public callback / auto-verify sweeper.
type Service struct {
	store       PaymentStore
	coins       CoinCreditor
	gateway     *Gateway
	notifier    Notifier
	callbackURL string
}

func NewService(store PaymentStore, coins CoinCreditor, gateway *Gateway, notifier Notifier, callbackURL string) *Service {
	return &Service{store: store, coins: coins, gateway: gateway, notifier: notifier, callbackURL: callbackURL}
}

// BuyPackage creates a pending coin_package payment and returns the pay URL.
// Dev users skip the gateway: the payment is completed immediately.
func (s *Service) BuyPackage(ctx context.Context, botID uuid.UUID, userID int64, telegramID int64, packageID string, coins, priceTomans int64) (payURL string, err error) {
	p, err := s.store.Create(ctx, &domain.Payment{
		BotID:       botID,
		UserID:      &userID,
		AmountRials: priceTomans * 10,
		Kind:        KindCoinPackage,
		Gateway:     s.gateway.host,
		Status:      "pending",
		Coins:       int(coins),
		PackageID:   &packageID,
	})
	if err != nil {
		return "", err
	}
	if s.gateway.DevAllowed(telegramID) {
		return "dev://payment-success", s.fulfill(ctx, p.ID)
	}
	res, err := s.gateway.CreatePayment(ctx, p.ID.String(), priceTomans, s.callbackURL)
	if err != nil {
		_, _ = s.store.MarkFailed(ctx, p.ID, "failed")
		return "", err
	}
	return res.PayURL, nil
}

// BuySubscription creates a pending subscription payment (30 days on verify).
func (s *Service) BuySubscription(ctx context.Context, botID uuid.UUID, userID int64, telegramID int64, priceTomans int64) (payURL string, err error) {
	p, err := s.store.Create(ctx, &domain.Payment{
		BotID:       botID,
		UserID:      &userID,
		AmountRials: priceTomans * 10,
		Kind:        KindSubscription,
		Gateway:     s.gateway.host,
		Status:      "pending",
	})
	if err != nil {
		return "", err
	}
	if s.gateway.DevAllowed(telegramID) {
		return "dev://payment-success", s.fulfill(ctx, p.ID)
	}
	res, err := s.gateway.CreatePayment(ctx, p.ID.String(), priceTomans, s.callbackURL)
	if err != nil {
		_, _ = s.store.MarkFailed(ctx, p.ID, "failed")
		return "", err
	}
	return res.PayURL, nil
}

// Complete verifies with the gateway and fulfills the payment exactly once
// (CAS pending→verified). Unknown gateway state leaves it pending for retry.
func (s *Service) Complete(ctx context.Context, paymentID uuid.UUID) error {
	p, err := s.store.GetByID(ctx, paymentID)
	if err != nil {
		return err
	}
	if p.Status != "pending" {
		return nil // already fulfilled
	}
	ok, err := s.gateway.Verify(ctx, paymentID.String())
	if err != nil {
		return err // retryable — stays pending
	}
	if !ok {
		return fmt.Errorf("payment %s not verified yet", paymentID)
	}
	return s.fulfill(ctx, paymentID)
}

// fulfill credits the user exactly once (CAS on pending→verified).
func (s *Service) fulfill(ctx context.Context, paymentID uuid.UUID) error {
	p, err := s.store.GetByID(ctx, paymentID)
	if err != nil {
		return err
	}
	if p.Status != "pending" {
		return nil
	}
	if p.UserID == nil {
		return fmt.Errorf("payment %s has no user", paymentID)
	}
	fulfilled, err := s.store.MarkVerified(ctx, paymentID)
	if err != nil {
		return err
	}
	if !fulfilled {
		return nil // another sweeper beat us — do not double-credit
	}
	switch p.Kind {
	case KindCoinPackage:
		if p.Coins > 0 {
			_, err = s.coins.AddCoins(ctx, p.BotID, *p.UserID, p.Coins)
		}
	case KindSubscription:
		_, err = s.store.ActivateSubscription(ctx, p.BotID, *p.UserID, p.ID, p.AmountRials, 30)
	}
	if err != nil {
		log.Printf("[payment] fulfill %s kind=%s: %v", paymentID, p.Kind, err)
		return err
	}
	if s.notifier != nil {
		text := fmt.Sprintf("✅ پرداخت تایید شد!\n+%d سکه به حسابت اضافه شد.", p.Coins)
		if p.Kind == KindSubscription {
			text = "✅ اشتراک پریموم فعال شد!\nتا ۳۰ روز جستجوی رایگان و ۳۰٪ تخفیف خرید سکه داری."
		}
		s.notifier.NotifyUser(p.BotID, *p.UserID, text)
	}
	return nil
}

// VerifyUserPayment checks the user's latest pending payment (manual button).
func (s *Service) VerifyUserPayment(ctx context.Context, p *domain.Payment) error {
	return s.Complete(ctx, p.ID)
}

// VerifyLatestPending finds the user's newest pending payment and completes it.
func (s *Service) VerifyLatestPending(ctx context.Context, botID uuid.UUID, userID int64) error {
	latest, err := s.store.LatestPendingForUser(ctx, botID, userID)
	if err != nil || latest == nil {
		return fmt.Errorf("no pending payment")
	}
	return s.Complete(ctx, latest.ID)
}
