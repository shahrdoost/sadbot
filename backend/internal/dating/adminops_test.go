package dating

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// --- P5: panel admin operations on dating data ---

// TestAdjustCoinsClampsAtZero: a panel debit larger than the balance must not
// violate the non-negative CHECK on dating_profiles.coins.
func TestAdjustCoinsClampsAtZero(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")

	p, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if p.Coins != 5 {
		t.Fatalf("expected 5 signup coins, got %d", p.Coins)
	}

	got, err := e.store.Repo().AdjustCoins(ctx, e.botID, p.UserID, 50)
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	if got != 55 {
		t.Fatalf("credit = %d, want 55", got)
	}

	got, err = e.store.Repo().AdjustCoins(ctx, e.botID, p.UserID, -20)
	if err != nil {
		t.Fatalf("debit: %v", err)
	}
	if got != 35 {
		t.Fatalf("debit = %d, want 35", got)
	}

	// overdraft clamps to zero instead of failing the CHECK constraint
	got, err = e.store.Repo().AdjustCoins(ctx, e.botID, p.UserID, -9999)
	if err != nil {
		t.Fatalf("overdraft must clamp, not fail: %v", err)
	}
	if got != 0 {
		t.Fatalf("overdraft = %d, want 0", got)
	}
}

// TestResetProfileKeepsRow: reset wipes content but the profile row survives,
// so the user is immediately usable without re-onboarding.
func TestResetProfileKeepsRow(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, 222)

	// give A state worth wiping: extra coins, a like, a streak, a block flag
	if _, err := e.store.Repo().AdjustCoins(ctx, e.botID, pA.UserID, 100); err != nil {
		t.Fatalf("seed coins: %v", err)
	}
	if _, err := e.store.Repo().InsertLike(ctx, e.botID, pA.UserID, pB.UserID, "super"); err != nil {
		t.Fatalf("seed like: %v", err)
	}
	if _, err := e.store.Repo().RecordStreak(ctx, e.botID, pA.UserID); err != nil {
		t.Fatalf("seed streak: %v", err)
	}
	if err := e.store.Repo().SetBlocked(ctx, e.botID, pA.UserID, true, "تست"); err != nil {
		t.Fatalf("seed block: %v", err)
	}

	ok, err := e.store.Repo().ResetProfile(ctx, e.botID, pA.UserID)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !ok {
		t.Fatal("reset must report the profile existed")
	}

	after, err := e.store.Profile(ctx, e.botID, pA.UserID)
	if err != nil || after == nil {
		t.Fatalf("profile row must survive reset: %v", err)
	}
	if after.Coins != 0 {
		t.Fatalf("coins after reset = %d, want 0", after.Coins)
	}
	if after.Status != "joined" {
		t.Fatalf("status after reset = %s, want joined", after.Status)
	}
	if after.StreakCount != 0 || after.StreakLastDate != nil {
		t.Fatalf("streak after reset = %d/%v", after.StreakCount, after.StreakLastDate)
	}
	if blocked, _ := e.store.Repo().IsAdminBlocked(ctx, e.botID, pA.UserID); blocked {
		t.Fatal("block flag must be cleared by reset")
	}
	// name is preserved: the user does not re-onboard
	if after.Name == nil || *after.Name != "امیر" {
		t.Fatalf("name must be preserved, got %v", after.Name)
	}
	// likes were removed
	liked, err := e.store.Repo().HasLiked(ctx, e.botID, pA.UserID, pB.UserID)
	if err != nil {
		t.Fatalf("hasliked: %v", err)
	}
	if liked {
		t.Fatal("likes must be wiped by reset")
	}
}

// TestSetBlockedPullsOutOfChat: blocking a user mid-chat clears their status
// and releases the partner, so nobody waits on a silent participant.
func TestSetBlockedPullsOutOfChat(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, 222)

	if _, err := e.store.ConnectUsers(ctx, e.botID, pA.UserID, pB.UserID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if p, _ := e.store.Profile(ctx, e.botID, pA.UserID); p.Status != "chatting" {
		t.Fatalf("precondition: A must be chatting, got %s", p.Status)
	}

	if err := e.store.Repo().SetBlocked(ctx, e.botID, pA.UserID, true, "تخلف"); err != nil {
		t.Fatalf("block: %v", err)
	}

	a, _ := e.store.Profile(ctx, e.botID, pA.UserID)
	b, _ := e.store.Profile(ctx, e.botID, pB.UserID)
	if a.Status != "joined" || a.PartnerUserID != nil {
		t.Fatalf("blocked user must leave the chat: %s partner=%v", a.Status, a.PartnerUserID)
	}
	if b.Status != "joined" || b.PartnerUserID != nil {
		t.Fatalf("partner must be released: %s partner=%v", b.Status, b.PartnerUserID)
	}

	blocked, err := e.store.Repo().IsAdminBlocked(ctx, e.botID, pA.UserID)
	if err != nil || !blocked {
		t.Fatalf("IsAdminBlocked = %v, %v", blocked, err)
	}
	if bBlocked, _ := e.store.Repo().IsAdminBlocked(ctx, e.botID, pB.UserID); bBlocked {
		t.Fatal("partner must not be blocked")
	}

	// unblock restores usable state
	if err := e.store.Repo().SetBlocked(ctx, e.botID, pA.UserID, false, ""); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	if blocked, _ := e.store.Repo().IsAdminBlocked(ctx, e.botID, pA.UserID); blocked {
		t.Fatal("unblock must clear the flag")
	}
}

