package turn

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dontfeedthecode/cc-xray/internal/record"
)

// build replays the fixture and returns the completed lighthouse turn.
func build(t *testing.T) *Turn {
	t.Helper()
	f, err := os.Open("testdata/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	b := New()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		var r record.Record
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		b.Add(r)
	}
	if b.Turn() == nil {
		t.Fatal("no turn built")
	}
	return b.Turn()
}

// TestFixtureTotals guards the four counting bugs found during design.
//
// testdata/session.jsonl is SYNTHETIC: the token arithmetic and block
// structure are taken from a real session, but every prompt, reply, path and
// identifier is fabricated. The numbers below are therefore the real ones
// while nothing private ships in this repository.
func TestFixtureTotals(t *testing.T) {
	tn := build(t)

	// Rule 1 — dedupe by requestId. Summing per record gives 13034.
	if got, want := tn.OutTokens, 6080; got != want {
		t.Errorf("output tokens = %d, want %d (per-record sum would be 13034)", got, want)
	}
	if got, want := tn.Requests, 15; got != want {
		t.Errorf("requests = %d, want %d", got, want)
	}
	if got, want := tn.CacheWrite, 34738; got != want {
		t.Errorf("cache write = %d, want %d", got, want)
	}
	// Rule 2 — context is a peak, not a sum (summing gives ~1.7M).
	if got, want := tn.PeakCtx, 63368; got != want {
		t.Errorf("peak ctx = %d, want %d", got, want)
	}
	// Rule 4 — authoritative turn duration.
	if got, want := tn.Duration, 107968*time.Millisecond; got != want {
		t.Errorf("duration = %v, want %v", got, want)
	}
	if !tn.Complete {
		t.Error("turn should be complete")
	}
}

func TestPromptAndState(t *testing.T) {
	tn := build(t)
	if tn.Prompt == "" {
		t.Fatal("prompt not captured")
	}
	if want := "Run a lighthouse audit"; len(tn.Prompt) < len(want) || tn.Prompt[:len(want)] != want {
		t.Errorf("prompt = %q", tn.Prompt)
	}
	if tn.Version != record.ValidatedVersion {
		t.Errorf("version = %q, want %q", tn.Version, record.ValidatedVersion)
	}
}

func TestActionRows(t *testing.T) {
	tn := build(t)
	var tools, answers, thinking int
	for _, r := range tn.Rows {
		a := r.Action
		if a == nil {
			continue
		}
		if a.Tool == "" {
			answers++ // a request with no tool call: the model answering
			if a.Say == "" {
				t.Error("answer row carries no narration")
			}
			continue
		}
		tools++
		if a.Thinking {
			thinking++
		}
		if a.Model != "opus-5" {
			t.Errorf("model = %q, want short form opus-5", a.Model)
		}
		if a.Effort != "high" {
			t.Errorf("effort = %q, want high", a.Effort)
		}
	}
	if want := 14; tools != want {
		t.Errorf("tool rows = %d, want %d", tools, want)
	}
	// 15 requests, 14 of which call a tool; the last is the concluding answer.
	if want := 1; answers != want {
		t.Errorf("answer rows = %d, want %d", answers, want)
	}
	// 10, not 8: thinking and tool_use sometimes share one record, which a
	// timestamp-matched hand count misses.
	if want := 10; thinking != want {
		t.Errorf("rows preceded by thinking = %d, want %d", thinking, want)
	}
}

// Narration must attach to the first call of its request, not every call.
func TestNarrationAttachesOnce(t *testing.T) {
	tn := build(t)
	var withSay int
	for _, r := range tn.Rows {
		if r.Action != nil && r.Action.Say != "" {
			withSay++
		}
	}
	if withSay == 0 {
		t.Fatal("no narration captured at all")
	}
	if withSay > tn.Requests {
		t.Errorf("%d narrated rows for %d requests — narration duplicated",
			withSay, tn.Requests)
	}
	t.Logf("%d of %d requests carried narration", withSay, tn.Requests)
}

// The skill boundary must appear, and must NOT claim the model changed —
// lighthouse-audit asked for claude-sonnet-5 and did not get it.
func TestSkillChangeIsNotApplied(t *testing.T) {
	tn := build(t)
	var changes int
	for _, r := range tn.Rows {
		if r.Change == nil {
			continue
		}
		changes++
		if r.Change.Applied {
			t.Errorf("change %q reported as applied, but model/effort never moved", r.Change.Label)
		}
	}
	if changes == 0 {
		t.Error("expected at least one skill boundary")
	}
}

