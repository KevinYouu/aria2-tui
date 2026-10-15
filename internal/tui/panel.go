package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/KevinYouu/aria2-tui/internal/aria2"
	"github.com/KevinYouu/aria2-tui/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

// splitMinTerminalWidth is the width threshold for enabling the two-column dashboard layout.
const splitMinTerminalWidth = 80

// splitMinTerminalHeight is the height threshold for enabling the two-column dashboard layout.
const splitMinTerminalHeight = 6

// renderPanelBox renders a titled unicode box around contentLines.
func renderPanelBox(title string, contentLines []string, width, height int, focused bool) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	if height < 3 {
		res := make([]string, 0, max(height, 1))
		for i := 0; i < height && i < len(contentLines); i++ {
			res = append(res, SafeTruncate(contentLines[i], width))
		}
		for len(res) < height {
			res = append(res, strings.Repeat(" ", width))
		}
		return res
	}

	borderColor := theme.BorderColor
	if focused {
		borderColor = theme.PrimaryColor
	}
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	innerWidth := width - 2
	topLine := renderTopBorder(title, innerWidth, borderStyle, focused)

	innerRows := height - 2
	lines := make([]string, 0, height)
	lines = append(lines, topLine)

	leftBorder := borderStyle.Render("│")
	rightBorder := borderStyle.Render("│")

	for i := range innerRows {
		line := ""
		if i < len(contentLines) {
			line = contentLines[i]
		}
		lines = append(lines, leftBorder+padContent(line, innerWidth)+rightBorder)
	}

	bottomRule := strings.Repeat("─", max(innerWidth, 0))
	bottomLine := borderStyle.Render("└" + bottomRule + "┘")
	lines = append(lines, bottomLine)

	return lines
}

func renderTopBorder(title string, innerWidth int, borderStyle lipgloss.Style, focused bool) string {
	if innerWidth <= 0 {
		return borderStyle.Render("┌┐")
	}
	if title == "" {
		return borderStyle.Render("┌" + strings.Repeat("─", innerWidth) + "┐")
	}

	titleColor := theme.MutedForeground
	if focused {
		titleColor = theme.PrimaryColor
	}
	titleStyled := lipgloss.NewStyle().Foreground(titleColor).Bold(true).Render(title)

	titleW := ansi.StringWidth(title)
	if titleW+4 > innerWidth {
		maxTitleW := max(innerWidth-4, 1)
		title = SafeTruncate(title, maxTitleW)
		titleW = ansi.StringWidth(title)
		titleStyled = lipgloss.NewStyle().Foreground(titleColor).Bold(true).Render(title)
	}

	remain := max(innerWidth-titleW-3, 0)
	prefix := borderStyle.Render("┌─ ")
	suffix := borderStyle.Render(" " + strings.Repeat("─", remain) + "┐")
	return prefix + titleStyled + suffix
}

func padContent(line string, width int) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(line)
	if w > width {
		return SafeTruncate(line, width)
	}
	return line + strings.Repeat(" ", width-w)
}

// renderLeftTaskRow renders one download item for the left queue list.
func renderLeftTaskRow(task aria2.Task, selected bool, width int) string {
	if width <= 0 {
		return ""
	}

	cursor := "  "
	if selected {
		cursor = cursorStyle().Render("❯") + " "
	}

	icon := statusStyle(task.Status).Render(statusIcon(task.Status)) + " "

	ratio := progressRatio(task)
	percent := padLeft(percentLabel(ratio), 6)

	speed := ""
	if task.Status == aria2.StatusActive && task.Speed > 0 {
		speed = " " + padLeft(formatSpeed(task.Speed), 11)
	} else if task.Status == aria2.StatusComplete {
		speed = " " + padLeft(statusStyle(task.Status).Render("done"), 11)
	} else if task.Status == aria2.StatusPaused {
		speed = " " + padLeft(statusStyle(task.Status).Render("paused"), 11)
	} else if task.Status == aria2.StatusError {
		speed = " " + padLeft(statusStyle(task.Status).Render("error"), 11)
	} else {
		speed = " " + padLeft(mutedStyle().Render(statusLabel(task.Status)), 11)
	}

	fixedWidth := ansi.StringWidth(cursor) + ansi.StringWidth(icon) + ansi.StringWidth(percent) + ansi.StringWidth(speed) + 1
	nameW := max(width-fixedWidth, 4)

	name := nameStyle(selected).Render(padRight(displayName(task, nameW), nameW))
	row := cursor + icon + name + " " + mutedStyle().Render(percent) + speed

	if ansi.StringWidth(row) > width {
		row = SafeTruncate(row, width)
	}
	return row
}

