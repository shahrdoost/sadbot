package bombland

import (
	"testing"
)

func TestDefaultPrices(t *testing.T) {
	tpl := &Template{}
	prices := tpl.DefaultPrices()
	if len(prices) == 0 {
		t.Fatal("DefaultPrices returned empty")
	}
}

func TestDefaultTexts(t *testing.T) {
	tpl := &Template{}
	texts := tpl.DefaultTexts()
	if len(texts) == 0 {
		t.Fatal("DefaultTexts returned empty")
	}
}
