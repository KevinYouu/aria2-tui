package commands

import (
	"fmt"

	"github.com/KevinYouu/aria2-tui/internal/version"
	"github.com/spf13/cobra"
)

// VersionCommand prints the version injected at build time.
func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Aliases: []string{"v"},
		Short:   "Print the aria2-tui version",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s %s\n", "aria2-tui", version.Version)
		},
	}
}
