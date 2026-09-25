package usage

import (
	"math"
	"testing"
)

// The totals Claude Code wrote in a real cost-state record (Claude Code
// 2.1.280). Pricing its per-model token counts must land on its own cost,
// which pins both the rates and the formula.
func TestMatchesCostState(t *testing.T) {
	cases := []struct {
		model string
		tok   Tokens
		want  float64
	}{
		{"claude-opus-5-5[1m]", Tokens{In: 2074, Out: 12867, CacheRead: 2042833, Write1h: 54784}, 1.1124746},
		{"claude-haiku-4-5-20251001", Tokens{In: 950, Out: 15}, 0.001025},
	}
	for _, c := range cases {
		got, ok := Cost(c.model, c.tok, false)
		if !ok {
			t.Fatalf("%s: no price", c.model)
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: cost = %.7f, want %.7f", c.model, got, c.want)
		}
	}
}

func TestCanonical(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-5-5[1m]":       "claude-opus-5-5",
		"claude-haiku-4-5-20251001": "claude-haiku-4-5",
		"claude-sonnet-5":           "claude-sonnet-5",
	} {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
}

// An unknown model must not be priced at zero silently.
func TestUnknownModelMarksLedger(t *testing.T) {
	var l Ledger
	l.Add("claude-opus-5-5", Tokens{Out: 1000}, false)
	if l.Unpriced {
		t.Fatal("known model marked unpriced")
	}
	l.Add("claude-something-9", Tokens{Out: 1000}, false)
	if !l.Unpriced {
		t.Error("unknown model left the ledger looking complete")
	}
}

func TestCacheShare(t *testing.T) {
	// /usage: 728 input, 1.1m cache read, 49.0k cache write -> 95%
	tok := Tokens{In: 728, CacheRead: 1_100_000, Write1h: 49_000}
	if s := tok.CacheShare(); s < 0.95 || s > 0.96 {
		t.Errorf("share = %.3f, want ~0.957", s)
	}
	if (Tokens{}).CacheShare() != -1 {
		t.Error("no input must report -1, not 0%")
	}
}

func TestFastModeDoubles(t *testing.T) {
	tok := Tokens{In: 1_000_000, Out: 1_000_000}
	std, _ := Cost("claude-opus-5-5", tok, false)
	fast, _ := Cost("claude-opus-5-5", tok, true)
	if fast != 2*std || std != 24 {
		t.Errorf("std = %v, fast = %v; want 24 and 48", std, fast)
	}
}
