package dating

import (
	"strings"
	"testing"

	"sadbot/internal/domain"
)

func datingProfileFixture(name string, age int, city string) domain.DatingProfile {
	return domain.DatingProfile{Name: &name, Age: &age, City: &city}
}

func TestBrowseLikeCost(t *testing.T) {
	bc := priceCtx(map[string]any{
		"browse_free_likes": float64(10),
		"browse_like_cost":  float64(1),
	})
	cases := []struct {
		subscribed bool
		likesToday int
		want       int
	}{
		{false, 0, 0},  // within free quota
		{false, 9, 0},  // last free like
		{false, 10, 1}, // quota exhausted → paid
		{false, 25, 1}, //
		{true, 50, 0},  // subscribers always free
	}
	for _, c := range cases {
		if got := browseLikeCost(bc, c.subscribed, c.likesToday); got != c.want {
			t.Fatalf("browseLikeCost(sub=%v, today=%d) = %d, want %d", c.subscribed, c.likesToday, got, c.want)
		}
	}
}

func TestBrowseLikeCost_CustomPrices(t *testing.T) {
	bc := priceCtx(map[string]any{
		"browse_free_likes": float64(3),
		"browse_like_cost":  float64(2),
	})
	if got := browseLikeCost(bc, false, 3); got != 2 {
		t.Fatalf("custom cost = %d", got)
	}
	bc2 := priceCtx(map[string]any{})
	if got := browseLikeCost(bc2, false, 10); got != 1 {
		t.Fatalf("default cost = %d", got)
	}
}

func TestOppositeGender(t *testing.T) {
	if oppositeGender("male") != "female" || oppositeGender("female") != "male" || oppositeGender("") != "" {
		t.Fatal("oppositeGender mapping wrong")
	}
}

func TestBrowseCaption_ShowsDynamicCost(t *testing.T) {
	p := datingProfileFixture("سارا", 22, "تهران")
	cap0 := browseCaption(&p, 0, 5, false)
	if !strings.Contains(cap0, "رایگان") || !strings.Contains(cap0, "سارا") {
		t.Fatalf("free caption = %q", cap0)
	}
	capPaid := browseCaption(&p, 1, 0, false)
	if !strings.Contains(capPaid, "1 سکه") {
		t.Fatalf("paid caption = %q", capPaid)
	}
	capSub := browseCaption(&p, 1, 0, true)
	if !strings.Contains(capSub, "پریموم") {
		t.Fatalf("sub caption = %q", capSub)
	}
}

func TestFiltersCompatible_AdvancedSearch(t *testing.T) {
	a := Searcher{City: "تهران", Age: 25, CityFilter: "تهران", AgeMin: 18, AgeMax: 30}
	b := Searcher{City: "تهران", Age: 24}
	if !filtersCompatible(a, b) {
		t.Fatal("matching filters must pass")
	}
	bFar := Searcher{City: "شیراز", Age: 24}
	if filtersCompatible(a, bFar) {
		t.Fatal("city filter mismatch must fail")
	}
	bOld := Searcher{City: "تهران", Age: 40}
	if filtersCompatible(a, bOld) {
		t.Fatal("age filter mismatch must fail")
	}
	// symmetric: b's filters constrain a
	c := Searcher{City: "تبریز", Age: 30}
	d := Searcher{City: "تبریز", Age: 28, CityFilter: "تبریز"}
	if !filtersCompatible(c, d) {
		t.Fatal("symmetric city match must pass")
	}
	e := Searcher{City: "تبریز", Age: 28, CityFilter: "اصفهان"}
	if filtersCompatible(c, e) {
		t.Fatal("symmetric city mismatch must fail")
	}
}

// TestBrowseHomeButtonsNavigate: the persistent home reply keyboard stays
// visible while browsing (cards are inline); tapping its buttons must
// navigate, not advance the deck. Regression: «💰 سکه» used to show a card.
func TestBrowseHomeButtonsNavigate(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	// open the deck — scene is browse, a card is on screen
	e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")

	// wallet button must open the wallet, not the next card
	e.feed(t, e.sendA, "💰 سکه")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "کیف پول") {
		t.Fatalf("wallet button swallowed by browse: %#v", e.sendA.sent)
	}

	// scene is home again — profile page reachable
	e.feed(t, e.sendA, "👤 پروفایل من")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "حساب شما") {
		t.Fatalf("profile page not reachable after wallet: %#v", e.sendA.sent)
	}
}
