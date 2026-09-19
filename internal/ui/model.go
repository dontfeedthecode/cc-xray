package ui

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dontfeedthecode/ccxray/internal/discover"
	"github.com/dontfeedthecode/ccxray/internal/record"
	"github.com/dontfeedthecode/ccxray/internal/tail"
	"github.com/dontfeedthecode/ccxray/internal/turn"
)

type tickMsg time.Time

type Model struct {
	th      Theme
	g       Glyphs
	tl      *tail.Tailer
	b       *turn.Builder
	dir     string
	width   int
	height  int
	err     error
	last    time.Time
	version string
	bad     int
	frame   int

	vp     viewport.Model
	spin   spinner.Model
	help   help.Model
	keys   keyMap
	follow bool // pinned to the live edge, like tail -f
	ready  bool
	forks  map[string]*forkWatch // agentId -> its own tailer and builder
}

// forkWatch follows one forked skill's subagent transcript. The schema is
// identical to the parent's, so record and turn are reused unchanged.
type forkWatch struct {
	tl *tail.Tailer
	b  *turn.Builder
}

type Options struct {
	Dir   string
	Path  string
	ASCII bool
}

func NewModel(o Options) Model {
	g := UnicodeGlyphs()
	if o.ASCII {
		g = ASCIIGlyphs()
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	h := help.New()
	return Model{
		th: NewTheme(), g: g, dir: o.Dir,
		tl: tail.New(o.Path), b: turn.New(),
		forks: map[string]*forkWatch{},
		width: 92, height: 30,
		spin: sp, help: h, keys: defaultKeys(), follow: true,
	}
}

func (m Model) Init() tea.Cmd { return tea.Batch(tick(), m.spin.Tick) }

func tick() tea.Cmd {
	return tea.Tick(tail.PollInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		h := m.chromeHeight()
		if !m.ready {
			m.vp = viewport.New(msg.Width, max(1, msg.Height-h))
			m.ready = true
		} else {
			m.vp.Width, m.vp.Height = msg.Width, max(1, msg.Height-h)
		}
		m.refresh()
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Bottom):
			m.follow = true
			m.vp.GotoBottom()
			return m, nil
		case key.Matches(msg, m.keys.Top):
			m.follow = false
			m.vp.GotoTop()
			return m, nil
		}
		// any other navigation key hands control to the viewport and stops
		// following, so scrollback is not yanked away by incoming rows
		before := m.vp.YOffset
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		if m.vp.YOffset != before {
			m.follow = m.vp.AtBottom()
		}
		return m, cmd

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		m.follow = m.vp.AtBottom()
		return m, cmd

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		cmds = append(cmds, cmd)
		m.refresh()
		return m, tea.Batch(cmds...)

	case tickMsg:
		m.frame++
		m.poll()
		m.resize()
		m.refresh()
		return m, tick()
	}
	return m, nil
}

// chromeHeight measures what the header and footer actually render, so the
// viewport stays correct when either collapses (an empty turn draws neither).
func (m Model) chromeHeight() int {
	o := Opts{Width: m.width, Frame: m.frame, Live: m.live(), Spin: m.spin.View()}
	n := strings.Count(RenderHeader(m.b.Turn(), m.th, m.g, o), "\n") +
		strings.Count(RenderFooter(m.b.Turn(), m.th, m.g, o, ""), "\n")
	return n + 2 // help line + the newline after the viewport
}

// refresh re-renders the body into the viewport, holding the scroll position
// unless we are following the live edge.
// resize keeps the viewport in step as the chrome grows or shrinks.
func (m *Model) resize() {
	if !m.ready {
		return
	}
	if h := max(1, m.height-m.chromeHeight()); h != m.vp.Height {
		m.vp.Height = h
	}
}

func (m *Model) refresh() {
	if !m.ready {
		return
	}
	body := RenderBody(m.b.Turn(), m.th, m.g, Opts{
		Width: m.width, Frame: m.frame, Live: m.live(), Spin: m.spin.View(),
	})
	atBottom := m.vp.AtBottom()
	m.vp.SetContent(body)
	if m.follow || atBottom {
		m.vp.GotoBottom()
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// poll drains any appended lines, re-detecting the session if a newer one
// appears (a /clear or a new session in the same project).
func (m *Model) poll() {
	if m.dir != "" {
		if p, err := discover.Newest(m.dir); err == nil && p != "" && p != m.tl.Path() {
			m.tl.Reset(p)
			m.b = turn.New()
			m.forks = map[string]*forkWatch{}
		}
	}
	lines, err := m.tl.Read()
	if err != nil {
		m.err = err
		return
	}
	m.err = nil
	for _, ln := range lines {
		var r record.Record
		if json.Unmarshal(ln, &r) != nil {
			m.bad++ // one bad line must never be fatal
			continue
		}
		if r.Version != "" {
			m.version = r.Version
		}
		m.b.Add(r)
		m.last = time.Now()
	}
	m.pollForks()
}

// pollForks reads each forked skill's transcript and attaches the resulting
// turn to its marker row, so the renderer can nest it.
func (m *Model) pollForks() {
	for _, f := range m.b.Forks() {
		w := m.forks[f.AgentID]
		if w == nil {
			path := discover.ForkPath(m.tl.Path(), f.AgentID)
			if _, err := os.Stat(path); err != nil {
				continue // not written yet; try again next tick
			}
			w = &forkWatch{tl: tail.New(path), b: turn.New()}
			m.forks[f.AgentID] = w
		}
		lines, err := w.tl.Read()
		if err != nil {
			continue
		}
		for _, ln := range lines {
			var r record.Record
			if json.Unmarshal(ln, &r) != nil {
				m.bad++
				continue
			}
			w.b.Add(r)
			m.last = time.Now()
		}
		f.Nested = w.b.Turn()
	}
}

// live reports whether the turn is still running. It deliberately does NOT
// look at recent file activity: the model can think for 30 seconds before
// writing anything, and treating that silence as "finished" made the panel
// claim there were no actions while work was plainly happening.
func (m Model) live() bool {
	t := m.b.Turn()
	return t != nil && !t.Complete
}

func (m Model) View() string {
	if m.err != nil {
		return m.th.Dim.Render("\n  waiting for a session transcript\u2026\n  ") +
			m.th.Faint.Render(m.err.Error()) + "\n"
	}
	if !m.ready {
		return m.th.Dim.Render("  starting\u2026")
	}
	o := Opts{Width: m.width, Frame: m.frame, Live: m.live(), Spin: m.spin.View()}

	var b strings.Builder
	b.WriteString(RenderHeader(m.b.Turn(), m.th, m.g, o))
	b.WriteString(m.vp.View() + "\n")
	b.WriteString(RenderFooter(m.b.Turn(), m.th, m.g, o, m.scrollNote()))
	b.WriteString(m.th.Faint.Render("  " + m.help.View(m.keys)))
	return b.String()
}

// scrollNote tells the reader they are looking at history, not the live edge.
func (m Model) scrollNote() string {
	if m.follow || m.vp.AtBottom() {
		return ""
	}
	return "scrolled \u00b7 G to follow"
}
