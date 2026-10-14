package tui

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/KevinYouu/aria2-tui/internal/config"
	"github.com/charmbracelet/x/ansi"
)

func TestAppStartsInTheAddViewWhenNothingIsQueued(t *testing.T) {
	fake := newFakeRPC(t)
	h := newHarness(t, fake, nil)

	if h.app.view != viewAdd {
		t.Errorf("view = %v, want the paste view for an empty list", h.app.view)
	}
	if view := ansi.Strip(h.view()); !strings.Contains(view, "Add downloads") {
		t.Errorf("view = %q, want the paste view title", view)
	}
}

func TestAppAddsPastedLinksAndLandsOnTheList(t *testing.T) {
	fake := newFakeRPC(t)
	h := newHarness(t, fake, nil)

	h.paste("https://example.org/one.iso\n  https://example.org/two.iso  \n\n")
	h.ctrl('s')

	added := fake.added()
	if len(added) != 2 {
		t.Fatalf("added %v, want the two pasted links", added)
	}
	if added[0] != "https://example.org/one.iso" || added[1] != "https://example.org/two.iso" {
		t.Errorf("added %v, want the trimmed links", added)
	}
	if h.app.view != viewList {
		t.Errorf("view = %v, want the list view after submitting", h.app.view)
	}
	if view := ansi.Strip(h.view()); !strings.Contains(view, "added 2 download(s)") {
		t.Errorf("view = %q, want the submission notice", view)
	}

	links, _ := h.app.add.links()
	if len(links) != 0 {
		t.Errorf("paste box still holds %v, want it cleared", links)
	}
}

func TestAppCountsInvalidPastedLines(t *testing.T) {
	fake := newFakeRPC(t)
	h := newHarness(t, fake, nil)

	h.paste("not a url\nhttps://example.org/one.iso\nfile:///etc/hosts\nftp://example.org/two.iso\n")
	h.ctrl('s')

	if got := fake.added(); len(got) != 2 {
		t.Fatalf("added %v, want the two valid links", got)
	}
	if view := ansi.Strip(h.view()); !strings.Contains(view, "2 invalid entries ignored") {
		t.Errorf("view = %q, want the invalid entry count", view)
	}
}

func TestAppRejectsSubmitWithoutValidLinks(t *testing.T) {
	fake := newFakeRPC(t)
	h := newHarness(t, fake, nil)

	h.paste("hello world\n")
	h.ctrl('s')

	if got := fake.added(); len(got) != 0 {
		t.Errorf("added %v, want nothing for invalid input", got)
	}
	if h.app.view != viewAdd {
		t.Errorf("view = %v, want to stay in the paste view", h.app.view)
	}
	if view := ansi.Strip(h.view()); !strings.Contains(view, "no valid links") {
		t.Errorf("view = %q, want the rejection message", view)
	}
}

func TestAppAddsLinksFromTheCommandLine(t *testing.T) {
	fake := newFakeRPC(t)
	h := newHarness(t, fake, []string{"https://example.org/one.iso"})

	if h.app.view != viewList {
		t.Errorf("view = %v, want the list view for command line links", h.app.view)
	}
	if got := fake.added(); len(got) != 1 || got[0] != "https://example.org/one.iso" {
		t.Errorf("added %v, want the command line link", got)
	}
}

func TestAppPausesAndResumesTheSelectedDownload(t *testing.T) {
	fake := newFakeRPC(t)
	fake.setTasks([]map[string]any{taskJSON("a1", "one.iso", "active", 100, 50, 10, 4)}, nil, nil)
	h := newHarness(t, fake, nil)

	h.key('p')

	pauses := fake.callsFor("aria2.pause")
	if len(pauses) != 1 {
		t.Fatalf("aria2.pause calls = %d, want 1", len(pauses))
	}
	if len(pauses[0].params) != 2 || pauses[0].params[1] != "a1" {
		t.Errorf("pause params = %v, want the token and gid a1", pauses[0].params)
	}

	// The daemon now reports the download as paused.
	fake.setTasks(nil, []map[string]any{taskJSON("a1", "one.iso", "paused", 100, 50, 0, 0)}, nil)
	h.refresh()
	h.key('p')

	unpauses := fake.callsFor("aria2.unpause")
	if len(unpauses) != 1 {
		t.Fatalf("aria2.unpause calls = %d, want 1", len(unpauses))
	}
	if unpauses[0].params[1] != "a1" {
		t.Errorf("unpause params = %v, want gid a1", unpauses[0].params)
	}
}

