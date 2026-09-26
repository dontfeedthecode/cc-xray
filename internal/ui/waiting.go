package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderWaiting draws the empty state shown before any session is followed,
// centred in a width×height area, with where it is watching beneath the
// heading. Each line is styled on its own: styling a multi-line string pads
// it to its widest line, which pushed text sideways.
func RenderWaiting(th Theme, g Glyphs, width, height, frame int, where string) string {
	lines := []string{
		th.Live.Render(builtinSpin(g, frame)) + "  " +
			th.Text.Render("Waiting for a Claude Code session"),
	}
	if where != "" {
		lines = append(lines, "   "+th.Dim.Render(where))
	}
	lines = append(lines, "",
		"   "+th.Dimmer.Render("Send a prompt in Claude Code and it will appear here."))

	// Place centres each line on its own; pad them to one width first so the
	// block moves as a unit and keeps a shared left edge.
	w := 0
	for _, ln := range lines {
		w = max(w, lipgloss.Width(ln))
	}
	for i, ln := range lines {
		lines[i] = ln + strings.Repeat(" ", w-lipgloss.Width(ln))
	}
	block := strings.Join(lines, "\n")
	return lipgloss.Place(max(1, width), max(1, height), lipgloss.Center, lipgloss.Center, block)
}

// tildePath shortens a path under $HOME to ~/…, which is how it is typed.
func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return "~" + string(filepath.Separator) + rel
	}
	return p
}
