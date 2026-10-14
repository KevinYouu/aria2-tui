package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/KevinYouu/aria2-tui/internal/theme"
)

// HelpKey is one entry of the help bar: the key prefix and its action.
type HelpKey struct {
	Key    string
	Action string
}

// helpKeys converts bindings into help bar entries and skips bindings without
// help text; the bindings own both the keys and their label.
func helpKeys(bindings ...key.Binding) []HelpKey {
	keys := make([]HelpKey, 0, len(bindings))
	for _, binding := range bindings {
		help := binding.Help()
		if help.Key == "" || help.Desc == "" {
			continue
		}
		keys = append(keys, HelpKey{Key: help.Key, Action: help.Desc})
	}
	return keys
}

// RenderHelpBar renders "[key] action" pairs separated by ·, truncated to the
// terminal width.
func RenderHelpBar(keys []HelpKey, width int) string {
	if len(keys) == 0 || width <= 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, item := range keys {
		parts = append(parts, keyStyle().Render("["+item.Key+"]")+" "+actionStyle().Render(item.Action))
	}
	return SafeTruncate(strings.Join(parts, " · "), width)
}

// keyStyle highlights the key prefix; the brackets are the visual boundary, so
// no background color is used.
func keyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.PrimaryColor).Bold(true)
}

// actionStyle dims the action description.
func actionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.MutedForeground)
}