func TestAppPauseIgnoresFinishedDownloads(t *testing.T) {
	fake := newFakeRPC(t)
	fake.setTasks(nil, nil, []map[string]any{taskJSON("s1", "done.iso", "complete", 100, 100, 0, 0)})
	h := newHarness(t, fake, nil)

	h.key('p')

	if calls := fake.callsFor("aria2.pause"); len(calls) != 0 {
		t.Errorf("aria2.pause calls = %d, want none for a finished download", len(calls))
	}
	if view := ansi.Strip(h.view()); !strings.Contains(view, "can be paused") {
		t.Errorf("view = %q, want an explanation", view)
	}
}

func TestAppRemovesDownloads(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		wantCall string
	}{
		{name: "active download is removed", status: "active", wantCall: "aria2.remove"},
		{name: "paused download is removed", status: "paused", wantCall: "aria2.remove"},
		{name: "finished download drops its result", status: "complete", wantCall: "aria2.removeDownloadResult"},
		{name: "failed download drops its result", status: "error", wantCall: "aria2.removeDownloadResult"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeRPC(t)
			task := taskJSON("g1", "one.iso", tt.status, 100, 50, 0, 0)
			stopped := []map[string]any{task}
			if tt.status == "complete" || tt.status == "error" {
				fake.setTasks(nil, nil, stopped)
			} else {
				fake.setTasks(stopped, nil, nil)
			}
			h := newHarness(t, fake, nil)

			h.key('d')

			calls := fake.callsFor(tt.wantCall)
			if len(calls) != 1 {
				t.Fatalf("%s calls = %d, want 1", tt.wantCall, len(calls))
			}
			if calls[0].params[1] != "g1" {
				t.Errorf("%s params = %v, want gid g1", tt.wantCall, calls[0].params)
			}
		})
	}
}

func TestAppChangesConcurrencyAndPersistsIt(t *testing.T) {
	fake := newFakeRPC(t)
	fake.setTasks([]map[string]any{taskJSON("a1", "one.iso", "active", 100, 50, 10, 4)}, nil, nil)
	cfg := defaultTestConfig()
	cfg.MaxConcurrentDownloads = 5
	h := newHarnessWithConfig(t, fake, cfg, nil)

	h.key('+')

	calls := fake.callsFor("aria2.changeGlobalOption")
	if len(calls) != 1 {
		t.Fatalf("changeGlobalOption calls = %d, want 1", len(calls))
	}
	options, ok := calls[0].params[1].(map[string]any)
	if !ok {
		t.Fatalf("changeGlobalOption params = %v, want an options object", calls[0].params)
	}
	if options["max-concurrent-downloads"] != "6" {
		t.Errorf("options = %v, want max-concurrent-downloads 6", options)
	}
	if h.app.config.MaxConcurrentDownloads != 6 {
		t.Errorf("in-memory concurrency = %d, want 6", h.app.config.MaxConcurrentDownloads)
	}

	// The new value must survive a restart.
	persisted, err := os.ReadFile(h.app.deps.Store.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", h.app.deps.Store.Path, err)
	}
	var stored config.Config
	if err := json.Unmarshal(persisted, &stored); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", persisted, err)
	}
	if stored.MaxConcurrentDownloads != 6 {
		t.Errorf("persisted concurrency = %d, want 6", stored.MaxConcurrentDownloads)
	}

	h.key('-')
	h.key('-')
	if got := h.app.config.MaxConcurrentDownloads; got != 4 {
		t.Errorf("concurrency = %d after two decrements, want 4", got)
	}
}

