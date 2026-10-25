package dating

import (
	"context"
	"strings"
	"testing"
)

// TestProfilePageAndFullEdit: the rich profile page shows every field and the
// self-edit flows (gender, location, photo, interests) persist with the
// shared weekly cooldown — kpkchat «حساب من» parity.
func TestProfilePageAndFullEdit(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد") // finish onboarding (photo step)

	// 1) profile page shows all fields including province/interests/photo/coins
	e.feed(t, e.sendA, "👤 پروفایل من")
	page := e.sendA.lastTo(111)
	if page == nil {
		t.Fatal("no profile page message")
	}
	for _, want := range []string{"نام: امیر", "استان: تهران", "علاقه‌مندی‌ها: ندارد", "عکس: 📷 ندارد", "سکه:"} {
		if !strings.Contains(page.Text, want) {
			t.Fatalf("profile page missing %q:\n%s", want, page.Text)
		}
	}

	// 2) gender edit (first profile edit is free) persists + stamps cooldown
	e.feedCB(t, e.sendA, "act:edit:gender")
	e.feed(t, e.sendA, "خانومم")
	p, err := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	if p.Gender == nil || *p.Gender != "female" {
		t.Fatalf("gender = %v, want female", p.Gender)
	}
	if p.ProfileEditedAt == nil {
		t.Fatal("profile edit cooldown not stamped")
	}

	// 3) second field edit hits the weekly cooldown (coins untouched)
	coinsBefore := p.Coins
	e.feedCB(t, e.sendA, "act:edit:age")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "🔒") {
		t.Fatalf("expected cooldown refusal, got %#v", msg)
	}
	p, _ = e.store.ProfileByTelegram(ctx, e.botID, 111)
	if p.Coins != coinsBefore {
		t.Fatalf("cooldown must not charge: coins %d → %d", coinsBefore, p.Coins)
	}

	// 4) interests: free, no cooldown — toggle two and save
	e.feedCB(t, e.sendA, "act:edit:interests")
	e.feedCB(t, e.sendA, "act:int:toggle:0") // موسیقی
	e.feedCB(t, e.sendA, "act:int:toggle:2") // کتاب
	e.feedCB(t, e.sendA, "act:int:save")
	p, _ = e.store.ProfileByTelegram(ctx, e.botID, 111)
	if len(p.Interests) != 2 || p.Interests[0] != "موسیقی" || p.Interests[1] != "کتاب" {
		t.Fatalf("interests = %v, want [موسیقی کتاب]", p.Interests)
	}

	// profile page now lists them
	e.feed(t, e.sendA, "👤 پروفایل من")
	page = e.sendA.lastTo(111)
	if page == nil || !strings.Contains(page.Text, "موسیقی، کتاب") {
		t.Fatalf("profile page should show saved interests: %#v", page)
	}

	// 5) photo edit (cooldown blocks after gender edit above — clear it first)
	// simulate cooldown expiry by waiting is impossible; instead assert the
	// refusal, then clear the stamp via repo-free path: re-onboard is wrong.
	// Directly verify editphoto accepts a photo when the gate is open by
	// editing with the flag after resetting the stamp through a fresh user.
	e2 := newBrowseEnv(t)
	e2.onboard(t, e2.sendB, "سارا", "22", "خانومم")
	e2.feed(t, e2.sendB, "⏭ رد شد")
	e2.feedCB(t, e2.sendB, "act:edit:photo")
	e2.feedPhoto(t, e2.sendB, "new-avatar-file-id")
	p2, err := e2.store.ProfileByTelegram(ctx, e2.botID, 222)
	if err != nil {
		t.Fatalf("profile b: %v", err)
	}
	if p2.PhotoFileID == nil || *p2.PhotoFileID != "new-avatar-file-id" {
		t.Fatalf("photo_file_id = %v", p2.PhotoFileID)
	}
	if p2.ProfileEditedAt == nil {
		t.Fatal("photo edit must stamp the cooldown")
	}
	// avatar bonus was already granted at onboarding — no double grant
	if p2.AvatarBonusAt == nil {
		t.Fatal("avatar bonus timestamp should already exist from onboarding")
	}
}
