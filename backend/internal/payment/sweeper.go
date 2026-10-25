package payment

import (
	"context"
	"log"
	"strings"
	"time"
)

// Sweeper mirrors kpkchat's schedulers: auto-verify fresh pending payments
// every minute and expire lapsed subscriptions every 6 hours.
type Sweeper struct {
	svc *Service
}

func NewSweeper(svc *Service) *Sweeper { return &Sweeper{svc: svc} }

const (
	autoVerifyMaxAge = 10 * time.Minute
	verifyTick       = time.Minute
	expireTick       = 6 * time.Hour
)

func (s *Sweeper) Run(ctx context.Context) {
	verify := time.NewTicker(verifyTick)
	expire := time.NewTicker(expireTick)
	defer verify.Stop()
	defer expire.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-verify.C:
			s.autoVerify(ctx)
		case <-expire.C:
			if n, err := s.svc.store.ExpireSubscriptions(ctx); err != nil {
				log.Printf("[payment] expire subs: %v", err)
			} else if n > 0 {
				log.Printf("[payment] expired %d subscriptions", n)
			}
		}
	}
}

func (s *Sweeper) autoVerify(ctx context.Context) {
	pending, err := s.svc.store.ListPending(ctx, autoVerifyMaxAge)
	if err != nil {
		log.Printf("[payment] list pending: %v", err)
		return
	}
	for _, p := range pending {
		if err := s.svc.Complete(ctx, p.ID); err != nil {
			// gateway said not-yet-paid is normal; log others
			if !isNotVerified(err) {
				log.Printf("[payment] auto-verify %s: %v", p.ID, err)
			}
			continue
		}
	}
	// sweep expired pendings that stayed beyond max age
	stale, err := s.svc.store.ListPending(ctx, time.Hour*24*365) // all pending
	if err != nil {
		return
	}
	for _, p := range stale {
		if time.Since(p.CreatedAt) <= autoVerifyMaxAge {
			continue
		}
		// status must stay inside the payments CHECK constraint
		// (pending/verified/failed/refunded). "expired" used to violate it,
		// MarkFailed errored silently, the row stayed pending forever and the
		// user got this message every minute.
		ok, err := s.svc.store.MarkFailed(ctx, p.ID, "failed")
		if err != nil {
			log.Printf("[payment] expire %s: %v", p.ID, err)
			continue
		}
		if ok && s.svc.notifier != nil && p.UserID != nil {
			s.svc.notifier.NotifyUser(p.BotID, *p.UserID,
				"⏰ مهلت پرداخت شما تمام شد.\nاز بخش خرید سکه دوباره اقدام کن.")
		}
	}
}

func isNotVerified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not verified yet")
}
