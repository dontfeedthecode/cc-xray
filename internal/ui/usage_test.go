package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dontfeedthecode/cc-xray/internal/turn"
	"github.com/dontfeedthecode/cc-xray/internal/usage"
)

// The footer carries the turn's cost and the session's, the session marked as
// an estimate since the fixture has no cost-state record.
func TestFooterShowsCost(t *testing.T) {
	m := seeded(t, 40)
	v := stripANSI(m.View())
	if !strings.Contains(v, " turn") || !strings.Contains(v, "~$") ||
		!strings.Contains(v, " session") {
		t.Errorf("footer lacks turn and session cost:\n%s", v)
	}
}

// u opens the per-model breakdown and closes it again, and the viewport gives
// up the room it takes.
func TestUsageKeyTogglesBreakdown(t *testing.T) {
	m := seeded(t, 40)
	before := m.vp.Height
	if strings.Contains(stripANSI(m.View()), "CACHE READ") {
		t.Fatal("breakdown drawn before it was asked for")
	}
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = mm.(Model)
	v := stripANSI(m.View())
	for _, want := range []string{"CACHE READ", "this turn", "session", "opus-5", "of input from cache"} {
		if !strings.Contains(v, want) {
			t.Errorf("breakdown missing %q:\n%s", want, v)
		}
	}
	if m.vp.Height >= before {
		t.Errorf("viewport %d did not shrink from %d for the breakdown", m.vp.Height, before)
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = mm.(Model)
	if strings.Contains(stripANSI(m.View()), "CACHE READ") || m.vp.Height != before {
		t.Error("second u did not close the breakdown")
	}
}

func TestUsageLinesFitWidth(t *testing.T) {
	tn := load(t)
	s := turn.Session{Ledger: tn.Usage}
	for _, w := range []int{92, 76, 56, 44} {
		out := Render(tn, NewTheme(), UnicodeGlyphs(), Opts{Width: w, Rows: 40, Session: &s, ShowUsage: true})
		for _, ln := range splitLines(out) {
			if got := visWidth(ln); got > w {
				t.Errorf("width %d: line is %d cells: %q", w, got, stripANSI(ln))
			}
		}
	}
}

// A model with no known price must not read as free.
func TestUnpricedModelIsAFloor(t *testing.T) {
	var l usage.Ledger
	l.Add("claude-opus-5-5", usage.Tokens{Out: 1_000_000}, false)
	l.Add("claude-unknown-9", usage.Tokens{Out: 1_000_000}, false)
	if got := money(l.Cost(), l.Unpriced); got != "≥$20.00" {
		t.Errorf("money = %q, want ≥$20.00", got)
	}
}
