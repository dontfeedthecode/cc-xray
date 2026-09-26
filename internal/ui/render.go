package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dontfeedthecode/cc-xray/internal/record"
	"github.com/dontfeedthecode/cc-xray/internal/turn"
	"github.com/dontfeedthecode/cc-xray/internal/usage"
	"github.com/mattn/go-runewidth"
)

// Column layout. Effort lives inside the model column as "opus-5 (high)":
// it changes so rarely that a column of its own was mostly whitespace.
const (
	colGutter = 2  // thinking / failure marker
	colModel  = 18 // widest: "sonnet-5 (medium)"
	colTool   = 8  // widest after abbreviation: Artifact, NbEdit
	colOut    = 6
	colDt     = 6
	minWidth  = 44
)

// layout decides which optional columns survive at this width. OUT goes
// before Δt: watching a live turn, "what is slow" beats "how many tokens".
type layout struct {
	desc        int
	showOut, dt bool
}

func fit(width int) layout {
	fixed := colGutter + colModel + colTool + 1
	l := layout{showOut: true, dt: true}
	if d := width - fixed - (colOut + 2 + colDt); d >= 24 {
		l.desc = d
		return l
	}
	l.showOut = false
	if d := width - fixed - (2 + colDt); d >= 18 {
		l.desc = d
		return l
	}
	l.dt = false
	if l.desc = width - fixed; l.desc < 10 {
		l.desc = 10
	}
	return l
}

// modelCell renders "opus-5 (high)", degrading to just the model if the
// effort is unknown.
func modelCell(model, effort string) string {
	if effort == "" {
		return model
	}
	return model + " (" + effort + ")"
}

