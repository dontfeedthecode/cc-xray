package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dontfeedthecode/cc-xray/internal/turn"
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

// Claude Code shows the model's narration before its tool call, so the panel
// must too, or its first move looks missed. It sits under the first call of
// its own request only, never repeated down a request's calls, and the old
// "› " continuation marker stays gone.
func TestNarrationUnderItsOwnCall(t *testing.T) {
	lines := splitLines(stripANSI(Render(load(t), NewTheme(), UnicodeGlyphs(), Opts{Width: 92, Rows: 40})))
	at := -1
	for i, ln := range lines {
		if strings.Contains(ln, "Verify the URL argument is provided") {
			at = i
		}
	}
	if at < 0 || at+1 >= len(lines) ||
		!strings.Contains(lines[at+1], "Starting with the required check command.") {
		t.Fatal("narration missing from under the call it led to")
	}
	if n := strings.Count(strings.Join(lines, "\n"), "Starting with the required check command."); n != 1 {
		t.Errorf("narration drawn %d times, want once", n)
	}
	if strings.Contains(strings.Join(lines, "\n"), "› ") {
		t.Error("the old narration continuation marker is back")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "answer") {
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

// Claude Code on Windows runs shell commands through PowerShell as well as
// Bash, and a turn mixes the two, so neither is named.
func TestPowerShellIsNotNamed(t *testing.T) {
	tn := &turn.Turn{Prompt: "p", Rows: []turn.Row{
		{Action: &turn.Action{Model: "opus-5", Tool: "PowerShell", Desc: "List project slugs"}},
		{Action: &turn.Action{Model: "opus-5", Tool: "Bash", Desc: "Show working tree status"}},
		{Action: &turn.Action{Model: "opus-5", Tool: "Read", Desc: "main.go"}},
	}}
	body := stripANSI(RenderBody(tn, NewTheme(), UnicodeGlyphs(), Opts{Width: 92, Rows: 40}))
	if strings.Contains(body, "PowerShell") || strings.Contains(body, "Bash") {
		t.Errorf("a shell row was named:\n%s", body)
	}
	if !strings.Contains(body, "List project slugs") || !strings.Contains(body, "Read  main.go") {
		t.Errorf("rows lost their descriptions:\n%s", body)
	}
}

// A skill entered through the Skill tool draws one row, not the call followed
// by a band repeating its name.
func TestSkillCallAndBandAreOneRow(t *testing.T) {
	v := stripANSI(Render(load(t), NewTheme(), UnicodeGlyphs(), Opts{Width: 92, Rows: 40}))
	if strings.Contains(v, "SKILL  lighthouse-audit") {
		t.Errorf("the band still repeats the Skill call:\n%s", v)
	}
	if n := strings.Count(v, "▸ Skill  lighthouse-audit"); n != 1 {
		t.Errorf("skill entry drawn %d times, want once:\n%s", n, v)
	}
}

// A skill typed as /name has no call row to merge into, so its band stays.
func TestSlashSkillKeepsItsBand(t *testing.T) {
	tn := &turn.Turn{Prompt: "/lighthouse-audit", Rows: []turn.Row{
		{Change: &turn.Change{Label: "SKILL  lighthouse-audit", Model: "opus-5"}},
		{Action: &turn.Action{Model: "opus-5", Tool: "Bash", Desc: "step"}},
	}}
	v := stripANSI(Render(tn, NewTheme(), UnicodeGlyphs(), Opts{Width: 92, Rows: 40}))
	if !strings.Contains(v, "SKILL  lighthouse-audit") {
		t.Errorf("slash-invoked skill lost its band:\n%s", v)
	}
}
