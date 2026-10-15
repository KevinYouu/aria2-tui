package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/KevinYouu/aria2-tui/internal/config"
)

func TestSettingsStepAndEdit(t *testing.T) {
	fake := newFakeRPC(t)
	cfg := config.Default()
	cfg.DownloadDir = t.TempDir()
	cfg.Theme = "dark"
	h := newHarnessWithConfig(t, fake, cfg, []string{"https://example.com/a.bin"})

	h.key('s')
	if h.app.view != viewSettings {
		t.Fatalf("view = %v, want settings", h.app.view)
	}

	// Second field is the concurrency: right raises it by one.
	h.special(tea.KeyDown).special(tea.KeyRight)
	if got := h.app.config.MaxConcurrentDownloads; got != cfg.MaxConcurrentDownloads+1 {
		t.Errorf("MaxConcurrentDownloads = %d, want %d", got, cfg.MaxConcurrentDownloads+1)
	}

	// Move to the speed limit, edit it and commit.
	h.special(tea.KeyDown).special(tea.KeyDown).special(tea.KeyDown).special(tea.KeyDown)
	h.special(tea.KeyEnter)
	if !h.app.settings.editing {
		t.Fatal("enter did not start editing")
	}
	for range 3 {
		h.special(tea.KeyBackspace)
	}
	h.key('2').key('M').special(tea.KeyEnter)
	if got := h.app.config.DownloadLimit; got != "2M" {
		t.Errorf("DownloadLimit = %q, want 2M", got)
	}

	calls := fake.callsFor("aria2.changeGlobalOption")
	if len(calls) < 2 {
		t.Fatalf("changeGlobalOption calls = %d, want at least 2", len(calls))
	}

	saved, err := h.app.deps.Store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if saved.DownloadLimit != "2M" {
		t.Errorf("persisted DownloadLimit = %q, want 2M", saved.DownloadLimit)
	}

	// An invalid value is rejected and keeps the old one.
	h.special(tea.KeyEnter)
	for range 3 {
		h.special(tea.KeyBackspace)
	}
	h.key('x').special(tea.KeyEnter)
	if !h.app.settings.editing {
		t.Error("invalid input closed the editor")
	}
	if got := h.app.config.DownloadLimit; got != "2M" {
		t.Errorf("DownloadLimit = %q after invalid input, want 2M", got)
	}
	h.special(tea.KeyEscape)

	// r resets to the default.
	h.key('r')
	if got := h.app.config.DownloadLimit; got != config.DefaultDownloadSpeedLimit {
		t.Errorf("DownloadLimit = %q after reset, want %q", got, config.DefaultDownloadSpeedLimit)
	}

	h.special(tea.KeyEscape)
	if h.app.view != viewList {
		t.Errorf("view = %v, want list after esc", h.app.view)
	}
}