// renderTaskDetails generates detail lines for the right pane inspector.
func renderTaskDetails(task *aria2.Task, downloadDir string, width, height int) []string {
	if task == nil {
		lines := []string{}
		if height > 4 {
			lines = append(lines, "")
		}
		lines = append(lines, "  "+mutedStyle().Render("No active downloads"))
		return lines
	}

	ratio := progressRatio(*task)
	lines := make([]string, 0, max(height, 12))

	formatField := func(label, value string) string {
		labelCol := padRight("  "+label, 12)
		return mutedStyle().Render(labelCol) + value
	}

	// 1. Name
	lines = append(lines, formatField("Name", titleStyle().Render(displayName(*task, max(width-14, 8)))))

	// 2. Status
	statusVal := statusStyle(task.Status).Render(statusIcon(task.Status)+" "+statusLabel(task.Status)) + " " +
		badgeStyle(task.Status).Render("["+strings.ToUpper(statusLabel(task.Status))+"]")
	lines = append(lines, formatField("Status", statusVal))

	// For tight vertical space (e.g. narrow vertical stacked layout), combine fields
	if height <= 6 {
		sizeText := fmt.Sprintf("%s / %s (%s)", formatBytes(task.Completed), formatBytes(task.Total), percentLabel(ratio))
		lines = append(lines, formatField("Progress", sizeText))

		speedEta := "↓ " + formatSpeed(task.Speed) + "  ETA " + formatETA(*task)
		lines = append(lines, formatField("Speed", speedEta))

		connVal := connectionsLabel(*task)
		if connVal == "" {
			connVal = "0 conn"
		}
		lines = append(lines, formatField("Connections", connVal))

		if task.ErrorCode != 0 || task.ErrorMessage != "" {
			errText := fmt.Sprintf("Code %d: %s", task.ErrorCode, task.ErrorMessage)
			lines = append(lines, formatField("Error", lipgloss.NewStyle().Foreground(theme.ErrorColor).Bold(true).Render(errText)))
		}
		return lines
	}

	// 3. GID
	lines = append(lines, formatField("GID", mutedStyle().Render(task.GID)))

	// 4. Progress bar
	barW := clampInt(width-28, 8, 24)
	bar := newProgressBar()
	bar.SetWidth(barW)
	progressVal := bar.ViewAs(ratio) + " " + mutedStyle().Render(percentLabel(ratio))
	lines = append(lines, formatField("Progress", progressVal))

	// 5. Size
	sizeText := fmt.Sprintf("%s / %s", formatBytes(task.Completed), formatBytes(task.Total))
	if task.Total > task.Completed && task.Total > 0 {
		sizeText += fmt.Sprintf(" (remaining: %s)", formatBytes(task.Total-task.Completed))
	}
	lines = append(lines, formatField("Size", sizeText))

	// 6. Transfer speed & ETA
	speedVal := "↓ " + formatSpeed(task.Speed)
	lines = append(lines, formatField("Speed", speedVal))

	etaVal := "ETA " + formatETA(*task)
	lines = append(lines, formatField("ETA", etaVal))

	// 7. Connections
	connVal := connectionsLabel(*task)
	if connVal == "" {
		connVal = "0 conn"
	}
	lines = append(lines, formatField("Connections", connVal))

	// 8. Target Directory
	targetDir := downloadDir
	if targetDir == "" {
		targetDir = "~/Downloads"
	}
	lines = append(lines, formatField("Save Dir", SafeTruncate(targetDir, max(width-16, 10))))

	// 9. Error info
	if task.ErrorCode != 0 || task.ErrorMessage != "" {
		errText := fmt.Sprintf("Code %d: %s", task.ErrorCode, task.ErrorMessage)
		lines = append(lines, formatField("Error", lipgloss.NewStyle().Foreground(theme.ErrorColor).Bold(true).Render(errText)))
	} else {
		lines = append(lines, formatField("Diagnostics", theme.InfoStyle.Render("None (Healthy)")))
	}

	return lines
}

// renderAddWorkbench generates lines for the right pane in viewAdd mode.
func renderAddWorkbench(add *addModel, downloadDir string, width, height int) []string {
	if add == nil {
		return nil
	}

	// Keep textarea compact: bounded width and height to avoid empty sprawl
	innerW := clampInt(width-4, 16, 52)
	textareaH := clampInt(height-3, 2, 4)
	add.input.SetWidth(innerW)
	add.input.SetHeight(textareaH)

	lines := make([]string, 0, max(height, 8))
	if height > 4 {
		lines = append(lines, "")
	}

	// Append textarea rows
	inputView := add.input.View()
	textRows := strings.Split(inputView, "\n")
	for i := 0; i < len(textRows) && i < textareaH; i++ {
		lines = append(lines, "  "+textRows[i])
	}

	// Real-time link diagnostics (only shown when links are detected or invalid)
	valid, invalid := add.links()
	if len(valid) > 0 {
		statusText := lipgloss.NewStyle().Foreground(theme.SuccessColor).Render(fmt.Sprintf("✓ Detected %d link(s)", len(valid)))
		if invalid > 0 {
			statusText += "  " + lipgloss.NewStyle().Foreground(theme.WarningColor).Render(fmt.Sprintf("(%d invalid)", invalid))
		}
		lines = append(lines, "", "  "+statusText)
	} else if invalid > 0 {
		statusText := lipgloss.NewStyle().Foreground(theme.WarningColor).Render(fmt.Sprintf("⚠ %d invalid line(s)", invalid))
		lines = append(lines, "", "  "+statusText)
	}

	return lines
}

// joinPanelLines horizontally combines two sets of lines.
func joinPanelLines(leftLines, rightLines []string, height int) []string {
	rows := max(len(leftLines), len(rightLines))
	rows = max(rows, height)
	res := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}
		res = append(res, left+right)
	}
	return res
}