func TestAppKeepsConcurrencyWithinBounds(t *testing.T) {
	t.Run("upper bound", func(t *testing.T) {
		fake := newFakeRPC(t)
		fake.setTasks([]map[string]any{taskJSON("a1", "one.iso", "active", 100, 50, 10, 4)}, nil, nil)
		cfg := defaultTestConfig()
		cfg.MaxConcurrentDownloads = config.MaxConcurrentDownloads
		h := newHarnessWithConfig(t, fake, cfg, nil)

		h.key('+')

		if calls := fake.callsFor("aria2.changeGlobalOption"); len(calls) != 0 {
			t.Errorf("changeGlobalOption calls = %d, want none above the maximum", len(calls))
		}
	})

	t.Run("lower bound", func(t *testing.T) {
		fake := newFakeRPC(t)
		fake.setTasks([]map[string]any{taskJSON("a1", "one.iso", "active", 100, 50, 10, 4)}, nil, nil)
		cfg := defaultTestConfig()
		cfg.MaxConcurrentDownloads = config.MinConcurrentDownloads
		h := newHarnessWithConfig(t, fake, cfg, nil)

		h.key('-')

		if calls := fake.callsFor("aria2.changeGlobalOption"); len(calls) != 0 {
			t.Errorf("changeGlobalOption calls = %d, want none below the minimum", len(calls))
		}
	})
}

func TestAppRollsBackRejectedConcurrencyChange(t *testing.T) {
	fake := newFakeRPC(t)
	fake.setTasks([]map[string]any{taskJSON("a1", "one.iso", "active", 100, 50, 10, 4)}, nil, nil)
	h := newHarnessWithConfig(t, fake, defaultTestConfig(), nil)

	// The healthy fake cannot fail a call, so the client is pointed at a closed
	// server to force a transport error.
	fake.server.Close()
	h.app.deps.Client = fake.client()

	h.key('+')

	if got := h.app.config.MaxConcurrentDownloads; got != config.DefaultConcurrentDownloads {
		t.Errorf("concurrency = %d after a failed change, want the previous %d", got, config.DefaultConcurrentDownloads)
	}
	if view := ansi.Strip(h.view()); !strings.Contains(view, "changeGlobalOption") {
		t.Errorf("view = %q, want the failed call reported", view)
	}
}

func TestAppCursorKeepsTheSameDownloadAfterReorder(t *testing.T) {
	fake := newFakeRPC(t)
	fake.setTasks([]map[string]any{
		taskJSON("a1", "one.iso", "active", 100, 10, 1, 1),
		taskJSON("a2", "two.iso", "active", 100, 20, 1, 1),
		taskJSON("a3", "three.iso", "active", 100, 30, 1, 1),
	}, nil, nil)
	h := newHarness(t, fake, nil)

	h.key('j')
	if h.app.list.cursorGID != "a2" {
		t.Fatalf("cursorGID = %q, want a2 after one move down", h.app.list.cursorGID)
	}

	// The daemon reorders the list: the selection follows the download.
	fake.setTasks([]map[string]any{
		taskJSON("a3", "three.iso", "active", 100, 30, 1, 1),
		taskJSON("a2", "two.iso", "active", 100, 20, 1, 1),
		taskJSON("a1", "one.iso", "active", 100, 10, 1, 1),
	}, nil, nil)
	h.refresh()

	if h.app.list.cursorGID != "a2" {
		t.Errorf("cursorGID = %q after reordering, want a2", h.app.list.cursorGID)
	}
	task, ok := h.app.list.selected(h.app.snapshot.Tasks())
	if !ok || task.Name != "two.iso" {
		t.Errorf("selected = %+v, want two.iso", task)
	}
}

func TestAppQuitsImmediatelyWhenIdle(t *testing.T) {
	fake := newFakeRPC(t)
	h := newHarness(t, fake, nil)

	h.ctrl('c')

	if !h.quit {
		t.Error("ctrl+c did not quit while nothing is downloading")
	}
	if h.app.confirmQuit {
		t.Error("confirmQuit = true, want no confirmation while idle")
	}
}

