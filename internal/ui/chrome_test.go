package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The header and footer are drawn outside the viewport, so the body must not
// contain them. Missing this produced visibly duplicated chrome.
func TestBodyEmitsNoChrome(t *testing.T) {
	o := Opts{Width: 92, Rows: 40}
	body := stripANSI(RenderBody(load(t), NewTheme(), UnicodeGlyphs(), o))
	for _, banned := range []string{"MODEL", "ACTION", "OUT", "req  ·", "turn complete"} {
		if strings.Contains(body, banned) {
			t.Errorf("body contains chrome %q — it will be drawn twice", banned)
		}
	}
	if !strings.Contains(body, "Run the Lighthouse desktop audit") {
		t.Error("body lost its rows")
	}
}

// End to end through the real View(), which is where duplication showed up.
func TestViewDrawsChromeExactlyOnce(t *testing.T) {
	m := NewModel(Options{Path: "testdata/session.jsonl"})
	m.poll()
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 92, Height: 40})
	v := stripANSI(mm.(Model).View())
	for _, once := range []string{"MODEL", "ACTION"} {
		if n := strings.Count(v, once); n != 1 {
			t.Errorf("%q appears %d times, want 1", once, n)
		}
	}
	if n := strings.Count(v, "turn complete"); n > 1 {
		t.Errorf("footer appears %d times", n)
	}
}

// Effort now lives inside the model column.
func TestEffortMergedIntoModelColumn(t *testing.T) {
	v := stripANSI(Render(load(t), NewTheme(), UnicodeGlyphs(), Opts{Width: 92, Rows: 40}))
	if strings.Contains(v, "EFF") {
		t.Error("EFF column should be gone")
	}
	if !strings.Contains(v, "opus-5 (high)") {
		t.Errorf("expected 'opus-5 (high)' in the model column:\n%s", v)
	}
	if got := modelCell("sonnet-5", "medium"); visWidth(got) > colModel {
		t.Errorf("%q is %d cells, wider than the column (%d)", got, visWidth(got), colModel)
	}
	if got := modelCell("opus-5", ""); got != "opus-5" {
		t.Errorf("unknown effort should degrade to the bare model, got %q", got)
	}
}

// Narration under each row is gone; the answer row keeps its text.
func TestNoNarrationLines(t *testing.T) {
	v := stripANSI(Render(load(t), NewTheme(), UnicodeGlyphs(), Opts{Width: 92, Rows: 40}))
	if strings.Contains(v, "› ") {
		t.Error("narration continuation lines should be gone")
	}
	if !strings.Contains(v, "answer") {
		t.Error("the closing answer row should remain")
	}
}

// Bash is the assumed action. Naming it on every row crowded out the
// description, which is the part that says what actually happened.
func TestDefaultToolIsNotNamed(t *testing.T) {
	o := Opts{Width: 92, Rows: 60}
	body := stripANSI(RenderBody(load(t), NewTheme(), UnicodeGlyphs(), o))
	if strings.Contains(body, "Bash") {
		t.Error("Bash rows should carry the description alone")
	}
	// ...while anything else still announces itself.
	if !strings.Contains(body, "Skill  lighthouse-audit") {
		t.Error("a non-default tool must still be named")
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "Run the Lighthouse desktop audit") {
			if !strings.Contains(line, "  Run the Lighthouse") {
				t.Errorf("description not aligned into the action column: %q", line)
			}
			return
		}
	}
	t.Error("expected row not found")
}
