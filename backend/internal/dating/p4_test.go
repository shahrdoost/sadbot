package dating

import (
	"context"
	"testing"

	"sadbot/internal/domain"

	"github.com/google/uuid"
)

// --- P4: multi-channel membership ---

func TestMissionChannels(t *testing.T) {
	// "channels" array wins over legacy single "channel_id"
	bc := priceCtx(map[string]any{
		"channels":   []any{"@a", " @b ", ""},
		"channel_id": "@legacy",
	})
	got := missionChannels(bc)
	if len(got) != 2 || got[0] != "@a" || got[1] != "@b" {
		t.Fatalf("channels array = %v", got)
	}

	// legacy single channel_id honored as one-element list
	bcLegacy := priceCtx(map[string]any{"channel_id": "@legacy"})
	gotLegacy := missionChannels(bcLegacy)
	if len(gotLegacy) != 1 || gotLegacy[0] != "@legacy" {
		t.Fatalf("legacy channel_id = %v", gotLegacy)
	}

	// none configured
	if gotNone := missionChannels(priceCtx(map[string]any{})); len(gotNone) != 0 {
		t.Fatalf("empty prices = %v", gotNone)
	}
}

// TestMultiChannelMissionFlow: the channel mission checks every configured
// channel; missing one membership rejects, joining all pays the bonus once.
func TestMultiChannelMissionFlow(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.prices = map[string]any{
		"channels":      []any{"@ch_one", "@ch_two"},
		"channel_bonus": float64(12),
	}

	memberOf := map[string]bool{}
	var checked []string
	e.eng.SetData("channelcheck", ChannelChecker(func(_ uuid.UUID, channelID string, _ int64) (string, error) {
		checked = append(checked, channelID)
		if memberOf[channelID] {
			return "member", nil
		}
		return "left", nil
	}))

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	// member of first channel only → rejected, no bonus
	memberOf["@ch_one"] = true
	e.feedCB(t, e.sendA, "act:mission:channel")
	if len(checked) != 2 || checked[0] != "@ch_one" || checked[1] != "@ch_two" {
		t.Fatalf("both channels must be checked, got %v", checked)
	}
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "عضو همه کانال‌ها نیستی") {
		t.Fatalf("partial membership must reject: %+v", e.sendA.sent)
	}
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	if pA.Coins != 5 || pA.ChannelBonusAt != nil {
		t.Fatalf("no bonus on partial membership: coins=%d bonus=%v", pA.Coins, pA.ChannelBonusAt)
	}

	// join the second channel → bonus paid once
	memberOf["@ch_two"] = true
	e.feedCB(t, e.sendA, "act:mission:channel")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "عضویت کانال‌ها تایید شد") || !contains(msg.Text, "+12 سکه") {
		t.Fatalf("full membership must confirm: %+v", e.sendA.sent)
	}
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	if pA.Coins != 17 || pA.ChannelBonusAt == nil { // 5 signup + 12 bonus
		t.Fatalf("bonus must be paid once: coins=%d bonus=%v", pA.Coins, pA.ChannelBonusAt)
	}

	// second attempt is a no-op
	e.feedCB(t, e.sendA, "act:mission:channel")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "قبلاً عضویت کانال رو انجام دادی") {
		t.Fatalf("repeat must be idempotent: %+v", e.sendA.sent)
	}
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	if pA.Coins != 17 {
		t.Fatalf("repeat must not pay again: coins=%d", pA.Coins)
	}
}

// TestChannelMissionKatTrustsTap: Kat has no getChatMember, so the membership
// check must not even run there — the confirmation tap pays the bonus (the
// join link is right above the button). Telegram keeps the real check.
func TestChannelMissionKatTrustsTap(t *testing.T) {
	e := newBrowseEnv(t)
	e.platA = "kat"
	ctx := context.Background()
	e.prices = map[string]any{
		"channels":      []any{map[string]any{"id": "@ashna_dating", "title": "آشنا دیتینگ", "link": "https://t.me/ashna_dating"}},
		"channel_bonus": float64(10),
	}

	checked := 0
	e.eng.SetData("channelcheck", ChannelChecker(func(_ uuid.UUID, _ string, _ int64) (string, error) {
		checked++
		return "left", nil // even a rejecting checker must not be consulted on Kat
	}))

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	e.feedCB(t, e.sendA, "act:mission:channel")
	if checked != 0 {
		t.Fatalf("kat must not consult the membership checker, checked=%d", checked)
	}
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "تایید شد") {
		t.Fatalf("kat tap must confirm the mission: %+v", e.sendA.sent)
	}
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	if pA.ChannelBonusAt == nil {
		t.Fatalf("bonus must be recorded on kat: %+v", pA)
	}
}

