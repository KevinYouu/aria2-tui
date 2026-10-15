package tui

import (
	"strings"
	"testing"

	"github.com/KevinYouu/aria2-tui/internal/aria2"
	"github.com/charmbracelet/x/ansi"
)

func TestRowColumnsFor(t *testing.T) {
	tests := []struct {
		width           int
		wantBar         bool
		wantETA         bool
		wantConnections bool
	}{
		{width: 140, wantBar: true, wantETA: true, wantConnections: true},
		{width: 100, wantBar: true, wantETA: true, wantConnections: true},
		{width: 80, wantBar: true, wantETA: true, wantConnections: false},
		{width: 70, wantBar: true, wantETA: false, wantConnections: false},
		{width: 50, wantBar: false, wantETA: false, wantConnections: false},
		{width: 30, wantBar: false, wantETA: false, wantConnections: false},
		{width: 10, wantBar: false, wantETA: false, wantConnections: false},
	}

	for _, tt := range tests {
		columns := rowColumnsFor(tt.width)
		if columns.bar != tt.wantBar {
			t.Errorf("rowColumnsFor(%d).bar = %v, want %v", tt.width, columns.bar, tt.wantBar)
		}
		if columns.eta != tt.wantETA {
			t.Errorf("rowColumnsFor(%d).eta = %v, want %v", tt.width, columns.eta, tt.wantETA)
		}
		if columns.connections != tt.wantConnections {
			t.Errorf("rowColumnsFor(%d).connections = %v, want %v", tt.width, columns.connections, tt.wantConnections)
		}
		if columns.nameWidth < 1 {
			t.Errorf("rowColumnsFor(%d).nameWidth = %d, want at least 1", tt.width, columns.nameWidth)
		}
	}
}

func TestSafeTruncate(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		maxWidth int
		want     string
	}{
		{name: "fits", in: "hello", maxWidth: 10, want: "hello"},
		{name: "exact", in: "hello", maxWidth: 5, want: "hello"},
		{name: "adds an ellipsis", in: "hello world", maxWidth: 8, want: "hello..."},
		{name: "no ellipsis when there is no room", in: "hello", maxWidth: 3, want: "hel"},
		{name: "zero width", in: "hello", maxWidth: 0, want: ""},
		{name: "negative width", in: "hello", maxWidth: -1, want: ""},
		{name: "wide characters count as two cells", in: "下载管理器", maxWidth: 6, want: "下..."},
		{name: "wide characters keep the ellipsis when it fits", in: "下载管理器", maxWidth: 5, want: "下..."},
		{name: "wide characters with room left", in: "下载管理器", maxWidth: 8, want: "下载..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SafeTruncate(tt.in, tt.maxWidth)
			if got != tt.want {
				t.Errorf("SafeTruncate(%q, %d) = %q, want %q", tt.in, tt.maxWidth, got, tt.want)
			}
			if tt.maxWidth > 0 {
				if width := ansi.StringWidth(got); width > tt.maxWidth {
					t.Errorf("SafeTruncate(%q, %d) = %q has display width %d", tt.in, tt.maxWidth, got, width)
				}
			}
		})
	}
}

func TestPadRightAndPadLeft(t *testing.T) {
	if got := padRight("ab", 5); got != "ab   " {
		t.Errorf("padRight(ab, 5) = %q, want %q", got, "ab   ")
	}
	if got := padLeft("ab", 5); got != "   ab" {
		t.Errorf("padLeft(ab, 5) = %q, want %q", got, "   ab")
	}
	if got := padRight("abcdef", 3); got != "abc" {
		t.Errorf("padRight(abcdef, 3) = %q, want a plain truncation when the ellipsis cannot fit", got)
	}
	if got := padLeft("abcdef", 3); got != "abc" {
		t.Errorf("padLeft(abcdef, 3) = %q, want a plain truncation when the ellipsis cannot fit", got)
	}
	if got := padRight("", 0); got != "" {
		t.Errorf("padRight(\"\", 0) = %q, want an empty string", got)
	}
}