func TestAppConfirmsQuitWhileDownloading(t *testing.T) {
	fake := newFakeRPC(t)
	fake.setTasks([]map[string]any{taskJSON("a1", "one.iso", "active", 100, 50, 10, 4)}, nil, nil)
	h := newHarness(t, fake, nil)

	h.key('q')

	if h.quit {
		t.Fatal("q quit without confirmation while a download is in progress")
	}
	view := ansi.Strip(h.view())
	if !strings.Contains(view, "[y/N]") {
		t.Errorf("view = %q, want an inline confirmation prompt", view)
	}
	if !strings.Contains(view, "resume") {
		t.Errorf("view = %q, want the prompt to explain that partial data stays on disk", view)
	}
	if !strings.Contains(view, "aria2c stops") {
		t.Errorf("view = %q, want the prompt to explain that the aria2c session ends", view)
	}

	h.key('x')
	if h.app.confirmQuit {
		t.Error("confirmQuit = true, want any other key to cancel the prompt")
	}
	if h.quit {
		t.Error("a cancelled prompt still quit the app")
	}

	h.key('q')
	h.key('y')
	if !h.quit {
		t.Error("y did not confirm the quit")
	}
}

func TestAppReportsDaemonExit(t *testing.T) {
	fake := newFakeRPC(t)
	h := newHarness(t, fake, nil)

	h.send(daemonExitMsg{})

	if view := ansi.Strip(h.view()); !strings.Contains(view, "aria2c exited") {
		t.Errorf("view = %q, want the daemon exit message", view)
	}
}

func TestAppShowsAndClearsSnapshotErrors(t *testing.T) {
	fake := newFakeRPC(t)
	fake.setTasks([]map[string]any{taskJSON("a1", "one.iso", "active", 100, 50, 10, 4)}, nil, nil)
	h := newHarness(t, fake, nil)

	h.send(snapshotMsg{err: errors.New("aria2 error 1: Unauthorized")})

	view := ansi.Strip(h.view())
	if !strings.Contains(view, "Unauthorized") {
		t.Errorf("view = %q, want the rpc error", view)
	}
	// The last good snapshot stays on screen.
	if len(h.app.snapshot.Tasks()) != 1 {
		t.Errorf("tasks = %d, want the previous snapshot kept", len(h.app.snapshot.Tasks()))
	}

	h.refresh()

	if view := ansi.Strip(h.view()); strings.Contains(view, "Unauthorized") {
		t.Errorf("view = %q, want the stale error cleared after a successful poll", view)
	}
}

func TestAppIgnoresUnknownMessages(t *testing.T) {
	fake := newFakeRPC(t)
	h := newHarness(t, fake, nil)

	h.send(struct{ unknown string }{"payload"})

	if h.app.view != viewAdd {
		t.Errorf("view = %v, want the view unchanged", h.app.view)
	}
}

func TestAppEscapeReturnsToTheList(t *testing.T) {
	fake := newFakeRPC(t)
	fake.setTasks([]map[string]any{taskJSON("a1", "one.iso", "active", 100, 50, 10, 4)}, nil, nil)
	h := newHarness(t, fake, nil)

	h.key('a')
	if h.app.view != viewAdd {
		t.Fatalf("view = %v, want the paste view after a", h.app.view)
	}

	h.special(tea.KeyEscape)
	if h.app.view != viewList {
		t.Errorf("view = %v, want the list view after esc", h.app.view)
	}
}

func TestAppRendersArrowAndPagingKeys(t *testing.T) {
	fake := newFakeRPC(t)
	tasks := make([]map[string]any, 0, 8)
	for i := range 8 {
		tasks = append(tasks, taskJSON("gid-"+strconv.Itoa(i), "file-"+strconv.Itoa(i)+".iso", "active", 100, 10, 1, 1))
	}
	fake.setTasks(tasks, nil, nil)
	h := newHarnessWithConfig(t, fake, defaultTestConfig(), nil)
	h.resize(100, 10)

	h.special(tea.KeyDown)
	if h.app.list.cursorGID != "gid-1" {
		t.Errorf("cursorGID = %q after down, want gid-1", h.app.list.cursorGID)
	}
	h.key('k')
	if h.app.list.cursorGID != "gid-0" {
		t.Errorf("cursorGID = %q after k, want gid-0", h.app.list.cursorGID)
	}

	h.special(tea.KeyPgDown)
	if h.app.list.cursorGID == "gid-0" {
		t.Errorf("cursorGID = %q after pgdown, want it to move further", h.app.list.cursorGID)
	}
	h.special(tea.KeyPgUp)
	if h.app.list.cursorGID != "gid-0" {
		t.Errorf("cursorGID = %q after pgup, want gid-0", h.app.list.cursorGID)
	}
}
