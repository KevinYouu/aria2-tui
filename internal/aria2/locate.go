package aria2

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// installationHint is appended to lookup failures so the user knows where to
// get aria2c.
const installationHint = "install it and retry (macOS: brew install aria2, Debian/Ubuntu: apt install aria2, Fedora: dnf install aria2, Windows: scoop install aria2)"

// LocateAria2c resolves the aria2c executable: an explicit path from the
// configuration wins, otherwise aria2c is looked up in PATH.
func LocateAria2c(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		info, err := os.Stat(configured)
		if err != nil {
			return "", fmt.Errorf("configured aria2c path %q is not usable: %w", configured, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("configured aria2c path %q is a directory", configured)
		}
		// Windows has no execute bit, so only unix-like systems are checked.
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("configured aria2c path %q is not executable", configured)
		}
		return configured, nil
	}

	path, err := exec.LookPath("aria2c")
	if err != nil {
		return "", fmt.Errorf("aria2c was not found in PATH: %s", installationHint)
	}
	return path, nil
}
