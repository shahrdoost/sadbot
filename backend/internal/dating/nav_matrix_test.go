package dating

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestHomeNavFromEveryScene: the home reply keyboard stays visible in every
// scene whose own UI is inline-only (browse, wallet screens live in home,
// editphoto prompt). Each home-menu label must open its destination instead
// of being swallowed by the scene. This is the regression matrix for the
// «💰 سکه shows the next card» bug.
func TestHomeNavFromEveryScene(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")
	e.onboard(t, e.sendB, "سارا", "22", "خانومم")
	e.feed(t, e.sendB, "⏭ رد شد")

	stateOf := func() map[string]any {
		p, err := e.store.ProfileByTelegram(ctx, e.botID, 111)
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
		st := stateOf()
		s, _ := st["scene"].(string)
		return s
	}

	// each case: from the browse scene, press the label → destination marker
	// must be the LAST message (not a browse card).
	cases := []struct {
		button string
		marker string
		want   string // expected scene afterwards ("" = don't check)
	}{
		{"💰 سکه", "کیف پول شما", "home"},
		{"🎁 ماموریت‌ها", "ماموریت‌ها", "home"},
		{"👤 پروفایل من", "حساب شما", "home"},
		{"📖 راهنما", "راهنما", "home"},
		{"💌 اعتراف ناشناس", "اعتراف ناشناس", "confess"},
		{"📩 پشتیبانی", "پشتیبانی", "support"},
		{"🤖 چت با دستیار", "دستیار هوشمندم", "ai"},
		{"🔗 به یک ناشناس وصلم کن", "جستجوی ناشناس", "home"},
		{"💘 مرور پروفایل‌ها", "ساله", "browse"}, // re-enters browse on purpose
	}
	for _, tc := range cases {
		// always start from the browse scene with a card on screen
		e.feed(t, e.sendA, "🏠 منوی اصلی")
		e.feed(t, e.sendA, "💘 مرور پروفایل‌ها")
		if got := scene(); got != "browse" {
			t.Fatalf("setup for %q: scene = %q, want browse", tc.button, got)
		}

		e.feed(t, e.sendA, tc.button)
		msg := e.sendA.lastTo(111)
		if msg == nil || !strings.Contains(msg.Text, tc.marker) {
			t.Fatalf("button %q from browse: want last message to contain %q, got %#v", tc.button, tc.marker, msg)
		}
		if tc.want != "" && scene() != tc.want {
			t.Fatalf("button %q: scene = %q, want %q", tc.button, scene(), tc.want)
		}
	}

	// leaving the AI scene via its own home button must work (keyboard is
	// inline-only there too)
	e.feed(t, e.sendA, "🤖 چت با دستیار")
	if scene() != "ai" {
		t.Fatalf("expected ai scene, got %q", scene())
	}
	e.feed(t, e.sendA, "🏠 منوی اصلی")
	if scene() != "home" {
		t.Fatalf("home button inside ai scene must exit, scene = %q", scene())
	}

	// the timed-search scene's home button must also exit
	e.feed(t, e.sendA, "🔗 به یک ناشناس وصلم کن") // search menu (home scene)
	e.feed(t, e.sendA, "🔗 غریبه")                 // enters search scene
	if scene() != "search" {
		t.Fatalf("expected search scene, got %q", scene())
	}
	e.feed(t, e.sendA, "🏠 منوی اصلی")
	if scene() != "home" {
		t.Fatalf("home button inside search scene must exit, scene = %q", scene())
	}
}

// TestHomeNavFromInlineOnlyScenes: non-home menu labels pressed inside the
// scenes that keep the home keyboard visible (or accept typed text) must
// navigate home — not be swallowed as scene input. Live bug this locks: inside
// confess, pressing «📩 پشتیبانی» became the confession text.
func TestHomeNavFromInlineOnlyScenes(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	stateOf := func() map[string]any {
		p, err := e.store.ProfileByTelegram(ctx, e.botID, 111)
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
		s, _ := stateOf()["scene"].(string)
		return s
	}

	// enter := button that opens the scene; wantScene := scene after entering;
	// wantAfter := scene after pressing the menu label (home, or the target
	// scene when the label is itself a menu destination like «پشتیبانی»).
	cases := []struct {
		enter     string
		wantScene string
		button    string
		marker    string
		wantAfter string
	}{
		{"💌 اعتراف ناشناس", "confess", "💰 سکه", "کیف پول", "home"},
		{"💌 اعتراف ناشناس", "confess", "📩 پشتیبانی", "پشتیبانی", "support"}, // the live repro
		{"📩 پشتیبانی", "support", "📖 راهنما", "راهنما", "home"},
		{"🤖 چت با دستیار", "ai", "🎁 ماموریت‌ها", "ماموریت‌ها", "home"},
	}
	for _, tc := range cases {
		e.feed(t, e.sendA, "🏠 منوی اصلی")
		e.feed(t, e.sendA, tc.enter)
		if got := scene(); got != tc.wantScene {
			t.Fatalf("enter %q: scene = %q, want %q", tc.enter, got, tc.wantScene)
		}
		e.feed(t, e.sendA, tc.button)
		msg := e.sendA.lastTo(111)
		if msg == nil || !strings.Contains(msg.Text, tc.marker) {
			t.Fatalf("button %q from %s: want last message to contain %q, got %#v", tc.button, tc.wantScene, tc.marker, msg)
		}
		if got := scene(); got != tc.wantAfter {
			t.Fatalf("button %q from %s: scene = %q, want %q", tc.button, tc.wantScene, got, tc.wantAfter)
		}
	}

	// mid-confession-confirm: a menu label must discard the preview and go
	// home, not be treated as a confirm/cancel answer.
	e.feed(t, e.sendA, "🏠 منوی اصلی")
	e.feed(t, e.sendA, "💌 اعتراف ناشناس")
	e.feed(t, e.sendA, "متن آزمایشی اعتراف برای تست ناوبری")
	if step, _ := stateOf()["step"].(string); step != "confess_confirm" {
		t.Fatalf("expected confess_confirm step, got %q", step)
	}
	e.feed(t, e.sendA, "💰 سکه")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "کیف پول") {
		t.Fatalf("menu label at confirm step must open the wallet, got %#v", msg)
	}
	if got := scene(); got != "home" {
		t.Fatalf("menu label at confirm step: scene = %q, want home", got)
	}

	// timed-search scene: non-home labels navigate too (not just the home one)
	e.feed(t, e.sendA, "🏠 منوی اصلی")
	e.feed(t, e.sendA, "🔗 به یک ناشناس وصلم کن")
	e.feed(t, e.sendA, "🔗 غریبه")
	if got := scene(); got != "search" {
		t.Fatalf("expected search scene, got %q", got)
	}
	e.feed(t, e.sendA, "👤 پروفایل من")
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "حساب شما") {
		t.Fatalf("menu label from search must open the profile page, got %#v", msg)
	}
	if got := scene(); got != "home" {
		t.Fatalf("menu label from search: scene = %q, want home", got)
	}
}

