package aria2

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// readinessInterval is the delay between readiness probes.
	readinessInterval = 50 * time.Millisecond
	// readinessTimeout bounds how long the daemon may take to accept RPC.
	readinessTimeout = 5 * time.Second
	// shutdownGrace bounds the wait for a clean RPC shutdown before the
	// process is killed.
	shutdownGrace = 5 * time.Second
	// logTailBytes bounds the captured daemon output.
	logTailBytes = 8 << 10
	// secretBytes is the length of the per session RPC secret.
	secretBytes = 32
)

// StartOptions configures a daemon process.
type StartOptions struct {
	// BinaryPath overrides the aria2c lookup when not empty.
	BinaryPath string
	// DownloadDir is the aria2 --dir value; it must exist already.
	DownloadDir string
	// MaxConcurrentDownloads is the initial parallelism limit.
	MaxConcurrentDownloads int
	// Port is the RPC port; 0 picks a free one.
	Port int
	// Secret is the RPC secret; empty generates a random one.
	Secret string
	// Options are extra aria2 options (name without dashes); empty values are
	// skipped so an unset proxy does not reach the command line.
	Options map[string]string
}

// Daemon is a private aria2c process owned by this session.
type Daemon struct {
	cmd     *exec.Cmd
	ep      Endpoint
	client  *Client
	version string
	output  *ringBuffer
	done    chan struct{}
	exitErr error
}

// Start launches aria2c with RPC enabled and waits until it answers. The
// daemon is bound to the loopback interface only and is told to exit when this
// process disappears, which covers a terminal that is closed without a signal.
func Start(ctx context.Context, opts StartOptions) (*Daemon, error) {
	if strings.TrimSpace(opts.DownloadDir) == "" {
		return nil, errors.New("download directory is required to start aria2c")
	}

	binary, err := LocateAria2c(opts.BinaryPath)
	if err != nil {
		return nil, err
	}

	port := opts.Port
	if port == 0 {
		port, err = freePort()
		if err != nil {
			return nil, fmt.Errorf("find a free port for the aria2 rpc: %w", err)
		}
	}

	secret := opts.Secret
	if secret == "" {
		secret, err = randomSecret()
		if err != nil {
			return nil, fmt.Errorf("generate the aria2 rpc secret: %w", err)
		}
	}

	output := newRingBuffer(logTailBytes)
	cmd := exec.Command(binary, buildArgs(opts, port, secret)...)
	cmd.Stdin = nil
	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", binary, err)
	}

	daemon := &Daemon{
		cmd: cmd,
		ep: Endpoint{
			URL:    fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port),
			Secret: secret,
		},
		output: output,
		done:   make(chan struct{}),
	}
	daemon.client = NewClient(daemon.ep)
	go daemon.wait()

	if err := daemon.waitReady(ctx); err != nil {
		// Do not leak the child when it never became usable.
		_ = cmd.Process.Kill()
		<-daemon.done
		return nil, err
	}
	return daemon, nil
}

// buildArgs renders the aria2c command line. Every option that the tool
// depends on is set explicitly because aria2 loads the user's aria2.conf too,
// and a stray daemon=true or continue=false there would break this session.
func buildArgs(opts StartOptions, port int, secret string) []string {
	args := []string{
		"--enable-rpc=true",
		"--rpc-listen-all=false",
		fmt.Sprintf("--rpc-listen-port=%d", port),
		"--rpc-secret=" + secret,
		"--rpc-allow-origin-all=false",
		"--daemon=false",
		fmt.Sprintf("--stop-with-process=%d", os.Getpid()),
		"--dir=" + opts.DownloadDir,
		fmt.Sprintf("--max-concurrent-downloads=%d", opts.MaxConcurrentDownloads),
		"--continue=true",
		// warn keeps the notice level progress summary out of the ring buffer
		// while still capturing warn and fatal messages.
		"--console-log-level=warn",
		// A download manager must not keep seeding after the download.
		"--seed-time=0",
	}
	for _, name := range slices.Sorted(maps.Keys(opts.Options)) {
		if value := opts.Options[name]; value != "" {
			args = append(args, "--"+name+"="+value)
		}
	}
	return args
}

