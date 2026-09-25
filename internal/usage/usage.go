// Package usage prices the token counts a transcript records.
//
// Claude Code writes its own running total (a cost-state record) only when a
// session is left — on exit, /clear or /resume — so a live panel has to price
// requests itself. The formula reproduces cost-state to the cent; what it
// cannot see is the handful of requests Claude Code makes without writing
// them to the transcript (title generation and similar side calls), so a
// live estimate runs a few percent under /usage.
package usage

import (
	"regexp"
	"sort"
	"strings"
)

// Tokens is one request's usage, or a sum of them.
type Tokens struct {
	In, Out, CacheRead int
	// Cache writes are priced by TTL. A cost-state baseline reports only the
	// total, which is carried in Write1h; its cost comes from cost-state, not
	// from these fields, so the split only matters for display.
	Write5m, Write1h int
}

func (t *Tokens) Add(o Tokens) {
	t.In += o.In
	t.Out += o.Out
	t.CacheRead += o.CacheRead
	t.Write5m += o.Write5m
	t.Write1h += o.Write1h
}

func (t Tokens) Write() int { return t.Write5m + t.Write1h }

// Input is every input token, cached or not.
func (t Tokens) Input() int { return t.In + t.CacheRead + t.Write() }

// CacheShare is the fraction of input served from cache, as /usage reports
// it: "95% of input tokens from cache". It is -1 when there was no input.
func (t Tokens) CacheShare() float64 {
	if n := t.Input(); n > 0 {
		return float64(t.CacheRead) / float64(n)
	}
	return -1
}

// Price is dollars per million tokens.
type Price struct{ In, Out, CacheRead, Write5m, Write1h float64 }

// prices are Anthropic first-party API rates. Cache reads are listed per
// model because the discount is not uniform: 0.05x on Opus 5.5 and 0.025x on
// Fable 5.1 against 0.1x elsewhere. Writes are 1.25x input for the 5-minute
// TTL and 2x for the 1-hour one. The Opus 5.5 row is checked against a real
// cost-state total in usage_test.go.
var prices = map[string]Price{
	"claude-fable-5-1":  {10, 50, 0.25, 12.5, 20},
	"claude-mythos-5-1": {10, 50, 0.25, 12.5, 20},
	"claude-fable-5":    {10, 50, 1, 12.5, 20},
	"claude-mythos-5":   {10, 50, 1, 12.5, 20},
	"claude-opus-5-5":   {4, 20, 0.20, 5, 8},
	"claude-opus-5":     {5, 25, 0.5, 6.25, 10},
	"claude-opus-4-8":   {5, 25, 0.5, 6.25, 10},
	"claude-opus-4-7":   {5, 25, 0.5, 6.25, 10},
	"claude-opus-4-6":   {5, 25, 0.5, 6.25, 10},
	"claude-sonnet-5":   {2, 10, 0.2, 2.5, 4},
	"claude-sonnet-4-6": {3, 15, 0.3, 3.75, 6},
	"claude-haiku-4-5":  {1, 5, 0.1, 1.25, 2},
}

// fastMultiplier prices a request that ran in fast mode. It is published as
// 2x for input and output on both Opus models that offer it; the cache rates
// are assumed to scale with them.
const fastMultiplier = 2

var dated = regexp.MustCompile(`-\d{8}$`)

// Canonical strips what varies between spellings of one model: the context
// suffix Claude Code appends ("[1m]") and a snapshot date.
func Canonical(model string) string {
	if i := strings.IndexByte(model, '['); i >= 0 {
		model = model[:i]
	}
	return dated.ReplaceAllString(model, "")
}

// Cost prices one request. It reports false for a model with no known rate,
// so the caller can say the total is incomplete rather than show a wrong one.
func Cost(model string, t Tokens, fast bool) (float64, bool) {
	p, ok := prices[Canonical(model)]
	if !ok {
		return 0, false
	}
	c := (float64(t.In)*p.In + float64(t.Out)*p.Out +
		float64(t.CacheRead)*p.CacheRead + float64(t.Write5m)*p.Write5m +
		float64(t.Write1h)*p.Write1h) / 1e6
	if fast {
		c *= fastMultiplier
	}
	return c, true
}

// Line is one model's share of a ledger.
type Line struct {
	Model  string // canonical id
	Tokens Tokens
	Cost   float64
}

// Ledger sums usage and cost per model.
type Ledger struct {
	lines map[string]*Line
	// Unpriced is set when some request used a model with no known rate, so
	// the cost shown is a floor.
	Unpriced bool
}

func (l *Ledger) line(model string) *Line {
	if l.lines == nil {
		l.lines = map[string]*Line{}
	}
	k := Canonical(model)
	ln := l.lines[k]
	if ln == nil {
		ln = &Line{Model: k}
		l.lines[k] = ln
	}
	return ln
}

// Add prices and records one request.
func (l *Ledger) Add(model string, t Tokens, fast bool) {
	c, ok := Cost(model, t, fast)
	if !ok {
		l.Unpriced = true
	}
	ln := l.line(model)
	ln.Tokens.Add(t)
	ln.Cost += c
}

// AddPriced records usage whose cost is already known, as a cost-state
// baseline is.
func (l *Ledger) AddPriced(model string, t Tokens, cost float64) {
	ln := l.line(model)
	ln.Tokens.Add(t)
	ln.Cost += cost
}

// Merge folds another ledger into this one.
func (l *Ledger) Merge(o Ledger) {
	for _, ln := range o.lines {
		dst := l.line(ln.Model)
		dst.Tokens.Add(ln.Tokens)
		dst.Cost += ln.Cost
	}
	l.Unpriced = l.Unpriced || o.Unpriced
}

func (l Ledger) Empty() bool { return len(l.lines) == 0 }

// Lines lists models by cost, most expensive first.
func (l Ledger) Lines() []Line {
	out := make([]Line, 0, len(l.lines))
	for _, ln := range l.lines {
		out = append(out, *ln)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Cost != out[j].Cost {
			return out[i].Cost > out[j].Cost
		}
		return out[i].Model < out[j].Model
	})
	return out
}

func (l Ledger) Cost() float64 {
	var c float64
	for _, ln := range l.lines {
		c += ln.Cost
	}
	return c
}

func (l Ledger) Tokens() Tokens {
	var t Tokens
	for _, ln := range l.lines {
		t.Add(ln.Tokens)
	}
	return t
}
