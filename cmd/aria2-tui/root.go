package main

import (
	"strings"

	"github.com/KevinYouu/aria2-tui/cmd/aria2-tui/commands"
	"github.com/KevinYouu/aria2-tui/internal/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "aria2-tui [url...]",
	Short: "Terminal download manager built on aria2",
	Long: `aria2-tui runs a private aria2c daemon and drives it from a terminal UI.

Run it without arguments to paste links, or pass links directly:

  aria2-tui
  aria2-tui https://example.com/file.iso

The daemon binds to the loopback interface only and is stopped when
aria2-tui exits; unfinished downloads stay on disk and can be resumed by
adding the same link again.`,
	Args:         cobra.ArbitraryArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		links, err := validateLinks(args)
		if err != nil {
			return err
		}
		return commands.RunTUI(cmd.Context(), links)
	},
}

// validateLinks checks the positional links before any daemon is started, so
// an unusable argument fails fast with a non-zero exit code.
func validateLinks(args []string) ([]string, error) {
	links := make([]string, 0, len(args))
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			continue
		}
		if err := tui.ValidateURI(trimmed); err != nil {
			return nil, err
		}
		links = append(links, trimmed)
	}
	return links, nil
}

func init() {
	// The value is pre-parsed in main so the theme is already applied when
	// Cobra renders help.
	rootCmd.PersistentFlags().String("theme", "", "Set the theme (auto/dark/light)")

	rootCmd.AddCommand(commands.VersionCommand())
}
