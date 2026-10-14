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
	case key.Matches(msg, keyAddLinks):
		return listAddLinks
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
	index := indexByGID(tasks, m.cursorGID)
	if index < 0 {
		index = 0
	}
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
	cursor := indexByGID(tasks, m.cursorGID)
	if cursor < 0 {
		cursor = 0
	}
	m.scrollTo(cursor, geometry.rows, len(tasks))

	lines := make([]string, 0, in.height)
	lines = append(lines, headerLine(appTitle, in.width))

	if len(tasks) == 0 {
		lines = append(lines, mutedStyle().Render(SafeTruncate("No downloads yet — press a to add links", in.width)))
	} else {
		for i := m.offset; i < len(tasks) && i < m.offset+geometry.rows; i++ {
			lines = append(lines, m.taskRow(in.width, tasks[i], i == cursor))
		}
	}

	lines = append(lines, m.statusLine(in))
	if geometry.banner {
		lines = append(lines, in.footer.render(in.width))
	}
	if geometry.help {
		lines = append(lines, RenderHelpBar(listHelpKeys(), in.width))
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
		cursor = cursorStyle().Render("❯") + " "
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
		"aria2c " + in.aria2Version,
	}
	return SafeTruncate(mutedStyle().Render(strings.Join(parts, "  ")), in.width)
}
