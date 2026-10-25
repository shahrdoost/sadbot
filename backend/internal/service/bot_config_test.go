package service

import (
	"context"
	"encoding/json"
	"testing"

	"sadbot/internal/domain"
)

// TestUpdateConfig_NormalizesStringifiedFields covers the panel contract: the
// config form stringifies its textareas, so the PUT body carries prices/texts/
// menus as JSON *strings*. Stored verbatim those become a jsonb string and the
// runtime merge ignores every panel edit. UpdateConfig must unwrap them.
func TestUpdateConfig_NormalizesStringifiedFields(t *testing.T) {
	f := newTestBotService(t)
	ctx := context.Background()
	admin := superAdmin()
	bot, err := f.svc.Create(ctx, admin, CreateBotInput{Name: "cfg", Token: validBotToken()}, nil)
	if err != nil {
		t.Fatalf("create bot: %v", err)
	}

	// exactly the shape the panel sends: nested JSON encoded as strings
	body := []byte(`{
		"version": 1,
		"texts": "{\"welcome\":\"سلام\"}",
		"menus": "{}",
		"prices": "{\"channels\":[\"@a\",\"@b\"],\"channel_bonus\":10}"
	}`)
	var in UpdateConfigInput
	if err := json.Unmarshal(body, &in); err != nil {
		t.Fatalf("decode request: %v", err)
	}

	saved, err := f.svc.UpdateConfig(ctx, admin, bot.ID, in, nil)
	if err != nil {
		t.Fatalf("update config: %v", err)
	}

	var prices map[string]any
	if err := json.Unmarshal(saved.Prices, &prices); err != nil {
		t.Fatalf("prices must be stored as an object, not a JSON string: %v (raw=%s)", err, saved.Prices)
	}
	channels, ok := prices["channels"].([]any)
	if !ok || len(channels) != 2 {
		t.Fatalf("channels lost: %v (raw=%s)", prices["channels"], saved.Prices)
	}
	if bonus, ok := prices["channel_bonus"].(float64); !ok || bonus != 10 {
		t.Fatalf("channel_bonus lost: %v", prices["channel_bonus"])
	}

	var texts map[string]any
	if err := json.Unmarshal(saved.Texts, &texts); err != nil {
		t.Fatalf("texts must be stored as an object: %v (raw=%s)", err, saved.Texts)
	}
	if texts["welcome"] != "سلام" {
		t.Fatalf("texts content lost: %s", saved.Texts)
	}
}

// TestNormalizeConfigInput_Passthrough: object-shaped values are untouched, and
// non-JSON strings are left alone for validation to reject.
func TestNormalizeConfigInput_Passthrough(t *testing.T) {
	in := UpdateConfigInput{
		Texts:  json.RawMessage(`{"a":1}`),
		Prices: json.RawMessage(`{"b":2}`),
		Menus:  json.RawMessage(`"not json at all"`),
	}
	out := NormalizeConfigInput(in)
	if string(out.Texts) != `{"a":1}` {
		t.Fatalf("object body must pass through: %s", out.Texts)
	}
	if string(out.Prices) != `{"b":2}` {
		t.Fatalf("object body must pass through: %s", out.Prices)
	}
	if string(out.Menus) != `"not json at all"` {
		t.Fatalf("non-JSON string must stay for the validator: %s", out.Menus)
	}
	_ = domain.PermBotConfig
}