// Rule 5 — synthetic records must never open a segment or add tokens.
func TestSyntheticSkipped(t *testing.T) {
	b := New()
	b.Add(record.Record{Type: "user", Message: record.Message{Content: json.RawMessage(`"do a thing"`)}})
	b.Add(record.Record{
		Type: "assistant", UUID: "u1", RequestID: "r1",
		Message: record.Message{Model: record.SyntheticModel,
			Usage: record.Usage{OutputTokens: 9999}},
	})
	if tn := b.Turn(); tn.OutTokens != 0 || tn.Requests != 0 {
		t.Errorf("synthetic leaked: out=%d requests=%d", tn.OutTokens, tn.Requests)
	}
}

// Rule 1 in miniature — three records of one request, same token count.
func TestRepeatedTokensNotSummed(t *testing.T) {
	b := New()
	b.Add(record.Record{Type: "user", Message: record.Message{Content: json.RawMessage(`"go"`)}})
	for i, blk := range []string{"thinking", "text", "tool_use"} {
		b.Add(record.Record{
			Type: "assistant", UUID: string(rune('a' + i)), RequestID: "req-1",
			APIBlockIndex: i, Timestamp: "2026-09-19T08:26:53.511Z",
			Effort: "high",
			Message: record.Message{Model: "claude-opus-5",
				Usage:   record.Usage{OutputTokens: 410, CacheReadTokens: 53600},
				Content: json.RawMessage(`[{"type":"` + blk + `","name":"Bash","input":{"description":"x"}}]`)},
		})
	}
	tn := b.Turn()
	if tn.OutTokens != 410 {
		t.Errorf("output = %d, want 410 (naive sum would be 1230)", tn.OutTokens)
	}
	if tn.PeakCtx != 53600 {
		t.Errorf("peak ctx = %d, want 53600", tn.PeakCtx)
	}
	if tn.Requests != 1 {
		t.Errorf("requests = %d, want 1", tn.Requests)
	}
}

// Rule 6 — repeated identical permissionMode snapshots are not changes.
func TestPermissionSnapshotsNotEvents(t *testing.T) {
	b := New()
	b.Add(record.Record{Type: "user", Message: record.Message{Content: json.RawMessage(`"go"`)}})
	for i := 0; i < 20; i++ {
		b.Add(record.Record{Type: "permission-mode", PermissionMode: "plan"})
	}
	b.Add(record.Record{Type: "permission-mode", PermissionMode: "auto"})
	for i := 0; i < 20; i++ {
		b.Add(record.Record{Type: "permission-mode", PermissionMode: "auto"})
	}
	var n int
	for _, r := range b.Turn().Rows {
		if r.Change != nil {
			n++
		}
	}
	if n != 1 {
		t.Errorf("permission changes = %d, want 1 (41 snapshots, one transition)", n)
	}
}

// Malformed input must be survivable, never fatal.
func TestMalformedLinesTolerated(t *testing.T) {
	b := New()
	b.Add(record.Record{Type: "user", Message: record.Message{Content: json.RawMessage(`"go"`)}})
	b.Add(record.Record{Type: "assistant", UUID: "x", Message: record.Message{
		Model: "claude-opus-5", Content: json.RawMessage(`{"not":"an array"}`)}})
	b.Add(record.Record{Type: "wat", UUID: "y"})
	if b.Turn() == nil {
		t.Fatal("builder collapsed on odd input")
	}
}

