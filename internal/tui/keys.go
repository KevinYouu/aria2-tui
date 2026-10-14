package tui

import "charm.land/bubbles/v2/key"

// Key bindings. Bindings that carry help text are the ones shown in the help
// bar, so the bar cannot promise a key that is not handled. Only one binding
// of a key pair (up/down, +/-) carries the help text to keep the bar compact.
var (
	keyUp   = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/↓", "navigate"))
	keyDown = key.NewBinding(key.WithKeys("down", "j"))

	keyPageUp   = key.NewBinding(key.WithKeys("pgup"))
	keyPageDown = key.NewBinding(key.WithKeys("pgdown"))

	keyTogglePause = key.NewBinding(key.WithKeys("p", "space"), key.WithHelp("p/space", "pause · resume"))
	keyRemove      = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove"))

	keyMoreConnections  = key.NewBinding(key.WithKeys("+", "="), key.WithHelp("+/-", "connections"))
	keyFewerConnections = key.NewBinding(key.WithKeys("-"))

	keyAddLinks = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add links"))

	keySubmit = key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "download"))
	keyBack   = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyQuit   = key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit"))

	keyQuitList = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit"))
)

// listHelpKeys is the help bar of the download list.
func listHelpKeys() []HelpKey {
	return helpKeys(
		keyUp,
		keyTogglePause,
		keyRemove,
		keyMoreConnections,
		keyAddLinks,
		keyQuitList,
	)
}

// addHelpKeys is the help bar of the paste view.
func addHelpKeys() []HelpKey {
	return helpKeys(keySubmit, keyBack, keyQuit)
}
