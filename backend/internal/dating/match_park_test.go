package dating

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"sadbot/internal/domain"

	"github.com/google/uuid"
)

// TestMatchParksWhilePlatformDown: a cross-platform pair is never committed
// while one side can't deliver — no phantom (status=chatting, partner never
// told). It claims atomically as soon as the side comes back, with both
// notifications, and third parties can't steal the parked users meanwhile.
func TestMatchParksWhilePlatformDown(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.platB = domain.PlatformKat

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, domain.PlatformTelegram, 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, domain.PlatformKat, 222)
	if pA == nil || pB == nil {
		t.Fatal("profiles missing")
	}
	if err := e.store.StartSearch(ctx, e.botID, pA.UserID, "", 0); err != nil {
		t.Fatalf("A start search: %v", err)
	}
	if err := e.store.StartSearch(ctx, e.botID, pB.UserID, "", 0); err != nil {
		t.Fatalf("B start search: %v", err)
	}

	var matchMu sync.Mutex
	var gotTexts []string
	m := NewMatcher(e.store, staticBots{e.botID},
		func(botID uuid.UUID, platform string, chatID int64, text string, kbd json.RawMessage) error {
			matchMu.Lock()
			gotTexts = append(gotTexts, platform+":"+text)
			matchMu.Unlock()
			return nil
		})

	katUp := false
	m.SetReachFunc(func(_ uuid.UUID, platform string) bool {
		if platform == domain.PlatformKat {
			return katUp
		}
		return true
	})

	// pass 1: kat down → pair must park, NOT claim, NOT notify
	m.matchOnceGated(ctx, e.botID)
	pA, _ = e.store.Profile(ctx, e.botID, pA.UserID)
	pB, _ = e.store.Profile(ctx, e.botID, pB.UserID)
	if pA.Status != domain.DatingSearching || pB.Status != domain.DatingSearching {
		t.Fatalf("pair must wait in queue: A=%s B=%s", pA.Status, pB.Status)
	}
	if len(m.parked[e.botID]) != 1 {
		t.Fatalf("expected exactly 1 parked pair, got %d", len(m.parked[e.botID]))
	}
	matchMu.Lock()
	notifyWhileDown := len(gotTexts)
	matchMu.Unlock()
	if notifyWhileDown != 0 {
		t.Fatalf("no notification may fire while parked: %v", gotTexts)
	}

	// a lone third searcher: must not be paired with a parked user
	// (the bot_end_users row must exist first — profile FK; user_id is the
	// sequence id Upsert returns, NOT the platform chat id)
	cUID, err := e.users.Upsert(ctx, e.botID.String(), domain.PlatformTelegram, 333, nil, nil, nil)
	if err != nil {
		t.Fatalf("C user: %v", err)
	}
	pC, err := e.store.EnsureProfile(ctx, e.botID, cUID, domain.PlatformTelegram, 333)
	if err != nil || pC == nil {
		t.Fatalf("C profile: %v", err)
	}
	// C enters the queue directly (status may be a fresh row; plain CAS to
	// searching avoids the joined pre-requirement of StartSearch)
	if _, err := e.store.Repo().SetStatus(ctx, e.botID, pC.UserID, "", domain.DatingSearching); err != nil {
		t.Fatalf("C enter queue: %v", err)
	}
	m.matchOnceGated(ctx, e.botID)
	if len(m.parked[e.botID]) != 1 {
		t.Fatalf("C must not steal parked users: parked=%d", len(m.parked[e.botID]))
	}
	pC2, _ := e.store.Profile(ctx, e.botID, pC.UserID)
	if pC2.Status == domain.DatingChatting {
		t.Fatal("C must remain waiting")
	}

	// kat comes back → drain claims the original pair atomically
	katUp = true
	m.matchOnceGated(ctx, e.botID)
	pA, _ = e.store.Profile(ctx, e.botID, pA.UserID)
	pB, _ = e.store.Profile(ctx, e.botID, pB.UserID)
	if pA.Status != domain.DatingChatting || pB.Status != domain.DatingChatting {
		t.Fatalf("drain must claim the pair: A=%s B=%s (parked=%d)",
			pA.Status, pB.Status, len(m.parked[e.botID]))
	}
	if pA.PartnerUserID == nil || *pA.PartnerUserID != pB.UserID {
		t.Fatalf("A's partner wrong: %+v", pA.PartnerUserID)
	}
	matchMu.Lock()
	n := len(gotTexts)
	matchMu.Unlock()
	if n != 2 {
		t.Fatalf("both parked users must be notified once drained, got %d: %v", n, gotTexts)
	}
	if len(m.parked[e.botID]) != 0 {
		t.Fatalf("parked must be empty after drain: %d", len(m.parked[e.botID]))
	}
}
