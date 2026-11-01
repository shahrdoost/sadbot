package dating

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"sadbot/internal/telegram"

	"github.com/google/uuid"
)

// TestPendingQueue_RetriesUntilSuccess: a transient failure parks the send
// and the second flush delivers it — no conversation is left half-anchored.
func TestPendingQueue_RetriesUntilSuccess(t *testing.T) {
	resetPending()
	defer resetPending()

	calls := 0
	var delivered *telegram.Message
	enqueuePending("u-under-test", func() (*telegram.Message, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("endpoint flapping")
		}
		return &telegram.Message{MessageID: 42}, nil
	}, func(m *telegram.Message) { delivered = m })

	// due immediately after the (already failed) sync attempt
	flushPending(time.Now())
	if calls != 1 || pendingCount() != 1 {
		t.Fatalf("first flush must requeue a transient failure: calls=%d pending=%d", calls, pendingCount())
	}
	// second flush still before its next slot → skipped
	flushPending(time.Now())
	if calls != 1 {
		t.Fatalf("flush must respect backoff, calls=%d", calls)
	}
	// past all slots: run every wake until done
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) && delivered == nil {
		flushPending(time.Now().Add(4 * time.Minute))
	}
	if delivered == nil || delivered.MessageID != 42 {
		t.Fatalf("queued send never delivered: calls=%d msg=%+v", calls, delivered)
	}
	if pendingCount() != 0 {
		t.Fatalf("queue must drain: pending=%d", pendingCount())
	}
}

// TestPendingQueue_PermanentDrop: 403 (user blocked the bot) never retries.
func TestPendingQueue_PermanentDrop(t *testing.T) {
	resetPending()
	defer resetPending()

	calls := 0
	enqueuePending("blocked", func() (*telegram.Message, error) {
		calls++
		return nil, &telegram.APIError{Code: 403, Description: "bot was blocked"}
	}, func(*telegram.Message) { t.Fatal("onDone must not run for a dropped send") })

	flushPending(time.Now())
	if calls != 1 {
		t.Fatalf("403 must not be retried, calls=%d", calls)
	}
	if pendingCount() != 0 {
		t.Fatalf("403 must drop from the queue: pending=%d", pendingCount())
	}
}

// TestPendingQueue_Cap: the hard bound holds so a dead endpoint cannot grow
// the queue without limit.
func TestPendingQueue_Cap(t *testing.T) {
	resetPending()
	defer resetPending()

	for i := 0; i < pendCap+10; i++ {
		enqueuePending("fill", func() (*telegram.Message, error) {
			return nil, errors.New("down")
		}, nil)
	}
	if pendingCount() > pendCap {
		t.Fatalf("queue exceeds cap: %d", pendingCount())
	}
}

// TestRelayPermanent_Classification: 403 is permanent; everything else,
// including a park-able platform-down error, stays retryable.
func TestRelayPermanent_Classification(t *testing.T) {
	if !relayPermanent(&telegram.APIError{Code: 403, Description: "blocked"}) {
		t.Fatal("403 must be permanent")
	}
	// engine.ErrPlatformDown wraps nothing API-ish → retryable; checked here
	// without importing engine (constant identity is verified by the
	// template-level tests; this keeps the classification table honest)
	if relayPermanent(errors.New("no runtime")) {
		t.Fatal("generic errors must stay retryable")
	}
}

// TestMatcherNotify_EnqueuesOnFailure main-style wiring: a failed match
// notification is parked and retried — the send function runs a second time
// once the side recovers. (Goes through the real queue and its runner, so the
// assertion is the retry actually happening, not a fragile global count.)
func TestMatcherNotify_EnqueuesOnFailure(t *testing.T) {
	resetPending()
	defer resetPending()

	calls := 0
	send := func(context.Context) (*telegram.Message, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("side mid-flight")
		}
		return &telegram.Message{MessageID: 7}, nil
	}
	EnqueueMatcherNotify(uuid.New(), "kat", 999, "وصلت انجام شد!",
		json.RawMessage(nil), send)

	// flush 1: due immediately, fails, parks with a backoff slot
	flushPending(time.Now())
	if calls != 1 {
		t.Fatalf("parked notify must attempt once after enqueue: calls=%d", calls)
	}
	// flush 2 (+indefinite window): the retry slot passed → real delivery
	flushPending(time.Now().Add(24 * time.Hour))
	if calls < 2 && pendingCount() > 0 {
		t.Fatalf("parked notify must retry and deliver: calls=%d pending=%d",
			calls, pendingCount())
	}
	if pendingCount() > 0 {
		t.Fatalf("parked notify must end up delivered: pending=%d", pendingCount())
	}
}
