// Package turn rebuilds a single Claude Code turn from transcript records.
//
// Token accounting here is deliberate. See the four rules noted inline; each
// corresponds to a counting bug that produced plausible-but-wrong numbers.
package turn

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dontfeedthecode/ccxray/internal/record"
)

type ToolCall struct {
	Say    string // the model's own words from the same request
	Name   string
	Desc   string
	At     time.Time
	ID     string
	Failed bool
}

// request groups the several records that make up one assistant message.
// Rule 1: output_tokens is repeated on every record of a request, so it is
// stored once here and never accumulated per record.
type request struct {
	ID       string
	First    time.Time
	Last     time.Time
	Model    string
	Effort   string
	Skill    string
	Out      int
	CacheW   int
	CacheR   int
	Thinking bool
	Say      string
	Tools    []ToolCall
	Stop     string
}

type Action struct {
	Time     time.Time
	Model    string
	Effort   string
	Tool     string
	ToolID   string // tool_use id, used to seat a fork under its own call
	Desc     string
	Thinking bool
	Failed   bool
	Pending  bool   // in flight: the model has started but not yet acted
	Say      string // set only on answer/pending rows, where it IS the content
	Out      int
	Dt       time.Duration
}

type Change struct {
	Time    time.Time
	Label   string
	Detail  string
	Model   string
	Effort  string
	Applied bool // did model/effort actually move
}

// Fork marks a skill that ran as a subagent (`context: fork`). Its own
// actions live in a separate transcript; Nested is attached by the caller
// once that transcript has been read.
type Fork struct {
	AgentID    string
	ParentID   string // tool_use id of the Skill call that launched it
	Skill      string
	Background bool
	At         time.Time
	Nested     *Turn   // nil until the fork transcript is loaded
	Peers      int     // other forks running at the same time; see MarkConcurrency
	Group      int     // forks launched in this turn, this one included
	From, To   float64 // where this run sat in the group's span, 0..1
}

// MarkConcurrency records how many other forks overlapped each one in time.
// Nothing in the transcript states that forks ran in parallel: several Skill
// calls in one assistant message do, several across separate messages do not,
// and both look identical as records. Overlapping run spans are the only
// evidence, so they are what this measures. Call it after the Nested
// transcripts are attached, since the spans come from them.
func MarkConcurrency(forks []*Fork) {
	type span struct{ start, end time.Time }
	spans := make([]span, len(forks))
	for i, f := range forks {
		s := f.At
		e := s
		if f.Nested != nil {
			if !f.Nested.Start.IsZero() {
				s = f.Nested.Start
			}
			if !f.Nested.End.IsZero() {
				e = f.Nested.End
			}
		}
		spans[i] = span{s, e}
	}
	for i := range forks {
		n := 0
		for j := range forks {
			// A fork still running has end == start and overlaps nothing;
			// that reads as "unknown", which beats guessing parallelism.
			if i != j && spans[i].start.Before(spans[j].end) &&
				spans[j].start.Before(spans[i].end) {
				n++
			}
		}
		forks[i].Peers = n
		forks[i].Group = len(forks)
	}

	// Position each run inside the group's whole span, so the bars the panel
	// draws share one time axis and can be compared down the column.
	var lo, hi time.Time
	for _, s := range spans {
		if s.start.IsZero() {
			continue
		}
		if lo.IsZero() || s.start.Before(lo) {
			lo = s.start
		}
		if hi.IsZero() || s.end.After(hi) {
			hi = s.end
		}
	}
	total := hi.Sub(lo)
	for i := range forks {
		forks[i].From, forks[i].To = 0, 0
		if total <= 0 || spans[i].start.IsZero() {
			continue
		}
		forks[i].From = float64(spans[i].start.Sub(lo)) / float64(total)
		forks[i].To = float64(spans[i].end.Sub(lo)) / float64(total)
	}
}

type Row struct {
	Action *Action
	Change *Change
	Fork   *Fork
}

type Turn struct {
	Prompt     string
	Title      string
	Start      time.Time
	End        time.Time
	Version    string
	Rows       []Row
	Requests   int
	OutTokens  int
	CacheWrite int
	PeakCtx    int
	Duration   time.Duration
	Complete   bool
	Malformed  int
}

