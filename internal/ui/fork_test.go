package ui

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dontfeedthecode/cc-xray/internal/record"
	"github.com/dontfeedthecode/cc-xray/internal/turn"
)

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

// A fork transcript contains no user prompt: the skill body arrives as an
// isMeta record and every later user record only carries tool results. Built
// with turn.New() it never opened a turn, so Nested stayed nil and the panel
// sat on "waiting for the subagent transcript" long after the subagent had
// returned. turn.NewFork() opens the turn up front.
func TestForkTranscriptBuildsWithoutPrompt(t *testing.T) {
	plain := turn.New()
	if err := feed(plain, "testdata/fork-agent.jsonl"); err != nil {
		t.Fatal(err)
	}
	if plain.Turn() != nil {
		t.Fatal("fixture has a user prompt; it no longer covers the fork case")
	}

	nb := turn.NewFork()
	if err := feed(nb, "testdata/fork-agent.jsonl"); err != nil {
		t.Fatal(err)
	}
	nt := nb.Turn()
	if nt == nil {
		t.Fatal("NewFork produced no turn")
	}
	if len(nt.Rows) == 0 || nt.Requests == 0 {
		t.Fatalf("empty fork turn: rows=%d requests=%d", len(nt.Rows), nt.Requests)
	}
	if !nt.Complete {
		t.Error("fork ended on end_turn but was not marked complete")
	}
	if nt.OutTokens == 0 {
		t.Error("fork turn reported no output tokens")
	}
	if nt.Start.IsZero() {
		t.Error("fork turn has no start time, so its duration cannot be right")
	}

	f := &turn.Fork{AgentID: "a3031baaebce59c81", Skill: "lighthouse-audit", Nested: nt}
	tn := &turn.Turn{Prompt: "run the audit", Rows: []turn.Row{{Fork: f}}}
	out := stripANSI(Render(tn, NewTheme(), UnicodeGlyphs(), Opts{Width: 100, Rows: 40}))
	if strings.Contains(out, "waiting for the subagent transcript") {
		t.Error("completed fork still renders as waiting for its transcript")
	}
	if strings.Contains(out, "starting…") {
		t.Error("completed fork still renders as starting")
	}
	if !strings.Contains(out, "returned") {
		t.Errorf("completed fork does not report returning:\n%s", out)
	}
}

func forkWithRows(id string, models [][2]string) *turn.Fork {
	n := &turn.Turn{Requests: len(models), Complete: true}
	for _, m := range models {
		n.Rows = append(n.Rows, turn.Row{Action: &turn.Action{
			Model: m[0], Effort: m[1], Desc: "do a thing", Tool: "Bash",
		}})
	}
	return &turn.Fork{AgentID: id, Skill: "lighthouse-audit", Nested: n}
}

// A fork runs every row on one model, so repeating it down the block is
// noise; the column is kept only when a row actually diverges.
func TestForkModelColumnOnlyWhenItVaries(t *testing.T) {
	same := forkWithRows("aaaaaaaa", [][2]string{
		{"sonnet-5", "medium"}, {"sonnet-5", "medium"}, {"sonnet-5", "medium"}})
	out := stripANSI(Render(&turn.Turn{Prompt: "p", Rows: []turn.Row{{Fork: same}}},
		NewTheme(), UnicodeGlyphs(), Opts{Width: 100, Rows: 40}))
	if n := strings.Count(out, "sonnet-5"); n != 1 {
		t.Errorf("uniform fork names its model %d times, want 1 (the header):\n%s", n, out)
	}

	mixed := forkWithRows("bbbbbbbb", [][2]string{
		{"sonnet-5", "medium"}, {"opus-5", "high"}, {"sonnet-5", "medium"}})
	out = stripANSI(Render(&turn.Turn{Prompt: "p", Rows: []turn.Row{{Fork: mixed}}},
		NewTheme(), UnicodeGlyphs(), Opts{Width: 100, Rows: 40}))
	if !strings.Contains(out, "opus-5") {
		t.Errorf("a diverging row must still name its model:\n%s", out)
	}
	if n := strings.Count(out, "sonnet-5"); n < 3 {
		t.Errorf("mixed fork keeps the column for every row, got %d mentions:\n%s", n, out)
	}
}

