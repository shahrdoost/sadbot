package dating

// Pending cross-platform delivery.
//
// A transient failure (endpoint flapping, side rebooting) must not tear an
// active conversation down: the send parks here and the runner retries with
// backoff. Only a permanent outcome (403 = the user blocked the bot) drops it.
// In-process by design — state that must survive a redeploy belongs in the
// database, and a message that cannot be redelivered after a reboot is rare,
// logged, and of less value than a dead chat.

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// pendRetrySchedule is the wait before retry n (1-indexed, after the first
// failed sync attempt). The fifth give-up lands ≈4½ minutes out — enough to
// ride out an endpoint blip without keeping conversations hostage.
var pendRetrySchedule = []time.Duration{
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
}

const pendCap = 500 // hard bound: a dead endpoint must not grow forever

type pendSend struct {
	label    string
	send     func() (*telegram.Message, error)
	onDone   func(*telegram.Message) // nil for side-effect-only sends
	attempts int
	next     time.Time
	gen      int64 // queue generation; resetPending() orphans older in-flight items
}

var (
	pendMu   sync.Mutex
	pend     []*pendSend
	pendGen  int64
	pendWake = make(chan struct{}, 1)
	pendOnce sync.Once
)

// enqueuePending parks a failed cross-platform send for async retry.
// send must be a self-contained closure (captures botID/platform/chatID/finals).
func enqueuePending(label string, send func() (*telegram.Message, error), onDone func(*telegram.Message)) {
	pendMu.Lock()
	if len(pend) >= pendCap {
		pendMu.Unlock()
		log.Printf("[dating] outbound full — dropping %s", label)
		return
	}
	pend = append(pend, &pendSend{
		label:  label,
		send:   send,
		onDone: onDone,
		next:   time.Now(), // the sync attempt already failed; retry at once
		gen:    pendGen,
	})
	pendMu.Unlock()
	pendOnce.Do(func() { go pendingRunner() })
	select {
	case pendWake <- struct{}{}:
	default:
	}
}

// pendingRunner wakes on enqueue and on its own tick. It outlives handler
// contexts on purpose: an outage must keep the retries alive.
func pendingRunner() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-pendWake:
			flushPending(time.Now())
		case <-t.C:
			flushPending(time.Now())
		}
	}
}

// flushPending delivers everything due at now. Exported to tests.
func flushPending(now time.Time) {
	pendMu.Lock()
	due := make([]*pendSend, 0, len(pend))
	rest := pend[:0]
	for _, item := range pend {
		if item.next.After(now) {
			rest = append(rest, item)
			continue
		}
		due = append(due, item)
	}
	pend = rest
	pendMu.Unlock()

	for _, item := range due {
		msg, err := item.send()
		switch {
		case err == nil:
			if item.onDone != nil {
				item.onDone(msg)
			}
		case relayPermanent(err):
			log.Printf("[dating] outbound %s dropped (permanent): %v", item.label, err)
		default:
			item.attempts++
			if item.attempts >= len(pendRetrySchedule) {
				log.Printf("[dating] outbound %s given up after %d attempts: %v",
					item.label, item.attempts, err)
				continue
			}
			// schedule[n] = wait BEFORE attempt n+1; attempts already counts the
			// attempt just made, so the next wait is schedule[attempts-1]
			item.next = now.Add(pendRetrySchedule[item.attempts-1])
			pendMu.Lock()
			if item.gen != pendGen {
				// resetPending() ran mid-flight: this item belongs to a
				// cleared generation and must not resurrect in the queue
				pendMu.Unlock()
				continue
			}
			if len(pend) < pendCap {
				pend = append(pend, item)
			} else {
				log.Printf("[dating] outbound full — dropping %s", item.label)
			}
			pendMu.Unlock()
		}
	}
}

// pendingCount reports queued sends (tests).
func pendingCount() int {
	pendMu.Lock()
	defer pendMu.Unlock()
	return len(pend)
}

func resetPending() {
	pendMu.Lock()
	pendGen++
	pend = nil
	pendMu.Unlock()
}

// relayPermanent reports failures no retry can fix: the recipient blocked
// the bot. Everything else (401 dead side, timeouts) is worth the schedule —
// a back-up endpoint or token swap can make it succeed.
func relayPermanent(err error) bool {
	var ae *telegram.APIError
	return errors.As(err, &ae) && ae.IsForbidden()
}

// sendWithBudget bounds one outbound attempt so an audit-unavailable
// endpoint cannot pin the queue runner.
func sendWithBudget(send func(context.Context) (*telegram.Message, error)) func() (*telegram.Message, error) {
	return func() (*telegram.Message, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return send(ctx)
	}
}

// EnqueueMatcherNotify parks a match-related notification (main wires the
// matcher's Notifier through here): the delivery side is reported as an error
// so ClaimMatch's reachability gate can park the pair — and this keeps the
// message alive so the pair claims on the next successful flush instead of
// stranding a matched couple.
func EnqueueMatcherNotify(botID uuid.UUID, platform string, chatID int64, text string,
	kbd json.RawMessage, send func(context.Context) (*telegram.Message, error)) {
	label := "match:" + platform + ":" + strconv.FormatInt(chatID, 10)
	enqueuePending(label, sendWithBudget(send), nil)
	_ = botID
	_ = text
	_ = kbd
}
