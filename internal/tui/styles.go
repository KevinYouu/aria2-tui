package tui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
	"github.com/KevinYouu/aria2-tui/internal/aria2"
	"github.com/KevinYouu/aria2-tui/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

// Styles are built on demand because the palette can change at runtime:
// package level styles would keep the colors of the mode they were built in.

// bannerKind selects the icon and color of the message line.
type bannerKind int

const (
	bannerInfo bannerKind = iota
	bannerWarn
	bannerError
)

// footerLine is the single line message below the content of a view.
type footerLine struct {
	text string
	kind bannerKind
	set  bool
}

func (f footerLine) render(width int) string {
	return bannerLine(f.text, f.kind, width)
}

func titleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.PrimaryColor).Bold(true)
}

func mutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.MutedForeground)
}

// cursorStyle highlights the selected row indicator.
func cursorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.PrimaryColor).Bold(true)
}

// nameStyle dims unselected download names so the selection stands out.
func nameStyle(selected bool) lipgloss.Style {
	style := lipgloss.NewStyle().Foreground(theme.MutedForeground)
	if selected {
		style = style.Foreground(theme.PrimaryColor).Bold(true)
	}
	return style
}

// statusStyle colors a download state semantically.
func statusStyle(status aria2.Status) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(statusColor(status))
}

func statusColor(status aria2.Status) color.Color {
	switch status {
	case aria2.StatusActive:
		return theme.InfoColor
	case aria2.StatusComplete:
		return theme.SuccessColor
	case aria2.StatusError:
		return theme.ErrorColor
	case aria2.StatusPaused:
		return theme.WarningColor
	default:
		return theme.MutedForeground
	}
}

// newProgressBar renders the solid █░ bar: block characters are stable on old
// conhost and need no glyph fallback.
func newProgressBar() progress.Model {
	bar := progress.New(
		progress.WithoutPercentage(),
		progress.WithFillCharacters(progress.DefaultFullCharFullBlock, progress.DefaultEmptyCharBlock),
	)
	bar.FullColor = theme.SuccessColor
	bar.EmptyColor = theme.BorderColor
	return bar
}

// headerLine renders a view title with a rule filling the remaining width.
func headerLine(title string, width int) string {
	title = SafeTruncate(title, width)
	rendered := titleStyle().Render(title)
	if remaining := width - ansi.StringWidth(title) - 1; remaining > 0 {
		return rendered + " " + theme.GetHorizontalRule(remaining)
	}
	return rendered
}

// bannerLine renders one line of text with the icon of its kind. Whitespace is
// collapsed first so the line never wraps into a second row.
func bannerLine(text string, kind bannerKind, width int) string {
	text = strings.Join(strings.Fields(text), " ")

	style, iconStyle, icon := theme.InfoStyle, theme.InfoIconStyle, "ℹ"
	switch kind {
	case bannerWarn:
		style, iconStyle, icon = theme.WarningStyle, theme.WarningIconStyle, "⚠"
	case bannerError:
		style, iconStyle, icon = theme.ErrorStyle, theme.ErrorIconStyle, "✗"
	}
	return SafeTruncate(iconStyle.Render(icon)+" "+style.Render(text), width)
}