// Builder consumes records in stream order and maintains the current turn.
type Builder struct {
	cur    *Turn
	reqs   []*request
	byID   map[string]*request
	seen   map[string]bool // uuid dedupe
	lastPM string
	// entryKey is the model/effort/skill in force when this turn opened, and
	// lastKey the state it ended on. A turn used to start from a blank slate,
	// so a change that lands exactly on the boundary — a skill invoked as
	// /name setting its own effort, or /effort between turns — drew nothing
	// and the table simply began at the new value with no sign it had moved.
	entryKey stateKey
	lastKey  stateKey
	started  bool
	fork     bool // fork transcript: no user prompt ever opens the turn
	// A slash command the user typed, held until we know whether it started
	// real work. /clear and /config start none and must open no turn; a skill
	// invoked as /name does, and used to be discarded with them.
	pendingCmd string
	forks      []*Fork
	forkSeen   map[string]bool
	failed     map[string]bool // tool_use id -> the call errored
	authDur    bool            // Duration came from a system/turn_duration record

	// A compact_boundary lands after the previous turn has closed and before
	// the next prompt arrives, so it is held here until there is a turn to
	// attach it to.
	pendingCompact *Change

	// marks are changes that come from outside the request stream, such as a
	// permission-mode switch or a compaction. rebuild replaces Rows wholesale,
	// so they are held here and spliced back in by timestamp on every pass.
	marks []*Change
}

type stateKey struct{ model, effort, skill string }

func New() *Builder {
	return &Builder{byID: map[string]*request{}, seen: map[string]bool{},
		forkSeen: map[string]bool{}, failed: map[string]bool{}}
}

// NewFork returns a Builder for a forked skill's own transcript. A fork has
// no user prompt to open a turn with: Claude Code injects the skill body as
// an isMeta record and every later user record only carries tool results, so
// IsUserPrompt is false for the whole file. Feeding one to New() left cur nil
// forever and the panel sat on "waiting for the subagent transcript" even
// after the subagent had returned. The turn is therefore opened up front and
// its Start taken from the first record seen.
func NewFork() *Builder {
	b := New()
	b.fork = true
	b.cur = &Turn{}
	b.started = true
	return b
}

func (b *Builder) Turn() *Turn { return b.cur }

// Forks lists the subagent skills launched in the current turn.
func (b *Builder) Forks() []*Fork { return b.forks }

