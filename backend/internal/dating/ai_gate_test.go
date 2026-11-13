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
		p, err := e.store.ProfileByTelegram(ctx, e.botID, "", 111)
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

// TestAssistantHiddenFromHomeWhenDisabled: turning ai_enabled off must remove
// «🤖 چت با دستیار» from the home keyboard itself (both platforms render the
// same keyboard), not just refuse the scene — the request was to take the menu
// entry away, and a dead button is what users keep pressing.
func TestAssistantHiddenFromHomeWhenDisabled(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	keyboard := func() string {
		msg := e.sendA.lastTo(111)
		if msg == nil {
			return ""
		}
		return string(msg.ReplyMark)
	}

	// on (default): the button is on the menu
	e.feed(t, e.sendA, "/start")
	if kb := keyboard(); !strings.Contains(kb, assistantLabel) {
		t.Fatalf("assistant button missing while enabled: %s", kb)
	}

	// off: gone from the menu
	e.prices["ai_enabled"] = 0
	e.feed(t, e.sendA, "/start")
	if kb := keyboard(); strings.Contains(kb, assistantLabel) {
		t.Fatalf("assistant button still on the menu while disabled: %s", kb)
	}
	if kb := keyboard(); !strings.Contains(kb, "پشتیبانی") {
		t.Fatalf("the rest of the menu must survive: %s", kb)
	}

	// a stale press of the old button (or the ☰ /ai entry) lands home with the
	// notice instead of opening the disabled scene
	e.feed(t, e.sendA, assistantLabel)
	if got := currentScene(t, e); got != "home" {
		t.Fatalf("disabled assistant press opened scene %q", got)
	}
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "غیرفعال") {
		t.Fatalf("disabled assistant press must explain itself, got %#v", msg)
	}

	// re-enabling brings it back
	e.prices["ai_enabled"] = 1
	e.feed(t, e.sendA, "/start")
	if kb := keyboard(); !strings.Contains(kb, assistantLabel) {
		t.Fatalf("assistant button must return when re-enabled: %s", kb)
	}
}
