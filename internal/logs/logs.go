package logs

import (
	"fmt"
	"os"

	"github.com/KevinYouu/aria2-tui/internal/theme"
)

// Warning prints a warning to stderr.
func Warning(text string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", theme.WarningIconStyle.Render("!"), theme.WarningStyle.Render(text))
}

// Error prints an error to stderr.
func Error(text string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", theme.ErrorIconStyle.Render("×"), theme.ErrorStyle.Render(text))
}
