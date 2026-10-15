package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/KevinYouu/aria2-tui/internal/config"
	"github.com/KevinYouu/aria2-tui/internal/theme"
)

// settingsAction is what the settings view asks the app to do.
type settingsAction int

const (
	settingsNoop settingsAction = iota
	settingsBack
	settingsQuit
	// settingsChanged carries a modified configuration to validate, apply
	// and persist.
	settingsChanged
)

// settingsResult is the outcome of one key press in the settings view.
type settingsResult struct {
	action settingsAction
	cfg    config.Config
	// label names the field that changed, for the confirmation message.
	label string
	// err is an input error to show without changing anything.
	err error
}

// settingField describes one editable option. Fields with a step function
// can be adjusted with the left and right keys.
type settingField struct {
	label string
	hint  string
	// restart marks options that only take effect on the next start.
	restart bool
	get     func(config.Config) string
	set     func(*config.Config, string) error
	step    func(*config.Config, int)
}

func intField(label, hint string, low, high int, get func(*config.Config) *int) settingField {
	return settingField{
		label: label,
		hint:  hint,
		get: func(c config.Config) string {
			return strconv.Itoa(*get(&c))
		},
		set: func(c *config.Config, value string) error {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < low || n > high {
				return fmt.Errorf("%s must be a number from %d to %d", strings.ToLower(label), low, high)
			}
			*get(c) = n
			return nil
		},
		step: func(c *config.Config, delta int) {
			p := get(c)
			*p = clampInt(*p+delta, low, high)
		},
	}
}

func textField(label, hint string, restart bool, get func(*config.Config) *string) settingField {
	return settingField{
		label:   label,
		hint:    hint,
		restart: restart,
		get: func(c config.Config) string {
			return *get(&c)
		},
		set: func(c *config.Config, value string) error {
			*get(c) = strings.TrimSpace(value)
			return nil
		},
	}
}

var themeModes = []theme.Mode{theme.ModeAuto, theme.ModeDark, theme.ModeLight}

func themeField() settingField {
	return settingField{
		label: "Theme",
		hint:  "auto follows the terminal background; ←/→ to switch",
		get:   func(c config.Config) string { return string(c.Theme) },
		set: func(c *config.Config, value string) error {
			mode := theme.Mode(strings.ToLower(strings.TrimSpace(value)))
			if !theme.ValidMode(mode) {
				return fmt.Errorf("theme must be auto, dark or light")
			}
			c.Theme = mode
			return nil
		},
		step: func(c *config.Config, delta int) {
			index := 0
			for i, mode := range themeModes {
				if mode == c.Theme {
					index = i
				}
			}
			c.Theme = themeModes[(index+delta+len(themeModes))%len(themeModes)]
		},
	}
}

func settingFields() []settingField {
	return []settingField{
		textField("Download directory", "where files are saved; ~ is expanded and the folder is created", false,
			func(c *config.Config) *string { return &c.DownloadDir }),
		intField("Max concurrent downloads", "downloads running at the same time (also +/- in the list)",
			config.MinConcurrentDownloads, config.MaxConcurrentDownloads,
			func(c *config.Config) *int { return &c.MaxConcurrentDownloads }),
		intField("Connections per server", "connections to one server for one download (1-16)",
			config.MinConnections, config.MaxConnections,
			func(c *config.Config) *int { return &c.MaxConnectionPerServer }),
		intField("Split per download", "pieces one download is cut into (1-16)",
			config.MinConnections, config.MaxConnections,
			func(c *config.Config) *int { return &c.Split }),
		textField("Min split size", "smallest piece worth its own connection, e.g. 1M, 4M, 20M", false,
			func(c *config.Config) *string { return &c.MinSplitSize }),
		textField("Speed limit", "overall download cap, e.g. 500K or 2M; 0 means unlimited", false,
			func(c *config.Config) *string { return &c.DownloadLimit }),
		textField("Proxy", "proxy for all downloads, e.g. http://127.0.0.1:7890; empty for none", false,
			func(c *config.Config) *string { return &c.Proxy }),
		themeField(),
		textField("aria2c path", "empty looks aria2c up in PATH; applies after restart", true,
			func(c *config.Config) *string { return &c.Aria2cPath }),
	}
}

// settingsModel is the settings view: a field list with inline editing.
type settingsModel struct {
	fields  []settingField
	cursor  int
	editing bool
	input   textinput.Model
}

func newSettingsModel() *settingsModel {
	input := textinput.New()
	input.Prompt = ""
	input.SetVirtualCursor(true)
	return &settingsModel{fields: settingFields(), input: input}
}

