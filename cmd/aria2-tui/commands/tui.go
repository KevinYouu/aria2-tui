package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/KevinYouu/aria2-tui/internal/aria2"
	"github.com/KevinYouu/aria2-tui/internal/config"
	"github.com/KevinYouu/aria2-tui/internal/logs"
	"github.com/KevinYouu/aria2-tui/internal/tui"
)

// shutdownTimeout bounds stopping aria2c once the UI is gone.
const shutdownTimeout = 10 * time.Second

// RunTUI starts a private aria2c daemon and drives it from the TUI until the
// user quits. The session owns the daemon, so it is always stopped on the way
// out, including when ctx was cancelled by a signal.
func RunTUI(ctx context.Context, links []string) error {
	configPath, err := config.Path()
	if err != nil {
		return err
	}
	store := config.NewStore(configPath)
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	cfg, err = cfg.Validate()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DownloadDir, 0o755); err != nil {
		return fmt.Errorf("create download directory %s: %w", cfg.DownloadDir, err)
	}

	daemon, err := aria2.Start(ctx, aria2.StartOptions{
		BinaryPath:             cfg.Aria2cPath,
		DownloadDir:            cfg.DownloadDir,
		MaxConcurrentDownloads: cfg.MaxConcurrentDownloads,
		Options:                cfg.Aria2Options(),
	})
	if err != nil {
		return err
	}

	defer func() {
		// ctx may already be cancelled by a signal, so stopping the daemon
		// gets a fresh deadline of its own.
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := daemon.Stop(stopCtx); err != nil {
			logs.Warning(err.Error())
		}
	}()

	return tui.Run(ctx, tui.Deps{
		Client:       aria2.NewClient(daemon.Endpoint()),
		Store:        store,
		Config:       cfg,
		InitialURIs:  links,
		DaemonDone:   daemon.Done(),
		Aria2Version: daemon.Version(),
	})
}