func TestListGeometryFor(t *testing.T) {
	tests := []struct {
		name       string
		height     int
		hasBanner  bool
		wantRows   int
		wantHelp   bool
		wantBanner bool
	}{
		{name: "typical terminal", height: 24, hasBanner: false, wantRows: 21, wantHelp: true},
		{name: "with a banner", height: 24, hasBanner: true, wantRows: 20, wantHelp: true, wantBanner: true},
		{name: "short terminal keeps the banner and one row", height: 4, hasBanner: true, wantRows: 1, wantBanner: true},
		{name: "shorter terminal drops the banner", height: 3, hasBanner: true, wantRows: 1},
		{name: "very short terminal keeps one row", height: 1, hasBanner: true, wantRows: 1},
		{name: "zero height keeps one row", height: 0, hasBanner: false, wantRows: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geometry := listGeometryFor(tt.height, tt.hasBanner)
			if geometry.rows != tt.wantRows {
				t.Errorf("rows = %d, want %d", geometry.rows, tt.wantRows)
			}
			if geometry.help != tt.wantHelp {
				t.Errorf("help = %v, want %v", geometry.help, tt.wantHelp)
			}
			if geometry.banner != tt.wantBanner {
				t.Errorf("banner = %v, want %v", geometry.banner, tt.wantBanner)
			}
		})
	}
}

func TestListModelCursorFollowsGID(t *testing.T) {
	model := newListModel()
	model.sync([]aria2.Task{{GID: "a"}, {GID: "b"}, {GID: "c"}})

	if model.cursorGID != "a" {
		t.Fatalf("cursorGID = %q, want the first task for an empty selection", model.cursorGID)
	}

	tasks := []aria2.Task{{GID: "a"}, {GID: "b"}, {GID: "c"}}
	model.move(tasks, 1)
	if model.cursorGID != "b" {
		t.Fatalf("cursorGID = %q after moving down, want b", model.cursorGID)
	}

	model.move(tasks, -5)
	if model.cursorGID != "a" {
		t.Errorf("cursorGID = %q at the top, want it clamped to a", model.cursorGID)
	}

	model.move(tasks, 9)
	if model.cursorGID != "c" {
		t.Errorf("cursorGID = %q at the bottom, want it clamped to c", model.cursorGID)
	}

	// The selected download disappeared: fall back to the first row.
	model.sync([]aria2.Task{{GID: "a"}, {GID: "d"}})
	if model.cursorGID != "a" {
		t.Errorf("cursorGID = %q after the selection was removed, want a", model.cursorGID)
	}
}

func TestListModelScrollsToKeepTheCursorVisible(t *testing.T) {
	model := newListModel()

	// Cursor below the window.
	model.offset = 0
	model.scrollTo(9, 5, 20)
	if model.offset != 5 {
		t.Errorf("offset = %d with the cursor at 9 and 5 rows, want 5", model.offset)
	}

	// Cursor above the window.
	model.offset = 8
	model.scrollTo(2, 5, 20)
	if model.offset != 2 {
		t.Errorf("offset = %d with the cursor at 2, want 2", model.offset)
	}

	// Cursor inside the window keeps the offset.
	model.offset = 5
	model.scrollTo(7, 5, 20)
	if model.offset != 5 {
		t.Errorf("offset = %d with the cursor already visible, want 5", model.offset)
	}

	// The window cannot scroll past the end.
	model.offset = 0
	model.scrollTo(19, 5, 20)
	if model.offset != 15 {
		t.Errorf("offset = %d at the end of a 20 row list with 5 rows, want 15", model.offset)
	}

	// Degenerate inputs.
	model.offset = 7
	model.scrollTo(0, 0, 0)
	if model.offset != 0 {
		t.Errorf("offset = %d for an empty list, want 0", model.offset)
	}
}

