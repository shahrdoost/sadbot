package dating

import (
	"encoding/json"
	"testing"

	"sadbot/internal/engine"
)

// TestDefaultTextsComplete: every key the template looks up via ttext must
// exist in DefaultTexts, otherwise panel edits silently miss it.
func TestDefaultTextsComplete(t *testing.T) {
	tpl := &Template{}
	var defaults map[string]any
	if err := json.Unmarshal(tpl.DefaultTexts(), &defaults); err != nil {
		t.Fatalf("DefaultTexts invalid JSON: %v", err)
	}
	for _, key := range []string{
		"home.welcome", "home.guide", "search.waiting", "search.empty",
		"match.search", "match.browse", "chat.ended", "ai.intro",
		"wallet.title", "onboarding.done",
	} {
		if _, ok := defaults[key].(string); !ok {
			t.Errorf("DefaultTexts missing key %q", key)
		}
	}
}

// TestTTextFallback: a context without an override for the key falls back to
// the hardcoded default (override path is covered by engine's Text tests).
func TestTTextFallback(t *testing.T) {
	bc := &engine.BotContext{}
	if got := ttext(bc, "home.welcome", "def"); got != "def" {
		t.Fatalf("fallback = %q", got)
	}
}