// Add feeds one record. Records must arrive in transcript order.
func (b *Builder) Add(r record.Record) {
	if r.UUID != "" {
		if b.seen[r.UUID] {
			return
		}
		b.seen[r.UUID] = true
	}

	// A fork's turn is already open, but its clock only starts at the first
	// record actually carrying a timestamp.
	if b.fork && b.cur.Start.IsZero() {
		b.cur.Start = r.Time()
	}

	// A forked skill is announced on the user record carrying the tool result.
	// Recorded before the prompt check so it is never mistaken for a new turn.
	if id, skill, ok := r.Forked(); ok && b.cur != nil {
		if !b.forkSeen[id] {
			b.forkSeen[id] = true
			res, _ := r.Result()
			b.forks = append(b.forks, &Fork{
				AgentID: id, ParentID: r.ForkParent(), Skill: skill,
				Background: res.Background, At: b.lastStamp(),
			})
			b.rebuild()
		}
		return
	}

	switch {
	// Checked before IsUserPrompt: a command typed bare reaches us as an
	// ordinary user record and would otherwise be taken for a prompt.
	case !b.fork && r.SlashCommand() != "":
		b.pendingCmd = r.SlashCommand()
		return
	case r.IsUserPrompt():
		// A fork owns one turn for the life of its transcript; nothing in it
		// may reset that turn and discard the rows already gathered.
		if !b.fork {
			b.pendingCmd = "" // a typed prompt outranks a pending command
			b.startTurn(r)
		}
		return
	case r.Type == "last-prompt" && b.cur != nil && b.cur.Prompt == "":
		b.cur.Prompt = r.LastPrompt
		return
	case r.Type == "ai-title" && b.cur != nil:
		b.cur.Title = r.AITitle
		return
	case r.Type == "permission-mode":
		// Rule 6: a snapshot, not an event. Only a differing value is a change.
		if b.lastPM != "" && r.PermissionMode != b.lastPM && b.cur != nil {
			b.marks = append(b.marks, &Change{
				Time:  b.lastStamp(),
				Label: "MODE  " + b.lastPM + " → " + r.PermissionMode,
			})
			b.rebuild()
		}
		b.lastPM = r.PermissionMode
		return
	case r.Type == "system" && r.Subtype == "local_command":
		// A slash-launched fork is announced before any assistant record, so
		// it is itself the proof that the command started work: the turn is
		// opened here, or the fork would land on the previous one.
		id, skill, ok := r.ForkedLaunch()
		if !ok || b.fork {
			return
		}
		if b.pendingCmd != "" {
			b.startCommandTurn(b.pendingCmd, r.Time())
			b.pendingCmd = ""
		}
		if b.cur != nil && !b.forkSeen[id] {
			b.forkSeen[id] = true
			b.forks = append(b.forks, &Fork{
				AgentID: id, Skill: skill, Background: true, At: r.Time(),
			})
			b.rebuild()
		}
		return
	case r.Type == "system" && r.Subtype == "compact_boundary":
		if m := r.CompactMetadata; m != nil {
			trigger := m.Trigger
			if trigger == "" {
				trigger = "compact"
			}
			b.pendingCompact = &Change{
				Time: r.Time(),
				Label: "COMPACT  " + trigger + "  " + kilo(m.PreTokens) +
					" \u2192 " + kilo(m.PostTokens) + " ctx",
			}
		}
		return
	case r.Type == "system" && r.Subtype == "turn_duration":
		// Rule 4: authoritative turn end and total.
		if b.cur != nil {
			b.cur.Duration = time.Duration(r.DurationMs) * time.Millisecond
			b.authDur = true
			b.cur.End = r.Time()
			b.cur.Complete = true
			b.flush()
		}
		return
	case r.Type != "assistant":
		return
	}

	// A user record may carry tool_result blocks; mark the failures so the
	// row that caused them can be flagged.
	if r.Type == "user" && b.cur != nil {
		var marked bool
		for _, blk := range r.Message.Blocks() {
			if blk.Failed() && blk.ToolUseID != "" {
				b.failed[blk.ToolUseID] = true
				marked = true
			}
		}
		if marked {
			b.rebuild()
		}
		return
	}

	// Rule 5: synthetic messages never reach the aggregator.
	if r.Synthetic() {
		return
	}
	// Assistant work following a slash command is what proves the command
	// opened a turn, so the turn is started here rather than on the command
	// record itself.
	if b.pendingCmd != "" {
		b.startCommandTurn(b.pendingCmd, r.Time())
		b.pendingCmd = ""
	}
	if b.cur == nil {
		return
	}
	if r.Version != "" {
		b.cur.Version = r.Version
	}

	req := b.byID[r.RequestID]
	if req == nil {
		req = &request{
			ID:     r.RequestID,
			First:  r.Time(),
			Model:  r.Message.Model,
			Effort: r.EffortLevel(),
			Skill:  r.AttributionSkill,
			Out:    r.Message.Usage.OutputTokens,
			CacheW: r.Message.Usage.CacheCreationTokens,
			CacheR: r.Message.Usage.CacheReadTokens,
		}
		b.byID[r.RequestID] = req
		b.reqs = append(b.reqs, req)
	}
	req.Last = r.Time()
	req.Stop = r.Message.StopReason
	// Rule 1 again: take the maximum rather than adding, so a re-stated value
	// on a later block of the same request cannot inflate the total.
	if v := r.Message.Usage.OutputTokens; v > req.Out {
		req.Out = v
	}
	if v := r.Message.Usage.CacheCreationTokens; v > req.CacheW {
		req.CacheW = v
	}
	if v := r.Message.Usage.CacheReadTokens; v > req.CacheR {
		req.CacheR = v
	}

	for _, blk := range r.Message.Blocks() {
		switch blk.Type {
		case "thinking":
			req.Thinking = true
		case "text":
			// The model narrates before acting, so the text of a request
			// belongs to the tool call that follows it in the same request.
			if req.Say == "" {
				req.Say = firstLine(blk.Text)
			}
		case "tool_use":
			req.Tools = append(req.Tools, ToolCall{
				Name: blk.Name, Desc: describe(blk.Name, blk.Input),
				At: r.Time(), ID: blk.ID,
			})
		}
	}
	b.rebuild()
}

func (b *Builder) startTurn(r record.Record) {
	b.cur = &Turn{Prompt: strings.TrimSpace(r.Message.PromptText()), Start: r.Time()}
	b.marks = nil
	if b.pendingCompact != nil {
		// The compaction happened between turns, but it is this turn whose
		// context it reset, so it reads as the first thing that happened.
		b.marks = append(b.marks, b.pendingCompact)
		b.cur.Rows = append(b.cur.Rows, Row{Change: b.pendingCompact})
		b.pendingCompact = nil
	}
	b.reqs = nil
	b.byID = map[string]*request{}
	b.forks = nil
	b.forkSeen = map[string]bool{}
	b.authDur = false
	// Model and effort are session state and carry into the next turn; a
	// skill is scoped to the turn that invoked it. Carrying its name across
	// made every turn after a skill open with "SKILL ENDS", reporting in the
	// new turn something that had happened at the close of the previous one.
	b.entryKey = b.lastKey
	b.entryKey.skill = ""
	b.started = true
}

