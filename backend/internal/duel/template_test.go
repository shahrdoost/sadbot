package duel

import (
	"testing"
)

func TestDetermineWinner(t *testing.T) {
	tests := []struct {
		player, opponent, want string
	}{
		{"rock", "scissors", "win"},
		{"rock", "paper", "lose"},
		{"rock", "rock", "draw"},
		{"paper", "rock", "win"},
		{"paper", "scissors", "lose"},
		{"paper", "paper", "draw"},
		{"scissors", "paper", "win"},
		{"scissors", "rock", "lose"},
		{"scissors", "scissors", "draw"},
	}
	for _, tt := range tests {
		got := determineWinner(tt.player, tt.opponent)
		if got != tt.want {
			t.Errorf("determineWinner(%q, %q) = %q, want %q", tt.player, tt.opponent, got, tt.want)
		}
	}
}

func TestDefaultPrices(t *testing.T) {
	tpl := &Template{}
	prices := tpl.DefaultPrices()
	if len(prices) == 0 {
		t.Fatal("DefaultPrices returned empty")
	}
}