// Duration must track elapsed while running, not freeze at its first value.
// A fork transcript has no turn_duration record, so this is its only source.
func TestDurationTracksElapsed(t *testing.T) {
	b := New()
	b.Add(record.Record{Type: "user", Timestamp: "2026-09-19T11:41:17.000Z",
		Message: record.Message{Content: json.RawMessage(`"go"`)}})
	add := func(id, ts, stop string) {
		b.Add(record.Record{Type: "assistant", UUID: id, RequestID: id,
			Timestamp: ts, Effort: "high",
			Message: record.Message{Model: "claude-sonnet-5", StopReason: stop,
				Usage:   record.Usage{OutputTokens: 10},
				Content: json.RawMessage(`[{"type":"tool_use","name":"Bash","input":{"description":"x"}}]`)}})
	}
	add("r1", "2026-09-19T11:41:20.200Z", "tool_use")
	if got := b.Turn().Duration; got != 3200*time.Millisecond {
		t.Errorf("after first request: %v, want 3.2s", got)
	}
	add("r2", "2026-09-19T11:42:13.000Z", "end_turn")
	if got, want := b.Turn().Duration, 56*time.Second; got != want {
		t.Errorf("after completion: %v, want %v (must not freeze at 3.2s)", got, want)
	}
	if !b.Turn().Complete {
		t.Error("end_turn should complete a fork turn")
	}
}

// A turn_duration record is authoritative and must not be recomputed.
func TestTurnDurationWins(t *testing.T) {
	b := New()
	b.Add(record.Record{Type: "user", Timestamp: "2026-09-19T08:26:35.000Z",
		Message: record.Message{Content: json.RawMessage(`"go"`)}})
	b.Add(record.Record{Type: "assistant", UUID: "a", RequestID: "a",
		Timestamp: "2026-09-19T08:26:40.000Z", Effort: "high",
		Message: record.Message{Model: "claude-opus-5", StopReason: "tool_use",
			Usage: record.Usage{OutputTokens: 5}}})
	b.Add(record.Record{Type: "system", Subtype: "turn_duration",
		DurationMs: 107968, Timestamp: "2026-09-19T08:28:23.640Z"})
	if got, want := b.Turn().Duration, 107968*time.Millisecond; got != want {
		t.Errorf("duration = %v, want authoritative %v", got, want)
	}
}

// Forks launched in one assistant message run in parallel; forks in separate
// messages do not. The records look identical either way, so overlapping run
// spans are the only evidence and MarkConcurrency is what reads them.
func TestMarkConcurrency(t *testing.T) {
	at := func(s int) time.Time {
		return time.Date(2026, 9, 20, 0, 0, s, 0, time.UTC)
	}
	mk := func(id string, from, to int) *Fork {
		return &Fork{AgentID: id, Nested: &Turn{Start: at(from), End: at(to)}}
	}

	t.Run("parallel", func(t *testing.T) {
		f := []*Fork{mk("a", 0, 40), mk("b", 1, 40)}
		MarkConcurrency(f)
		for _, x := range f {
			if x.Peers != 1 {
				t.Errorf("%s: peers=%d, want 1", x.AgentID, x.Peers)
			}
			if x.Group != 2 {
				t.Errorf("%s: group=%d, want 2", x.AgentID, x.Group)
			}
		}
	})

	t.Run("sequential", func(t *testing.T) {
		f := []*Fork{mk("a", 0, 10), mk("b", 10, 20)}
		MarkConcurrency(f)
		for _, x := range f {
			if x.Peers != 0 {
				t.Errorf("%s: peers=%d, want 0", x.AgentID, x.Peers)
			}
		}
		// back to back, so each fills exactly half the group's span
		if f[0].From != 0 || f[0].To != 0.5 {
			t.Errorf("first span = %v..%v, want 0..0.5", f[0].From, f[0].To)
		}
		if f[1].From != 0.5 || f[1].To != 1 {
			t.Errorf("second span = %v..%v, want 0.5..1", f[1].From, f[1].To)
		}
	})

	t.Run("partial overlap counts as concurrent", func(t *testing.T) {
		f := []*Fork{mk("a", 0, 10), mk("b", 9, 20), mk("c", 30, 40)}
		MarkConcurrency(f)
		want := []int{1, 1, 0}
		for i, x := range f {
			if x.Peers != want[i] {
				t.Errorf("%s: peers=%d, want %d", x.AgentID, x.Peers, want[i])
			}
		}
	})

	t.Run("no spans does not divide by zero", func(t *testing.T) {
		f := []*Fork{{AgentID: "a"}, {AgentID: "b"}}
		MarkConcurrency(f) // must not panic
		for _, x := range f {
			if x.Peers != 0 || x.To != 0 {
				t.Errorf("%s: peers=%d to=%v, want 0/0", x.AgentID, x.Peers, x.To)
			}
		}
	})
}

