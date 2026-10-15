package tui

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/KevinYouu/aria2-tui/internal/theme"
)

// allowedSchemes lists the URI schemes accepted by the paste box and by the
// command line.
var allowedSchemes = []string{"http", "https", "ftp", "magnet"}

// ValidateURI reports whether raw is a link this tool can hand to aria2.
func ValidateURI(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("empty download link")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid download link %q", trimmed)
	}
	if !schemeAllowed(parsed.Scheme) {
		return fmt.Errorf("unsupported scheme %q in %q: expected one of %s",
			parsed.Scheme, trimmed, strings.Join(allowedSchemes, ", "))
	}
	return nil
}

// ParseURIs splits pasted text into links, one per line. Blank lines are
// ignored; every other line that is not a usable link is counted as invalid.
func ParseURIs(text string) (links []string, invalid int) {
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if err := ValidateURI(trimmed); err != nil {
			invalid++
			continue
		}
		links = append(links, trimmed)
	}
	return links, invalid
}

func schemeAllowed(scheme string) bool {
	scheme = strings.ToLower(scheme)
	return slices.Contains(allowedSchemes, scheme)
}

// addAction is what the paste view asks the app to do after a key press.
type addAction int

const (
	addNoop addAction = iota
	addSubmit
	addBack
	addQuit
	addSwitchPane
)

// addModel is the paste view: a multi line text area plus link parsing.
type addModel struct {
	input textarea.Model
}

func newAddModel() *addModel {
	input := textarea.New()

	styles := textarea.DefaultStyles(theme.IsDark())
	styles.Focused.Text = lipgloss.NewStyle().Foreground(theme.PrimaryColor)
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(theme.MutedForeground).Italic(true)
	styles.Focused.CursorLine = lipgloss.NewStyle() // no line highlight: the rows are links, not editor lines
	styles.Cursor.Color = theme.PrimaryColor
	styles.Cursor.Blink = false
	input.SetStyles(styles)

	input.Placeholder = "Paste one link per line (HTTP, HTTPS, FTP, Magnet)"
	input.Prompt = ""
	input.ShowLineNumbers = false
	// A virtual cursor keeps the text area self contained: the block cursor is
	// part of the rendered string, so no absolute cursor position is needed in
	// a view that also draws a title, status and help bar. Without blinking,
	// Focus returns no command and no blink message stays in flight.
	input.SetVirtualCursor(true)
	input.Focus()

	return &addModel{input: input}
}

// update handles a key press; every other message (paste, window size) is
// forwarded to the text area.
func (m *addModel) update(msg tea.Msg) (addAction, tea.Cmd) {
	if pressed, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(pressed, keySubmit):
			return addSubmit, nil
		case key.Matches(pressed, keySwitchPane):
			return addSwitchPane, nil
		case key.Matches(pressed, keyBack):
			return addBack, nil
		case key.Matches(pressed, keyQuit):
			return addQuit, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return addNoop, cmd
}

// links returns the submitted links and the number of rejected lines.
func (m *addModel) links() ([]string, int) {
	return ParseURIs(m.input.Value())
}

// reset clears the text area after a successful submit.
func (m *addModel) reset() {
	m.input.Reset()
}

// view renders the paste view; footer is the shared app message line.
func (m *addModel) view(width, height int, footer footerLine) string {
	innerWidth := max(width-4, 10)
	inputHeight := clampInt(height-7, 3, 5)
	if width > 0 {
		m.input.SetWidth(innerWidth)
	}
	m.input.SetHeight(inputHeight)

	contentLines := make([]string, 0, inputHeight+2)
	contentLines = append(contentLines, "")

	textRows := strings.Split(m.input.View(), "\n")
	for i := 0; i < len(textRows) && i < inputHeight; i++ {
		contentLines = append(contentLines, "  "+textRows[i])
	}

	valid, invalid := m.links()
	if len(valid) > 0 {
		statusText := lipgloss.NewStyle().Foreground(theme.SuccessColor).Render(fmt.Sprintf("√ Detected %d link(s)", len(valid)))
		if invalid > 0 {
			statusText += "  " + lipgloss.NewStyle().Foreground(theme.WarningColor).Render(fmt.Sprintf("(%d invalid)", invalid))
		}
		contentLines = append(contentLines, "  "+statusText)
	} else if invalid > 0 {
		statusText := lipgloss.NewStyle().Foreground(theme.WarningColor).Render(fmt.Sprintf("! %d invalid line(s)", invalid))
		contentLines = append(contentLines, "  "+statusText)
	}

	boxHeight := len(contentLines) + 2
	boxLines := renderPanelBox("Add downloads", contentLines, width, boxHeight, true)

	lines := make([]string, 0, height)
	lines = append(lines, headerLine(appTitle, width))
	lines = append(lines, boxLines...)

	if footer.set {
		lines = append(lines, footer.render(width))
	}
	lines = append(lines, RenderHelpBar(addHelpKeys(), width))

	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
