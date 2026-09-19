package ui

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dontfeedthecode/ccxray/internal/record"
	"github.com/dontfeedthecode/ccxray/internal/turn"
)

// feedUntilNextTurn stops at the user prompt following a fork, because forks
// belong to the turn that launched them and startTurn clears them.
func feedUntilNextTurn(b *turn.Builder, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		var r record.Record
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		if r.IsUserPrompt() && len(b.Forks()) > 0 {
			break
		}
		b.Add(r)
	}
	return nil
}

func feed(b *turn.Builder, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		var r record.Record
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			b.Add(r)
		}
	}
	return nil
}

// Fork rendering is exercised through the synthetic fixture; the live-session
// variant this replaced only ran on one machine.
func TestForkRenderShape(t *testing.T) {
	f := &turn.Fork{AgentID: "a1b2c3d4e5f6", Skill: "lighthouse-audit"}
	nb := turn.New()
	if err := feed(nb, "testdata/session.jsonl"); err != nil {
		t.Fatal(err)
	}
	f.Nested = nb.Turn()

	tn := &turn.Turn{Prompt: "run the audit"}
	tn.Rows = []turn.Row{{Fork: f}}
	for _, w := range []int{72, 92, 120} {
		out := Render(tn, NewTheme(), UnicodeGlyphs(), Opts{Width: w, Live: false, Rows: 40})
		plain := stripANSI(out)
		if !strings.Contains(plain, "forked") {
			t.Errorf("width %d: fork not marked", w)
		}
		if !strings.Contains(plain, f.Skill) {
			t.Errorf("width %d: skill name missing", w)
		}
		for i, ln := range splitLines(out) {
			if n := visWidth(ln); n > w {
				t.Errorf("width %d: line %d is %d cells", w, i, n)
			}
		}
	}
}