// A turn spans everything the agent and its subagents do, and ends only when
// the user speaks again. A background agent's completion notice arrives as a
// user record, and treating it as a prompt reset the panel mid-turn: the
// prompt was replaced and the fork rows already gathered were thrown away.
func TestTaskNotificationDoesNotEndTheTurn(t *testing.T) {
	b := New()
	add := func(raw string) {
		var r record.Record
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		b.Add(r)
	}

	add(`{"type":"user","uuid":"u1","timestamp":"2026-09-20T03:15:29Z",
	      "message":{"role":"user","content":"audit three urls concurrently"}}`)
	add(`{"type":"assistant","uuid":"a1","timestamp":"2026-09-20T03:15:30Z",
	      "message":{"role":"assistant","model":"claude-opus-5","usage":{"output_tokens":10},
	      "content":[{"type":"tool_use","id":"t1","name":"Skill","input":{"skill":"lighthouse-audit"}}]}}`)
	add(`{"type":"user","uuid":"u2","timestamp":"2026-09-20T03:15:31Z",
	      "toolUseResult":{"status":"forked","agentId":"agent1","commandName":"lighthouse-audit"}}`)

	if got := len(b.Forks()); got != 1 {
		t.Fatalf("fork not recorded: %d", got)
	}
	prompt := b.Turn().Prompt

	// the notification: a real user record, no isMeta, plain string content
	add(`{"type":"user","uuid":"u3","timestamp":"2026-09-20T03:17:17Z",
	      "message":{"role":"user","content":"<task-notification>\n<task-id>x</task-id>\n<status>completed</status>\n</task-notification>"}}`)

	if got := len(b.Forks()); got != 1 {
		t.Errorf("notification discarded the turn's forks: %d left", got)
	}
	if b.Turn().Prompt != prompt {
		t.Errorf("notification replaced the prompt: %q", b.Turn().Prompt)
	}

	// ...but the user's next real message does end it
	add(`{"type":"user","uuid":"u4","timestamp":"2026-09-20T03:19:11Z",
	      "message":{"role":"user","content":"one observation, ccxray restarted"}}`)
	if got := len(b.Forks()); got != 0 {
		t.Errorf("a real prompt must start a fresh turn, %d forks carried over", got)
	}
	if b.Turn().Prompt == prompt {
		t.Error("a real prompt must replace the pinned prompt")
	}
}

// Three skills launched in one message produce three adjacent calls, and each
// fork is announced only when its subagent finishes — up to a minute later.
// Placing forks by timestamp therefore stacked all three at the bottom and
// nothing said which belonged to which. The parent tool_use id is exact.
func TestForkSitsUnderItsOwnSkillCall(t *testing.T) {
	b := New()
	add := func(raw string) {
		var r record.Record
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		b.Add(r)
	}

	add(`{"type":"user","uuid":"u1","timestamp":"2026-09-20T03:29:50Z",
	      "message":{"role":"user","content":"audit three urls"}}`)
	// one assistant message, three Skill calls
	add(`{"type":"assistant","uuid":"a1","timestamp":"2026-09-20T03:29:54Z",
	      "message":{"role":"assistant","model":"claude-opus-5","usage":{"output_tokens":9},
	      "content":[{"type":"tool_use","id":"call-A","name":"Skill","input":{"skill":"lighthouse-audit","args":"/"}},
	                 {"type":"tool_use","id":"call-B","name":"Skill","input":{"skill":"lighthouse-audit","args":"/insights"}},
	                 {"type":"tool_use","id":"call-C","name":"Skill","input":{"skill":"lighthouse-audit","args":"/article"}}]}}`)

	// the forks return out of order and long after the calls
	fork := func(uuid, agent, parent, ts string) string {
		return `{"type":"user","uuid":"` + uuid + `","timestamp":"` + ts + `",
		  "toolUseResult":{"status":"forked","agentId":"` + agent + `","commandName":"lighthouse-audit"},
		  "message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + parent + `"}]}}`
	}
	add(fork("u2", "agentC", "call-C", "2026-09-20T03:31:12Z"))
	add(fork("u3", "agentA", "call-A", "2026-09-20T03:29:54Z"))
	add(fork("u4", "agentB", "call-B", "2026-09-20T03:30:32Z"))

	var seq []string
	for _, r := range b.Turn().Rows {
		switch {
		case r.Fork != nil:
			seq = append(seq, "fork:"+r.Fork.AgentID)
		case r.Action != nil && r.Action.ToolID != "":
			seq = append(seq, "call:"+r.Action.ToolID)
		}
	}
	want := []string{
		"call:call-A", "fork:agentA",
		"call:call-B", "fork:agentB",
		"call:call-C", "fork:agentC",
	}
	if len(seq) != len(want) {
		t.Fatalf("got %v, want %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("row %d = %s, want %s\nfull order: %v", i, seq[i], want[i], seq)
		}
	}
}

