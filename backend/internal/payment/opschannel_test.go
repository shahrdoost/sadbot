package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"sadbot/internal/domain"
	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// opsRecorder stands in for the Bot API endpoint the ops channel posts to.
type opsRecorder struct {
	mu     sync.Mutex
	posts  []string
	status int
}

func (o *opsRecorder) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var payload struct {
		ChatID any    `json:"chat_id"`
		Text   string `json:"text"`
	}
	_ = json.Unmarshal(body, &payload)
	o.mu.Lock()
	o.posts = append(o.posts, payload.Text)
	status := o.status
	o.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"ok":false,"description":"boom"}`))
		return
	}
	_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
}

func (o *opsRecorder) recorded() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.posts...)
}

func newOpsService(t *testing.T, rec *opsRecorder, store *memPayments, coins *memCoins) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(rec.handler))
	t.Cleanup(srv.Close)

	svc := NewService(store, coins, nil, nil, "cb")
	ops := &OpsChannel{
		client: telegram.NewClientWithBase(srv.URL+"/bot", "TESTTOKEN"),
		chatID: "-1004306622791",
	}
	svc.SetOpsChannel(ops, nil)
	return svc
}

// A verified payment must land in the ops channel exactly once, with the
// amount, the bot, and the buyer's platform in it.
func TestOpsChannelPostsOnVerifiedPayment(t *testing.T) {
	rec := &opsRecorder{}
	store := newMemPayments()
	svc := newOpsService(t, rec, store, newMemCoins())

	botID := uuid.New()
	userID := int64(7)
	pkg, _ := svc.store.Create(context.Background(), &domain.Payment{
		BotID: botID, UserID: &userID, AmountRials: 100_000,
		Kind: KindCoinPackage, Gateway: "catroom", Status: "pending",
		Coins: 1, PackageID: strptr("pkg_1"),
	})

	// Complete() re-checks with the gateway; drive fulfill directly the way
	// a settled payment does.
	if err := svc.fulfill(context.Background(), pkg.ID); err != nil {
		t.Fatalf("fulfill: %v", err)
	}

	posts := rec.recorded()
	if len(posts) != 1 {
		t.Fatalf("posts = %d (want 1): %v", len(posts), posts)
	}
	text := posts[0]
	for _, want := range []string{"پرداخت موفق", "10,000", "pkg_1", "1 سکه"} {
		if !strings.Contains(text, want) {
			t.Errorf("post missing %q:\n%s", want, text)
		}
	}
}

// Re-delivering the same settled payment must not post twice: the buyer's
// credit is CAS-guarded and so is the channel notice.
func TestOpsChannelDoesNotRepeatOnRedelivery(t *testing.T) {
	rec := &opsRecorder{}
	store := newMemPayments()
	svc := newOpsService(t, rec, store, newMemCoins())

	botID := uuid.New()
	userID := int64(7)
	pkg, _ := svc.store.Create(context.Background(), &domain.Payment{
		BotID: botID, UserID: &userID, AmountRials: 100_000,
		Kind: KindCoinPackage, Gateway: "catroom", Status: "pending",
		Coins: 1, PackageID: strptr("pkg_1"),
	})

	if err := svc.fulfill(context.Background(), pkg.ID); err != nil {
		t.Fatalf("fulfill: %v", err)
	}
	if err := svc.fulfill(context.Background(), pkg.ID); err != nil {
		t.Fatalf("re-fulfill: %v", err)
	}

	if got := len(rec.recorded()); got != 1 {
		t.Fatalf("posts = %d (want 1 — duplicate notice)", got)
	}
}

// An unreachable channel must never fail the payment: the buyer paid, and the
// coins are already credited.
func TestOpsChannelFailureDoesNotBreakSettlement(t *testing.T) {
	rec := &opsRecorder{status: http.StatusInternalServerError}
	store := newMemPayments()
	coins := newMemCoins()
	svc := newOpsService(t, rec, store, coins)

	botID := uuid.New()
	userID := int64(7)
	pkg, _ := svc.store.Create(context.Background(), &domain.Payment{
		BotID: botID, UserID: &userID, AmountRials: 100_000,
		Kind: KindCoinPackage, Gateway: "catroom", Status: "pending",
		Coins: 1, PackageID: strptr("pkg_1"),
	})

	if err := svc.fulfill(context.Background(), pkg.ID); err != nil {
		t.Fatalf("fulfill returned %v; an ops-channel outage must not fail settlement", err)
	}
	if coins.balance[fmt.Sprintf("%s%d", botID, userID)] != 1 {
		t.Fatal("coins were not credited")
	}
	got, _ := store.GetByID(context.Background(), pkg.ID)
	if got.Status != "verified" {
		t.Fatalf("status = %s (want verified)", got.Status)
	}
}

func TestThousandsGrouping(t *testing.T) {
	cases := map[int64]string{
		0: "0", 999: "999", 1000: "1,000", 10000: "10,000",
		1234567: "1,234,567", -45000: "-45,000",
	}
	for in, want := range cases {
		if got := thousands(in); got != want {
			t.Errorf("thousands(%d) = %q (want %q)", in, got, want)
		}
	}
}

func strptr(s string) *string { return &s }

// A rejected payment must leave a trail: the buyer sees a message and the
// operator sees the failure in the channel.
func TestOpsChannelPostsOnRejectedPayment(t *testing.T) {
	rec := &opsRecorder{}
	store := newMemPayments()
	svc := newOpsService(t, rec, store, newMemCoins())

	botID := uuid.New()
	userID := int64(7)
	p, _ := svc.store.Create(context.Background(), &domain.Payment{
		BotID: botID, UserID: &userID, AmountRials: 500_000,
		Kind: KindCoinPackage, Gateway: "catroom", Status: "pending",
		Coins: 5, PackageID: strptr("pkg_5"),
	})

	svc.reportFailure(context.Background(), p, "درگاه پرداخت را رد کرد", false)

	posts := rec.recorded()
	if len(posts) != 1 {
		t.Fatalf("posts = %d (want 1): %v", len(posts), posts)
	}
	for _, want := range []string{"پرداخت ناموفق", "50,000", "درگاه پرداخت را رد کرد"} {
		if !strings.Contains(posts[0], want) {
			t.Errorf("failure post missing %q:\n%s", want, posts[0])
		}
	}
}

// The operator must see a human handle, and a name must never print blank.
func TestPayerDisplayNameFallback(t *testing.T) {
	cases := []struct {
		name string
		in   Payer
		want string
	}{
		{"full name wins", Payer{FirstName: "کوشان", LastName: "شهردوست", Username: "k", TelegramID: 5}, "کوشان شهردوست"},
		{"profile name when no telegram name", Payer{ProfileID: "نگار", Username: "n", TelegramID: 5}, "نگار"},
		{"username when no name", Payer{Username: "amir", TelegramID: 5}, "@amir"},
		{"numeric id as last resort", Payer{TelegramID: 1780243204}, "1780243204"},
		{"nothing at all stays empty", Payer{}, ""},
	}
	for _, tc := range cases {
		if got := tc.in.DisplayName(); got != tc.want {
			t.Errorf("%s: DisplayName() = %q (want %q)", tc.name, got, tc.want)
		}
	}
}

func TestPayerReferenceCarriesIds(t *testing.T) {
	p := Payer{Username: "amir", TelegramID: 1780243204}
	if got := p.Reference(); got != "@amir — 1780243204" {
		t.Errorf("Reference() = %q", got)
	}
	if got := (Payer{TelegramID: 42}).Reference(); got != "42" {
		t.Errorf("id-only Reference() = %q", got)
	}
}

// The channel post itself must carry the name, not just the id.
func TestOpsChannelPostShowsUserName(t *testing.T) {
	rec := &opsRecorder{}
	store := newMemPayments()
	svc := newOpsService(t, rec, store, newMemCoins())

	botID := uuid.New()
	userID := int64(7)
	p, _ := svc.store.Create(context.Background(), &domain.Payment{
		BotID: botID, UserID: &userID, AmountRials: 100_000,
		Kind: KindCoinPackage, Gateway: "catroom", Status: "pending",
		Coins: 1, PackageID: strptr("pkg_1"),
	})

	who := Payer{Platform: "kat", TelegramID: 1780243204, FirstName: "کوشان"}
	svc.ops.PaymentSettled(context.Background(), p, who)

	posts := rec.recorded()
	if len(posts) != 1 {
		t.Fatalf("posts = %d", len(posts))
	}
	for _, want := range []string{"کاربر: کوشان", "شناسه: 1780243204", "کت (cr0.ir)"} {
		if !strings.Contains(posts[0], want) {
			t.Errorf("post missing %q:\n%s", want, posts[0])
		}
	}
}

// A platform that only knows a phone number must still produce a usable line.
func TestExternalReportUserFallbacks(t *testing.T) {
	cases := []struct{ name string; in ExternalReport; wantUser, wantRef string }{
		{"phone only", ExternalReport{Phone: "+989121234567"}, "+989121234567", "+989121234567"},
		{"username", ExternalReport{Username: "u1"}, "@u1", "@u1"},
		{"full name + ids", ExternalReport{FirstName: "علی", LastName: "ر", Username: "a", UserID: "9"}, "علی ر", "@a — 9"},
		{"raw id last", ExternalReport{UserID: "9"}, "9", "9"},
	}
	for _, tc := range cases {
		if got := tc.in.displayUser(); got != tc.wantUser {
			t.Errorf("%s: displayUser() = %q (want %q)", tc.name, got, tc.wantUser)
		}
		if got := tc.in.userReference(); got != tc.wantRef {
			t.Errorf("%s: userReference() = %q (want %q)", tc.name, got, tc.wantRef)
		}
	}
}
