package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// The reported failure: launching ccxray replayed an old run instead of
// waiting for the conversation about to start. A transcript already on disk
// must be ignored until one is written after launch.
func TestStartsEmptyAndAttachesToTheNextSession(t *testing.T) {
	d := t.TempDir()
	src, err := os.ReadFile("testdata/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(d, "old.jsonl")
	os.WriteFile(old, src, 0o644)
	hour := time.Now().Add(-time.Hour)
	os.Chtimes(old, hour, hour)

	m := NewModel(Options{Watch: []string{d}, Since: time.Now().Add(-time.Second), CWD: "/work/proj"})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 92, Height: 40})
	m = mm.(Model)
	m.poll()
	v := stripANSI(m.View())
	if !strings.Contains(v, "Waiting for a Claude Code session") || strings.Contains(v, "MODEL") {
		t.Fatalf("stale session was shown instead of waiting:\n%s", v)
	}

	live := filepath.Join(d, "live.jsonl")
	os.WriteFile(live, src, 0o644)
	m.poll()
	if m.tl == nil || m.tl.Path() != live {
		t.Fatalf("did not attach to the new session")
	}
	if m.dir != d {
		t.Errorf("dir = %q, want %q so later /clear switches still work", m.dir, d)
	}
	m.refresh()
	if v := stripANSI(m.View()); !strings.Contains(v, "MODEL") {
		t.Errorf("attached but drew no turn:\n%s", v)
	}
}

// The reported failure: an editor opens its terminals at the workspace root,
// so a fresh session there lands in the parent's project dir. Once the
// session being followed has gone quiet, that one must take over, not only a
// newer session in the dir first attached to.
func TestSwitchesToANewSessionInAParentDir(t *testing.T) {
	child, parent := t.TempDir(), t.TempDir()
	src, err := os.ReadFile("testdata/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(child, "first.jsonl")
	os.WriteFile(first, src, 0o644)

	m := NewModel(Options{Watch: []string{child, parent}, Since: time.Now().Add(-time.Minute)})
	m.poll()
	if m.tl == nil || m.tl.Path() != first {
		t.Fatal("did not attach to the first session")
	}

	quiet := time.Now().Add(-time.Minute)
	os.Chtimes(first, quiet, quiet)
	next := filepath.Join(parent, "next.jsonl")
	os.WriteFile(next, src, 0o644)
	m.poll()
	if m.tl.Path() != next {
		t.Fatalf("still following %s, want the new session in the parent dir", m.tl.Path())
	}
	if m.dir != parent {
		t.Errorf("dir = %q, want %q", m.dir, parent)
	}
}

// Pressing c empties the panel and waits for the next write, even from the
// session that was already being followed.
func TestClearWaitsForTheNextWrite(t *testing.T) {
	d := t.TempDir()
	src, err := os.ReadFile("testdata/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(d, "live.jsonl")
	os.WriteFile(live, src, 0o644)

	m := NewModel(Options{Watch: []string{d}, Since: time.Now().Add(-time.Minute)})
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 92, Height: 40})
	m = mm.(Model)
	m.poll()
	if m.tl == nil {
		t.Fatal("did not attach before clearing")
	}

	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = mm.(Model)
	past := time.Now().Add(-time.Second)
	os.Chtimes(live, past, past) // untouched since the clear
	m.poll()
	if v := stripANSI(m.View()); !strings.Contains(v, "Waiting for a Claude Code session") {
		t.Fatalf("clear did not empty the panel:\n%s", v)
	}

	f, _ := os.OpenFile(live, os.O_APPEND|os.O_WRONLY, 0o644)
	f.Write([]byte("\n"))
	f.Close()
	m.poll()
	if m.tl == nil || m.tl.Path() != live {
		t.Error("did not reattach once the session was written again")
	}
}

// With --session the panel must stay on that session after a clear, not jump
// to whichever other session in the project writes next.
func TestClearKeepsAPinnedSession(t *testing.T) {
	d := t.TempDir()
	pin := filepath.Join(d, "pin.jsonl")
	os.WriteFile(pin, []byte("{}\n"), 0o644)

	m := NewModel(Options{Path: pin, Watch: []string{d}, Since: time.Now()})
	m.poll()
	m.clear()
	os.WriteFile(filepath.Join(d, "other.jsonl"), []byte("{}\n"), 0o644)
	past := time.Now().Add(-time.Second)
	os.Chtimes(pin, past, past)
	m.poll()
	if m.tl != nil {
		t.Fatalf("attached to %s, want to keep waiting on the pinned session", m.tl.Path())
	}
}

// The waiting screen once rendered its path far to the right: lipgloss padded
// the styled heading to full width before the path was appended. Every line
// of the block must share one left edge, and the block must be centred.
func TestWaitingScreenIsAlignedAndCentred(t *testing.T) {
	t.Setenv("HOME", "/Users/you")
	v := stripANSI(RenderWaiting(NewTheme(), UnicodeGlyphs(), 100, 21, 0, "/Users/you/work/proj"))
	lines := strings.Split(v, "\n")
	if len(lines) != 21 {
		t.Fatalf("got %d lines, want the full 21-line area", len(lines))
	}
	var head, path, hint int = -1, -1, -1
	for i, ln := range lines {
		switch {
		case strings.Contains(ln, "Waiting for a Claude Code session"):
			head = i
		case strings.Contains(ln, "~/work/proj"):
			path = i
		case strings.Contains(ln, "Send a prompt"):
			hint = i
		}
	}
	if head < 0 || path != head+1 || hint != head+3 {
		t.Fatalf("lines out of order (head %d, path %d, hint %d):\n%s", head, path, hint, v)
	}
	indent := func(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }
	// the spinner sits in a gutter; the text beside it lines up with the path
	// columns, not bytes: the braille spinner is three bytes but one cell
	col := func(s, sub string) int { return visWidth(s[:strings.Index(s, sub)]) }
	textCol := col(lines[head], "Waiting")
	if got := col(lines[path], "~"); got != textCol {
		t.Errorf("path starts at column %d, heading text at %d:\n%s", got, textCol, v)
	}
	if got := col(lines[hint], "Send"); got != textCol {
		t.Errorf("hint starts at column %d, heading text at %d", got, textCol)
	}
	if l := indent(lines[hint]); l < 10 || l > 100-len(strings.TrimSpace(lines[hint]))-10 {
		t.Errorf("block not centred horizontally (indent %d)", l)
	}
	if head < 5 || head > 12 {
		t.Errorf("block not centred vertically (heading on line %d of 21)", head)
	}
}

// The spinner must move between frames and honour --ascii.
func TestWaitingSpinnerAnimates(t *testing.T) {
	a := RenderWaiting(NewTheme(), UnicodeGlyphs(), 80, 10, 0, "")
	b := RenderWaiting(NewTheme(), UnicodeGlyphs(), 80, 10, 1, "")
	if a == b {
		t.Error("spinner did not advance between frames")
	}
	v := stripANSI(RenderWaiting(NewTheme(), ASCIIGlyphs(), 80, 10, 0, ""))
	for _, r := range v {
		if r > 127 {
			t.Fatalf("ASCII mode drew %q", r)
		}
	}
}
