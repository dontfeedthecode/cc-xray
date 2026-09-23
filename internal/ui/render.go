package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dontfeedthecode/ccxray/internal/turn"
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

	for _, r := range rows {
		switch {
		case r.Fork != nil:
			b.WriteString(renderFork(r.Fork, th, g, width, l))
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
		}
	}
	// The lanes read every fork in the turn, not the visible slice, so
	// scrolling the table never changes what the timeline claims.
	b.WriteString(renderTimeline(t, th, g, width))
	return b.String()
}

// DefaultTool is assumed and never named on a row. Nearly every action in a
// normal turn is a shell command, so printing "Bash" on line after line said
// nothing and cost the description eight columns. Anything else is announced.
const DefaultTool = "Bash"

func renderAction(a *turn.Action, th Theme, g Glyphs, o Opts, l layout) string {
	gut, gutStyle := "  ", th.Faint
	if a.Thinking {
		gut = g.Think + " "
	}
	toolStyle, descStyle := th.Tool, th.Dim
	tool, desc := shortTool(a.Tool), a.Desc
	if a.Tool == DefaultTool {
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
	line := footer(t, th, g, width, o.Live, o.Frame, o.spinFrame(g))
	if note != "" {
		line = overlayRight(line, th.Gold.Render(note), width)
	}
	b.WriteString(line + "\n")
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
			if a.Tool == DefaultTool {
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
func footer(t *turn.Turn, th Theme, g Glyphs, width int, live bool, frame int, spin string) string {
	label, plainLabel := th.Dimmer.Render("turn complete"), "turn complete"
	if live {
		if spin == "" {
			spin = builtinSpin(g, frame)
		}
		label, plainLabel = th.Live.Render(spin+" running"), spin+" running"
	}
	stat := fmt.Sprintf("   %d req  ·  %s ctx", t.Requests, kilo(t.PeakCtx))
	totals := comma(t.OutTokens) + "  " + dur(t.Duration)

	for _, v := range []struct{ stat, totals string }{
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