// startCommandTurn opens a turn for a slash command, pinning the command
// itself as the prompt: it is what the user typed.
func (b *Builder) startCommandTurn(name string, at time.Time) {
	b.cur = &Turn{Prompt: name, Start: at}
	b.marks = nil
	if b.pendingCompact != nil {
		b.marks = append(b.marks, b.pendingCompact)
		b.cur.Rows = append(b.cur.Rows, Row{Change: b.pendingCompact})
		b.pendingCompact = nil
	}
	b.reqs = nil
	b.byID = map[string]*request{}
	b.forks = nil
	b.forkSeen = map[string]bool{}
	b.authDur = false
	// Model and effort are session state and carry into the next turn; a
	// skill is scoped to the turn that invoked it. Carrying its name across
	// made every turn after a skill open with "SKILL ENDS", reporting in the
	// new turn something that had happened at the close of the previous one.
	b.entryKey = b.lastKey
	b.entryKey.skill = ""
	b.started = true
}

// kilo abbreviates a token count for a row label: 542963 -> 542k.
func kilo(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%dk", n/1000)
}

func (b *Builder) lastStamp() time.Time {
	if n := len(b.reqs); n > 0 {
		return b.reqs[n-1].Last
	}
	if b.cur != nil {
		return b.cur.Start
	}
	return time.Time{}
}

func (b *Builder) flush() { b.rebuild() }

// rebuild regenerates rows and totals from the accumulated requests.
func (b *Builder) rebuild() {
	if b.cur == nil {
		return
	}
	t := b.cur
	rows := make([]Row, 0, len(b.reqs)*2)
	out, cw, peak := 0, 0, 0
	// Comparing the first request against the state the turn opened in is what
	// makes a boundary change visible; only a turn with nothing before it
	// starts blank.
	prev := b.entryKey
	first := prev == stateKey{}

	for _, rq := range b.reqs {
		// Rule 1: once per request.
		out += rq.Out
		cw += rq.CacheW
		// Rule 2: context is a peak, never a sum.
		if rq.CacheR > peak {
			peak = rq.CacheR
		}

		key := stateKey{rq.Model, rq.Effort, rq.Skill}
		if !first && key != prev {
			rows = append(rows, Row{Change: changeFor(prev, key, rq)})
		}
		prev, first = key, false

		if len(rq.Tools) == 0 {
			// A request with no tool call is either the model answering, or a
			// request still in flight. Both must be visible: a long opening
			// think would otherwise leave the panel blank for 30s or more.
			rows = append(rows, Row{Action: &Action{
				Time: rq.First, Model: record.ShortModel(rq.Model),
				Effort: rq.Effort, Thinking: rq.Thinking,
				Pending: rq.Stop == "" || rq.Stop == "null",
				Say:     rq.Say, Out: rq.Out,
			}})
			continue
		}
		for _, tc := range rq.Tools {
			rows = append(rows, Row{Action: &Action{
				Time: tc.At, Model: record.ShortModel(rq.Model), Effort: rq.Effort,
				Tool: tc.Name, Desc: tc.Desc, Thinking: rq.Thinking,
				Failed: b.failed[tc.ID], Out: rq.Out, ToolID: tc.ID,
			}})
		}
	}

	// Rule 3 applied per row: Δt is the gap to the next action, so a segment's
	// span is always measured between real records, never across user idle.
	for i := range rows {
		a := rows[i].Action
		if a == nil {
			continue
		}
		for j := i + 1; j < len(rows); j++ {
			if n := rows[j].Action; n != nil {
				a.Dt = n.Time.Sub(a.Time)
				break
			}
		}
	}

	// splice out-of-band changes back in at the point they occurred
	for _, c := range b.marks {
		at := len(rows)
		for i, r := range rows {
			if r.Action != nil && r.Action.Time.After(c.Time) {
				at = i
				break
			}
		}
		rows = append(rows[:at:at], append([]Row{{Change: c}}, rows[at:]...)...)
	}

	// Seat each fork directly under the Skill call that launched it. Placing
	// by timestamp put them all at the bottom: a fork is announced only when
	// its subagent finishes, so three skills launched together produced three
	// adjacent calls followed by three adjacent blocks, and nothing on screen
	// said which belonged to which. The tool_use id is exact, so it wins;
	// the timestamp remains the fallback for a fork with no parent recorded.
	for _, f := range b.forks {
		at := -1
		if f.ParentID != "" {
			for i, r := range rows {
				if r.Action != nil && r.Action.ToolID == f.ParentID {
					at = i + 1 // immediately below its own call
					break
				}
			}
		}
		if at < 0 {
			at = len(rows)
			for i, r := range rows {
				if r.Action != nil && r.Action.Time.After(f.At) {
					at = i
					break
				}
			}
		}
		rows = append(rows[:at:at], append([]Row{{Fork: f}}, rows[at:]...)...)
	}

	b.lastKey = prev
	t.Rows, t.Requests, t.OutTokens, t.CacheWrite, t.PeakCtx = rows, len(b.reqs), out, cw, peak
	if len(b.reqs) > 0 {
		last := b.reqs[len(b.reqs)-1]
		if !t.Complete {
			t.End = last.Last
			// Fork transcripts carry no system/turn_duration record, so
			// end_turn on the final request is the only completion signal.
			if last.Stop == "end_turn" {
				t.Complete = true
			}
		}
		// Recompute every rebuild: while running this is elapsed-so-far, and
		// on the final rebuild it becomes the total. Only a turn_duration
		// record is authoritative enough to freeze.
		if !b.authDur && !t.Start.IsZero() {
			t.Duration = t.End.Sub(t.Start)
		}
	}
}