func TestForkTimelineBar(t *testing.T) {
	g := UnicodeGlyphs()
	const cells = 12

	t.Run("lone fork gets no bar", func(t *testing.T) {
		if b := forkBar(&turn.Fork{Group: 1, From: 0, To: 1}, g, cells); b != "" {
			t.Errorf("got %q, want empty", b)
		}
	})
	t.Run("parallel forks both fill", func(t *testing.T) {
		a := forkBar(&turn.Fork{Group: 2, From: 0, To: 1}, g, cells)
		b := forkBar(&turn.Fork{Group: 2, From: 0.02, To: 1}, g, cells)
		if a != b {
			t.Errorf("near-identical spans drew differently:\n%s\n%s", a, b)
		}
		if strings.Contains(a, g.TlOff) {
			t.Errorf("a fork spanning the group should be solid, got %s", a)
		}
	})
	t.Run("sequential forks do not overlap", func(t *testing.T) {
		first := forkBar(&turn.Fork{Group: 2, From: 0, To: 0.5}, g, cells)
		second := forkBar(&turn.Fork{Group: 2, From: 0.5, To: 1}, g, cells)
		if first == second {
			t.Fatal("back-to-back forks drew the same bar")
		}
		if !strings.HasPrefix(first, g.TlOn) || !strings.HasSuffix(first, g.TlOff) {
			t.Errorf("first bar %s should start filled and end idle", first)
		}
		if !strings.HasPrefix(second, g.TlOff) || !strings.HasSuffix(second, g.TlOn) {
			t.Errorf("second bar %s should start idle and end filled", second)
		}
	})
	t.Run("a very short run still shows", func(t *testing.T) {
		b := forkBar(&turn.Fork{Group: 3, From: 0.5, To: 0.5001}, g, cells)
		if !strings.Contains(b, g.TlOn) {
			t.Errorf("sub-cell run vanished: %s", b)
		}
	})
	t.Run("bar is exactly the cells asked for", func(t *testing.T) {
		for _, n := range []int{2, 12, 40} {
			b := forkBar(&turn.Fork{Group: 2, From: 0.25, To: 0.75}, ASCIIGlyphs(), n)
			if visWidth(b) != n {
				t.Errorf("asked for %d cells, drew %d", n, visWidth(b))
			}
		}
	})
}

// The indented block and the lanes are both extras on an already full line.
func TestForkBarNeverOverrunsWidth(t *testing.T) {
	a := forkWithRows("cccccccc", [][2]string{{"sonnet-5", "medium"}})
	a.Group, a.From, a.To = 2, 0, 0.5
	bb := forkWithRows("dddddddd", [][2]string{{"sonnet-5", "medium"}})
	bb.Group, bb.From, bb.To = 2, 0.5, 1
	rows := []turn.Row{
		{Action: &turn.Action{Model: "opus-5", Effort: "high", Tool: "Skill",
			Desc: "lighthouse-audit  http://localhost:10018/insights", ToolID: "c1"}},
		{Fork: a},
		{Fork: bb},
	}
	for _, w := range []int{44, 56, 64, 72, 92, 120, 200} {
		out := Render(&turn.Turn{Prompt: "p", Rows: rows},
			NewTheme(), UnicodeGlyphs(), Opts{Width: w, Rows: 40})
		for i, ln := range splitLines(out) {
			if n := visWidth(ln); n > w {
				t.Errorf("width %d: line %d is %d cells: %q", w, i, n, stripANSI(ln))
			}
		}
	}
}

func timelineTurn() *turn.Turn {
	base := time.Date(2026, 9, 20, 3, 29, 54, 0, time.UTC)
	mk := func(id, parent string, from, to int) *turn.Fork {
		return &turn.Fork{AgentID: id, ParentID: parent, Skill: "lighthouse-audit",
			Nested: &turn.Turn{Requests: 5, Complete: true,
				Start: base.Add(time.Duration(from) * time.Second),
				End:   base.Add(time.Duration(to) * time.Second),
				Rows: []turn.Row{{Action: &turn.Action{
					Model: "sonnet-5", Effort: "medium", Tool: "Bash", Desc: "run audit"}}}}}
	}
	// the shape of the real run: two in parallel, a third queued behind them
	forks := []*turn.Fork{
		mk("a1512bb74a09", "c1", 0, 39),
		mk("a06658d908c9", "c2", 0, 37),
		mk("aab86ee94d72", "c3", 37, 78),
	}
	turn.MarkConcurrency(forks)
	var rows []turn.Row
	for i, f := range forks {
		rows = append(rows,
			turn.Row{Action: &turn.Action{Model: "opus-5", Effort: "high", Tool: "Skill",
				Desc: "lighthouse-audit  url-" + string(rune('A'+i)), ToolID: f.ParentID}},
			turn.Row{Fork: f})
	}
	return &turn.Turn{Prompt: "audit three urls", Rows: rows}
}

