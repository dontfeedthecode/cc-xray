package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dontfeedthecode/cc-xray/internal/record"
	"github.com/dontfeedthecode/cc-xray/internal/turn"
)

func load(t *testing.T) *turn.Turn {
	t.Helper()
	f, err := os.Open("testdata/session.jsonl")
	if err != nil {
		t.Skip("fixture missing")
	}
	defer f.Close()
	b := turn.New()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		var r record.Record
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			b.Add(r)
		}
	}
	return b.Turn()
}

// TestRenderDemo prints the real panel so it can be eyeballed in a terminal.
func TestRenderDemo(t *testing.T) {
	fmt.Print("\n" + Render(load(t), NewTheme(), UnicodeGlyphs(), Opts{Width: 92, Live: false, Rows: 40}))
}

// Every rendered line must fit the panel exactly; a shear here means a
// runewidth bug that would corrupt the grid in a real terminal.
func TestNoLineExceedsWidth(t *testing.T) {
	for _, w := range []int{44, 56, 64, 72, 76, 84, 92, 120} {
		out := Render(load(t), NewTheme(), UnicodeGlyphs(), Opts{Width: w, Live: true, Rows: 40})
		for i, ln := range splitLines(out) {
			if n := visWidth(ln); n > w {
				t.Errorf("width %d: line %d is %d cells:\n%q", w, i, n, ln)
			}
		}
	}
}

func splitLines(s string) []string {
	var out, cur = []string{}, ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// withFork returns the fixture turn with a fork block and a failed call
// spliced in. Neither appears in the fixture, so neither was width-checked.
func withFork(t *testing.T) *turn.Turn {
	t.Helper()
	tn := load(t)
	nested := &turn.Turn{Complete: true, Requests: 9, OutTokens: 3437,
		PeakCtx: 49000, Duration: 71 * time.Second,
		Rows: []turn.Row{
			{Action: &turn.Action{Model: "sonnet-5", Effort: "high", Tool: "Bash",
				Desc:     "Run the Lighthouse desktop audit against the staging origin",
				Thinking: true, Out: 410, Dt: 20300 * time.Millisecond}},
			{Action: &turn.Action{Model: "sonnet-5", Effort: "medium", Tool: "Read",
				Desc: "report.json", Out: 747, Dt: 8500 * time.Millisecond}},
		}}
	extra := []turn.Row{
		{Fork: &turn.Fork{AgentID: "a8aa6be79e2101574", Skill: "lighthouse-audit",
			Nested: nested}},
		{Fork: &turn.Fork{AgentID: "b1c2d3e4f5a6b7c8", Skill: "pending-skill"}},
		{Action: &turn.Action{Model: "opus-5", Effort: "high", Tool: "Bash",
			Desc:   "A command that failed and should be visibly marked",
			Failed: true, Out: 12, Dt: time.Second}},
	}
	tn.Rows = append(append([]turn.Row{}, tn.Rows...), extra...)
	return tn
}

func TestForkAndFailureRowsFitWidth(t *testing.T) {
	for _, w := range []int{44, 56, 64, 72, 76, 84, 92, 120} {
		out := Render(withFork(t), NewTheme(), UnicodeGlyphs(), Opts{Width: w, Rows: 60})
		for i, ln := range splitLines(out) {
			if n := visWidth(ln); n > w {
				t.Errorf("width %d: line %d is %d cells:\n%q", w, i, n, ln)
			}
		}
	}
}

// The rule that drops Bash applies inside a fork block too, or the nested
// rows read differently from the ones around them.
func TestForkRowsAlsoDropDefaultTool(t *testing.T) {
	out := stripANSI(Render(withFork(t), NewTheme(), UnicodeGlyphs(), Opts{Width: 92, Rows: 60}))
	for _, ln := range splitLines(out) {
		if strings.Contains(ln, "Run the Lighthouse desktop audit") && strings.Contains(ln, "Bash") {
			t.Errorf("nested row still names Bash: %q", ln)
		}
	}
	if !strings.Contains(out, "Read  report.json") {
		t.Error("nested row lost its non-default tool name")
	}
}