// --- P4: one-sided superlike notification ---

// TestOneSidedSuperlikeNotifiesRecipient: A superlikes B without a mutual
// like — B gets the ping, no chat is opened, coins are spent.
func TestOneSidedSuperlikeNotifiesRecipient(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "سارا") {
		t.Fatalf("A did not see B's card: %+v", e.sendA.sent)
	}

	e.feedCB(t, e.sendA, "act:browse:super")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pA.Coins != 2 { // 5 signup - 3 superlike
		t.Fatalf("superlike must cost 3 coins, got %d", pA.Coins)
	}
	if pA.Status != domain.DatingJoined || pB.Status != domain.DatingJoined {
		t.Fatalf("one-sided superlike must not connect: A=%s B=%s", pA.Status, pB.Status)
	}
	if liked, _ := e.store.Repo().HasLiked(ctx, e.botID, pA.UserID, pB.UserID); !liked {
		t.Fatal("superlike not recorded")
	}

	// recipient ping (sent via the bot client to B's private chat)
	if msg := e.sendA.lastTo(222); msg == nil || !contains(msg.Text, "سوپرلایک زد") {
		t.Fatalf("recipient must be notified of superlike: %+v", e.sendA.sent)
	}
	// the old card is deleted when the deck advances (no edited stub left)
	if len(e.sendA.deletes) == 0 {
		t.Fatalf("sender card must be deleted after superlike: %+v", e.sendA.sent)
	}
	if msg := e.sendA.lastEditTo(111); msg != nil && contains(msg.Text, "سوپرلایک ثبت شد") {
		t.Fatalf("superlike confirmation must not linger as an edited card: %+v", e.sendA.edits)
	}
}

// TestBlockEndLandsPartnerOnRating: B blocks A; A's star press must register
// the rating even though A never navigated anywhere (live bug: A's scene was
// left at "chat" so the star hit the dead-chat path and the rating was lost).
func TestBlockEndLandsPartnerOnRating(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendA, "act:browse:chat")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pA.Status != domain.DatingChatting || pB.Status != domain.DatingChatting {
		t.Fatalf("direct chat must connect: A=%s B=%s", pA.Status, pB.Status)
	}

	e.feed(t, e.sendB, "🚫 بلاک")

	// the notice is sent to A during B's update — it lands on B's sender
	// with A's chat id
	if msg := e.sendB.lastTo(111); msg == nil || !contains(msg.Text, "بلاک") {
		t.Fatalf("A must be told the chat is over: %+v", e.sendB.sent)
	}

	// A rates straight away — must land despite the stale scene
	e.feed(t, e.sendA, "⭐⭐⭐⭐⭐")

	pB, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pB.TotalRatings != 1 || pB.RatingSum != 5 {
		t.Fatalf("A's rating must be applied to B: B=%+v", pB)
	}
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "ثبت شد") {
		t.Fatalf("A must get the rating confirmation: %+v", e.sendA.sent)
	}
}

// TestRatingReconcileFromStaleScene: pending rate + a scene that never
// flipped to "rating" (home here) — a star press must still apply.
func TestRatingReconcileFromStaleScene(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if err := e.store.Repo().SetPendingRate(ctx, e.botID, pA.UserID, pB.UserID); err != nil {
		t.Fatalf("set pending: %v", err)
	}
	_ = e.store.SetScene(ctx, e.botID.String(), pA.UserID, "home")

	e.feed(t, e.sendA, "⭐⭐⭐⭐⭐")

	pB, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pB.TotalRatings != 1 || pB.RatingSum != 5 {
		t.Fatalf("rating must land from a stale scene: B=%+v", pB)
	}
}

