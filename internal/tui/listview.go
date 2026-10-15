package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/KevinYouu/aria2-tui/internal/aria2"
	"github.com/charmbracelet/x/ansi"
)

// listAction is what the download list asks the app to do after a key press.
type listAction int

const (
	listNoop listAction = iota
	listQuit
	listAddLinks
	listTogglePause
	listRemove
	listMoreConnections
	listFewerConnections
	listSwitchPane
	listSettings
)

// listModel is the download list state: the selected download and the first
// visible row. The selection is tracked by GID, so it survives reordering.
type listModel struct {
	cursorGID string
	offset    int
}

func newListModel() *listModel {
	return &listModel{}
}

// listView is everything the list rendering depends on.
type listView struct {
	tasks          []aria2.Task
	stat           aria2.GlobalStat
	width          int
	height         int
	maxConnections int
	aria2Version   string
	footer         footerLine
	downloadDir    string
	view           viewKind
	add            *addModel
}

// update applies a key press and reports the app level action it maps to.
func (m *listModel) update(msg tea.KeyPressMsg, tasks []aria2.Task, rows int) listAction {
	switch {
	case key.Matches(msg, keyUp):
		m.move(tasks, -1)
	case key.Matches(msg, keyDown):
		m.move(tasks, 1)
	case key.Matches(msg, keyPageUp):
		m.move(tasks, -rows)
	case key.Matches(msg, keyPageDown):
		m.move(tasks, rows)
	case key.Matches(msg, keyTogglePause):
		return listTogglePause
	case key.Matches(msg, keyRemove):
		return listRemove
	case key.Matches(msg, keyMoreConnections):
		return listMoreConnections
	case key.Matches(msg, keyFewerConnections):
		return listFewerConnections
	case key.Matches(msg, keySwitchPane):
		return listSwitchPane
	case key.Matches(msg, keyAddLinks):
		return listAddLinks
	case key.Matches(msg, keySettings):
		return listSettings
	case key.Matches(msg, keyQuitList):
		return listQuit
	}
	return listNoop
}

// sync keeps the selection pointing at the same download: when it is gone the
// first row is selected.
func (m *listModel) sync(tasks []aria2.Task) {
	if len(tasks) == 0 {
		m.cursorGID = ""
		m.offset = 0
		return
	}
	if indexByGID(tasks, m.cursorGID) < 0 {
		m.cursorGID = tasks[0].GID
	}
}

// selected returns the download under the cursor.
func (m *listModel) selected(tasks []aria2.Task) (aria2.Task, bool) {
	index := indexByGID(tasks, m.cursorGID)
	if index < 0 {
		return aria2.Task{}, false
	}
	return tasks[index], true
}

// move shifts the selection by delta rows, clamped to the list.
func (m *listModel) move(tasks []aria2.Task, delta int) {
	if len(tasks) == 0 {
		return
	}
	index := max(indexByGID(tasks, m.cursorGID), 0)
	m.cursorGID = tasks[clampInt(index+delta, 0, len(tasks)-1)].GID
}

// scrollTo puts the cursor row inside the visible window.
func (m *listModel) scrollTo(cursor, rows, total int) {
	if rows <= 0 || total <= 0 {
		m.offset = 0
		return
	}
	cursor = clampInt(cursor, 0, total-1)
	if cursor < m.offset {
		m.offset = cursor
	}
	if cursor >= m.offset+rows {
		m.offset = cursor - rows + 1
	}
	m.offset = clampInt(m.offset, 0, max(total-rows, 0))
}

func indexByGID(tasks []aria2.Task, gid string) int {
	if gid == "" {
		return -1
	}
	for i, task := range tasks {
		if task.GID == gid {
			return i
		}
	}
	return -1
}

