package dating

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestAIGateFromPanel: the ai_enabled prices flag is the panel's switch for
// the AI companion. When off, the scene refuses to open; when on, it works.
// This is the bot-side half of the AI harness (the panel half lives in
// service.AdminOpsService.SetAIFeature which writes the same key).
func TestAIGateFromPanel(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد") // finish onboarding (photo step is optional)

	scene := func() string {
		p, err := e.store.ProfileByTelegram(ctx, e.botID, 111)
		if err != nil || p == nil {
			return ""
		}
		raw, err := e.users.GetState(ctx, e.botID.String(), p.UserID)
		if err != nil || len(raw) == 0 {
			return ""
		}
		var st map[string]any
		_ = json.Unmarshal(raw, &st)
		s, _ := st["scene"].(string)
		return s
	}

	// default (no flag set): the gate is open
	e.feed(t, e.sendA, "🤖 چت با دستیار")
	if got := scene(); got != "ai" {
		t.Fatalf("scene = %q, want ai (gate open by default)", got)
	}

	// panel turns the flag off
	e.prices["ai_enabled"] = 0
	e.feed(t, e.sendA, "/start") // back home
	e.feed(t, e.sendA, "🤖 چت با دستیار")
	if got := scene(); got == "ai" {
		t.Fatalf("scene = ai although ai_enabled=0 — gate leaked")
	}
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "غیرفعال") {
		t.Fatalf("expected disabled notice, got %#v", msg)
	}

	// re-enable: the scene opens again; disable mid-chat bounces home
	e.prices["ai_enabled"] = 1
	e.feed(t, e.sendA, "🤖 چت با دستیار")
	if got := scene(); got != "ai" {
		t.Fatalf("scene = %q, want ai after re-enable", got)
	}
	e.prices["ai_enabled"] = 0
	e.feed(t, e.sendA, "سلام")
	if got := scene(); got == "ai" {
		t.Fatalf("user stuck in ai scene after disable; scene = %q", got)
	}
}
