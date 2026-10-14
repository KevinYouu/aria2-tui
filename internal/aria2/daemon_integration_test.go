package aria2

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestDaemonLifecycle drives a real aria2c through the whole session: start,
// readiness, add, poll, change the parallelism and shut down without leaving a
// process behind. It is skipped when aria2c is not installed.
func TestDaemonLifecycle(t *testing.T) {
	if _, err := exec.LookPath("aria2c"); err != nil {
		t.Skip("aria2c is not installed")
	}

	ctx := t.Context()
	daemon, err := Start(ctx, StartOptions{
		DownloadDir:            t.TempDir(),
		MaxConcurrentDownloads: 2,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	pid := daemon.cmd.Process.Pid
	client := NewClient(daemon.Endpoint())

	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := daemon.Stop(stopCtx); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	})

	if version := daemon.Version(); version == "" {
		t.Error("Version() is empty, want the version reported by the daemon")
	}

	// The payload is served slowly enough to be observable while downloading.
	payload := bytes.Repeat([]byte("aria2-tui integration payload\n"), 800)
	half := len(payload) / 2
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload[:half])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(1500 * time.Millisecond)
		_, _ = w.Write(payload[half:])
	}))
	defer fileServer.Close()

	gids, err := client.AddURIs(ctx, []string{fileServer.URL + "/payload.bin"}, nil)
	if err != nil {
		t.Fatalf("AddURIs() error = %v", err)
	}
	if len(gids) != 1 || gids[0] == "" {
		t.Fatalf("AddURIs() = %v, want exactly one gid", gids)
	}
	gid := gids[0]

	// aria2 reports the task before it has resolved the file, so wait for the
	// metadata (total length) to arrive; until then the name falls back to the
	// URI.
	active := waitForTask(t, client, gid, 10*time.Second, func(task Task) bool {
		return task.Status == StatusActive && task.Total > 0
	})
	if active.Total != uint64(len(payload)) {
		t.Errorf("Total = %d, want %d", active.Total, len(payload))
	}
	if active.Name != "payload.bin" {
		t.Errorf("Name = %q, want payload.bin", active.Name)
	}

	complete := waitForTask(t, client, gid, 30*time.Second, func(task Task) bool {
		return task.Status == StatusComplete || task.Status == StatusError
	})
	if complete.Status != StatusComplete {
		t.Fatalf("status = %q (%s), want %q", complete.Status, complete.ErrorMessage, StatusComplete)
	}
	if complete.Completed != uint64(len(payload)) {
		t.Errorf("Completed = %d, want %d", complete.Completed, len(payload))
	}

	if err := client.SetMaxConcurrent(ctx, 3); err != nil {
		t.Errorf("SetMaxConcurrent() error = %v", err)
	}
	if err := client.PurgeResult(ctx, gid); err != nil {
		t.Errorf("PurgeResult() error = %v", err)
	}

	if err := daemon.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-daemon.Done():
	default:
		t.Fatal("Stop() returned while the daemon is still running")
	}
	if err := daemon.ExitError(); err != nil {
		t.Errorf("ExitError() = %v, want a clean exit", err)
	}
	if runtime.GOOS != "windows" {
		if err := syscall.Kill(pid, 0); err == nil {
			t.Errorf("aria2c process %d is still alive after Stop()", pid)
		}
	}
}

// waitForTask polls the daemon until the predicate matches.
func waitForTask(t *testing.T, client *Client, gid string, timeout time.Duration, match func(Task) bool) Task {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last Task
	for time.Now().Before(deadline) {
		snapshot, err := client.Snapshot(t.Context())
		if err != nil {
			t.Fatalf("Snapshot() error = %v", err)
		}
		for _, task := range snapshot.Tasks() {
			if task.GID != gid {
				continue
			}
			last = task
			if match(task) {
				return task
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("gid %s did not reach the expected state within %s (last seen: %+v)", gid, timeout, last)
	return Task{}
}

// TestStopWaitsForAnActiveDownload covers the case the tmux session hit: aria2
// answers aria2.shutdown immediately but needs a few seconds to flush a running
// download before it exits, so Stop must not return while the process lives.
func TestStopWaitsForAnActiveDownload(t *testing.T) {
	if _, err := exec.LookPath("aria2c"); err != nil {
		t.Skip("aria2c is not installed")
	}

	ctx := t.Context()
	daemon, err := Start(ctx, StartOptions{
		DownloadDir:            t.TempDir(),
		MaxConcurrentDownloads: 1,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	pid := daemon.cmd.Process.Pid
	client := NewClient(daemon.Endpoint())

	payload := bytes.Repeat([]byte("aria2-tui stop test\n"), 4096) // 80 KiB
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload[:len(payload)/4])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(2 * time.Second)
		_, _ = w.Write(payload[len(payload)/4:])
	}))
	defer fileServer.Close()

	gids, err := client.AddURIs(ctx, []string{fileServer.URL + "/slow.bin"}, nil)
	if err != nil {
		t.Fatalf("AddURIs() error = %v", err)
	}
	if len(gids) != 1 {
		t.Fatalf("AddURIs() = %v, want one gid", gids)
	}
	waitForTask(t, client, gids[0], 10*time.Second, func(task Task) bool {
		return task.Status == StatusActive
	})

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*shutdownGrace)
	defer cancel()

	started := time.Now()
	if err := daemon.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	t.Logf("Stop() returned after %s", time.Since(started).Round(time.Millisecond))

	if runtime.GOOS != "windows" {
		if err := syscall.Kill(pid, 0); err == nil {
			t.Errorf("aria2c %d is still alive after Stop() returned", pid)
		}
	}
}