// view renders the title, the download rows, the status line and the footer.
func (m *listModel) view(in listView) string {
	geometry := listGeometryFor(in.height, in.footer.set)

	tasks := in.tasks
	cursor := max(indexByGID(tasks, m.cursorGID), 0)

	lines := make([]string, 0, in.height)
	lines = append(lines, headerLine(appTitle, in.width))

	if in.width >= splitMinTerminalWidth && in.height >= splitMinTerminalHeight {
		// Two-column split dashboard layout:
		// 1. Content-driven compact width: left column fits cursor, status icon,
		// reasonable name length, percentage and speed comfortably without bloat.
		leftWidth := clampInt(in.width*42/100, 36, 48)
		rightWidth := in.width - leftWidth

		// 2. Stable compact height: independent of in.view (viewList vs viewAdd)
		// to eliminate layout jitter when switching pane with Tab.
		var idealContentRows int
		if len(tasks) == 0 {
			idealContentRows = 6
		} else {
			idealContentRows = clampInt(max(len(tasks), 11), 6, max(geometry.rows-2, 6))
		}
		idealPanelHeight := idealContentRows + 2 // include top and bottom borders

		// Bound by available geometry budget from terminal window
		panelHeight := clampInt(idealPanelHeight, splitMinTerminalHeight, geometry.rows)

		innerRows := max(panelHeight-2, 1)
		m.scrollTo(cursor, innerRows, len(tasks))

		// Left: Task list
		leftContent := make([]string, 0, innerRows)
		if len(tasks) == 0 {
			leftContent = append(leftContent, "", "  "+mutedStyle().Render("No downloads yet — press a to add links"))
		} else {
			for i := m.offset; i < len(tasks) && i < m.offset+innerRows; i++ {
				leftContent = append(leftContent, renderLeftTaskRow(tasks[i], i == cursor, leftWidth-2))
			}
		}
		leftTitle := fmt.Sprintf("Downloads (%d)", len(tasks))
		leftBox := renderPanelBox(leftTitle, leftContent, leftWidth, panelHeight, in.view != viewAdd)

		// Right: Task details or Add workbench
		var rightBox []string
		if in.view == viewAdd && in.add != nil {
			rightContent := renderAddWorkbench(in.add, in.downloadDir, rightWidth-2, panelHeight-2)
			rightBox = renderPanelBox("Add downloads", rightContent, rightWidth, panelHeight, true)
		} else {
			var selectedTask *aria2.Task
			if t, ok := m.selected(tasks); ok {
				selectedTask = &t
			}
			rightContent := renderTaskDetails(selectedTask, in.downloadDir, rightWidth-2, panelHeight-2)
			rightTitle := "Task Details"
			if selectedTask != nil {
				rightTitle = "Task Details: " + displayName(*selectedTask, max(rightWidth-20, 10))
			}
			rightBox = renderPanelBox(rightTitle, rightContent, rightWidth, panelHeight, false)
		}

		splitRows := joinPanelLines(leftBox, rightBox, panelHeight)
		lines = append(lines, splitRows...)
	} else {
		// Narrow terminal: Two-pane vertical stacked layout (Top: Downloads, Bottom: Details/Add Workbench)
		// 1. Available height calculation:
		chrome := 2 // appTitle header + statusLine
		if geometry.banner {
			chrome++
		}
		if geometry.help {
			chrome++
		}
		availableH := max(in.height-chrome, 6)

		// 2. Top pane (Downloads list):
		leftDesired := max(len(tasks), 1)
		maxTop := clampInt(availableH*45/100, 3, 7)
		topHeight := clampInt(leftDesired+2, 3, maxTop)

		innerRows := max(topHeight-2, 1)
		m.scrollTo(cursor, innerRows, len(tasks))

		topContent := make([]string, 0, innerRows)
		if len(tasks) == 0 {
			topContent = append(topContent, "  "+mutedStyle().Render("No downloads yet"))
		} else {
			for i := m.offset; i < len(tasks) && i < m.offset+innerRows; i++ {
				topContent = append(topContent, renderLeftTaskRow(tasks[i], i == cursor, in.width-2))
			}
		}
		topTitle := fmt.Sprintf("Downloads (%d)", len(tasks))
		topBox := renderPanelBox(topTitle, topContent, in.width, topHeight, in.view != viewAdd)
		lines = append(lines, topBox...)

		// 3. Bottom pane (Task Details or Add Workbench):
		// Stable height independent of in.view to eliminate layout jitter on Tab.
		remainingH := max(availableH-topHeight, 3)
		var bottomDesired int
		if len(tasks) == 0 {
			bottomDesired = 5
		} else {
			bottomDesired = 6
		}
		bottomHeight := clampInt(bottomDesired, 3, remainingH)

		if in.view == viewAdd && in.add != nil {
			addContent := renderAddWorkbench(in.add, in.downloadDir, in.width-2, bottomHeight-2)
			bottomBox := renderPanelBox("Add downloads", addContent, in.width, bottomHeight, true)
			lines = append(lines, bottomBox...)
		} else {
			var selectedTask *aria2.Task
			if t, ok := m.selected(tasks); ok {
				selectedTask = &t
			}
			detailContent := renderTaskDetails(selectedTask, in.downloadDir, in.width-2, bottomHeight-2)
			detailTitle := "Task Details"
			if selectedTask != nil {
				detailTitle = "Task Details: " + displayName(*selectedTask, max(in.width-20, 8))
			}
			detailBox := renderPanelBox(detailTitle, detailContent, in.width, bottomHeight, false)
			lines = append(lines, detailBox...)
		}
	}

	lines = append(lines, m.statusLine(in))
	if geometry.banner {
		lines = append(lines, in.footer.render(in.width))
	}
	if geometry.help {
		helpKeys := listHelpKeys()
		if in.view == viewAdd {
			helpKeys = addHelpKeys()
		}
		lines = append(lines, RenderHelpBar(helpKeys, in.width))
	}
	if in.height > 0 && len(lines) > in.height {
		lines = lines[:in.height]
	}
	return strings.Join(lines, "\n")
}