// A skill invoked as /name is a real turn: rule 8 rejected it along with the
// local UI echoes, so the whole run was invisible and the panel sat on
// "waiting for a turn". The effort change such a skill makes was invisible
// with it, which is why `effort:` looked like it never applied in-thread.
func TestSlashCommandOpensATurnOnlyWhenWorkFollows(t *testing.T) {
	cmd := func(uuid, name, ts string) string {
		return `{"type":"user","uuid":"` + uuid + `","timestamp":"` + ts + `",
		  "message":{"role":"user","content":"<command-message>` + name[1:] +
			`</command-message>\n<command-name>` + name + `</command-name>"}}`
	}
	work := func(uuid, rid, effort, skill, ts string) string {
		return `{"type":"assistant","uuid":"` + uuid + `","requestId":"` + rid + `",
		  "timestamp":"` + ts + `","effort":"` + effort + `","attributionSkill":"` + skill + `",
		  "message":{"role":"assistant","model":"claude-opus-5","usage":{"output_tokens":10},
		  "content":[{"type":"tool_use","id":"t-` + rid + `","name":"Bash","input":{"description":"do it"}}]}}`
	}
	build := func(raws ...string) *Builder {
		b := New()
		for _, raw := range raws {
			var r record.Record
			if err := json.Unmarshal([]byte(raw), &r); err != nil {
				t.Fatal(err)
			}
			b.Add(r)
		}
		return b
	}

	t.Run("a UI command that starts nothing opens no turn", func(t *testing.T) {
		b := build(cmd("u1", "/clear", "2026-09-19T14:39:00Z"))
		if b.Turn() != nil {
			t.Errorf("/clear opened a turn: %+v", b.Turn())
		}
	})

	t.Run("a skill command with work behind it opens a turn", func(t *testing.T) {
		b := build(
			cmd("u1", "/ccxray-demo", "2026-09-19T14:39:00Z"),
			work("a1", "r1", "high", "ccxray-demo", "2026-09-19T14:39:05Z"),
		)
		tn := b.Turn()
		if tn == nil {
			t.Fatal("skill command opened no turn")
		}
		if tn.Prompt != "/ccxray-demo" {
			t.Errorf("prompt = %q, want the command", tn.Prompt)
		}
		if len(tn.Rows) == 0 {
			t.Error("turn has no rows")
		}
	})

	t.Run("effort moving inside that turn is recorded", func(t *testing.T) {
		b := build(
			cmd("u1", "/ccxray-demo", "2026-09-19T14:39:00Z"),
			work("a1", "r1", "high", "ccxray-demo", "2026-09-19T14:39:05Z"),
			work("a2", "r2", "max", "ccxray-demo-effort", "2026-09-19T14:39:42Z"),
		)
		var change *Change
		for _, r := range b.Turn().Rows {
			if r.Change != nil && strings.Contains(r.Change.Detail, "effort") {
				change = r.Change
			}
		}
		if change == nil {
			t.Fatalf("no effort change row; rows=%+v", b.Turn().Rows)
		}
		if !change.Applied {
			t.Error("effort moved but the change was not marked applied")
		}
		if !strings.Contains(change.Detail, "high → max") {
			t.Errorf("detail = %q, want it to name both levels", change.Detail)
		}
	})

	t.Run("a typed prompt outranks a pending command", func(t *testing.T) {
		b := build(
			cmd("u1", "/config", "2026-09-19T14:39:00Z"),
			`{"type":"user","uuid":"u2","timestamp":"2026-09-19T14:39:10Z",
			  "message":{"role":"user","content":"actually do this instead"}}`,
			work("a1", "r1", "high", "", "2026-09-19T14:39:12Z"),
		)
		if got := b.Turn().Prompt; got != "actually do this instead" {
			t.Errorf("prompt = %q, want the typed one", got)
		}
	})
}

