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
	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Coins != 5 || pA.ChannelBonusAt != nil {
		t.Fatalf("no bonus on partial membership: coins=%d bonus=%v", pA.Coins, pA.ChannelBonusAt)
	}

	// join the second channel → bonus paid once
	memberOf["@ch_two"] = true
	e.feedCB(t, e.sendA, "act:mission:channel")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "عضویت کانال‌ها تایید شد") || !contains(msg.Text, "+12 سکه") {
		t.Fatalf("full membership must confirm: %+v", e.sendA.sent)
	}
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Coins != 17 || pA.ChannelBonusAt == nil { // 5 signup + 12 bonus
		t.Fatalf("bonus must be paid once: coins=%d bonus=%v", pA.Coins, pA.ChannelBonusAt)
	}

	// second attempt is a no-op
	e.feedCB(t, e.sendA, "act:mission:channel")
	if msg := e.sendA.lastTo(111); msg == nil || !contains(msg.Text, "قبلاً عضویت کانال رو انجام دادی") {
		t.Fatalf("repeat must be idempotent: %+v", e.sendA.sent)
	}
	pA, _ = e.store.ProfileByTelegram(ctx, e.botID, 111)
	if pA.Coins != 17 {
		t.Fatalf("repeat must not pay again: coins=%d", pA.Coins)
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

	pA, _ := e.store.ProfileByTelegram(ctx, e.botID, 111)
	pB, _ := e.store.ProfileByTelegram(ctx, e.botID, 222)
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
	// sender confirmation edits the card in place (finishCard → Edit)
	if msg := e.sendA.lastEditTo(111); msg == nil || !contains(msg.Text, "سوپرلایک ثبت شد") {
		t.Fatalf("sender must get superlike confirmation: %+v", e.sendA.edits)
	}
}

// --- panel user cards ---

// TestUserCardsWithoutDatingProfile: a bot user who never started onboarding
// has no dating_profiles row; the LEFT JOIN must still scan (is_ghost NULL).
func TestUserCardsWithoutDatingProfile(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	// user with no dating profile row at all
	u, err := e.users.Upsert(ctx, e.botID.String(), 999, nil, strPtrTest("بی‌پروفایل"), nil)
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
