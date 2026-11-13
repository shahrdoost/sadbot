package engine

import (
	"context"
	"testing"
)

// TestSceneGuardAgainstPersistedState: the scene guard used by delayed replies
// (AI think-time, ghost typing bubbles) must read the SAVED state, not the
// in-memory map of the update that scheduled the send — otherwise a message
// due seconds later still lands after the user went home and reads as a live
// chat.
func TestSceneGuardAgainstPersistedState(t *testing.T) {
	ctx := context.Background()
	users := newMemUsers()
	sender := &recSender{}
	userID, _ := users.Upsert(ctx, "b", "telegram", 111, nil, nil, nil)

	bc := botContextFor(users, sender, userID)
	bc.State = map[string]any{"scene": "ai", "data": map[string]any{}}
	bc.SaveState()

	if !bc.StillInScene("ai") {
		t.Fatal("guard must report the scene it just saved")
	}

	// a LATER update saves a different scene (the user pressed home)
	bc2 := botContextFor(users, sender, userID)
	bc2.State = map[string]any{"scene": "home", "data": map[string]any{}}
	bc2.SaveState()

	if bc.StillInScene("ai") {
		t.Fatal("a saved-away scene must not read as current")
	}

	// and the delayed send for the stale scene is dropped, not delivered
	before := len(sender.sent)
	if bc.SendIfStillInScene("ai", "پاسخ دیررس", nil) {
		t.Fatal("SendIfStillInScene must refuse a scene the user left")
	}
	if len(sender.sent) != before {
		t.Fatal("late reply leaked into the menu")
	}

	// while the scene is still current, the send goes through
	bc3 := botContextFor(users, sender, userID)
	bc3.State = map[string]any{"scene": "ai", "data": map[string]any{}}
	bc3.SaveState()
	if !bc.SendIfStillInScene("ai", "پاسخ به‌موقع", nil) {
		t.Fatal("send must go through while the scene is current")
	}
	if got := sender.last().Text; got != "پاسخ به‌موقع" {
		t.Fatalf("delivered %q", got)
	}
}

func botContextFor(users *memUsers, sender *recSender, userID int64) *BotContext {
	return &BotContext{
		BotID:  "b",
		UserID: userID,
		ChatID: 111,
		Sender: sender,
		users:  users,
	}
}