// The lanes exist to separate a fan-out that really ran in parallel from one
// that queued: two full-width bars against one that starts where they end.
func TestTimelineLanesShowTheQueuedFork(t *testing.T) {
	out := stripANSI(Render(timelineTurn(), NewTheme(), UnicodeGlyphs(),
		Opts{Width: 100, Rows: 60}))
	g := UnicodeGlyphs()

	if !strings.Contains(out, "FORKS") {
		t.Fatalf("no timeline drawn:\n%s", out)
	}
	// Peak simultaneity: the third fork starts as the second ends, while the
	// first is still running, so two ran at once and never three.
	if !strings.Contains(out, "3 forks") || !strings.Contains(out, "2 at once") {
		t.Errorf("timeline must report peak concurrency, not any overlap:\n%s", out)
	}

	var lanes []string
	for _, ln := range splitLines(stripANSI(out)) {
		if strings.Contains(ln, g.TlOn) || strings.Contains(ln, g.TlOff) {
			lanes = append(lanes, strings.TrimRight(ln, " "))
		}
	}
	if len(lanes) != 3 {
		t.Fatalf("want 3 lanes, got %d:\n%s", len(lanes), strings.Join(lanes, "\n"))
	}
	// the queued fork's lane must start idle where the others start filled
	firstRun := func(s string) int { return strings.Index(s, g.TlOn) }
	if firstRun(lanes[2]) <= firstRun(lanes[0]) {
		t.Errorf("queued fork should start later than the parallel pair:\n%s",
			strings.Join(lanes, "\n"))
	}
	if firstRun(lanes[0]) != firstRun(lanes[1]) {
		t.Errorf("the parallel pair should start together:\n%s",
			strings.Join(lanes, "\n"))
	}
}

// A fork block hangs off the Skill row above it, so the hook and the indent
// are what say which call spawned it.
func TestForkBlockIsHookedAndIndented(t *testing.T) {
	out := stripANSI(Render(timelineTurn(), NewTheme(), UnicodeGlyphs(),
		Opts{Width: 100, Rows: 60}))
	g := UnicodeGlyphs()
	for _, ln := range splitLines(out) {
		if !strings.Contains(ln, "forked →") {
			continue
		}
		if !strings.Contains(ln, g.Hook) {
			t.Errorf("fork header has no hook back to its Skill row: %q", ln)
		}
		// display cells, not bytes: the hook glyph is multi-byte
		if i := visWidth(ln[:strings.Index(ln, "╭")]); i != forkIndent {
			t.Errorf("fork block corner at column %d, want %d: %q", i, forkIndent, ln)
		}
	}
}

// One fork has nothing to sit against, so the lanes stay off.
func TestNoTimelineForASingleFork(t *testing.T) {
	tn := timelineTurn()
	tn.Rows = tn.Rows[:2] // one Skill row and its fork
	out := stripANSI(Render(tn, NewTheme(), UnicodeGlyphs(), Opts{Width: 100, Rows: 60}))
	if strings.Contains(out, "FORKS") {
		t.Errorf("a lone fork should draw no timeline:\n%s", out)
	}
}

// A slash-launched fork has no Skill row above it — it answers the prompt
// itself — so the hook would point at nothing and the block sits top level.
func TestParentlessForkDropsTheHook(t *testing.T) {
	g := UnicodeGlyphs()
	f := forkWithRows("a282e7de", [][2]string{{"sonnet-5", "low"}})
	f.ParentID = "" // launched by /lighthouse-audit, not by a Skill call
	out := stripANSI(Render(&turn.Turn{Prompt: "/lighthouse-audit",
		Rows: []turn.Row{{Fork: f}}}, NewTheme(), g, Opts{Width: 96, Rows: 40}))

	for _, ln := range splitLines(out) {
		if !strings.Contains(ln, "forked →") {
			continue
		}
		if strings.Contains(ln, g.Hook) {
			t.Errorf("parentless fork drew a hook to nothing: %q", ln)
		}
		if i := visWidth(ln[:strings.Index(ln, "╭")]); i != colGutter {
			t.Errorf("block corner at column %d, want %d: %q", i, colGutter, ln)
		}
	}
	if !strings.Contains(out, "sonnet-5 (low)") {
		t.Errorf("fork header lost its model/effort:\n%s", out)
	}
}
