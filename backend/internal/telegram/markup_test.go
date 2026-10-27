package telegram

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInlineizeMarkup_ConvertsReplyKeyboard(t *testing.T) {
	in := json.RawMessage(`{"keyboard":[[{"text":"یک"}],[{"text":"دو"}]],"resize_keyboard":true}`)
	out := inlineizeMarkup(in)
	var m struct {
		InlineKeyboard [][]struct {
			Text         string `json:"text"`
			CallbackData string `json:"callback_data"`
		} `json:"inline_keyboard"`
	}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("bad output %s: %v", out, err)
	}
	if len(m.InlineKeyboard) != 2 || len(m.InlineKeyboard[0]) != 1 {
		t.Fatalf("rows = %d, want 2x1", len(m.InlineKeyboard))
	}
	if m.InlineKeyboard[0][0].Text != "یک" || m.InlineKeyboard[0][0].CallbackData != "یک" {
		t.Fatalf("button = %+v", m.InlineKeyboard[0][0])
	}
	if strings.Contains(string(out), `"keyboard"`) {
		t.Fatalf("reply keyboard must be gone: %s", out)
	}
}

func TestInlineizeMarkup_StringButtonsAndPassThrough(t *testing.T) {
	out := inlineizeMarkup(json.RawMessage(`{"keyboard":[["کوتاه"]]}`))
	if !strings.Contains(string(out), `"callback_data":"کوتاه"`) {
		t.Fatalf("string button not converted: %s", out)
	}
	// inline keyboards pass through untouched
	inline := json.RawMessage(`{"inline_keyboard":[[{"text":"x","callback_data":"y"}]]}`)
	if string(inlineizeMarkup(inline)) != string(inline) {
		t.Fatal("inline markup must pass through")
	}
	// remove_keyboard drops the markup entirely
	if got := inlineizeMarkup(json.RawMessage(`{"remove_keyboard":true}`)); got != nil {
		t.Fatalf("remove_keyboard should drop, got %s", got)
	}
	// malformed / empty passthrough
	if got := inlineizeMarkup(nil); got != nil {
		t.Fatal("nil must stay nil")
	}
}