func changeFor(prev, next stateKey, rq *request) *Change {
	applied := prev.model != next.model || prev.effort != next.effort
	c := &Change{
		Time: rq.First, Model: record.ShortModel(next.model),
		Effort: next.effort, Applied: applied,
	}
	switch {
	case next.skill != "" && prev.skill == "":
		c.Label = "SKILL  " + next.skill
	case next.skill == "" && prev.skill != "":
		c.Label = "SKILL ENDS  " + prev.skill
	default:
		c.Label = "STATE"
	}
	if applied {
		var d []string
		if prev.model != next.model {
			d = append(d, "model: "+record.ShortModel(prev.model)+" → "+record.ShortModel(next.model))
		}
		if prev.effort != next.effort {
			d = append(d, "effort: "+prev.effort+" → "+next.effort)
		}
		c.Detail = strings.Join(d, "   ")
	}
	return c
}

// describe produces a human label for a tool call. Bash dominates most turns,
// but the tools that appear rarely are the ones worth naming precisely, so
// each gets its own extraction rather than a generic key sweep.
func describe(name string, raw json.RawMessage) string {
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	str := func(k string) string {
		v, _ := m[k].(string)
		return firstLine(v)
	}

	// MCP tools arrive as mcp__<server>__<tool>; the tail is the useful part.
	if strings.HasPrefix(name, "mcp__") {
		if p := strings.Split(name, "__"); len(p) >= 3 {
			name = p[len(p)-1]
		}
	}

	switch name {
	case "Bash":
		if d := str("description"); d != "" {
			return d
		}
		return str("command")
	case "Read", "Write", "Edit", "NotebookEdit":
		return base(str("file_path"))
	case "Glob", "Grep":
		if p := str("pattern"); p != "" {
			if d := str("path"); d != "" {
				return p + "  in " + base(d)
			}
			return p
		}
	case "Skill":
		if sk := str("skill"); sk != "" {
			if a := str("args"); a != "" {
				return sk + "  " + a
			}
			return sk
		}
	case "Agent", "Task":
		if d := str("description"); d != "" {
			return d
		}
		return str("subagent_type")
	case "AskUserQuestion":
		if qs, ok := m["questions"].([]any); ok && len(qs) > 0 {
			if q, ok := qs[0].(map[string]any); ok {
				if h, _ := q["header"].(string); h != "" {
					return fmt.Sprintf("%s (+%d)", h, len(qs)-1)
				}
				if t, _ := q["question"].(string); t != "" {
					return firstLine(t)
				}
			}
		}
		return "asked the user"
	case "Artifact":
		act := str("action")
		if act == "" {
			act = "publish"
		}
		for _, k := range []string{"title", "file_path", "url"} {
			if v := str(k); v != "" {
				return act + "  " + base(v)
			}
		}
		return act
	case "ExitPlanMode":
		return "submitted the plan for approval"
	case "ToolSearch":
		return str("query")
	case "TodoWrite":
		if t, ok := m["todos"].([]any); ok {
			return fmt.Sprintf("%d items", len(t))
		}
	case "WebFetch", "navigate":
		return str("url")
	case "WebSearch":
		return str("query")
	}

	// fall back to the first plausible label
	for _, k := range []string{"description", "query", "prompt", "pattern",
		"file_path", "url", "command", "skill", "name"} {
		if v := str(k); v != "" {
			return v
		}
	}
	return ""
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

func base(p string) string {
	if p == "" {
		return ""
	}
	if i := strings.LastIndexByte(p, '/'); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return p
}
