package dating

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestFilteredBrowseWizard: the kpkchat-parity «جستجوی ویژه» / «جستجوی کاربران»
// buttons open the filter wizard in browse mode and end in a FREE filtered
// card deck — not a paid timed search.
func TestFilteredBrowseWizard(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()

	// A: Tehran/25 male. B: Isfahan/22 female (custom city).
	e.feed(t, e.sendA, "/start")
	e.feed(t, e.sendA, "امیر")
	e.feed(t, e.sendA, "25")
	e.feed(t, e.sendA, "تهران")
	e.feed(t, e.sendA, "تهران")
	e.feed(t, e.sendA, "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	e.feed(t, e.sendB, "/start")
	e.feed(t, e.sendB, "سارا")
	e.feed(t, e.sendB, "22")
	e.feed(t, e.sendB, "اصفهان")
	e.feed(t, e.sendB, "اصفهان")
	e.feed(t, e.sendB, "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	stateOf := func(tg int64) map[string]any {
		p, err := e.store.ProfileByTelegram(ctx, e.botID, tg)
		if err != nil || p == nil {
			return nil
		}
		raw, err := e.users.GetState(ctx, e.botID.String(), p.UserID)
		if err != nil || len(raw) == 0 {
			return nil
		}
		var st map[string]any
		if json.Unmarshal(raw, &st) != nil {
			return nil
		}
		return st
	}
	scene := func() string {
		s, _ := stateOf(111)["scene"].(string)
		return s
	}
	coinsOf := func(tg int64) int {
		p, err := e.store.ProfileByTelegram(ctx, e.botID, tg)
		if err != nil || p == nil {
			return -1
		}
		return p.Coins
	}

	coinsBefore := coinsOf(111)

	// «جستجوی کاربران» → wizard in browse mode
	e.feed(t, e.sendA, "👥 جستجوی کاربران")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "رایگان") {
		t.Fatalf("browse-mode wizard intro must mention it's free, got %#v", msg)
	}
	e.feed(t, e.sendA, "همه")        // gender
	e.feed(t, e.sendA, "اصفهان")     // province
	e.feed(t, e.sendA, "اصفهان")     // city (اصفهان is a city of استان اصفهان)
	e.feed(t, e.sendA, "بدون محدودیت") // age range
	e.feed(t, e.sendA, "✔️ تایید")    // confirm interests (empty = no filter)
	msg := e.sendA.lastTo(111)
	if msg == nil || !strings.Contains(msg.Text, "رایگان") || !strings.Contains(msg.Text, "اصفهان") {
		t.Fatalf("browse-mode confirm must show free + city filter, got %#v", msg)
	}

	// start → filtered deck shows B (Isfahan), scene=browse, NO coins spent
	e.feed(t, e.sendA, "🔎 نمایش پروفایل‌ها")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "سارا") {
		t.Fatalf("filtered deck must show the Isfahan profile, got %#v", msg)
	}
	if scene() != "browse" {
		t.Fatalf("scene = %q, want browse", scene())
	}
	if got := coinsOf(111); got != coinsBefore {
		t.Fatalf("filtered browse must be free: coins %d → %d", coinsBefore, got)
	}

	// exit, then «جستجوی ویژه» with an age range that excludes B → empty deck
	e.feed(t, e.sendA, "🏠 منوی اصلی")
	e.feed(t, e.sendA, "🔍 جستجوی ویژه")
	e.feed(t, e.sendA, "همه")           // gender
	e.feed(t, e.sendA, "همه استان‌ها") // province (skip city)
	e.feed(t, e.sendA, "۲۸-۳۲")         // age range 28-32, B is 22 so excluded
	e.feed(t, e.sendA, "✔️ تایید")      // confirm interests (empty)
	e.feed(t, e.sendA, "🔎 نمایش پروفایل‌ها")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "باقی نمونده") {
		t.Fatalf("age-filtered empty deck must say empty, got %#v", msg)
	}
	if scene() != "home" {
		t.Fatalf("empty deck must return home, scene = %q", scene())
	}

	// sanity: the timed advanced search entry is unchanged (paid copy)
	e.feed(t, e.sendA, "🔗 به یک ناشناس وصلم کن")
	e.feed(t, e.sendA, "🔍 جستجوی پیشرفته")
	e.feed(t, e.sendA, "همه")           // gender
	e.feed(t, e.sendA, "همه استان‌ها") // province (skip city)
	e.feed(t, e.sendA, "بدون محدودیت")  // age range
	e.feed(t, e.sendA, "✔️ تایید")      // confirm interests (empty)
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "جستجوی زمان‌دار") {
		t.Fatalf("timed advanced wizard must keep its paid confirm copy, got %#v", msg)
	}
	e.feed(t, e.sendA, "🏠 منوی اصلی")
	if scene() != "home" {
		t.Fatalf("home from wizard confirm must exit, scene = %q", scene())
	}
}
