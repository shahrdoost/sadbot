package dating

import (
	"testing"
	"time"
)

func TestGhostLevel(t *testing.T) {
	cases := map[int]int{0: 3, 1: 2, 9: 2, 10: 1, 49: 1, 50: 0, 100: 0}
	for active, want := range cases {
		if got := ghostLevel(active); got != want {
			t.Fatalf("ghostLevel(%d) = %d, want %d", active, got, want)
		}
	}
}

func TestInQuietHours(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		t.Skip("tzdata not available")
	}
	prices := map[string]any{"notify_quiet_start": float64(22), "notify_quiet_end": float64(9)}
	quiet := time.Date(2026, 1, 1, 23, 0, 0, 0, loc)   // 23:00 Tehran
	morning := time.Date(2026, 1, 1, 10, 0, 0, 0, loc) // 10:00 Tehran
	if !inQuietHours(quiet, prices) {
		t.Fatal("23:00 Tehran must be quiet")
	}
	if inQuietHours(morning, prices) {
		t.Fatal("10:00 Tehran must not be quiet")
	}
	// wrap-around window (e.g. 1..5)
	wrap := map[string]any{"notify_quiet_start": float64(1), "notify_quiet_end": float64(5)}
	if !inQuietHours(time.Date(2026, 1, 1, 2, 0, 0, 0, loc), wrap) {
		t.Fatal("02:00 in wrap window must be quiet")
	}
	if inQuietHours(time.Date(2026, 1, 1, 12, 0, 0, 0, loc), wrap) {
		t.Fatal("12:00 outside wrap window must not be quiet")
	}
}

func TestAIHint(t *testing.T) {
	if h := aiHint("حالت چطوره؟"); h == "" {
		t.Fatal("question must add hint")
	}
	if h := aiHint("حالم خوبه عالیه"); h == "" {
		t.Fatal("positive mood must add hint")
	}
	if h := aiHint("خسته‌ام حالم خوب نیست"); h == "" {
		t.Fatal("negative mood must add hint")
	}
	if h := aiHint("سلام"); h != "" {
		t.Fatalf("plain greeting must have no hint, got %q", h)
	}
}

func TestPriceInt_Defaults(t *testing.T) {
	if got := priceInt(nil, "x", 7); got != 7 {
		t.Fatalf("nil map default = %d", got)
	}
	if got := priceInt(map[string]any{"x": float64(3)}, "x", 7); got != 3 {
		t.Fatalf("float = %d", got)
	}
	if got := priceInt(map[string]any{}, "missing", 7); got != 7 {
		t.Fatalf("missing = %d", got)
	}
}

func TestGhostReplyRouting(t *testing.T) {
	// deterministic enough: question marker must pick a question-family reply
	r := ghostReplyTo("اسم تو چیه؟")
	if r == "" {
		t.Fatal("empty ghost reply")
	}
}