// TestHomeMenuColoredButtons: the reply keyboards must carry kpkchat-style
// Bot API 7.11 button colors — primary on «به یک ناشناس وصلم کن» and «غریبه»,
// success on «مرور پروفایل‌ها». Guards the Style passthrough in
// engine.ReplyKBDStyled / telegram.ReplyKeyboardButton.
func TestHomeMenuColoredButtons(t *testing.T) {
	e := newBrowseEnv(t)
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	kbdJSON := func() string {
		m := e.sendA.lastTo(111)
		if m == nil {
			return ""
		}
		return string(m.ReplyMark)
	}

	// home keyboard: primary on connect, success on browse
	e.feed(t, e.sendA, "🏠 منوی اصلی")
	kbd := kbdJSON()
	if !strings.Contains(kbd, `"style":"primary"`) || !strings.Contains(kbd, `"style":"success"`) {
		t.Fatalf("home keyboard must carry primary+success styles, got: %s", kbd)
	}
	if !strings.Contains(kbd, "به یک ناشناس وصلم کن") || !strings.Contains(kbd, "مرور پروفایل‌ها") {
		t.Fatalf("home keyboard lost its labels: %s", kbd)
	}

	// search menu: primary on «غریبه»
	e.feed(t, e.sendA, "🔗 به یک ناشناس وصلم کن")
	kbd = kbdJSON()
	if !strings.Contains(kbd, `"style":"primary"`) || !strings.Contains(kbd, "غریبه") {
		t.Fatalf("search keyboard must color «غریبه» primary, got: %s", kbd)
	}

	// sanity: label-only derivation must keep every home label intact
	if len(homeRows) != len(homeKB) {
		t.Fatalf("homeRows derivation drifted: %d vs %d rows", len(homeRows), len(homeKB))
	}
	if homeRows[0][0] != "🔗 به یک ناشناس وصلم کن" || searchRows[0][0] != "🔗 غریبه" {
		t.Fatalf("kbLabelRows must preserve labels, got %v / %v", homeRows[0], searchRows[0])
	}
}

// TestEditPhotoPromptEscapesHome: the paid photo-edit prompt shows the home
// keyboard; pressing it must return home without stamping the cooldown.
func TestEditPhotoPromptEscapesHome(t *testing.T) {
	e := newBrowseEnv(t)
	ctx := context.Background()
	e.onboard(t, e.sendA, "امیر", "25", "آقام")
	e.feed(t, e.sendA, "⏭ رد شد")

	stateOf := func() map[string]any {
		p, err := e.store.ProfileByTelegram(ctx, e.botID, 111)
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
		s, _ := stateOf()["scene"].(string)
		return s
	}
	step := func() string {
		s, _ := stateOf()["step"].(string)
		return s
	}

	e.feedCB(t, e.sendA, "act:edit:photo")
	if scene() != "setup" || step() != "editphoto" {
		t.Fatalf("expected setup/editphoto, got %q/%q", scene(), step())
	}

	// escape via home — must land home, no stamp
	e.feed(t, e.sendA, "🏠 منوی اصلی")
	if scene() != "home" {
		t.Fatalf("home escape from editphoto failed, scene = %q", scene())
	}
	p, err := e.store.ProfileByTelegram(ctx, e.botID, 111)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	if p.ProfileEditedAt != nil {
		t.Fatal("abandoned photo edit must not stamp the cooldown")
	}

	// a stray non-photo text at the prompt must re-prompt, not crash
	e.feedCB(t, e.sendA, "act:edit:photo")
	e.feed(t, e.sendA, "متن معمولی")
	if step() != "editphoto" {
		t.Fatalf("plain text should keep the editphoto step, got %q", step())
	}
	if msg := e.sendA.lastTo(111); msg == nil || !strings.Contains(msg.Text, "فقط عکس") {
		t.Fatalf("expected re-prompt, got %#v", msg)
	}
}
