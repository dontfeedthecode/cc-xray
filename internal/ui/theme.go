package ui

import "github.com/charmbracelet/lipgloss"

// Palette from the signed-off design study. Colours carry meaning: blue is
// model identity, gold is a state change, teal is live.
type Theme struct {
	Model, Effort, Text, Dim, Dimmer, Faint, Head, Live, Gold lipgloss.Style
	Band, BandExit, Fail, Tool                                lipgloss.Style
}

func NewTheme() Theme {
	return Theme{
		Model:  lipgloss.NewStyle().Foreground(lipgloss.Color("#7FA6DB")),
		Effort: lipgloss.NewStyle().Foreground(lipgloss.Color("#C0CAD4")),
		Text:   lipgloss.NewStyle().Foreground(lipgloss.Color("#C0CAD4")),
		Dim:    lipgloss.NewStyle().Foreground(lipgloss.Color("#7A8798")),
		Dimmer: lipgloss.NewStyle().Foreground(lipgloss.Color("#5A6878")),
		Faint:  lipgloss.NewStyle().Foreground(lipgloss.Color("#3E4A58")),
		Head:   lipgloss.NewStyle().Foreground(lipgloss.Color("#6E7F93")),
		Live:   lipgloss.NewStyle().Foreground(lipgloss.Color("#48B7A8")),
		Gold:   lipgloss.NewStyle().Foreground(lipgloss.Color("#D0A85C")),
		Fail:   lipgloss.NewStyle().Foreground(lipgloss.Color("#D9736A")),
		// Only tools other than the default are named, so they are worth
		// picking out: brighter than the description they precede.
		Tool: lipgloss.NewStyle().Foreground(lipgloss.Color("#D7DEE6")).Bold(true),
		Band: lipgloss.NewStyle().Foreground(lipgloss.Color("#D0A85C")).
			Background(lipgloss.Color("#2B2315")),
		BandExit: lipgloss.NewStyle().Foreground(lipgloss.Color("#7A8798")).
			Background(lipgloss.Color("#1A1F28")),
	}
}

// Glyphs are box-drawing only. All are single-width and none has an emoji
// presentation variant. ASCII mode exists because eight of them are
// East-Asian Ambiguous and widen under a CJK locale.
type Glyphs struct {
	Bar, Rule, Think, Enter, Live, Fail string
	Spin                                []string
}

func UnicodeGlyphs() Glyphs {
	return Glyphs{
		Bar: "▎", Rule: "─", Think: "✎",
		Enter: "▸", Live: "●", Fail: "×",
		// braille spinner: single-width, present in every programming font
		Spin: []string{"⠋", "⠙", "⠹", "⠸", "⠼",
			"⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

func ASCIIGlyphs() Glyphs {
	return Glyphs{
		Bar: "|", Rule: "-", Think: "*", Enter: ">", Live: "*", Fail: "x",
		Spin: []string{"|", "/", "-", "\\"},
	}
}