// A change landing exactly on a turn boundary — a skill invoked as /name that
// sets its own effort, or /effort typed between turns — used to draw nothing,
// because each turn started from a blank slate and so had nothing to differ
// from. The table just began at the new value with no sign it had moved.
func TestStateChangeAtATurnBoundaryIsDrawn(t *testing.T) {
	add := func(b *Builder, raw string) {
		var r record.Record
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		b.Add(r)
	}
	prompt := func(uuid, text, ts string) string {
		return `{"type":"user","uuid":"` + uuid + `","timestamp":"` + ts + `",
		  "message":{"role":"user","content":"` + text + `"}}`
	}
	work := func(uuid, rid, effort, skill, ts string) string {
		return `{"type":"assistant","uuid":"` + uuid + `","requestId":"` + rid + `",
		  "timestamp":"` + ts + `","effort":"` + effort + `","attributionSkill":"` + skill + `",
		  "message":{"role":"assistant","model":"claude-opus-5","usage":{"output_tokens":10},
		  "content":[{"type":"tool_use","id":"t-` + rid + `","name":"Bash","input":{"description":"step"}}]}}`
	}
	detail := func(b *Builder) string {
		for _, r := range b.Turn().Rows {
			if r.Change != nil {
				return r.Change.Label + " | " + r.Change.Detail
			}
		}
		return ""
	}

	b := New()
	// a turn at high, then a /name skill turn that drops to low
	add(b, prompt("u1", "do a thing", "2026-09-20T15:00:00Z"))
	add(b, work("a1", "r1", "high", "", "2026-09-20T15:00:02Z"))
	if d := detail(b); d != "" {
		t.Errorf("first turn of a session must start blank, got %q", d)
	}

	add(b, `{"type":"user","uuid":"u2","timestamp":"2026-09-20T15:28:00Z",
	  "message":{"role":"user","content":"<command-name>/lighthouse-audit</command-name>"}}`)
	add(b, work("a2", "r2", "low", "lighthouse-audit", "2026-09-20T15:28:10Z"))

	d := detail(b)
	if d == "" {
		t.Fatalf("no change drawn at the boundary; rows=%+v", b.Turn().Rows)
	}
	if !strings.Contains(d, "high → low") {
		t.Errorf("detail %q should name both effort levels", d)
	}
	if !strings.Contains(d, "SKILL") {
		t.Errorf("label %q should mark the skill being entered", d)
	}

	// ...and leaving the skill turn reports the effort going back up
	add(b, prompt("u3", "now something else", "2026-09-20T15:30:00Z"))
	add(b, work("a3", "r3", "high", "", "2026-09-20T15:30:02Z"))
	if d := detail(b); !strings.Contains(d, "low → high") {
		t.Errorf("leaving the skill should report the effort returning, got %q", d)
	}
}

