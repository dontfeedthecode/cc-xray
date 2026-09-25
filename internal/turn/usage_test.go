package turn

import (
	"math"
	"strings"
	"testing"
	"time"
)

// A session that was left once (so Claude Code wrote its own total) and then
// resumed. Every identifier here is invented; the usage shapes are real.
const costSequence = `
{"type":"user","uuid":"u1","timestamp":"2026-01-02T10:00:00.000Z","message":{"role":"user","content":"first"}}
{"type":"assistant","uuid":"a1","requestId":"r1","timestamp":"2026-01-02T10:00:02.000Z","message":{"model":"claude-opus-5-5","stop_reason":"end_turn","content":[{"type":"thinking","thinking":""}],"usage":{"input_tokens":2,"output_tokens":100,"cache_read_input_tokens":1000,"cache_creation_input_tokens":500,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":500}}}}
{"type":"assistant","uuid":"a2","requestId":"r1","timestamp":"2026-01-02T10:00:03.000Z","message":{"model":"claude-opus-5-5","stop_reason":"end_turn","content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":2,"output_tokens":100,"cache_read_input_tokens":1000,"cache_creation_input_tokens":500,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":500}}}}
{"type":"system","subtype":"turn_duration","uuid":"s1","timestamp":"2026-01-02T10:00:04.000Z","durationMs":4000}
{"type":"cost-state","totalCostUSD":0.5,"modelUsage":{"claude-opus-5-5[1m]":{"inputTokens":900,"outputTokens":150,"cacheReadInputTokens":1000,"cacheCreationInputTokens":500,"costUSD":0.499},"claude-haiku-4-5-20251001":{"inputTokens":900,"outputTokens":15,"costUSD":0.001}}}
`

const afterResume = `
{"type":"user","uuid":"u2","timestamp":"2026-01-02T11:00:00.000Z","message":{"role":"user","content":"second"}}
{"type":"assistant","uuid":"a3","requestId":"r2","timestamp":"2026-01-02T11:00:02.000Z","message":{"model":"claude-opus-5-5","stop_reason":"end_turn","content":[{"type":"text","text":"Again."}],"usage":{"input_tokens":0,"output_tokens":1000000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
`

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// A request's usage is restated on each of its records; counting it twice
// would double the turn's cost (rule 1).
func TestTurnUsageCountsARequestOnce(t *testing.T) {
	b := replay(t, strings.SplitN(costSequence, `{"type":"cost-state"`, 2)[0])
	tok := b.Turn().Usage.Tokens()
	if tok.Out != 100 || tok.CacheRead != 1000 || tok.Write1h != 500 {
		t.Fatalf("turn tokens = %+v, want one request's worth", tok)
	}
	// 2*4 + 100*20 + 1000*0.2 + 500*8, per million
	if c := b.Turn().Usage.Cost(); !near(c, 0.006208) {
		t.Errorf("turn cost = %.6f, want 0.006208", c)
	}
	if s := b.Session(); s.Exact || !near(s.Ledger.Cost(), 0.006208) {
		t.Errorf("session before any cost-state = %+v, want the same estimate", s)
	}
}

// Claude Code's own total replaces the estimate for everything it covers.
func TestCostStateIsExact(t *testing.T) {
	s := replay(t, costSequence).Session()
	if !s.Exact {
		t.Error("nothing ran after the cost-state, so the total is exact")
	}
	if !near(s.Ledger.Cost(), 0.5) {
		t.Errorf("session cost = %v, want cost-state's 0.5", s.Ledger.Cost())
	}
	if want := time.Date(2026, 1, 2, 10, 0, 4, 0, time.UTC); !s.Base.Equal(want) {
		t.Errorf("base = %v, want the last timestamp before it, %v", s.Base, want)
	}
}

// After a resume, new requests are priced on top of the carried total.
func TestResumedSessionAddsToTheTotal(t *testing.T) {
	s := replay(t, costSequence+afterResume).Session()
	if s.Exact {
		t.Error("a request after the cost-state makes the total an estimate")
	}
	// 0.5 carried + 1m output tokens at $20/M
	if !near(s.Ledger.Cost(), 20.5) {
		t.Errorf("session cost = %v, want 20.5", s.Ledger.Cost())
	}
}

// A fork's requests before the parent's cost-state are inside that total.
func TestUsageSinceSkipsWhatTheBaseCovers(t *testing.T) {
	b := replay(t, costSequence+afterResume)
	base := b.Session().Base
	if got := b.UsageSince(base).Tokens().Out; got != 1000000 {
		t.Errorf("usage since base = %d output tokens, want only the resumed request's", got)
	}
}
