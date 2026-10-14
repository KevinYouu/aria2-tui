package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/KevinYouu/aria2-tui/internal/config"
	"github.com/KevinYouu/aria2-tui/internal/logs"
	"github.com/KevinYouu/aria2-tui/internal/theme"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		logs.Error(err.Error())
		os.Exit(1)
	}
}

// run loads the configuration, applies the theme and dispatches the command.
// The context is cancelled on SIGINT and SIGTERM, which also shuts down a
// running TUI and its aria2c child.
func run(ctx context.Context) error {
	configPath, err := config.Path()
	if err != nil {
		return err
	}
	store := config.NewStore(configPath)
	cfg, err := store.Load()
	if err != nil {
		return err
	}

	mode, provided, err := resolveTheme(os.Args, cfg.Theme)
	if err != nil {
		return err
	}
	theme.ApplyMode(mode)
	if provided {
		cfg.Theme = mode
		if err := store.Save(cfg); err != nil {
			logs.Warning(err.Error())
		}
	}

	return rootCmd.ExecuteContext(ctx)
}

// resolveTheme returns the theme mode to apply. The --theme flag is parsed
// before Cobra runs so that help output already uses the requested theme; it
// overrides the configuration and is written back when provided.
func resolveTheme(args []string, configured theme.Mode) (mode theme.Mode, provided bool, err error) {
	flagValue := ""
	for i, arg := range args {
		switch {
		case arg == "--theme" && i+1 < len(args):
			flagValue = args[i+1]
		case strings.HasPrefix(arg, "--theme="):
			flagValue = strings.TrimPrefix(arg, "--theme=")
		default:
			continue
		}
		break
	}

	if flagValue == "" {
		return configured, false, nil
	}

	mode = theme.Mode(strings.ToLower(flagValue))
	if !theme.ValidMode(mode) {
		return "", false, fmt.Errorf("invalid --theme %q: expected auto, dark or light", flagValue)
	}
	return mode, true, nil
}
