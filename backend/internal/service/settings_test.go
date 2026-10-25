package service

import (
	"encoding/json"
	"testing"
)

// TestApplySettingsPreservesUnknownKeys: the numeric form must not drop price
// keys it does not render (channels, packages, streak_rewards). Losing them
// would silently disable multi-channel missions and coin packages.
func TestApplySettingsPreservesUnknownKeys(t *testing.T) {
	current := json.RawMessage(`{
		"channels": ["@a","@b"],
		"channel_id": "@legacy",
		"packages": [{"id":"pkg_1","coins":1,"price":10000}],
		"streak_rewards": {"3":5,"7":15},
		"signup_bonus": 5
	}`)

	out, err := ApplySettings(current, map[string]int{"signup_bonus": 20, "superlike_cost": 7})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("result must be a JSON object: %v (raw=%s)", err, out)
	}

	// edited keys applied
	if v, _ := got["signup_bonus"].(float64); v != 20 {
		t.Fatalf("signup_bonus = %v, want 20", got["signup_bonus"])
	}
	if v, _ := got["superlike_cost"].(float64); v != 7 {
		t.Fatalf("superlike_cost = %v, want 7", got["superlike_cost"])
	}
	// unknown keys survived untouched
	channels, ok := got["channels"].([]any)
	if !ok || len(channels) != 2 {
		t.Fatalf("channels lost by the settings form: %v", got["channels"])
	}
	if got["channel_id"] != "@legacy" {
		t.Fatalf("channel_id lost: %v", got["channel_id"])
	}
	if _, ok := got["packages"].([]any); !ok {
		t.Fatalf("packages lost: %v", got["packages"])
	}
	if _, ok := got["streak_rewards"].(map[string]any); !ok {
		t.Fatalf("streak_rewards lost: %v", got["streak_rewards"])
	}
}

// TestApplySettingsBounds: out-of-range values are rejected with a field-named
// error rather than silently clamped.
func TestApplySettingsBounds(t *testing.T) {
	cur := json.RawMessage(`{}`)
	if _, err := ApplySettings(cur, map[string]int{"sub_discount_pct": 150}); err == nil {
		t.Fatal("percent over 100 must be rejected")
	}
	if _, err := ApplySettings(cur, map[string]int{"signup_bonus": -1}); err == nil {
		t.Fatal("negative bonus must be rejected")
	}
	if _, err := ApplySettings(cur, map[string]int{"ghost_enabled": 5}); err == nil {
		t.Fatal("bool field out of 0/1 must be rejected")
	}
	if _, err := ApplySettings(cur, map[string]int{"made_up_key": 1}); err == nil {
		t.Fatal("unknown key must be rejected")
	}
	if _, err := ApplySettings(cur, map[string]int{}); err == nil {
		t.Fatal("empty submission must be rejected")
	}
	// boundary values are allowed
	if _, err := ApplySettings(cur, map[string]int{"sub_discount_pct": 100}); err != nil {
		t.Fatalf("100 must be allowed: %v", err)
	}
	if _, err := ApplySettings(cur, map[string]int{"ghost_enabled": 0}); err != nil {
		t.Fatalf("bool 0 must be allowed: %v", err)
	}
}

// TestSettingsOfUsesDefaultsThenOverrides: the form shows template defaults
// when nothing is configured, and the bot's own values once set.
func TestSettingsOfUsesDefaultsThenOverrides(t *testing.T) {
	defaults := []byte(`{"signup_bonus":5,"superlike_cost":3,"ghost_enabled":1}`)

	fields, err := SettingsOf(defaults, nil)
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	got := map[string]int{}
	for _, f := range fields {
		got[f.Key] = f.Value
	}
	if got["signup_bonus"] != 5 || got["superlike_cost"] != 3 || got["ghost_enabled"] != 1 {
		t.Fatalf("defaults not surfaced: %+v", got)
	}

	fields, err = SettingsOf(defaults, json.RawMessage(`{"signup_bonus":99}`))
	if err != nil {
		t.Fatalf("override: %v", err)
	}
	got = map[string]int{}
	for _, f := range fields {
		got[f.Key] = f.Value
	}
	if got["signup_bonus"] != 99 {
		t.Fatalf("override not applied: %d", got["signup_bonus"])
	}
	// untouched keys keep their defaults
	if got["superlike_cost"] != 3 {
		t.Fatalf("default lost for unset key: %d", got["superlike_cost"])
	}
	// every field carries a label and a sane bound
	for _, f := range fields {
		if f.Label == "" || f.Group == "" {
			t.Fatalf("field missing label/group: %+v", f)
		}
		if f.Min > f.Max {
			t.Fatalf("field has inverted bounds: %+v", f)
		}
		if f.Value < f.Min || f.Value > f.Max {
			t.Fatalf("default value outside its own bounds: %+v", f)
		}
	}
}

// TestApplySettingsBoolRoundTrip: bool flags are stored as 0/1 ints, which the
// bot reads through PriceInt.
func TestApplySettingsBoolRoundTrip(t *testing.T) {
	out, err := ApplySettings(json.RawMessage(`{}`), map[string]int{"notify_enabled": 0, "ghost_enabled": 1})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, _ := got["notify_enabled"].(float64); v != 0 {
		t.Fatalf("notify_enabled = %v", got["notify_enabled"])
	}
	if v, _ := got["ghost_enabled"].(float64); v != 1 {
		t.Fatalf("ghost_enabled = %v", got["ghost_enabled"])
	}
}
