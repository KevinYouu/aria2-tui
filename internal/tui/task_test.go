package tui

import (
	"testing"
	"time"

	"github.com/KevinYouu/aria2-tui/internal/aria2"
)

func TestProgressRatio(t *testing.T) {
	tests := []struct {
		name string
		task aria2.Task
		want float64
	}{
		{name: "unknown total", task: aria2.Task{Completed: 10}, want: 0},
		{name: "half way", task: aria2.Task{Total: 200, Completed: 100}, want: 0.5},
		{name: "complete", task: aria2.Task{Total: 200, Completed: 200}, want: 1},
		{name: "clamped when aria2 overshoots", task: aria2.Task{Total: 200, Completed: 260}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := progressRatio(tt.task); got != tt.want {
				t.Errorf("progressRatio(%+v) = %v, want %v", tt.task, got, tt.want)
			}
		})
	}
}

func TestFormatETA(t *testing.T) {
	tests := []struct {
		name string
		task aria2.Task
		want string
	}{
		{name: "no speed yet", task: aria2.Task{Total: 1000, Completed: 0, Speed: 0}, want: unknownETA},
		{name: "seconds to minutes", task: aria2.Task{Total: 620, Completed: 0, Speed: 10}, want: "01:02"},
		{name: "hours", task: aria2.Task{Total: 7200, Completed: 0, Speed: 1}, want: "02:00:00"},
		{name: "nothing left", task: aria2.Task{Total: 100, Completed: 100, Speed: 0}, want: unknownETA},
		{name: "sub second remainder rounds up to a second", task: aria2.Task{Total: 100, Completed: 99, Speed: 100}, want: "00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatETA(tt.task); got != tt.want {
				t.Errorf("formatETA(%+v) = %q, want %q", tt.task, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{in: 0, want: "00:00"},
		{in: 31 * time.Second, want: "00:31"},
		{in: 90 * time.Second, want: "01:30"},
		{in: time.Hour + 2*time.Minute + 3*time.Second, want: "01:02:03"},
		{in: 200 * time.Hour, want: veryLongETA},
		{in: -time.Second, want: "00:00"},
	}

	for _, tt := range tests {
		if got := formatDuration(tt.in); got != tt.want {
			t.Errorf("formatDuration(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatBytesAndSpeed(t *testing.T) {
	tests := []struct {
		in   uint64
		want string
	}{
		{in: 0, want: "0 B"},
		{in: 999, want: "999 B"},
		{in: 1024, want: "1.0 KiB"},
		{in: 1536, want: "1.5 KiB"},
		{in: 12 * 1024 * 1024, want: "12.0 MiB"},
		{in: 3 * 1024 * 1024 * 1024, want: "3.0 GiB"},
		{in: 1024 * 1024 * 1024 * 1024, want: "1.0 TiB"},
		{in: 1024 * 1024 * 1024 * 1024 * 1024, want: "1.0 PiB"},
		{in: 2 * 1024 * 1024 * 1024 * 1024 * 1024, want: "2.0 PiB"},
	}

	for _, tt := range tests {
		if got := formatBytes(tt.in); got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}

	if got := formatSpeed(1536); got != "1.5 KiB/s" {
		t.Errorf("formatSpeed(1536) = %q, want 1.5 KiB/s", got)
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		task     aria2.Task
		maxWidth int
		want     string
	}{
		{name: "keeps a short name", task: aria2.Task{Name: "one.iso"}, maxWidth: 20, want: "one.iso"},
		{name: "placeholder when unnamed", task: aria2.Task{}, maxWidth: 20, want: unknownName},
		{name: "truncates with an ellipsis", task: aria2.Task{Name: "a-very-long-download-name.iso"}, maxWidth: 10, want: "a-very-..."},
		{name: "truncates without an ellipsis when there is no room", task: aria2.Task{Name: "abcdef"}, maxWidth: 2, want: "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayName(tt.task, tt.maxWidth); got != tt.want {
				t.Errorf("displayName(%+v, %d) = %q, want %q", tt.task, tt.maxWidth, got, tt.want)
			}
		})
	}
}

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		in   aria2.Status
		want string
	}{
		{in: aria2.StatusActive, want: "active"},
		{in: aria2.StatusWaiting, want: "waiting"},
		{in: aria2.StatusPaused, want: "paused"},
		{in: aria2.StatusComplete, want: "complete"},
		{in: aria2.StatusError, want: "error"},
		{in: aria2.StatusRemoved, want: "removed"},
		{in: "", want: "unknown"},
		{in: aria2.Status("verifying"), want: "verifying"},
	}

	for _, tt := range tests {
		if got := statusLabel(tt.in); got != tt.want {
			t.Errorf("statusLabel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestConnectionsLabel(t *testing.T) {
	if got := connectionsLabel(aria2.Task{Connections: 0}); got != "" {
		t.Errorf("connectionsLabel(0 connections) = %q, want an empty column", got)
	}
	if got := connectionsLabel(aria2.Task{Connections: 8}); got != "8 conn" {
		t.Errorf("connectionsLabel(8) = %q, want 8 conn", got)
	}
}