// taskRow renders one download as fixed width columns.
func (m *listModel) taskRow(width int, task aria2.Task, selected bool) string {
	columns := rowColumnsFor(width)

	cursor := "  "
	if selected {
		cursor = cursorStyle().Render("►") + " "
	}
	segments := []string{cursor, nameStyle(selected).Render(padRight(displayName(task, columns.nameWidth), columns.nameWidth))}

	ratio := progressRatio(task)
	if columns.bar {
		bar := newProgressBar()
		bar.SetWidth(progressColumnWidth)
		segments = append(segments, bar.ViewAs(ratio))
	}
	segments = append(segments, mutedStyle().Render(padLeft(percentLabel(ratio), percentColumnWidth)))
	segments = append(segments, mutedStyle().Render(padLeft(formatSpeed(task.Speed), speedColumnWidth)))
	if columns.eta {
		segments = append(segments, mutedStyle().Render(padLeft("ETA "+formatETA(task), etaColumnWidth)))
	}
	segments = append(segments, statusStyle(task.Status).Render(padLeft(statusLabel(task.Status), statusColumnWidth)))
	if columns.connections {
		segments = append(segments, mutedStyle().Render(padLeft(connectionsLabel(task), connectionsColumnWidth)))
	}

	row := strings.Join(segments, " ")
	// A terminal narrower than the fixed columns cannot fit a row; cut it
	// instead of letting the renderer wrap.
	if ansi.StringWidth(row) > width {
		row = ansi.Truncate(row, width, "")
	}
	return row
}

// statusLine renders the daemon wide counters below the list.
func (m *listModel) statusLine(in listView) string {
	parts := []string{
		fmt.Sprintf("%d connections", in.maxConnections),
		"↓ " + formatSpeed(in.stat.DownloadSpeed),
		"↑ " + formatSpeed(in.stat.UploadSpeed),
		fmt.Sprintf("%d active · %d waiting · %d done", in.stat.NumActive, in.stat.NumWaiting, in.stat.NumStopped),
	}
	if in.downloadDir != "" {
		parts = append(parts, "dir: "+in.downloadDir)
	}
	parts = append(parts, "aria2c "+in.aria2Version)
	return SafeTruncate(mutedStyle().Render(strings.Join(parts, "  ")), in.width)
}
