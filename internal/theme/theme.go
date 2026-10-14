package theme

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// Design tokens - Shadcn / Neutral palettes (dark and light).

// ─── Theme modes ─────────────────────────────────────────────────────────────

// Mode is the theme mode: auto follows the terminal background.
type Mode string

const (
	ModeAuto  Mode = "auto"
	ModeDark  Mode = "dark"
	ModeLight Mode = "light"
)

// ValidMode reports whether mode is one of the supported theme modes.
func ValidMode(mode Mode) bool {
	return mode == ModeAuto || mode == ModeDark || mode == ModeLight
}

// palette holds the neutral tokens of one mode; semantic colors are shared.
// In lipgloss v2 lipgloss.Color is a function returning color.Color, so the
// tokens are stored as interfaces.
type palette struct {
	isDark  bool
	primary color.Color
	muted   color.Color
	border  color.Color
}

// darkPalette is the Neutral Dark palette.
var darkPalette = palette{
	isDark:  true,
	primary: lipgloss.Color("#fafafa"), // foreground / primary (Neutral 50)
	muted:   lipgloss.Color("#a3a3a3"), // muted text (Neutral 400)
	border:  lipgloss.Color("#404040"), // border / input (Neutral 700)
}

// lightPalette is the Neutral Light palette.
var lightPalette = palette{
	isDark:  false,
	primary: lipgloss.Color("#18181b"), // foreground / primary (Neutral 900)
	muted:   lipgloss.Color("#737373"), // muted text (Neutral 500)
	border:  lipgloss.Color("#e4e4e7"), // border / input (Neutral 200)
}

var (
	currentMode Mode    = ModeDark // mode in effect (auto is already resolved)
	current     palette = darkPalette
)

// Neutral tokens: evaluated on call, so ApplyMode affects the whole package.
var (
	PrimaryColor    = current.primary
	MutedForeground = current.muted
	BorderColor     = current.border
)

// Semantic colors: identical in both modes and independent of ApplyMode.
var (
	SuccessColor = lipgloss.Color("#10b981") // Emerald 500
	ErrorColor   = lipgloss.Color("#ef4444") // Shadcn Red
	WarningColor = lipgloss.Color("#f59e0b") // Amber 500
	InfoColor    = lipgloss.Color("#3b82f6") // Blue 500
)

func init() {
	applyPalette(darkPalette)
}

// ─── Mode switching ──────────────────────────────────────────────────────────

// DetectDarkBackground probes the terminal background color: true is dark.
// It falls back to dark when the query fails or the output is not a TTY.
func DetectDarkBackground() bool {
	return lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
}

// ResolveMode resolves a mode to dark or light: auto follows the terminal
// background, an unknown value is treated like auto.
func ResolveMode(mode Mode) Mode {
	if !ValidMode(mode) || mode == ModeAuto {
		if DetectDarkBackground() {
			return ModeDark
		}
		return ModeLight
	}
	return mode
}

// ApplyMode applies the theme (auto is resolved first) and returns the mode
// that is actually in effect.
func ApplyMode(mode Mode) Mode {
	resolved := ResolveMode(mode)
	currentMode = resolved
	if resolved == ModeLight {
		applyPalette(lightPalette)
	} else {
		applyPalette(darkPalette)
	}
	return resolved
}

// CurrentMode returns the mode in effect (already resolved to dark or light).
func CurrentMode() Mode { return currentMode }

// IsDark reports whether the dark palette is in effect.
func IsDark() bool { return current.isDark }

// applyPalette installs a palette and rebuilds the package level styles.
func applyPalette(p palette) {
	current = p
	PrimaryColor = p.primary
	MutedForeground = p.muted
	BorderColor = p.border
	rebuildStyles()
}

// Semantic styles - foreground only, terminal friendly. Styles capture their
// colors at build time, so applyPalette rebuilds them on every mode switch.
var (
	ErrorStyle       lipgloss.Style // text style: always the primary color
	SuccessStyle     lipgloss.Style
	WarningStyle     lipgloss.Style
	InfoStyle        lipgloss.Style
	ErrorIconStyle   lipgloss.Style // icon style: carries the semantic color
	SuccessIconStyle lipgloss.Style
	WarningIconStyle lipgloss.Style
	InfoIconStyle    lipgloss.Style
)

// rebuildStyles rebuilds the package level styles.
func rebuildStyles() {
	ErrorStyle = lipgloss.NewStyle().
		Foreground(PrimaryColor).
		Bold(true)

	SuccessStyle = lipgloss.NewStyle().
		Foreground(PrimaryColor).
		Bold(true)

	WarningStyle = lipgloss.NewStyle().
		Foreground(PrimaryColor).
		Bold(true)

	InfoStyle = lipgloss.NewStyle().
		Foreground(PrimaryColor)

	ErrorIconStyle = lipgloss.NewStyle().
		Foreground(ErrorColor).
		Bold(true)

	SuccessIconStyle = lipgloss.NewStyle().
		Foreground(SuccessColor).
		Bold(true)

	WarningIconStyle = lipgloss.NewStyle().
		Foreground(WarningColor).
		Bold(true)

	InfoIconStyle = lipgloss.NewStyle().
		Foreground(InfoColor)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// GetHorizontalRule returns a rule of the given width.
func GetHorizontalRule(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(BorderColor).
		Width(width).
		Render(strings.Repeat("─", width))
}