func padR(s string, n int) string {
	if w := runewidth.StringWidth(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return runewidth.Truncate(s, n, "")
}

func padL(s string, n int) string {
	if w := runewidth.StringWidth(s); w < n {
		return strings.Repeat(" ", n-w) + s
	}
	return runewidth.Truncate(s, n, "")
}

// padDesc pads or ellipsises, so a cut description is visibly cut.
func padDesc(s string, n int) string {
	if runewidth.StringWidth(s) > n {
		return runewidth.Truncate(s, n, "…")
	}
	return padR(s, n)
}

func comma(n int) string {
	s := fmt.Sprint(n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

func dur(d time.Duration) string {
	switch {
	case d <= 0:
		return "—"
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		r := d.Round(time.Second)
		return fmt.Sprintf("%dm %02ds", int(r/time.Minute), int(r/time.Second)%60)
	}
}

func kilo(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%dk", n/1000)
}

// Opts carries presentation state shared by the three render functions.
type Opts struct {
	Width, Rows, Frame int
	Live               bool
	Spin               string // pre-rendered spinner frame from bubbles
	// Session is the whole transcript's usage; nil draws no session figure.
	Session   *turn.Session
	ShowUsage bool // draw the per-model breakdown under the totals
}

func (o Opts) spinFrame(g Glyphs) string {
	if o.Spin != "" {
		return strings.TrimSpace(o.Spin)
	}
	return builtinSpin(g, o.Frame)
}

func (o Opts) w() int {
	if o.Width < minWidth {
		return minWidth
	}
	return o.Width
}

// RenderHeader draws the pinned prompt and, when there are rows to head, the
// column titles. It sits above the scrolling body and is never scrolled away.
func RenderHeader(t *turn.Turn, th Theme, g Glyphs, o Opts) string {
	width := o.w()
	var b strings.Builder
	prompt := ""
	if t != nil {
		prompt = t.Prompt
	}
	for _, ln := range wrap(prompt, width-2, 2) {
		b.WriteString(th.Live.Render(g.Bar) + " " + th.Text.Render(ln) + "\n")
	}
	b.WriteString("\n")
	if t == nil || len(t.Rows) == 0 {
		return b.String() // no column titles over an empty body
	}
	l := fit(width)
	head := "  " + padR("MODEL", colModel) + padR("ACTION", colTool+1+l.desc)
	if l.showOut {
		head += padL("OUT", colOut)
	}
	if l.dt {
		head += "  " + padL("Δt", colDt)
	}
	b.WriteString(th.Head.Render(head) + "\n")
	b.WriteString(th.Faint.Render("  "+strings.Repeat(g.Rule, width-4)) + "\n")
	return b.String()
}

// RenderBody draws only the scrollable rows. It emits no chrome of its own:
// the header and footer are rendered outside the viewport.
func RenderBody(t *turn.Turn, th Theme, g Glyphs, o Opts) string {
	width := o.w()
	var b strings.Builder

	if t == nil {
		return th.Dim.Render("  waiting for a turn…") + "\n"
	}
	if len(t.Rows) == 0 {
		if !o.Live {
			return th.Faint.Render("  ○ no actions in this turn") + "\n"
		}
		// The model can think for 30s before acting; keep the clock moving.
		el := t.Duration
		if el == 0 && !t.Start.IsZero() {
			el = time.Since(t.Start)
		}
		return "  " + th.Live.Render(o.spinFrame(g)) + " " +
			th.Dim.Render("thinking") + th.Faint.Render(ellipsis(o.Frame)+"   "+dur(el)) + "\n"
	}

	l := fit(width)
	rows := t.Rows
	if o.Rows > 0 && len(rows) > o.Rows {
		rows = rows[len(rows)-o.Rows:]
	}

	for i := 0; i < len(rows); i++ {
		r := rows[i]
		// A skill entered through the Skill tool is announced twice: by the
		// call, and by the band that marks the requests after it running
		// under the skill. The call row takes the band's place.
		if a := r.Action; a != nil && i+1 < len(rows) {
			if c := rows[i+1].Change; c != nil && entersSkill(a, c) {
				b.WriteString(renderSkillEntry(a, c, th, g, o, l) + "\n")
				b.WriteString(warning(a, th, width))
				if n := narration(a, th, width); n != "" {
					b.WriteString(n + "\n")
				}
				if c.Detail != "" {
					b.WriteString(th.Faint.Render("  "+
						strings.Repeat(" ", colModel)+c.Detail) + "\n")
				}
				i++
				continue
			}
		}
		switch {
		case r.Fork != nil:
			b.WriteString(renderFork(r.Fork, th, g, width, l))
		case r.Change != nil && r.Change.Kind == turn.ChangePrompt:
			// Sent while a background fork ran, so it joins this turn rather
			// than replacing it; drawn the way the pinned prompt is.
			b.WriteString("  " + th.Live.Render(g.Bar) + " " +
				th.Text.Render(padDesc(r.Change.Label, width-6)) + "\n")
		case r.Change != nil && r.Change.Kind == turn.ChangeReturn:
			c := r.Change
			line := "  " + g.Enter + " returned  " + c.Label + "  ·  " + c.Detail
			b.WriteString(th.Gold.Render(padR(line, width-2)) + "\n")
		case r.Change != nil:
			c := r.Change
			style := th.Band
			if strings.HasPrefix(c.Label, "SKILL ENDS") {
				style = th.BandExit
			}
			line := "  " + padR(modelCell(c.Model, c.Effort), colModel) +
				g.Enter + " " + c.Label
			b.WriteString(style.Render(padR(line, width-2)) + "\n")
			if c.Detail != "" {
				b.WriteString(th.Faint.Render("  "+
					strings.Repeat(" ", colModel)+c.Detail) + "\n")
			}
		case r.Action != nil:
			b.WriteString(renderAction(r.Action, th, g, o, l) + "\n")
			b.WriteString(warning(r.Action, th, width))
			if n := narration(r.Action, th, width); n != "" {
				b.WriteString(n + "\n")
			}
		}
	}
	// The lanes read every fork in the turn, not the visible slice, so
	// scrolling the table never changes what the timeline claims.
	b.WriteString(renderTimeline(t, th, g, width))
	return b.String()
}

// isDefaultTool reports a tool that is assumed and never named on a row.
// Nearly every action in a normal turn is a shell command, so printing "Bash"
// on line after line said nothing and cost the description eight columns.
// Claude Code on Windows has a PowerShell tool beside Bash, and a turn mixes
// the two freely, so both count. Anything else is announced.
func isDefaultTool(name string) bool { return name == "Bash" || name == "PowerShell" }

func renderAction(a *turn.Action, th Theme, g Glyphs, o Opts, l layout) string {
	gut, gutStyle := "  ", th.Faint
	if a.Thinking {
		gut = g.Think + " "
	}
	toolStyle, descStyle := th.Tool, th.Dim
	tool, desc := shortTool(a.Tool), a.Desc
	if isDefaultTool(a.Tool) {
		tool = ""
	}

	switch {
	case a.Failed:
		// With the tool name gone from most rows the gutter alone carried the
		// failure, which was too quiet: tint the description too.
		gut, gutStyle, toolStyle, descStyle = g.Fail+" ", th.Fail, th.Fail, th.Fail
	case a.Pending:
		tool, toolStyle = o.spinFrame(g), th.Live
		if desc = a.Say; desc == "" {
			desc = "thinking" + ellipsis(o.Frame)
		}
	case a.Tool == "":
		// a request that called no tool: the model's closing answer
		tool, desc, toolStyle, descStyle = "answer", a.Say, th.Dimmer, th.Dimmer
	}

	row := gutStyle.Render(gut) +
		th.Model.Render(padR(modelCell(a.Model, a.Effort), colModel)) +
		actionCell(tool, desc, toolStyle, descStyle, colTool+1+l.desc)
	if l.showOut {
		row += th.Dim.Render(padL(comma(a.Out), colOut))
	}
	if l.dt {
		row += th.Dimmer.Render("  " + padL(dur(a.Dt), colDt))
	}
	return row
}

// entersSkill reports whether c is the band for the skill that call a just
// launched.
func entersSkill(a *turn.Action, c *turn.Change) bool {
	if a.Tool != "Skill" || !strings.HasPrefix(c.Label, "SKILL  ") {
		return false
	}
	name := strings.TrimPrefix(c.Label, "SKILL  ")
	f := strings.Fields(a.Desc)
	return len(f) > 0 && f[0] == name
}

// renderSkillEntry draws a Skill call in the band's colours, keeping its own
// OUT and Δt, so one row says both that the skill was called and that what
// follows runs under it.
func renderSkillEntry(a *turn.Action, c *turn.Change, th Theme, g Glyphs, o Opts, l layout) string {
	gut := "  "
	if a.Thinking {
		gut = g.Think + " "
	}
	tool := g.Enter + " Skill"
	desc := a.Desc
	w := colTool + 1 + l.desc
	cell := padDesc(tool+"  "+desc, w)
	row := gut + padR(modelCell(a.Model, a.Effort), colModel) + cell
	if l.showOut {
		row += padL(comma(a.Out), colOut)
	}
	if l.dt {
		row += "  " + padL(dur(a.Dt), colDt)
	}
	return th.Band.Render(row)
}

// warning draws what a skill asked for and did not get, directly under its
// call and above the narration, since it explains the rows that follow.
func warning(a *turn.Action, th Theme, width int) string {
	if a.Warn == "" {
		return ""
	}
	pad := colGutter + colModel
	return th.Gold.Render(strings.Repeat(" ", pad)+padDesc("! "+a.Warn, width-pad-2)) + "\n"
}

// narration draws the words the model led a tool call with, on a faint line
// under the call and aligned with the ACTION column. Claude Code shows this
// text first, often seconds before the call, so leaving it out made the panel
// look as if it had skipped the model's first move. Answer and pending rows
// already show their words as the description and get no second line.
func narration(a *turn.Action, th Theme, width int) string {
	if a.Say == "" || a.Tool == "" || a.Pending {
		return ""
	}
	pad := colGutter + colModel
	return th.Dimmer.Render(strings.Repeat(" ", pad) + padDesc(a.Say, width-pad-2))
}

// actionCell fills the ACTION column with an optional tool name followed by
// the description, padded as one unit so the columns after it stay aligned
// whether or not a name was printed.
func actionCell(tool, desc string, toolStyle, descStyle lipgloss.Style, width int) string {
	if tool == "" {
		return descStyle.Render(padDesc(desc, width))
	}
	tw := runewidth.StringWidth(tool) + 2
	if tw >= width {
		return toolStyle.Render(padDesc(tool, width))
	}
	return toolStyle.Render(tool) + "  " + descStyle.Render(padDesc(desc, width-tw))
}

// RenderFooter draws the closing rule and the turn totals.
func RenderFooter(t *turn.Turn, th Theme, g Glyphs, o Opts, note string) string {
	if t == nil || len(t.Rows) == 0 {
		return ""
	}
	width := o.w()
	var b strings.Builder
	b.WriteString(th.Faint.Render("  "+strings.Repeat(g.Rule, width-4)) + "\n")
	line := footer(t, th, g, width, o)
	if note != "" {
		line = overlayRight(line, th.Gold.Render(note), width)
	}
	b.WriteString(line + "\n")
	if o.ShowUsage {
		b.WriteString(renderUsage(t, o.Session, th, width))
	}
	return b.String()
}

// Render draws the whole panel in one string, for tests and one-shot output.
func Render(t *turn.Turn, th Theme, g Glyphs, o Opts) string {
	return RenderHeader(t, th, g, o) + RenderBody(t, th, g, o) +
		RenderFooter(t, th, g, o, "")
}

func overlayRight(line, note string, width int) string {
	plain := stripANSI(line)
	keep := width - runewidth.StringWidth(stripANSI(note)) - 2
	if keep < 0 {
		return line
	}
	return runewidth.Truncate(plain, keep, "") + "  " + note
}

// renderFork draws a forked skill as an indented block between rules. The
// header names the model the fork actually ran on, which is the point:
// `context: fork` is the only way a skill's frontmatter model takes effect.
func renderFork(f *turn.Fork, th Theme, g Glyphs, width int, l layout) string {
	var b strings.Builder
	model, effort, state := "…", "", "starting"
	if f.Nested != nil && len(f.Nested.Rows) > 0 {
		for _, r := range f.Nested.Rows {
			if r.Action != nil {
				model, effort = r.Action.Model, r.Action.Effort
				break
			}
		}
		if state = "running"; f.Nested.Complete {
			state = "done"
		}
	}

	// A fork runs every row on the model its frontmatter named, so repeating
	// that model down the block is noise. The column is kept only when some
	// row actually diverges from the header, which is the one case where the
	// repetition carries information; dropping it per-row instead would leave
	// the block ragged.
	varies := false
	if f.Nested != nil {
		for _, r := range f.Nested.Rows {
			if a := r.Action; a != nil && (a.Model != model || a.Effort != effort) {
				varies = true
				break
			}
		}
	}

	// The block is indented under its own Skill row and joined to it by a
	// hook, so a reader never has to infer which call spawned which fork.
	// A fork a slash command launched has no such row — it answers the
	// prompt itself — so it sits at the top level and the hook, which would
	// point at nothing, is dropped.
	indent, lead := forkIndent, "  "+g.Hook+" "
	if f.ParentID == "" {
		indent, lead = colGutter, "  "
	}
	head := "╭─ forked → " + modelCell(model, effort) + "  ·  " + f.Skill +
		"  ·  agent " + short(f.AgentID) + " "
	// The hook carries the block's own colour: dimmed, it read as chrome and
	// the eye did not join the block to the call above it.
	b.WriteString(th.Gold.Render(lead+padR(head, width-4-indent)) + "\n")

	rail := strings.Repeat(" ", indent)
	if f.Nested == nil || len(f.Nested.Rows) == 0 {
		b.WriteString(th.Faint.Render(rail+"│ ") + th.Dim.Render(state+"…") + "\n")
	} else {
		for _, r := range f.Nested.Rows {
			a := r.Action
			if a == nil {
				continue
			}
			gut := " "
			if a.Thinking {
				gut = g.Think
			}
			tool := shortTool(a.Tool)
			if isDefaultTool(a.Tool) {
				tool = ""
			}
			// the indent and rail cost seven cells against a top-level row
			desc := colTool + 1 + l.desc - (indent + 3 - colGutter)
			row := th.Faint.Render(rail+"│"+gut) + " "
			if varies {
				row += th.Gold.Render(padR(modelCell(a.Model, a.Effort), colModel))
			} else {
				desc += colModel // the model column's width goes to the action
			}
			row += actionCell(tool, a.Desc, th.Tool, th.Dim, desc)
			if l.showOut {
				row += th.Dim.Render(padL(comma(a.Out), colOut))
			}
			if l.dt {
				row += th.Dimmer.Render("  " + padL(dur(a.Dt), colDt))
			}
			b.WriteString(row + "\n")
		}
	}

	tail := "╰─ "
	if f.Nested != nil && f.Nested.Requests > 0 {
		n := f.Nested
		verb := "running"
		if n.Complete {
			verb = "returned"
		}
		tail += fmt.Sprintf("%s  %d req  ·  %s out  ·  %s ctx  ·  %s ",
			verb, n.Requests, comma(n.OutTokens), kilo(n.PeakCtx), dur(n.Duration))
	} else {
		tail += "waiting for the subagent transcript "
	}

	b.WriteString(th.Faint.Render(rail+padR(tail, width-4-indent)) + "\n")
	return b.String()
}

// renderTimeline draws one lane per fork beneath the table, all on a single
// time axis. The per-block tail could only say how long a fork took, never
// how its run sat against the others, so a fan-out that quietly ran two at a
// time and queued the third looked identical to one that ran all three.
func renderTimeline(t *turn.Turn, th Theme, g Glyphs, width int) string {
	var forks []*turn.Fork
	desc := map[string]string{}
	for _, r := range t.Rows {
		switch {
		case r.Fork != nil:
			forks = append(forks, r.Fork)
		case r.Action != nil && r.Action.ToolID != "":
			desc[r.Action.ToolID] = r.Action.Desc
		}
	}
	// One fork has nothing to sit against, and a cramped panel needs its
	// width for the table.
	if len(forks) < 2 || width < 64 {
		return ""
	}

	var lo, hi time.Time
	done := 0
	for _, f := range forks {
		if f.Nested == nil || f.Nested.Start.IsZero() {
			continue
		}
		done++
		if lo.IsZero() || f.Nested.Start.Before(lo) {
			lo = f.Nested.Start
		}
		if hi.IsZero() || f.Nested.End.After(hi) {
			hi = f.Nested.End
		}
	}
	if done < 2 || !hi.After(lo) {
		return "" // nothing to scale against yet
	}
	span := hi.Sub(lo)

	// Peak simultaneity, not "how many overlapped something": with a cap of
	// two, a third fork starts the moment one finishes and so overlaps the
	// one still running, which made a plain overlap count report every fork
	// as concurrent and hid the queueing entirely.
	peak := 0
	for _, a := range forks {
		if a.Nested == nil || a.Nested.Start.IsZero() {
			continue
		}
		n := 0
		for _, c := range forks {
			if c.Nested == nil || c.Nested.Start.IsZero() {
				continue
			}
			if !c.Nested.Start.After(a.Nested.Start) && c.Nested.End.After(a.Nested.Start) {
				n++
			}
		}
		peak = max(peak, n)
	}

	bar := min(max(width/3, 14), 40)
	const idW, gap = 8, 2
	label := width - 4 - idW - gap - bar - gap
	if label < 10 {
		return ""
	}
	lead := 2 + idW + gap + label + gap // cells before a lane's bar starts

	var b strings.Builder
	b.WriteString(th.Faint.Render("  "+strings.Repeat(g.Rule, width-4)) + "\n")

	note := fmt.Sprintf("%d forks  ·  %d at once  ·  %s wall",
		len(forks), peak, dur(span))
	axis := padR("0s", bar-runewidth.StringWidth(dur(span))) + dur(span)
	b.WriteString(th.Head.Render("  FORKS  ") +
		th.Dimmer.Render(padR(note, lead-9)) +
		th.Faint.Render(axis) + "\n")

	for _, f := range forks {
		name := desc[f.ParentID]
		if name == "" {
			name = f.Skill
		}
		lane := forkBar(f, g, bar)
		if lane == "" {
			lane = strings.Repeat(g.TlOff, bar)
		}
		b.WriteString("  " + th.Dim.Render(padR(short(f.AgentID), idW)) +
			strings.Repeat(" ", gap) + th.Dimmer.Render(padDesc(name, label)) +
			strings.Repeat(" ", gap) + th.Gold.Render(lane) + "\n")
	}
	return b.String()
}

// forkIndent is where a fork block's rail sits, far enough right of a
// top-level row that the block plainly hangs off the Skill call above it.
const forkIndent = 6

// forkBar draws where a fork's run sat inside its group's overall span, which
// is the only way to tell forks that genuinely ran in parallel from ones that
// merely appear next to each other. A lone fork gets no bar: with nothing to
// compare against it would always be full, which says nothing.
func forkBar(f *turn.Fork, g Glyphs, cells int) string {
	if f.Group < 2 || f.To <= f.From || cells < 2 {
		return ""
	}
	lo := int(math.Round(f.From * float64(cells)))
	hi := int(math.Round(f.To * float64(cells)))
	lo = min(max(lo, 0), cells-1)
	if hi <= lo {
		hi = lo + 1 // a run too short to fill a cell still gets one
	}
	hi = min(hi, cells)
	return strings.Repeat(g.TlOff, lo) + strings.Repeat(g.TlOn, hi-lo) +
		strings.Repeat(g.TlOff, cells-hi)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// footer sheds its parts rather than overrunning a narrow panel.
func footer(t *turn.Turn, th Theme, g Glyphs, width int, o Opts) string {
	label, plainLabel := th.Dimmer.Render("turn complete"), "turn complete"
	if o.Live {
		spin := o.spinFrame(g)
		label, plainLabel = th.Live.Render(spin+" running"), spin+" running"
	}
	stat := fmt.Sprintf("   %d req  ·  %s ctx", t.Requests, kilo(t.PeakCtx))
	totals := comma(t.OutTokens) + "  " + dur(t.Duration)

	// Cost joins the left-hand stats: the right-hand totals sit under the
	// OUT and Δt columns and must stay there. Each figure drops in turn as
	// the panel narrows, least useful first.
	led := turnUsage(t)
	var cached, turnCost, sessCost string
	if s := led.Tokens().CacheShare(); s >= 0 {
		cached = fmt.Sprintf("  ·  %d%% cached", int(s*100))
	}
	if !led.Empty() {
		turnCost = "  ·  " + money(led.Cost(), led.Unpriced) + " turn"
	}
	if ss := o.Session; ss != nil && !ss.Ledger.Empty() {
		sessCost = "  ·  " + sessionMoney(*ss) + " session"
	}

	for _, v := range []struct{ stat, totals string }{
		{stat + cached + turnCost + sessCost, totals},
		{stat + turnCost + sessCost, totals},
		{stat + turnCost, totals},
		{stat, totals}, {"", totals}, {"", dur(t.Duration)}, {"", ""},
	} {
		used := 2 + runewidth.StringWidth(plainLabel) +
			runewidth.StringWidth(v.stat) + runewidth.StringWidth(v.totals)
		if pad := width - used; pad >= 1 {
			return "  " + label + th.Dim.Render(v.stat) +
				strings.Repeat(" ", pad) + th.Text.Render(v.totals)
		}
	}
	return "  " + label
}

// turnUsage is the turn's own requests plus every fork it launched, which is
// what the turn actually cost.
func turnUsage(t *turn.Turn) usage.Ledger {
	var l usage.Ledger
	l.Merge(t.Usage)
	for _, r := range t.Rows {
		if r.Fork != nil && r.Fork.Nested != nil {
			l.Merge(r.Fork.Nested.Usage)
		}
	}
	return l
}

// money formats a cost. A total that includes an unpriced model is a floor,
// and says so.
func money(c float64, floor bool) string {
	s := fmt.Sprintf("$%.2f", c)
	if c > 0 && c < 0.005 {
		s = "<$0.01"
	}
	if floor {
		s = "≥" + s
	}
	return s
}

// sessionMoney marks an estimate with ~. Only Claude Code's own total, with
// nothing after it, is exact.
func sessionMoney(s turn.Session) string {
	m := money(s.Ledger.Cost(), s.Ledger.Unpriced)
	if !s.Exact {
		m = "~" + m
	}
	return m
}

// tokens abbreviates a token count the way /usage does: 7.3k, 1.1m.
func tokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return fmt.Sprintf("%.1fm", float64(n)/1e6)
	}
}

// renderUsage draws the per-model breakdown /usage gives, for the turn and
// for the session, under the totals line.
func renderUsage(t *turn.Turn, s *turn.Session, th Theme, width int) string {
	var b strings.Builder
	line := func(style lipgloss.Style, text string) {
		b.WriteString(style.Render(padR(text, width-2)) + "\n")
	}
	row := func(name string, tk usage.Tokens, cost string) string {
		return "    " + padR(name, colModel-2) + padL(tokens(tk.In), 8) +
			padL(tokens(tk.Out), 9) + padL(tokens(tk.CacheRead), 12) +
			padL(tokens(tk.Write()), 13) + padL(cost, 10)
	}
	section := func(title, note string, l usage.Ledger) {
		b.WriteString(th.Text.Render("  "+title) + th.Faint.Render(padR(note, width-4-runewidth.StringWidth(title))) + "\n")
		if l.Empty() {
			line(th.Faint, "    no requests yet")
			return
		}
		for _, ln := range l.Lines() {
			cost := money(ln.Cost, false)
			if _, ok := usage.Cost(ln.Model, usage.Tokens{}, false); !ok {
				cost = "no price"
			}
			line(th.Dim, row(record.ShortModel(ln.Model), ln.Tokens, cost))
		}
		if sh := l.Tokens().CacheShare(); sh >= 0 {
			line(th.Faint, fmt.Sprintf("    %d%% of input from cache", int(sh*100)))
		}
	}

	line(th.Head, "  "+padR("USAGE", colModel)+padL("INPUT", 8)+padL("OUTPUT", 9)+
		padL("CACHE READ", 12)+padL("CACHE WRITE", 13)+padL("COST", 10))
	section("this turn", "", turnUsage(t))
	if s != nil {
		note := "  estimate · a few Claude Code calls never reach the transcript"
		switch {
		case s.Exact:
			note = "  Claude Code's own total"
		case !s.Base.IsZero():
			note = "  Claude Code's total, plus an estimate since"
		}
		section("session", note, s.Ledger)
	}
	return b.String()
}

func builtinSpin(g Glyphs, frame int) string {
	f := g.Spin
	if len(f) == 0 {
		return "●"
	}
	return f[((frame%len(f))+len(f))%len(f)]
}

func ellipsis(frame int) string { return strings.Repeat(".", (frame/3)%4) }

// shortTool keeps the tool column narrow without truncating mid-word.
func shortTool(n string) string {
	if strings.HasPrefix(n, "mcp__") {
		if p := strings.Split(n, "__"); len(p) >= 3 {
			n = p[len(p)-1]
		}
	}
	switch n {
	case "AskUserQuestion":
		return "Ask"
	case "ExitPlanMode", "EnterPlanMode":
		return "Plan"
	case "NotebookEdit":
		return "NbEdit"
	case "ToolSearch":
		return "Search"
	case "WebSearch":
		return "Web"
	case "WebFetch":
		return "Fetch"
	}
	if runewidth.StringWidth(n) > colTool {
		return runewidth.Truncate(n, colTool, "")
	}
	return n
}

// wrap splits s into at most max lines of w cells, ellipsising the last.
func wrap(s string, w, max int) []string {
	s = strings.Join(strings.Fields(s), " ")
	var out []string
	for len(s) > 0 && len(out) < max {
		if runewidth.StringWidth(s) <= w {
			out = append(out, s)
			break
		}
		cut := w
		if i := strings.LastIndex(runewidth.Truncate(s, w, ""), " "); i > 0 && len(out) < max-1 {
			cut = i
		}
		seg := strings.TrimSpace(s[:cut])
		if len(out) == max-1 {
			seg = runewidth.Truncate(seg, w-1, "") + "…"
		}
		out = append(out, seg)
		s = strings.TrimSpace(s[cut:])
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && r == 'm':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// visWidth measures a rendered line in display cells, ignoring ANSI.
func visWidth(s string) int { return runewidth.StringWidth(stripANSI(s)) }
