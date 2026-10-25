package payment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"sadbot/internal/domain"

	"github.com/google/uuid"
)

type memPayments struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*domain.Payment
	subs map[string]*domain.Subscription
}

func newMemPayments() *memPayments {
	return &memPayments{rows: map[uuid.UUID]*domain.Payment{}, subs: map[string]*domain.Subscription{}}
}

func (m *memPayments) Create(_ context.Context, p *domain.Payment) (*domain.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *p
	cp.ID = uuid.New()
	cp.CreatedAt = time.Now()
	m.rows[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (m *memPayments) GetByID(_ context.Context, id uuid.UUID) (*domain.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.rows[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *p
	return &cp, nil
}

func (m *memPayments) ListPending(_ context.Context, maxAge time.Duration) ([]domain.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Payment
	for _, p := range m.rows {
		if p.Status == "pending" && time.Since(p.CreatedAt) < maxAge {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (m *memPayments) LatestPendingForUser(_ context.Context, botID uuid.UUID, userID int64) (*domain.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var newest *domain.Payment
	for _, p := range m.rows {
		if p.BotID == botID && p.UserID != nil && *p.UserID == userID && p.Status == "pending" {
			if newest == nil || p.CreatedAt.After(newest.CreatedAt) {
				cp := *p
				newest = &cp
			}
		}
	}
	if newest == nil {
		return nil, errors.New("no pending payment")
	}
	return newest, nil
}

func (m *memPayments) MarkVerified(_ context.Context, id uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.rows[id]
	if !ok || p.Status != "pending" {
		return false, nil
	}
	p.Status = "verified"
	now := time.Now()
	p.VerifiedAt = &now
	return true, nil
}

func (m *memPayments) MarkFailed(_ context.Context, id uuid.UUID, status string) (bool, error) {
	switch status {
	case "pending", "verified", "failed", "refunded":
	default:
		// mirror the payments CHECK constraint — "expired" must fail loudly
		return false, fmt.Errorf("invalid status %q", status)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.rows[id]; ok && p.Status == "pending" {
		p.Status = status
		return true, nil
	}
	return false, nil
}

func (m *memPayments) ActivateSubscription(_ context.Context, botID uuid.UUID, userID int64, paymentID uuid.UUID, amountRials int64, days int) (*domain.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := &domain.Subscription{
		ID: uuid.New(), BotID: botID, UserID: userID, Plan: "premium_monthly",
		Status: "active", AmountRials: amountRials, PaymentID: &paymentID,
		StartedAt: time.Now(), ExpiresAt: time.Now().Add(time.Duration(days) * 24 * time.Hour),
	}
	m.subs[botID.String()+":"+itoa(userID)] = s
	cp := *s
	return &cp, nil
}

func (m *memPayments) ExpireSubscriptions(context.Context) (int64, error) { return 0, nil }

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

type memCoins struct {
	mu      sync.Mutex
	balance map[string]int
}

func newMemCoins() *memCoins { return &memCoins{balance: map[string]int{}} }

func (c *memCoins) AddCoins(_ context.Context, botID uuid.UUID, userID int64, n int) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := botID.String() + itoa(userID)
	c.balance[k] += n
	return c.balance[k], nil
}

type memNotifier struct {
	texts []string
}

func (n *memNotifier) NotifyUser(_ uuid.UUID, _ int64, text string) {
	n.texts = append(n.texts, text)
}

func devGateway() *Gateway {
	return &Gateway{devMode: true, devSet: map[int64]bool{42: true}}
}

func TestBuyPackage_DevModeCreditsCoins(t *testing.T) {
	store := newMemPayments()
	coins := newMemCoins()
	notes := &memNotifier{}
	svc := NewService(store, coins, devGateway(), notes, "https://panel/api/v1/payments/callback")
	botID := uuid.New()

	url, err := svc.BuyPackage(context.Background(), botID, 7, 42, "pkg_5", 5, 50_000)
	if err != nil {
		t.Fatal(err)
	}
	if url != "dev://payment-success" {
		t.Fatalf("url = %q", url)
	}
	if coins.balance[botID.String()+"7"] != 5 {
		t.Fatalf("balance = %d", coins.balance[botID.String()+"7"])
	}
	if len(notes.texts) != 1 {
		t.Fatalf("notifications = %v", notes.texts)
	}
}

func TestBuySubscription_DevModeActivates(t *testing.T) {
	store := newMemPayments()
	coins := newMemCoins()
	notes := &memNotifier{}
	svc := NewService(store, coins, devGateway(), notes, "cb")
	botID := uuid.New()

	if _, err := svc.BuySubscription(context.Background(), botID, 9, 42, 280_000); err != nil {
		t.Fatal(err)
	}
	sub, ok := store.subs[botID.String()+":9"]
	if !ok || sub.Status != "active" {
		t.Fatalf("subs = %+v", store.subs)
	}
	if sub.ExpiresAt.Sub(sub.StartedAt) < 29*24*time.Hour {
		t.Fatalf("expiry = %v", sub.ExpiresAt)
	}
}

func TestComplete_Idempotent(t *testing.T) {
	store := newMemPayments()
	coins := newMemCoins()
	svc := NewService(store, coins, devGateway(), nil, "cb")
	botID := uuid.New()

	if _, err := svc.BuyPackage(context.Background(), botID, 7, 42, "pkg_1", 1, 10_000); err != nil {
		t.Fatal(err)
	}
	// second Complete on the same payment must not double-credit
	var pid uuid.UUID
	for id, p := range store.rows {
		if p.Kind == KindCoinPackage {
			pid = id
		}
	}
	if err := svc.Complete(context.Background(), pid); err != nil {
		t.Fatalf("re-complete: %v", err)
	}
	if coins.balance[botID.String()+"7"] != 1 {
		t.Fatalf("balance = %d (double credit?)", coins.balance[botID.String()+"7"])
	}
}

func TestGatewayMode_NotDev_CreatesPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(srv.Close)

	store := newMemPayments()
	gw := &Gateway{host: srv.URL, http: srv.Client(), devSet: map[int64]bool{}}
	svc := NewService(store, newMemCoins(), gw, nil, "cb")
	botID := uuid.New()
	userID := int64(7)

	// non-dev user with failing gateway → payment marked failed, error returned
	if _, err := svc.BuyPackage(context.Background(), botID, userID, 7, "pkg_1", 1, 10_000); err == nil {
		t.Fatal("expected gateway error")
	}
	for _, p := range store.rows {
		if p.Status != "failed" {
			t.Fatalf("status = %s (want failed)", p.Status)
		}
	}
}
