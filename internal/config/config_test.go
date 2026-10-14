package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KevinYouu/aria2-tui/internal/theme"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "missing.json"))

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Default()
	if cfg.DownloadDir != want.DownloadDir {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, want.DownloadDir)
	}
	if cfg.MaxConcurrentDownloads != DefaultConcurrentDownloads {
		t.Errorf("MaxConcurrentDownloads = %d, want %d", cfg.MaxConcurrentDownloads, DefaultConcurrentDownloads)
	}
	if cfg.Aria2cPath != "" {
		t.Errorf("Aria2cPath = %q, want empty", cfg.Aria2cPath)
	}
	if cfg.Theme != theme.ModeAuto {
		t.Errorf("Theme = %q, want %q", cfg.Theme, theme.ModeAuto)
	}
}

func TestLoadBrokenJSONReportsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2-tui.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := NewStore(path).Load()
	if err == nil {
		t.Fatal("Load() error = nil, want a parse error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("Load() error = %q, want it to mention %q", err, path)
	}
}

func TestLoadKeepsDefaultsForMissingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2-tui.json")
	if err := os.WriteFile(path, []byte(`{"download_dir":"/tmp/aria2-tui-test"}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := NewStore(path).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DownloadDir != "/tmp/aria2-tui-test" {
		t.Errorf("DownloadDir = %q, want /tmp/aria2-tui-test", cfg.DownloadDir)
	}
	if cfg.MaxConcurrentDownloads != DefaultConcurrentDownloads {
		t.Errorf("MaxConcurrentDownloads = %d, want the default %d", cfg.MaxConcurrentDownloads, DefaultConcurrentDownloads)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2-tui.json")
	store := NewStore(path)

	want := Config{
		DownloadDir:            t.TempDir(),
		MaxConcurrentDownloads: 7,
		Aria2cPath:             "/usr/local/bin/aria2c",
		Theme:                  theme.ModeLight,
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file mode = %o, want 600", perm)
	}
	if leftovers, err := filepath.Glob(path + ".tmp*"); err != nil {
		t.Fatalf("Glob() error = %v", err)
	} else if len(leftovers) != 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}

func TestSaveReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2-tui.json")
	store := NewStore(path)

	first := Config{DownloadDir: "/tmp/first", MaxConcurrentDownloads: 2, Theme: theme.ModeDark}
	if err := store.Save(first); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	second := Config{DownloadDir: "/tmp/second", MaxConcurrentDownloads: 3, Theme: theme.ModeLight}
	if err := store.Save(second); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != second {
		t.Errorf("Load() = %+v, want %+v", got, second)
	}
}

func TestSaveReportsUnwritableDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "aria2-tui.json")

	err := NewStore(path).Save(Default())
	if err == nil {
		t.Fatal("Save() error = nil, want an error for a missing directory")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("Save() error = %q, want it to mention %q", err, path)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Save() error = %v, want it to wrap %v", err, fs.ErrNotExist)
	}
}

func TestPathUsesHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if want := filepath.Join(home, fileName); path != want {
		t.Errorf("Path() = %q, want %q", path, want)
	}
}

func TestDefaultDownloadDirIsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := Default().DownloadDir
	if want := filepath.Join(home, "Downloads"); dir != want {
		t.Errorf("Default().DownloadDir = %q, want %q", dir, want)
	}
}

func TestValidate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name         string
		in           Config
		wantDir      string
		wantConc     int
		wantTheme    theme.Mode
		wantAria2    string
		expectsError bool
	}{
		{
			name:      "clamps too many downloads",
			in:        Config{DownloadDir: home, MaxConcurrentDownloads: 99, Theme: theme.ModeDark},
			wantDir:   home,
			wantConc:  MaxConcurrentDownloads,
			wantTheme: theme.ModeDark,
		},
		{
			name:      "clamps zero downloads",
			in:        Config{DownloadDir: home, MaxConcurrentDownloads: 0},
			wantDir:   home,
			wantConc:  MinConcurrentDownloads,
			wantTheme: theme.ModeAuto,
		},
		{
			name:      "invalid theme falls back to auto",
			in:        Config{DownloadDir: home, MaxConcurrentDownloads: 3, Theme: theme.Mode("solarized")},
			wantDir:   home,
			wantConc:  3,
			wantTheme: theme.ModeAuto,
		},
		{
			name:      "expands home prefix",
			in:        Config{DownloadDir: "~/Downloads", MaxConcurrentDownloads: 3, Theme: theme.ModeAuto},
			wantDir:   filepath.Join(home, "Downloads"),
			wantConc:  3,
			wantTheme: theme.ModeAuto,
		},
		{
			name:      "relativises download dir",
			in:        Config{DownloadDir: "downloads", MaxConcurrentDownloads: 3, Theme: theme.ModeAuto},
			wantConc:  3,
			wantTheme: theme.ModeAuto,
		},
		{
			name:      "trims aria2c path",
			in:        Config{DownloadDir: home, MaxConcurrentDownloads: 3, Theme: theme.ModeAuto, Aria2cPath: "  /opt/aria2c  "},
			wantDir:   home,
			wantConc:  3,
			wantTheme: theme.ModeAuto,
			wantAria2: "/opt/aria2c",
		},
		{
			name:         "rejects empty download dir",
			in:           Config{DownloadDir: "", MaxConcurrentDownloads: 3},
			expectsError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.in.Validate()
			if tt.expectsError {
				if err == nil {
					t.Fatal("Validate() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !filepath.IsAbs(got.DownloadDir) {
				t.Errorf("DownloadDir = %q, want an absolute path", got.DownloadDir)
			}
			if tt.wantDir != "" && got.DownloadDir != tt.wantDir {
				t.Errorf("DownloadDir = %q, want %q", got.DownloadDir, tt.wantDir)
			}
			if got.MaxConcurrentDownloads != tt.wantConc {
				t.Errorf("MaxConcurrentDownloads = %d, want %d", got.MaxConcurrentDownloads, tt.wantConc)
			}
			if got.Theme != tt.wantTheme {
				t.Errorf("Theme = %q, want %q", got.Theme, tt.wantTheme)
			}
			if tt.wantAria2 != "" && got.Aria2cPath != tt.wantAria2 {
				t.Errorf("Aria2cPath = %q, want %q", got.Aria2cPath, tt.wantAria2)
			}
		})
	}
}

func TestValidateExpandsAria2cPathHomePrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := Config{DownloadDir: home, MaxConcurrentDownloads: 3, Aria2cPath: "~/bin/aria2c"}.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if want := filepath.Join(home, "bin", "aria2c"); got.Aria2cPath != want {
		t.Errorf("Aria2cPath = %q, want %q", got.Aria2cPath, want)
	}
}

// The stored file is user editable, so unknown keys are ignored rather than
// rejected: a newer config field must not break an older binary.
func TestLoadIgnoresUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aria2-tui.json")
	payload, err := json.Marshal(map[string]any{
		"download_dir":             "/tmp/aria2-tui-unknown",
		"max_concurrent_downloads": 4,
		"future_option":            "value",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := NewStore(path).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MaxConcurrentDownloads != 4 {
		t.Errorf("MaxConcurrentDownloads = %d, want 4", cfg.MaxConcurrentDownloads)
	}
}
