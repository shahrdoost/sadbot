package dating

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestAILateReplyDroppedAfterLeavingScene: end-to-end through the engine. The
// AI reply is scheduled with a delay; leaving the AI scene before it lands must
// keep the menu clean (live symptom: the assistant kept answering on the home
// screen as if the chat were still open).
func TestAILateReplyDroppedAfterLeavingScene(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	e.feed(t, e.sendA, "🤖 چت با دستیار")
	if got := currentScene(t, e); got != "ai" {
		t.Fatalf("scene = %q, want ai", got)
	}

	// talk to the assistant, then leave before the delayed bubble (700–2500ms)
	// is due. The trip through home must be the LAST thing on screen.
	e.feed(t, e.sendA, "سلام")
	e.feed(t, e.sendA, "🏠 منوی اصلی")

	time.Sleep(3200 * time.Millisecond)

	msgs := e.sendA.sent
	last := msgs[len(msgs)-1]
	if strings.Contains(last.Text, "چه حرف جالبی") || strings.Contains(last.Text, "متوجه شدم") ||
		strings.Contains(last.Text, "شنیدم") || strings.Contains(last.Text, "بیشتر بگو") ||
		strings.Contains(last.Text, "توضیح بدی") {
		t.Fatalf("AI reply landed after leaving the scene: %q", last.Text)
	}
	if !strings.Contains(last.Text, "خوش اومدی") && !strings.Contains(last.Text, "چیکار برات") {
		t.Fatalf("last message should be the home menu, got %q", last.Text)
	}
	if got := currentScene(t, e); got != "home" {
		t.Fatalf("scene = %q, want home", got)
	}
}

func currentScene(t *testing.T, e *browseEnv) string {
	t.Helper()
	p, err := e.store.ProfileByTelegram(context.Background(), e.botID, "", 111)
	if err != nil || p == nil {
		return ""
	}
	raw, err := e.users.GetState(context.Background(), e.botID.String(), p.UserID)
	if err != nil || len(raw) == 0 {
		return ""
	}
	var st map[string]any
	if json.Unmarshal(raw, &st) != nil {
		return ""
	}
	s, _ := st["scene"].(string)
	return s
}