func TestListViewRendersTaskRow(t *testing.T) {
	model := newListModel()
	tasks := []aria2.Task{{
		GID:         "a1",
		Name:        "ubuntu.iso",
		Status:      aria2.StatusActive,
		Total:       1048576,
		Completed:   524288,
		Speed:       128 * 1024,
		Connections: 8,
	}}
	model.sync(tasks)

	view := ansi.Strip(model.view(listView{
		tasks:          tasks,
		stat:           aria2.GlobalStat{DownloadSpeed: 2048, NumActive: 1},
		width:          140,
		height:         24,
		maxConnections: 5,
		aria2Version:   "1.37.0",
	}))

	for _, want := range []string{
		"ubuntu.iso",
		"50.0%",
		"128.0 KiB/s",
		"ETA",
		"active",
		"8 conn",
		"5 connections",
		"↓ 2.0 KiB/s",
		"1 active · 0 waiting · 0 done",
		"aria2c 1.37.0",
		"[q] quit",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestListViewRendersPlaceholderWhenEmpty(t *testing.T) {
	model := newListModel()
	view := ansi.Strip(model.view(listView{width: 80, height: 24, aria2Version: "1.37.0"}))

	if !strings.Contains(view, "No downloads yet") {
		t.Errorf("view = %q, want an empty list placeholder", view)
	}
}

func TestListViewRendersFooter(t *testing.T) {
	model := newListModel()
	view := ansi.Strip(model.view(listView{
		width:  100,
		height: 24,
		footer: footerLine{text: "aria2 error 1: Unauthorized", kind: bannerError, set: true},
	}))

	if !strings.Contains(view, "Unauthorized") {
		t.Errorf("view = %q, want the footer message", view)
	}
}

func TestListViewFitsTheTerminalHeight(t *testing.T) {
	model := newListModel()
	tasks := make([]aria2.Task, 0, 40)
	for i := range 40 {
		tasks = append(tasks, aria2.Task{GID: string(rune('a' + i%26)), Status: aria2.StatusActive, Total: 100, Completed: 50})
	}
	model.sync(tasks)

	for _, height := range []int{1, 3, 6, 12, 24} {
		view := model.view(listView{
			tasks:        tasks,
			width:        100,
			height:       height,
			footer:       footerLine{text: "notice", kind: bannerInfo, set: true},
			aria2Version: "1.37.0",
		})
		if lines := strings.Count(view, "\n") + 1; lines > max(height, 1) {
			t.Errorf("height %d rendered %d lines, want at most %d", height, lines, height)
		}
	}
}

func TestRowFitsTheTerminalWidth(t *testing.T) {
	model := newListModel()
	tasks := []aria2.Task{{
		GID:         "a1",
		Name:        "a-very-long-download-name-that-will-not-fit.iso",
		Status:      aria2.StatusActive,
		Total:       1048576,
		Completed:   524288,
		Speed:       1024 * 1024 * 1024,
		Connections: 16,
	}}
	model.sync(tasks)

	for _, width := range []int{20, 40, 60, 80, 100, 200} {
		view := ansi.Strip(model.view(listView{tasks: tasks, width: width, height: 24, aria2Version: "1.37.0"}))
		for line := range strings.SplitSeq(view, "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Errorf("width %d: line width %d exceeds the terminal:\n%s", width, got, line)
			}
		}
	}
}

func TestDashboardRendersSplitPanesWhenWide(t *testing.T) {
	model := newListModel()
	tasks := []aria2.Task{{
		GID:         "gid12345",
		Name:        "ubuntu-24.04.iso",
		Status:      aria2.StatusActive,
		Total:       2 * 1024 * 1024 * 1024,
		Completed:   1 * 1024 * 1024 * 1024,
		Speed:       8 * 1024 * 1024,
		Connections: 8,
	}}
	model.sync(tasks)

	view := ansi.Strip(model.view(listView{
		tasks:          tasks,
		stat:           aria2.GlobalStat{DownloadSpeed: 8 * 1024 * 1024, NumActive: 1},
		width:          100,
		height:         24,
		maxConnections: 5,
		aria2Version:   "1.37.0",
		downloadDir:    "/home/user/Downloads",
		view:           viewList,
	}))

	for _, want := range []string{
		"Downloads (1)",
		"Task Details",
		"ubuntu-24.04.iso",
		"gid12345",
		"50.0%",
		"8.0 MiB/s",
		"8 conn",
		"/home/user/Downloads",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("split dashboard view missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardRendersAddWorkbench(t *testing.T) {
	model := newListModel()
	add := newAddModel()
	add.input.SetValue("https://example.com/file1.iso\nhttps://example.com/file2.iso")

	tasks := []aria2.Task{{
		GID:    "gid1",
		Name:   "existing.iso",
		Status: aria2.StatusActive,
	}}
	model.sync(tasks)

	view := ansi.Strip(model.view(listView{
		tasks:        tasks,
		width:        100,
		height:       24,
		downloadDir:  "/home/user/Downloads",
		view:         viewAdd,
		add:          add,
		aria2Version: "1.37.0",
	}))

	for _, want := range []string{
		"Downloads (1)",
		"Add downloads",
		"https://example.com/file1.iso",
		"Detected 2 link(s)",
		"[ctrl+s] download",
		"[esc] back",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("add workbench view missing %q:\n%s", want, view)
		}
	}

	// When empty, verify placeholder is shown in the workbench
	emptyAdd := newAddModel()
	emptyView := ansi.Strip(model.view(listView{
		tasks:        tasks,
		width:        100,
		height:       24,
		view:         viewAdd,
		add:          emptyAdd,
		aria2Version: "1.37.0",
	}))
	if !strings.Contains(emptyView, "Paste one link per line") {
		t.Errorf("empty add workbench view missing placeholder:\n%s", emptyView)
	}
}

func TestDashboardTaskDetailsWithError(t *testing.T) {
	model := newListModel()
	tasks := []aria2.Task{{
		GID:          "err_gid",
		Name:         "broken.iso",
		Status:       aria2.StatusError,
		ErrorCode:    3,
		ErrorMessage: "Resource not found",
	}}
	model.sync(tasks)

	view := ansi.Strip(model.view(listView{
		tasks:        tasks,
		width:        100,
		height:       24,
		view:         viewList,
		aria2Version: "1.37.0",
	}))

	if !strings.Contains(view, "Code 3: Resource not found") {
		t.Errorf("view missing error diagnostics:\n%s", view)
	}
}

func TestNarrowLayoutIsCompactWithoutExcessiveBlankLines(t *testing.T) {
	model := newListModel()
	tasks := []aria2.Task{
		{GID: "g1", Name: "ubuntu.iso", Status: aria2.StatusActive, Total: 100, Completed: 50},
		{GID: "g2", Name: "debian.iso", Status: aria2.StatusComplete, Total: 200, Completed: 200},
	}
	model.sync(tasks)

	// Narrow terminal: width 60, height 24
	view := ansi.Strip(model.view(listView{
		tasks:        tasks,
		width:        60,
		height:       24,
		view:         viewList,
		aria2Version: "1.37.0",
	}))

	// Verify it renders card box titles for BOTH top (Downloads) and bottom (Task Details)
	if !strings.Contains(view, "Downloads (2)") {
		t.Errorf("narrow view missing top card box title:\n%s", view)
	}
	if !strings.Contains(view, "Task Details") {
		t.Errorf("narrow view missing bottom card box title:\n%s", view)
	}

	// Verify line count is compact and does not fill entire 24 rows with empty blanks.
	// Two stacked cards with borders + title + status + help takes 13 compact lines.
	lines := strings.Split(view, "\n")
	if len(lines) > 14 {
		t.Errorf("narrow view with 2 tasks rendered %d lines, expected <= 14 compact lines:\n%s", len(lines), view)
	}

	// Also verify narrow add view in dashboard mode (vertical stacked: Top Downloads + Bottom Add downloads)
	add := newAddModel()
	narrowAddDashboard := ansi.Strip(model.view(listView{
		tasks:        tasks,
		width:        60,
		height:       24,
		view:         viewAdd,
		add:          add,
		aria2Version: "1.37.0",
	}))
	if !strings.Contains(narrowAddDashboard, "Downloads (2)") {
		t.Errorf("narrow add dashboard missing top card:\n%s", narrowAddDashboard)
	}
	if !strings.Contains(narrowAddDashboard, "Add downloads") {
		t.Errorf("narrow add dashboard missing bottom card:\n%s", narrowAddDashboard)
	}
	narrowAddLines := strings.Split(narrowAddDashboard, "\n")
	if len(narrowAddLines) > 14 {
		t.Errorf("narrow add dashboard rendered %d lines, expected <= 14 compact lines:\n%s", len(narrowAddLines), narrowAddDashboard)
	}
}

func TestTabSwitchingDoesNotCauseLayoutJitter(t *testing.T) {
	model := newListModel()
	add := newAddModel()

	scenarios := []struct {
		name   string
		width  int
		height int
		tasks  []aria2.Task
	}{
		{
			name:   "wide_screen_empty",
			width:  100,
			height: 24,
			tasks:  nil,
		},
		{
			name:   "wide_screen_with_tasks",
			width:  100,
			height: 24,
			tasks: []aria2.Task{
				{GID: "g1", Name: "ubuntu.iso", Status: aria2.StatusActive, Total: 100, Completed: 50},
			},
		},
		{
			name:   "narrow_screen_empty",
			width:  60,
			height: 24,
			tasks:  nil,
		},
		{
			name:   "narrow_screen_with_tasks",
			width:  60,
			height: 24,
			tasks: []aria2.Task{
				{GID: "g1", Name: "ubuntu.iso", Status: aria2.StatusActive, Total: 100, Completed: 50},
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			model.sync(sc.tasks)

			listViewOutput := ansi.Strip(model.view(listView{
				tasks:        sc.tasks,
				width:        sc.width,
				height:       sc.height,
				view:         viewList,
				add:          add,
				aria2Version: "1.37.0",
			}))

			addViewOutput := ansi.Strip(model.view(listView{
				tasks:        sc.tasks,
				width:        sc.width,
				height:       sc.height,
				view:         viewAdd,
				add:          add,
				aria2Version: "1.37.0",
			}))

			listLines := strings.Split(listViewOutput, "\n")
			addLines := strings.Split(addViewOutput, "\n")

			if len(listLines) != len(addLines) {
				t.Errorf("[%s] line count jitter detected on tab switch: list has %d lines, add has %d lines\nList view:\n%s\nAdd view:\n%s",
					sc.name, len(listLines), len(addLines), listViewOutput, addViewOutput)
			}
		})
	}
}
