package ui

import (
	"strings"
	"testing"

	"github.com/dontfeedthecode/ccxray/internal/turn"
)

// An empty turn must not draw table chrome — that was the "weird empty table".
func TestEmptyTurnDrawsNoTable(t *testing.T) {
	tn := &turn.Turn{Prompt: "do the thing"}
	out := Render(tn, NewTheme(), UnicodeGlyphs(), Opts{Width: 76, Live: true, Rows: 20})
	for _, bad := range []string{"MODEL", "EFF", "ACTION", "OUT", "req", "ctx"} {
		if strings.Contains(stripANSI(out), bad) {
			t.Errorf("empty turn rendered table chrome %q:\n%s", bad, stripANSI(out))
		}
	}
	// A live turn with no actions yet must show it is working, not sit blank:
	// the model can think for 30s before its first tool call.
	plain := stripANSI(out)
	if !strings.Contains(plain, "thinking") {
		t.Errorf("expected a thinking line, got:\n%s", plain)
	}
	// and the spinner must advance between frames
	a := stripANSI(Render(tn, NewTheme(), UnicodeGlyphs(), Opts{Width: 76, Live: true, Rows: 20, Frame: 0}))
	bb := stripANSI(Render(tn, NewTheme(), UnicodeGlyphs(), Opts{Width: 76, Live: true, Rows: 20, Frame: 1}))
	if a == bb {
		t.Error("panel is identical across frames — nothing is animating")
	}
}

func TestNoTimeColumn(t *testing.T) {
	out := stripANSI(Render(load(t), NewTheme(), UnicodeGlyphs(), Opts{Width: 92, Live: false, Rows: 40}))
	if strings.Contains(out, "TIME") {
		t.Error("TIME column should be gone")
	}
	if !strings.Contains(out, "MODEL") || !strings.Contains(out, "ACTION") {
		t.Error("MODEL/ACTION headers missing")
	}
}

// Narrowing order: OUT drops before Δt, because on a live turn "what is slow"
// matters more than token counts.
func TestNarrowingDropsOutBeforeDt(t *testing.T) {
	cases := []struct {
		w               int
		wantOut, wantDt bool
	}{
		{92, true, true},
		{76, true, true},
		{56, false, true},
		{44, false, false},
	}
	for _, c := range cases {
		l := fit(c.w)
		if l.showOut != c.wantOut || l.dt != c.wantDt {
			t.Errorf("width %d: out=%v dt=%v, want out=%v dt=%v",
				c.w, l.showOut, l.dt, c.wantOut, c.wantDt)
		}
		if l.desc < 10 {
			t.Errorf("width %d: description collapsed to %d", c.w, l.desc)
		}
	}
}

// A cut description must show that it was cut.
func TestTruncationIsVisible(t *testing.T) {
	got := padDesc("Check lighthouse availability and URL reachability", 20)
	if !strings.HasSuffix(got, "\u2026") {
		t.Errorf("truncated text should end with an ellipsis, got %q", got)
	}
	if visWidth(got) != 20 {
		t.Errorf("width = %d, want 20", visWidth(got))
	}
	if kept := padDesc("short", 20); strings.Contains(kept, "\u2026") {
		t.Errorf("untruncated text should not gain an ellipsis: %q", kept)
	}
}

// Dropping TIME must give the description its 10 cells.
func TestDescriptionGainedWidth(t *testing.T) {
	// old layout reserved 2+10+9+7+6+2+6+2+6 = 50
	const oldDesc = 76 - 50
	if now := fit(76).desc; now <= oldDesc {
		t.Errorf("description width %d, expected more than the old %d", now, oldDesc)
	} else {
		t.Logf("at 76 cols: description %d -> %d cells", oldDesc, now)
	}
}

// Long tool names must abbreviate, not truncate mid-word.
func TestShortToolNames(t *testing.T) {
	cases := map[string]string{
		"Bash": "Bash", "Artifact": "Artifact",
		"AskUserQuestion": "Ask", "ExitPlanMode": "Plan",
		"NotebookEdit": "NbEdit", "ToolSearch": "Search",
		"mcp__claude-in-chrome__navigate": "navigate",
	}
	for in, want := range cases {
		if got := shortTool(in); got != want {
			t.Errorf("shortTool(%q) = %q, want %q", in, got, want)
		}
		if visWidth(shortTool(in)) > colTool {
			t.Errorf("shortTool(%q) exceeds the column", in)
		}
	}
}