// update applies a key press. cfg is the current configuration.
func (m *settingsModel) update(msg tea.KeyPressMsg, cfg config.Config) (settingsResult, tea.Cmd) {
	field := m.fields[m.cursor]

	if m.editing {
		switch {
		case key.Matches(msg, keyQuit):
			return settingsResult{action: settingsQuit}, nil
		case key.Matches(msg, keyBack):
			m.editing = false
			m.input.Blur()
			return settingsResult{}, nil
		case msg.String() == "enter":
			next := cfg
			if err := field.set(&next, m.input.Value()); err != nil {
				return settingsResult{err: err}, nil
			}
			// Reject bad input while the edit is still open so it can be fixed.
			if _, err := next.Validate(); err != nil {
				return settingsResult{err: err}, nil
			}
			m.editing = false
			m.input.Blur()
			return settingsResult{action: settingsChanged, cfg: next, label: field.label}, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return settingsResult{}, cmd
	}

	switch {
	case key.Matches(msg, keyQuit):
		return settingsResult{action: settingsQuit}, nil
	case key.Matches(msg, keyBack), msg.String() == "q", key.Matches(msg, keySwitchPane):
		return settingsResult{action: settingsBack}, nil
	case key.Matches(msg, keyUp):
		m.cursor = clampInt(m.cursor-1, 0, len(m.fields)-1)
	case key.Matches(msg, keyDown):
		m.cursor = clampInt(m.cursor+1, 0, len(m.fields)-1)
	case msg.String() == "left" || msg.String() == "h":
		return m.stepResult(field, cfg, -1), nil
	case msg.String() == "right" || msg.String() == "l":
		return m.stepResult(field, cfg, 1), nil
	case msg.String() == "r":
		next := cfg
		_ = field.set(&next, field.get(config.Default()))
		return settingsResult{action: settingsChanged, cfg: next, label: field.label}, nil
	case msg.String() == "enter" || msg.String() == "space":
		if field.step != nil && field.label == "Theme" {
			return m.stepResult(field, cfg, 1), nil
		}
		m.editing = true
		m.input.SetValue(field.get(cfg))
		m.input.CursorEnd()
		return settingsResult{}, m.input.Focus()
	}
	return settingsResult{}, nil
}

// stepResult adjusts a steppable field; other fields ignore left and right.
func (m *settingsModel) stepResult(field settingField, cfg config.Config, delta int) settingsResult {
	if field.step == nil {
		return settingsResult{}
	}
	next := cfg
	field.step(&next, delta)
	if field.get(next) == field.get(cfg) {
		return settingsResult{}
	}
	return settingsResult{action: settingsChanged, cfg: next, label: field.label}
}

// view renders the settings screen.
func (m *settingsModel) view(width, height int, cfg config.Config, footer footerLine) string {
	labelWidth := 0
	for _, f := range m.fields {
		labelWidth = max(labelWidth, len([]rune(f.label)))
	}

	content := []string{""}
	for i, f := range m.fields {
		selected := i == m.cursor
		marker := "  "
		if selected {
			marker = cursorStyle().Render("►") + " "
		}
		label := nameStyle(selected).Render(padRight(f.label, labelWidth))

		value := f.get(cfg)
		switch {
		case selected && m.editing:
			m.input.SetWidth(max(width-labelWidth-12, 8))
			value = m.input.View()
		case value == "":
			value = mutedStyle().Render("(none)")
		case selected:
			value = lipgloss.NewStyle().Foreground(theme.PrimaryColor).Render(value)
			if f.step != nil {
				value = mutedStyle().Render("◄ ") + value + mutedStyle().Render(" ►")
			}
		}
		if f.restart {
			value += mutedStyle().Render("  *")
		}
		content = append(content, "  "+marker+label+"  "+value)
	}
	content = append(content, "", "  "+mutedStyle().Render(m.fields[m.cursor].hint))

	boxHeight := clampInt(len(content)+2, 3, max(height-2, 3))
	lines := []string{headerLine(appTitle+" · settings", width)}
	lines = append(lines, renderPanelBox("Settings", content, width, boxHeight, true)...)
	if footer.set {
		lines = append(lines, footer.render(width))
	}
	lines = append(lines, RenderHelpBar(settingsHelpKeys(m.editing), width))
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// settingsHelpKeys is the help bar of the settings view.
func settingsHelpKeys(editing bool) []HelpKey {
	if editing {
		return []HelpKey{{Key: "enter", Action: "save"}, {Key: "esc", Action: "cancel"}}
	}
	return []HelpKey{
		{Key: "↑/↓", Action: "navigate"},
		{Key: "←/→", Action: "adjust"},
		{Key: "enter", Action: "edit"},
		{Key: "r", Action: "reset"},
		{Key: "esc", Action: "back"},
	}
}
