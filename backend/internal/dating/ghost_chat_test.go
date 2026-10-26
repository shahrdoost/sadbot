package dating

import (
	"strings"
	"testing"

	"sadbot/internal/domain"
)

func TestGhostSystemPrompt_PersonaAndRules(t *testing.T) {
	name, gender, city := "مینا", "female", "تهران"
	age := 24
	p := &domain.DatingProfile{Name: &name, Gender: &gender, Age: &age, City: &city, Interests: []string{"فیلم", "پیاده‌روی"}}
	prompt := ghostSystemPrompt(p)
	for _, want := range []string{"مینا", "24", "تهران", "دختر", "فیلم", "رباتی؟", "محاوره‌ای"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestGhostBreaksCharacter(t *testing.T) {
	if !ghostBreaksCharacter("من هوش مصنوعی هستم") {
		t.Fatal("must flag AI disclosure")
	}
	if !ghostBreaksCharacter("راستش من یه رباتم") {
		t.Fatal("must flag bot admission")
	}
	if ghostBreaksCharacter("نه بابا 😄 چرا همچین فکری کردی؟") {
		t.Fatal("natural denial must pass")
	}
	if ghostBreaksCharacter("سلام خوبی؟ من مینام") {
		t.Fatal("normal reply must pass")
	}
}

func TestSplitBubbles(t *testing.T) {
	if got := splitBubbles("سلام"); len(got) != 1 || got[0] != "سلام" {
		t.Fatalf("short reply must stay one bubble: %v", got)
	}
	long := "آره داشتم فیلم می‌دیدم، خیلی قشنگ بود و کلی خندیدم روش. تو چیکار می‌کردی؟ این چند وقته چه خبر؟"
	got := splitBubbles(long)
	if len(got) != 2 {
		t.Fatalf("long reply must split, got %v", got)
	}
	if rr := []rune(got[0]); rr[len(rr)-1] != '.' && rr[len(rr)-1] != '؟' && rr[len(rr)-1] != '!' {
		t.Fatalf("first bubble must end at sentence: %q", got[0])
	}
	if got := splitBubbles(""); len(got) != 0 {
		t.Fatal("empty reply must yield nothing")
	}
}
