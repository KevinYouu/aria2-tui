package theme

import (
	"reflect"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestValidMode(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{ModeAuto, true},
		{ModeDark, true},
		{ModeLight, true},
		{Mode(""), false},
		{Mode("solarized"), false},
		{Mode("DARK"), false},
	}

	for _, tt := range tests {
		if got := ValidMode(tt.mode); got != tt.want {
			t.Errorf("ValidMode(%q) = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestApplyModeSwitchesPalette(t *testing.T) {
	t.Cleanup(func() { ApplyMode(ModeDark) })

	if got := ApplyMode(ModeDark); got != ModeDark {
		t.Fatalf("ApplyMode(ModeDark) = %q, want %q", got, ModeDark)
	}
	if CurrentMode() != ModeDark {
		t.Errorf("CurrentMode() = %q, want %q", CurrentMode(), ModeDark)
	}
	if !IsDark() {
		t.Error("IsDark() = false after applying the dark palette")
	}
	darkPrimary := PrimaryColor
	if !reflect.DeepEqual(darkPrimary, lipgloss.Color("#fafafa")) {
		t.Errorf("dark PrimaryColor = %v, want %v", darkPrimary, lipgloss.Color("#fafafa"))
	}

	if got := ApplyMode(ModeLight); got != ModeLight {
		t.Fatalf("ApplyMode(ModeLight) = %q, want %q", got, ModeLight)
	}
	if CurrentMode() != ModeLight {
		t.Errorf("CurrentMode() = %q, want %q", CurrentMode(), ModeLight)
	}
	if IsDark() {
		t.Error("IsDark() = true after applying the light palette")
	}
	lightPrimary := PrimaryColor
	if !reflect.DeepEqual(lightPrimary, lipgloss.Color("#18181b")) {
		t.Errorf("light PrimaryColor = %v, want %v", lightPrimary, lipgloss.Color("#18181b"))
	}
	if reflect.DeepEqual(darkPrimary, lightPrimary) {
		t.Error("the light palette kept the dark primary color")
	}
}

// ApplyMode resolves auto (and anything unknown) through terminal detection,
// which depends on the environment under test; only the resolved set is fixed.
func TestApplyModeResolvesAutoAndUnknown(t *testing.T) {
	t.Cleanup(func() { ApplyMode(ModeDark) })

	for _, mode := range []Mode{ModeAuto, Mode("solarized"), Mode("")} {
		resolved := ApplyMode(mode)
		if resolved != ModeDark && resolved != ModeLight {
			t.Fatalf("ApplyMode(%q) resolved to %q, want %q or %q", mode, resolved, ModeDark, ModeLight)
		}
		if CurrentMode() != resolved {
			t.Errorf("CurrentMode() = %q after ApplyMode(%q), want %q", CurrentMode(), mode, resolved)
		}
		wantDark := resolved == ModeDark
		if IsDark() != wantDark {
			t.Errorf("IsDark() = %v after ApplyMode(%q), want %v", IsDark(), mode, wantDark)
		}
	}
}

func TestResolveModeKeepsExplicitModes(t *testing.T) {
	for _, mode := range []Mode{ModeDark, ModeLight} {
		if got := ResolveMode(mode); got != mode {
			t.Errorf("ResolveMode(%q) = %q, want %q", mode, got, mode)
		}
	}
}

func TestGetHorizontalRuleWidth(t *testing.T) {
	t.Cleanup(func() { ApplyMode(ModeDark) })

	for _, width := range []int{0, 1, 10, 40} {
		if got := ansi.StringWidth(GetHorizontalRule(width)); got != width {
			t.Errorf("ansi.StringWidth(GetHorizontalRule(%d)) = %d, want %d", width, got, width)
		}
	}
}