// wait reaps the process; it runs for the daemon's whole lifetime.
func (d *Daemon) wait() {
	defer close(d.done)
	d.exitErr = d.cmd.Wait()
}

// waitReady polls the RPC endpoint until the daemon answers.
func (d *Daemon) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(readinessTimeout)
	for {
		probeCtx, cancel := context.WithTimeout(ctx, readinessInterval*4)
		version, err := d.client.Version(probeCtx)
		cancel()
		if err == nil {
			d.version = version
			return nil
		}

		select {
		case <-d.done:
			return d.startupError()
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("aria2c did not answer on %s within %s", d.ep.URL, readinessTimeout)
		}

		select {
		case <-d.done:
			return d.startupError()
		case <-ctx.Done():
			return fmt.Errorf("aria2c startup was interrupted: %w", ctx.Err())
		case <-time.After(readinessInterval):
		}
	}
}

// startupError reports a daemon that exited during startup, including the
// captured output so the cause is diagnosable.
func (d *Daemon) startupError() error {
	message := "aria2c exited before its rpc became ready"
	if d.exitErr != nil {
		message = fmt.Sprintf("%s: %v", message, d.exitErr)
	}
	if tail := strings.TrimSpace(d.output.String()); tail != "" {
		message = message + "\n" + tail
	}
	return errors.New(message)
}

// Endpoint returns the RPC endpoint of this daemon.
func (d *Daemon) Endpoint() Endpoint { return d.ep }

// Version returns the daemon version detected at startup.
func (d *Daemon) Version() string { return d.version }

// Done is closed once the daemon process has exited.
func (d *Daemon) Done() <-chan struct{} { return d.done }

// ExitError reports why the daemon exited: nil while it is still running and
// nil after a clean exit.
func (d *Daemon) ExitError() error {
	select {
	case <-d.done:
		return d.exitErr
	default:
		return nil
	}
}

// Stop shuts the daemon down: an RPC shutdown first, then a kill so a
// wedged process cannot outlive the session. ctx must be independent of the
// signal cancelled context that started the daemon.
func (d *Daemon) Stop(ctx context.Context) error {
	if d == nil {
		return nil
	}
	select {
	case <-d.done:
		return nil
	default:
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownGrace)
	defer cancel()

	// aria2 delays an RPC shutdown by several seconds, while SIGTERM makes it
	// save its state and exit within about a second. Windows cannot deliver
	// SIGTERM, so it falls back to the RPC call.
	if err := d.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = d.client.Shutdown(shutdownCtx)
	}

	select {
	case <-d.done:
		return nil
	case <-shutdownCtx.Done():
	}

	if err := d.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill aria2c: %w", err)
	}
	<-d.done
	return nil
}

// freePort asks the kernel for a free loopback port. The port is released
// immediately, so a collision between the lookup and aria2c binding is
// possible but requires another process to grab it in that window.
func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %s", listener.Addr())
	}
	return addr.Port, nil
}

// randomSecret returns a per session RPC secret. It is never written to disk.
func randomSecret() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// ringBuffer keeps the last limit bytes written to it. exec copies stdout and
// stderr from separate goroutines, so writes are locked.
type ringBuffer struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func newRingBuffer(limit int) *ringBuffer {
	return &ringBuffer{limit: limit}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(p) >= r.limit {
		r.buf = append(r.buf[:0], p[len(p)-r.limit:]...)
		return len(p), nil
	}
	r.buf = append(r.buf, p...)
	if len(r.buf) > r.limit {
		r.buf = append(r.buf[:0], r.buf[len(r.buf)-r.limit:]...)
	}
	return len(p), nil
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}
