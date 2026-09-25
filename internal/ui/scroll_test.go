package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dontfeedthecode/cc-xray/internal/turn"
)

func seeded(t *testing.T, h int) Model {
	t.Helper()
	m := NewModel(Options{Path: "testdata/session.jsonl"})
	m.poll()
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 92, Height: h})
	return mm.(Model)
}

// The whole point of the change: history must be reachable.
func TestScrollbackReachesOlderRows(t *testing.T) {
	m := seeded(t, 14) // deliberately shorter than the row count
	if m.b.Turn() == nil || len(m.b.Turn().Rows) < 10 {
		t.Fatalf("fixture did not produce enough rows")
	}
	bottom := m.vp.View()
	up, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = up.(Model)
	if m.vp.View() == bottom {
		t.Fatal("page up did not move the viewport")
	}
	if m.follow {
		t.Error("scrolling up must stop following the live edge")
	}
	// the first row of the turn should be reachable
	gm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = gm.(Model)
	if !m.vp.AtTop() {
		t.Error("g did not reach the top")
	}
}

// New rows must not yank the view away while the user is reading history.
func TestScrollPositionHeldWhileReadingHistory(t *testing.T) {
	m := seeded(t, 14)
	up, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = up.(Model)
	held := m.vp.YOffset

	m.refresh() // simulate new content arriving
	if m.vp.YOffset != held {
		t.Errorf("scroll jumped from %d to %d while following was off", held, m.vp.YOffset)
	}
	if m.scrollNote() == "" {
		t.Error("expected a notice that the view is not at the live edge")
	}
}

// G returns to following, and following pins to the bottom on new content.
func TestFollowResumes(t *testing.T) {
	m := seeded(t, 14)
	up, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = up.(Model)
	gm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = gm.(Model)
	if !m.follow || !m.vp.AtBottom() {
		t.Fatal("G should resume following at the bottom")
	}
	m.refresh()
	if !m.vp.AtBottom() {
		t.Error("following did not pin to the bottom after new content")
	}
	if m.scrollNote() != "" {
		t.Error("no scroll notice expected while following")
	}
}

// The viewport must resize when the chrome collapses (empty turn draws none).
func TestViewportTracksChromeHeight(t *testing.T) {
	full := seeded(t, 30)
	withRows := full.vp.Height

	empty := NewModel(Options{Path: "testdata/session.jsonl"})
	empty.b = turn.New()
	em, _ := empty.Update(tea.WindowSizeMsg{Width: 92, Height: 30})
	empty = em.(Model)
	if empty.vp.Height <= withRows {
		t.Errorf("empty turn viewport %d should exceed %d: no column header or totals",
			empty.vp.Height, withRows)
	}
}

// In a pane with room for a single body line, following the live edge must
// land on the newest row, not on a blank line after it.
func TestShortPaneShowsTheNewestRow(t *testing.T) {
	m := seeded(t, 30)
	h := m.chromeHeight() + 1
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 92, Height: h})
	m = mm.(Model)
	if m.vp.Height != 1 {
		t.Fatalf("viewport height = %d, want 1", m.vp.Height)
	}
	if got := strings.TrimSpace(stripANSI(m.vp.View())); !strings.Contains(got, "answer") {
		t.Errorf("one-line pane shows %q, want the closing answer row", got)
	}
}

func TestHelpLineListsBindings(t *testing.T) {
	m := seeded(t, 20)
	v := stripANSI(m.View())
	for _, want := range []string{"up", "down", "follow", "quit"} {
		if !strings.Contains(v, want) {
			t.Errorf("help line missing %q:\n%s", want, v)
		}
	}
}