// TestDeleteAccountFlow: 3-coin «حذف حساب» hides the profile from the deck,
// refuses browse while deactivated, and reactivate brings it all back.
func TestDeleteAccountFlow(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	// B is in A's deck
	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "سارا") {
		t.Fatalf("A must see B first: %+v", e.sendA.sent)
	}
	e.feedCB(t, e.sendA, "act:browse:exit")

	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	coinsBefore := pB.Coins

	e.feedCB(t, e.sendB, "act:account:delete")
	e.feedCB(t, e.sendB, "act:account:delete:yes")

	pB, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pB.DeactivatedAt == nil {
		t.Fatal("B must be deactivated after confirming")
	}
	if pB.Coins != coinsBefore-3 {
		t.Fatalf("delete must cost 3 coins: before=%d after=%d", coinsBefore, pB.Coins)
	}

	// B is out of A's deck
	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "باقی نمونده") {
		t.Fatalf("A's deck must be empty after B deleted: %+v", e.sendA.sent)
	}
	e.feedCB(t, e.sendA, "act:browse:exit")

	// B can't browse either
	e.feed(t, e.sendB, "💘 مرور پروفایل‌ها")
	if msg := e.sendB.lastTo(222); msg == nil || !contains(msg.Text, "غیرفعال") {
		t.Fatalf("B must be refused while deactivated: %+v", e.sendB.sent)
	}

	// reactivate restores the deck
	e.feedCB(t, e.sendB, "act:account:reactivate")
	pB, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pB.DeactivatedAt != nil {
		t.Fatal("reactivate must clear the flag")
	}
	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "سارا") {
		t.Fatalf("B must be back in the deck: %+v", e.sendA.sent)
	}
}

// TestDeleteAccountEndsLiveChat: deleting mid-chat ends the chat for both
// sides and the partner is asked to rate the leaver.
func TestDeleteAccountEndsLiveChat(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
	e.feedCB(t, e.sendA, "act:browse:chat")

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pA.Status != domain.DatingChatting || pB.Status != domain.DatingChatting {
		t.Fatalf("direct chat must connect: A=%s B=%s", pA.Status, pB.Status)
	}

	// B deletes their account mid-chat
	e.feedCB(t, e.sendB, "act:account:delete")
	e.feedCB(t, e.sendB, "act:account:delete:yes")

	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 111)
	pB, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pA.Status != domain.DatingJoined || pB.Status != domain.DatingJoined {
		t.Fatalf("chat must end for both: A=%s B=%s", pA.Status, pB.Status)
	}
	if pB.DeactivatedAt == nil {
		t.Fatal("B must be deactivated")
	}
	// A was left in the rating scene and rates the leaver (the prompt is sent
	// during B's delete action — recorded on B's sender)
	if msg := e.sendB.lastTo(111); msg == nil || !contains(msg.Text, "امتیاز بده") {
		t.Fatalf("A must be asked to rate: %+v", e.sendB.sent)
	}
	e.feed(t, e.sendA, "⭐⭐⭐⭐⭐")
	pB, _ = e.store.ProfileByTelegram(ctx, e.botID, "", 222)
	if pB.TotalRatings != 1 {
		t.Fatalf("A's rating must be applied to B: B=%+v", pB)
	}
}

// --- panel user cards ---

// TestUserCardsWithoutDatingProfile: a bot user who never started onboarding
// has no dating_profiles row; the LEFT JOIN must still scan (is_ghost NULL).
func TestUserCardsWithoutDatingProfile(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	// user with no dating profile row at all
	u, err := e.users.Upsert(ctx, e.botID.String(), "telegram", 999, nil, strPtrTest("بی‌پروفایل"), nil)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// and a normal, onboarded user for contrast
	e.onboard(t, e.sendA, "امیر", "25", "آقام")

	cards, total, err := e.store.Repo().UserCards(ctx, e.botID, "", 1, 50, false)
	if err != nil {
		t.Fatalf("UserCards with profile-less user must not fail: %v", err)
	}
	if total != 2 || len(cards) != 2 {
		t.Fatalf("expected 2 cards, got total=%d len=%d", total, len(cards))
	}
	var found bool
	for _, c := range cards {
		if c.UserID == u {
			found = true
			if c.IsGhost {
				t.Fatal("profile-less user must not be marked ghost")
			}
			if c.Status != nil || c.Coins != nil {
				t.Fatalf("profile-less user must have nil dating fields: %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("profile-less user missing from cards: %+v", cards)
	}
}

func strPtrTest(s string) *string { return &s }

// --- P4: confession seed list ---

// TestSeedConfessionsParity: the cold-start list keeps kpkchat parity —
// 24 non-empty unique texts the panel seed endpoint inserts.
func TestSeedConfessionsParity(t *testing.T) {
	if len(SeedConfessions) != 24 {
		t.Fatalf("seed count = %d, want 24 (kpkchat parity)", len(SeedConfessions))
	}
	seen := map[string]bool{}
	for i, s := range SeedConfessions {
		if s == "" {
			t.Fatalf("seed %d is empty", i)
		}
		if seen[s] {
			t.Fatalf("seed %d is a duplicate: %q", i, s)
		}
		seen[s] = true
	}
}
