package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Layout constants shared by the views and their scroll arithmetic.
const (
	listTitleLines  = 1
	listStatusLines = 1
	listBannerLines = 1
	listHelpLines   = 1

	// minVisibleRows keeps at least one download visible on tiny terminals.
	minVisibleRows = 1

	// listHelpMinHeight and listBannerMinHeight drop the optional lines on
	// very short terminals instead of squeezing the download list.
	listHelpMinHeight   = 5
	listBannerMinHeight = 4

	// Row columns; nameMinWidth is the width below which columns are dropped.
	nameMinWidth           = 14
	cursorWidth            = 2
	progressColumnWidth    = 16
	percentColumnWidth     = 7
	speedColumnWidth       = 11
	etaColumnWidth         = 12 // fits "ETA 99:59:59"
	statusColumnWidth      = 9
	connectionsColumnWidth = 10

	// ellipsis is appended by SafeTruncate when the text does not fit.
	ellipsis = "..."

	// noticeNameWidth bounds the file name quoted in a single line notice.
	noticeNameWidth = 40
)

// listGeometry describes how many download rows fit and which optional lines
// are rendered.
type listGeometry struct {
	rows   int
	help   bool
	banner bool
}

// listGeometryFor computes the row budget for a terminal height.
func listGeometryFor(height int, hasBanner bool) listGeometry {
	geometry := listGeometry{}
	if height >= listHelpMinHeight {
		geometry.help = true
	}
	if hasBanner && height >= listBannerMinHeight {
		geometry.banner = true
	}

	chrome := listTitleLines + listStatusLines
	if geometry.help {
		chrome += listHelpLines
	}
	if geometry.banner {
		chrome += listBannerLines
	}
	geometry.rows = max(height-chrome, minVisibleRows)
	return geometry
}

// rowColumns are the optional columns of a download row. Columns are dropped
// (connections, then ETA, then the bar) until the name column is usable.
type rowColumns struct {
	bar         bool
	eta         bool
	connections bool
	nameWidth   int
}

// rowColumnsFor computes the row layout for a terminal width. The fixed width
// is the sum of the rendered columns plus one space between each pair, so a
// row never exceeds the terminal.
func rowColumnsFor(width int) rowColumns {
	columns := rowColumns{bar: true, eta: true, connections: true}
	for {
		// cursor, name, percent, speed and status are always rendered.
		segments := 5
		fixed := cursorWidth + percentColumnWidth + speedColumnWidth + statusColumnWidth

		if columns.bar {
			segments++
			fixed += progressColumnWidth
		}
		if columns.eta {
			segments++
			fixed += etaColumnWidth
		}
		if columns.connections {
			segments++
			fixed += connectionsColumnWidth
		}
		fixed += segments - 1

		if width-fixed >= nameMinWidth {
			columns.nameWidth = width - fixed
			return columns
		}

		switch {
		case columns.connections:
			columns.connections = false
		case columns.eta:
			columns.eta = false
		case columns.bar:
			columns.bar = false
		default:
			columns.nameWidth = max(width-fixed, 1)
			return columns
		}
	}
}

// SafeTruncate truncates s to maxWidth display cells (wide characters count as
// two) without breaking UTF-8, appending an ellipsis when one fits.
func SafeTruncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if maxWidth <= len(ellipsis) {
		return ansi.Truncate(s, maxWidth, "")
	}
	return ansi.Truncate(s, maxWidth, ellipsis)
}

// padRight pads s to width display cells; longer text is truncated so a row
// never exceeds the terminal width.
func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	current := ansi.StringWidth(s)
	if current > width {
		return SafeTruncate(s, width)
	}
	return s + strings.Repeat(" ", width-current)
}

// padLeft right aligns s in width display cells.
func padLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	current := ansi.StringWidth(s)
	if current > width {
		return SafeTruncate(s, width)
	}
	return strings.Repeat(" ", width-current) + s
}

// clampInt limits value to the inclusive range.
func clampInt(value, low, high int) int {
	return max(low, min(value, high))
}
