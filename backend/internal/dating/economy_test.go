package dating

import (
	"testing"

	"sadbot/internal/engine"

	"github.com/google/uuid"
)

func priceCtx(prices map[string]any) *engine.BotContext {
	return &engine.BotContext{Prices: prices, State: map[string]any{"data": map[string]any{}}}
}

func TestSearchCost_KpkchatParity(t *testing.T) {
	bc := priceCtx(map[string]any{
		"search_stranger_male":   float64(1),
		"search_stranger_female": float64(0),
		"search_gender_male":     float64(5),
		"search_gender_female":   float64(1),
	})
	cases := []struct {
		filter, gender string
		subscribed     bool
		want           int
	}{
		{"any", "male", false, 1},
		{"any", "female", false, 0},
		{"male", "male", false, 5},
		{"female", "female", false, 1},
		{"any", "male", true, 0},  // subscribers search free
		{"male", "male", true, 0}, //
		{"any", "", false, 1},     // unknown gender → male default
	}
	for _, c := range cases {
		got := searchCost(bc, c.filter, c.gender, c.subscribed)
		if got != c.want {
			t.Fatalf("searchCost(%q,%q,sub=%v) = %d, want %d", c.filter, c.gender, c.subscribed, got, c.want)
		}
	}
}

func TestSearchCost_DefaultsWhenEmpty(t *testing.T) {
	bc := priceCtx(map[string]any{})
	if got := searchCost(bc, "any", "male", false); got != 1 {
		t.Fatalf("default stranger male = %d", got)
	}
	if got := searchCost(bc, "female", "male", false); got != 5 {
		t.Fatalf("default gender male = %d", got)
	}
}

func TestPriceInt_AndPackages(t *testing.T) {
	bc := priceCtx(map[string]any{
		"signup_bonus": float64(7),
		"packages": []any{
			map[string]any{"id": "pkg_1", "coins": float64(1), "price": float64(10000)},
			map[string]any{"id": "pkg_5", "coins": float64(5), "price": float64(50000)},
		},
	})
	if got := bc.PriceInt("signup_bonus", 5); got != 7 {
		t.Fatalf("signup_bonus = %d", got)
	}
	if got := bc.PriceInt("missing", 3); got != 3 {
		t.Fatalf("fallback = %d", got)
	}
	pkgs := bc.PricePackages()
	if len(pkgs) != 2 || pkgs[1]["id"] != "pkg_5" {
		t.Fatalf("packages = %+v", pkgs)
	}
}

func TestDiscountedPrice(t *testing.T) {
	bc := priceCtx(map[string]any{"sub_discount_pct": float64(30)})
	if got := discountedPrice(bc, 100_000); got != 70_000 {
		t.Fatalf("discounted = %d", got)
	}
	bc2 := priceCtx(map[string]any{})
	if got := discountedPrice(bc2, 100_000); got != 70_000 {
		t.Fatalf("default discount = %d", got)
	}
}

func TestFormatTomans(t *testing.T) {
	cases := map[int]string{10000: "10,000", 90000: "90,000", 350000: "350,000", 280000: "280,000", 5: "5"}
	for in, want := range cases {
		if got := formatTomans(in); got != want {
			t.Fatalf("formatTomans(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestCaptureReferralPayload(t *testing.T) {
	bc := priceCtx(nil)
	captureReferralPayload(bc, "ref_12345")
	data := bc.State["data"].(map[string]any)
	if data["ref"] != "12345" {
		t.Fatalf("ref = %v", data["ref"])
	}
	// garbage payloads ignored
	bc2 := priceCtx(nil)
	captureReferralPayload(bc2, "start")
	captureReferralPayload(bc2, "ref_notanumber")
	if bc2.State["data"].(map[string]any)["ref"] != nil {
		t.Fatal("invalid payload must be ignored")
	}
}

func TestBotUUID_ParsesState(t *testing.T) {
	id := uuid.New()
	bc := &engine.BotContext{BotID: id.String()}
	if botUUID(bc) != id {
		t.Fatal("botUUID mismatch")
	}
}