// TestStuckScanAndReset: stuck detection uses per-status thresholds and the
// reset releases both stuck users and the partners waiting on them.
func TestStuckScanAndReset(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, 222)

	if _, err := e.store.ConnectUsers(ctx, e.botID, pA.UserID, pB.UserID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	// a fresh chat is not stuck
	stuck, err := e.store.Repo().StuckUsers(ctx, e.botID, 5, 60)
	if err != nil {
		t.Fatalf("stuck scan: %v", err)
	}
	if len(stuck) != 0 {
		t.Fatalf("fresh chat must not be stuck: %+v", stuck)
	}
	// with a zero threshold everyone chatting counts as stuck
	stuck, err = e.store.Repo().StuckUsers(ctx, e.botID, 0, 0)
	if err != nil {
		t.Fatalf("stuck scan (0 threshold): %v", err)
	}
	if len(stuck) != 2 {
		t.Fatalf("both chatters must be stuck at threshold 0, got %d", len(stuck))
	}

	n, err := e.store.Repo().ResetStuck(ctx, e.botID, 0, 0)
	if err != nil {
		t.Fatalf("reset stuck: %v", err)
	}
	if n != 2 {
		t.Fatalf("reset stuck changed %d, want 2", n)
	}
	a, _ := e.store.Profile(ctx, e.botID, pA.UserID)
	b, _ := e.store.Profile(ctx, e.botID, pB.UserID)
	if a.Status != "joined" || b.Status != "joined" {
		t.Fatalf("both must be released: %s / %s", a.Status, b.Status)
	}
}

// TestResetAllChatsKeepsProfile: the bulk reset only changes status.
func TestResetAllChatsKeepsProfile(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if _, err := e.store.Repo().AdjustCoins(ctx, e.botID, pA.UserID, 42); err != nil {
		t.Fatalf("seed coins: %v", err)
	}
	if _, err := e.store.Repo().SetStatus(ctx, e.botID, pA.UserID, "joined", "searching"); err != nil {
		t.Fatalf("seed status: %v", err)
	}

	n, err := e.store.Repo().ResetAllChats(ctx, e.botID)
	if err != nil {
		t.Fatalf("reset all: %v", err)
	}
	if n < 1 {
		t.Fatalf("reset all changed %d, want >= 1", n)
	}
	after, _ := e.store.Profile(ctx, e.botID, pA.UserID)
	if after.Status != "joined" {
		t.Fatalf("status = %s, want joined", after.Status)
	}
	if after.Coins != 47 {
		t.Fatalf("reset-all must not touch coins, got %d", after.Coins)
	}
}

// TestResetStreak: clears streak counters.
func TestResetStreak(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	p, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)

	if _, err := e.store.Repo().RecordStreak(ctx, e.botID, p.UserID); err != nil {
		t.Fatalf("streak: %v", err)
	}
	if err := e.store.Repo().ResetStreak(ctx, e.botID, p.UserID); err != nil {
		t.Fatalf("reset streak: %v", err)
	}
	after, _ := e.store.Profile(ctx, e.botID, p.UserID)
	if after.StreakCount != 0 || after.StreakBest != 0 || after.StreakLastDate != nil {
		t.Fatalf("streak not cleared: %d/%d/%v", after.StreakCount, after.StreakBest, after.StreakLastDate)
	}
}

// TestUserDetailAssembles: the forensics view joins the end user, their
// profile, likes, payments, and recent events without crashing on a
// profile-less user.
func TestUserDetailAssembles(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)

	d, err := e.store.Repo().UserDetail(ctx, e.botID, pA.UserID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if d.TelegramID != 111 || !d.HasProfile {
		t.Fatalf("detail identity wrong: %+v", d)
	}
	if d.Name == nil || *d.Name != "امیر" {
		t.Fatalf("name missing: %v", d.Name)
	}

	// a user with no dating profile must not fail the LEFT JOIN
	u, err := e.users.Upsert(ctx, e.botID.String(), 4242, nil, strPtrTest("بی‌پروفایل"), nil)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	d2, err := e.store.Repo().UserDetail(ctx, e.botID, u)
	if err != nil {
		t.Fatalf("detail without profile must not fail: %v", err)
	}
	if d2.HasProfile {
		t.Fatal("profile-less user must report HasProfile=false")
	}
	if d2.Payments == nil || d2.RecentEvents == nil {
		t.Fatal("detail slices must be non-nil for the panel")
	}
	_ = uuid.Nil
}
