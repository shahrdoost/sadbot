package payment

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"sadbot/internal/telegram"

	"sadbot/internal/domain"
)

// OpsChannel posts a notification for every payment that settles, so the
// operator sees money arriving across all bots in one place.
type OpsChannel struct {
	client *telegram.Client
	chatID string
	bots   BotNamer
}

// BotNamer resolves a bot's display name. Narrow on purpose: the payment
// package should not depend on the whole bot repository.
type BotNamer interface {
	BotName(ctx context.Context, botID uuid.UUID) string
}

func NewOpsChannel(token, chatID string, bots BotNamer) *OpsChannel {
	if token == "" || chatID == "" {
		return nil
	}
	return &OpsChannel{client: telegram.NewClient(token), chatID: chatID, bots: bots}
}

// NewOpsChannelWithClient is for tests that stand in for the Bot API endpoint.
func NewOpsChannelWithClient(client *telegram.Client, chatID string, bots BotNamer) *OpsChannel {
	return &OpsChannel{client: client, chatID: chatID, bots: bots}
}

// PaymentSettled posts the notice. Failures are logged and swallowed: an
// unreachable ops channel must never affect a payment the buyer has paid.
func (o *OpsChannel) PaymentSettled(ctx context.Context, p *domain.Payment, who Payer) {
	if o == nil || p == nil {
		return
	}
	o.paymentPost(ctx, p, who, true, "")
}

// PaymentFailed posts the notice for a payment that did not complete, so a
// bot whose gateway rejects it — or whose buyer walks away — leaves a trail.
func (o *OpsChannel) PaymentFailed(ctx context.Context, p *domain.Payment, who Payer, reason string) {
	if o == nil || p == nil {
		return
	}
	o.paymentPost(ctx, p, who, false, reason)
}

// paymentPost is the one shape both outcomes share, so a failure and a success
// can be compared line by line in the channel.
func (o *OpsChannel) paymentPost(ctx context.Context, p *domain.Payment, who Payer, settled bool, reason string) {
	name := ""
	if o.bots != nil {
		name = o.bots.BotName(ctx, p.BotID)
	}
	if name == "" {
		name = p.BotID.String()[:8]
	}

	var b strings.Builder
	if settled {
		b.WriteString("💳 پرداخت موفق\n")
	} else {
		b.WriteString("⚠️ پرداخت ناموفق\n")
	}
	fmt.Fprintf(&b, "بات: %s\n", name)
	if line := who.DisplayName(); line != "" {
		fmt.Fprintf(&b, "کاربر: %s\n", line)
	}
	if ref := who.Reference(); ref != "" {
		fmt.Fprintf(&b, "شناسه: %s\n", ref)
	}
	if who.Platform != "" {
		fmt.Fprintf(&b, "پلتفرم: %s\n", displayPlatform(who.Platform))
	}
	fmt.Fprintf(&b, "مبلغ: %s تومان\n", thousands(p.AmountRials/10))
	switch p.Kind {
	case KindCoinPackage:
		pkg := ""
		if p.PackageID != nil {
			pkg = *p.PackageID
		}
		fmt.Fprintf(&b, "مورد: %s سکه (بستهٔ %s)\n", thousands(int64(p.Coins)), pkg)
	case KindSubscription:
		b.WriteString("مورد: اشتراک ۳۰ روزه\n")
	default:
		fmt.Fprintf(&b, "مورد: %s\n", p.Kind)
	}
	if reason != "" {
		fmt.Fprintf(&b, "علت: %s\n", reason)
	}
	fmt.Fprintf(&b, "زمان: %s", p.CreatedAt.In(tehran()).Format("2006-01-02 15:04"))

	o.post(ctx, b.String(), p.ID.String())
}

