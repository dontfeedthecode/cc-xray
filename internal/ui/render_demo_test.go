package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/dontfeedthecode/ccxray/internal/record"
	"github.com/dontfeedthecode/ccxray/internal/turn"
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