// A skill belongs to the turn that invoked it. Carrying its name into the
// next turn's opening state made every turn after a skill begin with
// "SKILL ENDS", reporting at the top of one turn something that had happened
// at the close of the previous one. Model and effort are session state and
// must still carry.
func TestSkillDoesNotEndAtTheStartOfTheNextTurn(t *testing.T) {
	add := func(b *Builder, raw string) {
		var r record.Record
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		b.Add(r)
	}
	prompt := func(uuid, text, ts string) string {
		return `{"type":"user","uuid":"` + uuid + `","timestamp":"` + ts + `",
		  "message":{"role":"user","content":"` + text + `"}}`
	}
	work := func(uuid, rid, effort, skill, ts string) string {
		return `{"type":"assistant","uuid":"` + uuid + `","requestId":"` + rid + `",
		  "timestamp":"` + ts + `","effort":"` + effort + `","attributionSkill":"` + skill + `",
		  "message":{"role":"assistant","model":"claude-opus-5","usage":{"output_tokens":10},
		  "content":[{"type":"tool_use","id":"t-` + rid + `","name":"Bash","input":{"description":"step"}}]}}`
	}
	labels := func(b *Builder) []string {
		var out []string
		for _, r := range b.Turn().Rows {
			if r.Change != nil {
				out = append(out, r.Change.Label)
			}
		}
		return out
	}

	b := New()
	// a turn that runs a skill right through to its final request
	add(b, prompt("u1", "do a lighthouse audit", "2026-09-21T00:52:00Z"))
	add(b, work("a1", "r1", "medium", "", "2026-09-21T00:52:02Z"))
	add(b, work("a2", "r2", "medium", "lighthouse-audit", "2026-09-21T00:52:10Z"))
	add(b, work("a3", "r3", "medium", "lighthouse-audit", "2026-09-21T00:52:40Z"))
	if got := labels(b); len(got) != 1 || !strings.HasPrefix(got[0], "SKILL ") ||
		strings.HasPrefix(got[0], "SKILL ENDS") {
		t.Fatalf("skill turn should open the skill exactly once, got %v", got)
	}

	// the next turn must not report the skill ending
	add(b, prompt("u2", "something unrelated", "2026-09-21T00:55:00Z"))
	add(b, work("a4", "r4", "medium", "", "2026-09-21T00:55:02Z"))
	for _, l := range labels(b) {
		if strings.HasPrefix(l, "SKILL ENDS") {
			t.Errorf("next turn opened with %q; the skill ended with the previous turn", l)
		}
	}

	// ...but a genuine effort change across the same boundary still draws
	add(b, prompt("u3", "and another", "2026-09-21T00:58:00Z"))
	add(b, work("a5", "r5", "high", "", "2026-09-21T00:58:02Z"))
	var seen string
	for _, r := range b.Turn().Rows {
		if r.Change != nil {
			seen = r.Change.Detail
		}
	}
	if !strings.Contains(seen, "medium → high") {
		t.Errorf("effort still carries across turns, want medium → high, got %q", seen)
	}
}

// A /compact is typed as a bare user record — no wrapper, no isMeta — so it
// read as a prompt, opened a turn and got no requests, and the panel spun on
// it until the next real prompt.
func TestBareCompactOpensNoTurn(t *testing.T) {
	add := func(b *Builder, raw string) {
		var r record.Record
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		b.Add(r)
	}
	bare := func(uuid, text, ts string) string {
		return `{"type":"user","uuid":"` + uuid + `","timestamp":"` + ts + `",
		  "message":{"role":"user","content":"` + text + `"}}`
	}
	work := func(uuid, ts string) string {
		return `{"type":"assistant","uuid":"` + uuid + `","requestId":"r-` + uuid + `",
		  "timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-opus-5",
		  "usage":{"output_tokens":5},"content":[{"type":"tool_use","id":"t1","name":"Bash",
		  "input":{"description":"do it"}}]}}`
	}

	t.Run("/compact alone opens nothing", func(t *testing.T) {
		b := New()
		add(b, bare("u1", "/compact", "2026-09-20T15:01:03Z"))
		if b.Turn() != nil {
			t.Errorf("/compact opened a turn: %+v", b.Turn())
		}
	})

	t.Run("the next prompt still owns the turn", func(t *testing.T) {
		b := New()
		add(b, bare("u1", "/compact", "2026-09-20T15:01:03Z"))
		add(b, bare("u2", "carry on then", "2026-09-20T15:02:00Z"))
		add(b, work("a1", "2026-09-20T15:02:01Z"))
		if got := b.Turn().Prompt; got != "carry on then" {
			t.Errorf("prompt = %q, want the typed one", got)
		}
	})

	t.Run("a bare skill command with work behind it does open one", func(t *testing.T) {
		b := New()
		add(b, bare("u1", "/lighthouse-audit http://localhost:10018/", "2026-09-20T15:01:03Z"))
		add(b, work("a1", "2026-09-20T15:01:05Z"))
		tn := b.Turn()
		if tn == nil {
			t.Fatal("bare skill command opened no turn")
		}
		if tn.Prompt != "/lighthouse-audit http://localhost:10018/" {
			t.Errorf("prompt = %q, want the whole line including its argument", tn.Prompt)
		}
	})

	t.Run("a prompt that merely starts with a path is untouched", func(t *testing.T) {
		b := New()
		add(b, bare("u1", "/Users/tim/ccxray is the one I mean", "2026-09-20T15:01:03Z"))
		if b.Turn() == nil {
			t.Fatal("a path-leading prompt was swallowed as a command")
		}
	})
}