// ExternalReport is one platform reporting its own payments to this channel.
// The platforms differ too much to share a schema (sadai bills a wallet,
// sadbot sells coins), so the shape is deliberately flat and free-form.
type ExternalReport struct {
	Platform   string `json:"platform"`
	Status     string `json:"status"` // "success" | "failed"
	Amount     int64  `json:"amount"` // tomans
	Currency   string `json:"currency,omitempty"`
	// User is the human handle. The first non-empty of these wins, so a
	// platform that has only a phone number can still send something the
	// operator recognises.
	User       string `json:"user,omitempty"`
	Username   string `json:"username,omitempty"`
	FirstName  string `json:"first_name,omitempty"`
	LastName   string `json:"last_name,omitempty"`
	Phone      string `json:"phone,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	Item       string `json:"item,omitempty"`   // what was bought
	Detail     string `json:"detail,omitempty"` // free text: plan, tokens, error
	ExternalID string `json:"external_id,omitempty"`
	At         string `json:"at,omitempty"` // RFC3339; defaults to now
}

// displayUser picks the most human identifier available, in the order the
// operator finds useful: a real name, then a @username, then a phone number,
// then a raw id.
func (r ExternalReport) displayUser() string {
	if name := strings.TrimSpace(r.FirstName + " " + r.LastName); name != "" {
		return name
	}
	if r.User != "" {
		return r.User
	}
	if r.Username != "" {
		return "@" + strings.TrimPrefix(r.Username, "@")
	}
	if r.Phone != "" {
		return r.Phone
	}
	return r.UserID
}

// userReference keeps whatever else identifies the payer, so a display name
// that repeats across users can still be traced.
func (r ExternalReport) userReference() string {
	var parts []string
	if r.Username != "" {
		parts = append(parts, "@"+strings.TrimPrefix(r.Username, "@"))
	}
	if r.Phone != "" {
		parts = append(parts, r.Phone)
	}
	if r.UserID != "" {
		parts = append(parts, r.UserID)
	}
	return strings.Join(parts, " — ")
}

// PaymentReported posts a payment another platform told us about.
func (o *OpsChannel) PaymentReported(ctx context.Context, r ExternalReport) {
	if o == nil {
		return
	}
	var b strings.Builder
	if strings.EqualFold(r.Status, "failed") {
		b.WriteString("⚠️ پرداخت ناموفق\n")
	} else {
		b.WriteString("💳 پرداخت موفق\n")
	}
	fmt.Fprintf(&b, "پلتفرم: %s\n", displayPlatform(r.Platform))
	currency := r.Currency
	if currency == "" {
		currency = "تومان"
	}
	fmt.Fprintf(&b, "مبلغ: %s %s\n", thousands(r.Amount), currency)
	if who := r.displayUser(); who != "" {
		fmt.Fprintf(&b, "کاربر: %s\n", who)
	}
	if ref := r.userReference(); ref != "" {
		fmt.Fprintf(&b, "شناسهٔ کاربر: %s\n", ref)
	}
	if r.Item != "" {
		fmt.Fprintf(&b, "مورد: %s\n", r.Item)
	}
	if r.Detail != "" {
		fmt.Fprintf(&b, "جزئیات: %s\n", r.Detail)
	}
	if r.ExternalID != "" {
		fmt.Fprintf(&b, "شمارهٔ تراکنش: %s\n", r.ExternalID)
	}
	at := time.Now()
	if r.At != "" {
		if parsed, err := time.Parse(time.RFC3339, r.At); err == nil {
			at = parsed
		}
	}
	fmt.Fprintf(&b, "زمان: %s", at.In(tehran()).Format("2006-01-02 15:04"))

	o.post(ctx, b.String(), r.ExternalID)
}

func (o *OpsChannel) post(ctx context.Context, text, ref string) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := o.client.SendMessage(cctx, telegram.SendMessagePayload{ChatID: o.chatID, Text: text}); err != nil {
		log.Printf("[payment] ops channel post %s: %v", ref, err)
	}
}

// displayPlatform keeps the channel readable for the platforms we receive
// reports from, falling back to whatever the caller sent.
func displayPlatform(p string) string {
	switch strings.ToLower(p) {
	case "sadai", "sadgpt", "sadgpt.ir":
		return "سعدی (sadgpt.ir)"
	case "kat", "cr0.ir":
		return "کت (cr0.ir)"
	case "telegram":
		return "تلگرام"
	default:
		if p == "" {
			return "نامشخص"
		}
		return p
	}
}

// thousands groups digits ("10000" -> "10,000").
func thousands(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func tehran() *time.Location {
	if loc, err := time.LoadLocation("Asia/Tehran"); err == nil {
		return loc
	}
	return time.UTC
}
