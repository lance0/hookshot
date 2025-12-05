package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keybindings
type KeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Replay  key.Binding
	Filter  key.Binding
	Clear   key.Binding
	Quit    key.Binding
	Help    key.Binding
	Enter   key.Binding
	TabNext key.Binding
}

// DefaultKeyMap returns the default keybindings
var DefaultKeyMap = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Replay: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "replay"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Clear: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "clear"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	TabNext: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next pane"),
	),
}

// ShortHelp returns a short help string
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Replay, k.Filter, k.Quit}
}

// FullHelp returns the full help string
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter},
		{k.Replay, k.Filter, k.Clear},
		{k.Quit, k.Help},
	}
}
