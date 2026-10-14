package aria2

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestBuildArgs(t *testing.T) {
	opts := StartOptions{
		DownloadDir:            "/downloads",
		MaxConcurrentDownloads: 3,
	}

	args := buildArgs(opts, 6800, "s3cret")
	joined := strings.Join(args, " ")

	required := []string{
		"--enable-rpc=true",
		"--rpc-listen-all=false",
		"--rpc-listen-port=6800",
		"--rpc-secret=s3cret",
		"--rpc-allow-origin-all=false",
		// A daemon=true in the user's aria2.conf would detach the process and
		// break both pid tracking and --stop-with-process.
		"--daemon=false",
		fmt.Sprintf("--stop-with-process=%d", os.Getpid()),
		"--dir=/downloads",
		"--max-concurrent-downloads=3",
		// aria2 defaults --continue to false, which would restart downloads.
		"--continue=true",
		// warn keeps notice level progress output out of the ring buffer while
		// still capturing warnings and fatal errors.
		"--console-log-level=warn",
		// A download manager must not keep seeding.
		"--seed-time=0",
	}
	for _, want := range required {
		if !hasArg(args, want) {
			t.Errorf("buildArgs() is missing %q; got %v", want, args)
		}
	}

	// The user's aria2.conf is still loaded on purpose: proxy, tracker and
	// similar settings must keep working. Only the options above are forced.
	if hasArg(args, "--no-conf") {
		t.Errorf("buildArgs() must not disable aria2.conf: %v", args)
	}
	if !strings.HasPrefix(joined, "--enable-rpc=true") {
		t.Errorf("buildArgs() should lead with the rpc flags: %v", args)
	}
}

func TestBuildArgsUsesProvidedPortAndSecret(t *testing.T) {
	args := buildArgs(StartOptions{DownloadDir: "/tmp", MaxConcurrentDownloads: 1}, 12345, "abc")
	if !hasArg(args, "--rpc-listen-port=12345") {
		t.Errorf("buildArgs() did not use the provided port: %v", args)
	}
	if !hasArg(args, "--rpc-secret=abc") {
		t.Errorf("buildArgs() did not use the provided secret: %v", args)
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestFreePortReturnsUsablePort(t *testing.T) {
	port, err := freePort()
	if err != nil {
		t.Fatalf("freePort() error = %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Fatalf("freePort() = %d, want a valid port", port)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("the reported port %d is not usable: %v", port, err)
	}
	_ = listener.Close()
}

func TestLocateAria2c(t *testing.T) {
	stubName := "aria2c"
	if runtime.GOOS == "windows" {
		stubName = "aria2c.exe"
	}

	t.Run("configured executable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), stubName)
		writeStub(t, path, 0o755)

		got, err := LocateAria2c(path)
		if err != nil {
			t.Fatalf("LocateAria2c() error = %v", err)
		}
		if got != path {
			t.Errorf("LocateAria2c() = %q, want %q", got, path)
		}
	})

	t.Run("configured path missing", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope")
		if _, err := LocateAria2c(missing); err == nil {
			t.Fatal("LocateAria2c() error = nil, want an error")
		}
	})

	t.Run("configured path is a directory", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := LocateAria2c(dir); err == nil {
			t.Fatal("LocateAria2c() error = nil, want an error")
		}
	})

	t.Run("configured path is not executable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("windows has no execute bit")
		}
		path := filepath.Join(t.TempDir(), stubName)
		writeStub(t, path, 0o644)

		if _, err := LocateAria2c(path); err == nil {
			t.Fatal("LocateAria2c() error = nil, want an error")
		}
	})

	t.Run("found in path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, stubName)
		writeStub(t, path, 0o755)
		t.Setenv("PATH", dir)

		got, err := LocateAria2c("")
		if err != nil {
			t.Fatalf("LocateAria2c() error = %v", err)
		}
		if got != path {
			t.Errorf("LocateAria2c() = %q, want %q", got, path)
		}
	})

	t.Run("missing from path mentions how to install", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())

		_, err := LocateAria2c("")
		if err == nil {
			t.Fatal("LocateAria2c() error = nil, want an error")
		}
		for _, want := range []string{"aria2c", "aria2"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("LocateAria2c() error = %q, want it to mention %q", err, want)
			}
		}
	})
}

func writeStub(t *testing.T, path string, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), perm); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func TestRingBufferKeepsTail(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{name: "under the limit", limit: 16, writes: []string{"abc"}, want: "abc"},
		{name: "trims the front", limit: 4, writes: []string{"0123456789"}, want: "6789"},
		{name: "keeps appending", limit: 8, writes: []string{"abcd", "efgh", "ij"}, want: "cdefghij"},
		{name: "single write larger than the limit", limit: 4, writes: []string{"0123456789"}, want: "6789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := newRingBuffer(tt.limit)
			for _, write := range tt.writes {
				n, err := buffer.Write([]byte(write))
				if err != nil {
					t.Fatalf("Write() error = %v", err)
				}
				if n != len(write) {
					t.Errorf("Write() = %d, want %d", n, len(write))
				}
			}
			if got := buffer.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			if buffer.String() != "" && len(buffer.String()) > tt.limit {
				t.Errorf("buffer holds %d bytes, want at most %d", len(buffer.String()), tt.limit)
			}
		})
	}
}

// exec copies stdout and stderr from separate goroutines, so the buffer must
// stay consistent under concurrent writes (run with -race).
func TestRingBufferConcurrentWrites(t *testing.T) {
	const (
		limit     = 64
		writers   = 8
		perWriter = 100
	)

	buffer := newRingBuffer(limit)
	var wg sync.WaitGroup
	for writer := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWriter {
				if _, err := buffer.Write([]byte(fmt.Sprintf("w%d-line\n", writer))); err != nil {
					t.Errorf("Write() error = %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if got := len(buffer.String()); got > limit {
		t.Errorf("buffer holds %d bytes, want at most %d", got, limit)
	}
}

func TestRandomSecretIsRandomHex(t *testing.T) {
	first, err := randomSecret()
	if err != nil {
		t.Fatalf("randomSecret() error = %v", err)
	}
	if _, err := hex.DecodeString(first); err != nil {
		t.Errorf("randomSecret() = %q, want hex", first)
	}
	if len(first) != secretBytes*2 {
		t.Errorf("len(randomSecret()) = %d, want %d", len(first), secretBytes*2)
	}

	second, err := randomSecret()
	if err != nil {
		t.Fatalf("randomSecret() error = %v", err)
	}
	if first == second {
		t.Error("randomSecret() returned the same secret twice")
	}
}

func TestStartRejectsMissingDownloadDir(t *testing.T) {
	_, err := Start(t.Context(), StartOptions{DownloadDir: "  "})
	if err == nil {
		t.Fatal("Start() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "download directory") {
		t.Errorf("Start() error = %q, want it to mention the download directory", err)
	}
}

func TestStartReportsStartupFailure(t *testing.T) {
	stubName := "aria2c-failure-stub"
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}
	path := filepath.Join(t.TempDir(), stubName)
	script := "#!/bin/sh\necho 'unrecognized option' >&2\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Start(t.Context(), StartOptions{
		BinaryPath:             path,
		DownloadDir:            t.TempDir(),
		MaxConcurrentDownloads: 1,
	})
	if err == nil {
		t.Fatal("Start() error = nil, want the startup failure reported")
	}
	if !strings.Contains(err.Error(), "unrecognized option") {
		t.Errorf("Start() error = %q, want the captured stderr tail", err)
	}
}
