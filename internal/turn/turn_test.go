package turn

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/dontfeedthecode/ccxray/internal/record"
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
