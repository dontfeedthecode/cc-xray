package ui

import "github.com/charmbracelet/bubbles/key"

// keyMap drives both the bindings and the help line, so they cannot drift.
type keyMap struct {
	Up, Down, PageUp, PageDown, Top, Bottom key.Binding
	Clear, Usage, Quit                      key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup", "b"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown", "f"), key.WithHelp("pgdn", "page down")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "follow")),
		Clear:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear")),
		Usage:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "usage")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c", "esc"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Bottom, k.Usage, k.Clear, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Top, k.Bottom, k.Usage, k.Clear, k.Quit},
	}
}
