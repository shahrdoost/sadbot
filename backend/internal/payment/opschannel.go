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

// PaymentSettled posts the notice. Failures are logged and swallowed: an
// unreachable ops channel must never affect a payment the buyer has paid.
func (o *OpsChannel) PaymentSettled(ctx context.Context, p *domain.Payment, platform, telegramID string) {
	if o == nil || p == nil {
		return
	}
	name := ""
	if o.bots != nil {
		name = o.bots.BotName(ctx, p.BotID)
	}
	if name == "" {
		name = p.BotID.String()[:8]
	}

	var b strings.Builder
	b.WriteString("💳 پرداخت موفق\n")
	fmt.Fprintf(&b, "بات: %s\n", name)
	if platform != "" {
		fmt.Fprintf(&b, "پلتفرم: %s", platform)
		if telegramID != "" {
			fmt.Fprintf(&b, " — %s", telegramID)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "مبلغ: %s تومان\n", thousands(p.AmountRials/10))
	switch p.Kind {
	case KindCoinPackage:
		pkg := ""
		if p.PackageID != nil {
			pkg = *p.PackageID
		}
		fmt.Fprintf(&b, "نوع: %s سکه (بستهٔ %s)\n", thousands(int64(p.Coins)), pkg)
	case KindSubscription:
		b.WriteString("نوع: اشتراک ۳۰ روزه\n")
	default:
		fmt.Fprintf(&b, "نوع: %s\n", p.Kind)
	}
	fmt.Fprintf(&b, "زمان: %s", p.CreatedAt.In(tehran()).Format("2006-01-02 15:04"))

	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := o.client.SendMessage(cctx, telegram.SendMessagePayload{ChatID: o.chatID, Text: b.String()}); err != nil {
		log.Printf("[payment] ops channel post for %s: %v", p.ID, err)
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
