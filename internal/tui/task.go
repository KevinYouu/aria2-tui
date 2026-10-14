package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/KevinYouu/aria2-tui/internal/aria2"
)

const (
	// unknownName is the placeholder for a download without any name yet.
	unknownName = "(unnamed)"
	// unknownETA is shown while no speed is known, or when nothing is left.
	unknownETA = "—"
	// veryLongETA is shown instead of an unusable multi day duration.
	veryLongETA = ">99h"
)

// displayName returns the name of a download truncated to maxWidth.
func displayName(task aria2.Task, maxWidth int) string {
	name := task.Name
	if name == "" {
		name = unknownName
	}
	return SafeTruncate(name, maxWidth)
}

// progressRatio returns the completed fraction in [0,1]; downloads without a
// known size report 0.
func progressRatio(task aria2.Task) float64 {
	if task.Total == 0 {
		return 0
	}
	return min(float64(task.Completed)/float64(task.Total), 1)
}

// formatETA renders the remaining time from the current speed.
func formatETA(task aria2.Task) string {
	if task.Speed == 0 {
		return unknownETA
	}
	remaining := uint64(0)
	if task.Total > task.Completed {
		remaining = task.Total - task.Completed
	}
	if remaining == 0 {
		return "00:00"
	}
	return formatDuration(time.Duration(remaining/task.Speed) * time.Second)
}

// formatDuration renders a duration as mm:ss, or hh:mm:ss past one hour.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d >= 100*time.Hour {
		return veryLongETA
	}
	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	seconds := (d % time.Minute) / time.Second
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// formatBytes renders a byte count with binary units, for example "12.3 MiB".
func formatBytes(bytes uint64) string {
	const step = 1024
	if bytes < step {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(bytes)
	unit := 0
	for value >= step && unit < len(units) {
		value /= step
		unit++
	}
	return fmt.Sprintf("%.1f %s", value, units[unit-1])
}

// formatSpeed renders a transfer rate, for example "12.3 MiB/s".
func formatSpeed(bytesPerSecond uint64) string {
	return formatBytes(bytesPerSecond) + "/s"
}

// statusLabel returns the label shown for a download state.
func statusLabel(status aria2.Status) string {
	switch status {
	case aria2.StatusActive:
		return "active"
	case aria2.StatusWaiting:
		return "waiting"
	case aria2.StatusPaused:
		return "paused"
	case aria2.StatusComplete:
		return "complete"
	case aria2.StatusError:
		return "error"
	case aria2.StatusRemoved:
		return "removed"
	case "":
		return "unknown"
	default:
		return string(status)
	}
}

// connectionsLabel renders the connection count column.
func connectionsLabel(task aria2.Task) string {
	if task.Connections <= 0 {
		return ""
	}
	return fmt.Sprintf("%d conn", task.Connections)
}

// noticeName is the name used in single line notices: the display name when
// aria2 has resolved one, otherwise the GID, which is all that is known while
// a download is still waiting for its metadata.
func noticeName(task aria2.Task) string {
	name := strings.TrimSpace(task.Name)
	if name == "" {
		return task.GID
	}
	return SafeTruncate(name, noticeNameWidth)
}

// percentLabel renders the completed percentage of a progress ratio.
func percentLabel(ratio float64) string {
	return fmt.Sprintf("%.1f%%", ratio*100)
}
