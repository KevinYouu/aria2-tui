package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/KevinYouu/aria2-tui/internal/theme"
)

// Limits for the number of parallel downloads; aria2 accepts the same range.
const (
	MinConcurrentDownloads     = 1
	MaxConcurrentDownloads     = 16
	DefaultConcurrentDownloads = 5
)

// Limits and defaults of the per download connection settings; aria2 accepts
// 1-16 for both.
const (
	MinConnections            = 1
	MaxConnections            = 16
	DefaultConnectionPerHost  = 16
	DefaultSplit              = 16
	DefaultMinSplitSize       = "4M"
	DefaultDownloadSpeedLimit = "0"
)

// sizePattern matches an aria2 size such as 0, 512K, 4M or 1G.
var sizePattern = regexp.MustCompile(`^[0-9]+[KkMmGg]?$`)

const (
	fileName = ".aria2-tui.json"
	filePerm = 0o600
)

// Config is the persisted user configuration.
type Config struct {
	DownloadDir            string     `json:"download_dir"`
	MaxConcurrentDownloads int        `json:"max_concurrent_downloads"`
	Aria2cPath             string     `json:"aria2c_path"`
	Theme                  theme.Mode `json:"theme"`

	// MaxConnectionPerServer and Split bound the connections of one download
	// (aria2 max-connection-per-server and split).
	MaxConnectionPerServer int `json:"max_connection_per_server"`
	Split                  int `json:"split"`
	// MinSplitSize is the smallest chunk worth a separate connection, e.g. 4M.
	MinSplitSize string `json:"min_split_size"`
	// DownloadLimit caps the overall download speed, e.g. 2M; 0 is unlimited.
	DownloadLimit string `json:"download_limit"`
	// Proxy is an optional proxy for all protocols, e.g. http://127.0.0.1:7890.
	Proxy string `json:"proxy"`
}

// Default returns the configuration used when no file exists yet.
func Default() Config {
	return Config{
		DownloadDir:            defaultDownloadDir(),
		MaxConcurrentDownloads: DefaultConcurrentDownloads,
		Theme:                  theme.ModeAuto,
		MaxConnectionPerServer: DefaultConnectionPerHost,
		Split:                  DefaultSplit,
		MinSplitSize:           DefaultMinSplitSize,
		DownloadLimit:          DefaultDownloadSpeedLimit,
	}
}

// defaultDownloadDir returns ~/Downloads, or an empty string when the home
// directory cannot be determined (Validate then reports it).
func defaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Downloads")
}

// Path returns the configuration file location (~/.aria2-tui.json).
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, fileName), nil
}

// Store reads and writes the configuration file at Path.
type Store struct {
	Path string
}

// NewStore returns a store for the given configuration file.
func NewStore(path string) *Store {
	return &Store{Path: path}
}

// Load reads the configuration. A missing file yields Default(); a corrupt
// file is reported with its path instead of silently resetting user settings.
func (s *Store) Load() (Config, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("read config %s: %w", s.Path, err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", s.Path, err)
	}
	return cfg, nil
}

// Save writes the configuration atomically: a temporary file in the target
// directory is renamed over the destination, so a crash cannot leave a
// half-written config behind.
func (s *Store) Save(cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.Path)
	tmp, err := os.CreateTemp(dir, fileName+".tmp")
	if err != nil {
		return fmt.Errorf("write config %s: %w", s.Path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write config %s: %w", s.Path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write config %s: %w", s.Path, err)
	}
	if err := os.Chmod(tmpPath, filePerm); err != nil {
		return fmt.Errorf("write config %s: %w", s.Path, err)
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("write config %s: %w", s.Path, err)
	}
	return nil
}

// Validate normalises the configuration: the download directory is expanded
// and made absolute, the concurrency is clamped and an unusable theme falls
// back to auto. It does not touch the filesystem.
func (c Config) Validate() (Config, error) {
	dir, err := expandHome(strings.TrimSpace(c.DownloadDir))
	if err != nil {
		return Config{}, err
	}
	if dir == "" {
		return Config{}, errors.New("download directory is not configured: set download_dir in the config file")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve download directory %s: %w", dir, err)
	}
	c.DownloadDir = absDir

	c.MaxConcurrentDownloads = clamp(c.MaxConcurrentDownloads, MinConcurrentDownloads, MaxConcurrentDownloads)
	if !theme.ValidMode(c.Theme) {
		c.Theme = theme.ModeAuto
	}

	c.MaxConnectionPerServer = clamp(c.MaxConnectionPerServer, MinConnections, MaxConnections)
	c.Split = clamp(c.Split, MinConnections, MaxConnections)

	c.MinSplitSize = strings.ToUpper(strings.TrimSpace(c.MinSplitSize))
	if c.MinSplitSize == "" {
		c.MinSplitSize = DefaultMinSplitSize
	}
	if !sizePattern.MatchString(c.MinSplitSize) {
		return Config{}, fmt.Errorf("invalid min split size %q: use a number with an optional K, M or G suffix", c.MinSplitSize)
	}
	c.DownloadLimit = strings.ToUpper(strings.TrimSpace(c.DownloadLimit))
	if c.DownloadLimit == "" {
		c.DownloadLimit = DefaultDownloadSpeedLimit
	}
	if !sizePattern.MatchString(c.DownloadLimit) {
		return Config{}, fmt.Errorf("invalid download limit %q: use 0 or a number with an optional K, M or G suffix", c.DownloadLimit)
	}
	c.Proxy = strings.TrimSpace(c.Proxy)

	aria2cPath, err := expandHome(strings.TrimSpace(c.Aria2cPath))
	if err != nil {
		return Config{}, err
	}
	c.Aria2cPath = aria2cPath

	return c, nil
}

// Aria2Options returns the aria2 options that mirror this configuration,
// except the download directory and the concurrency which have their own
// handling. The values are applied at daemon start and on every change.
func (c Config) Aria2Options() map[string]string {
	return map[string]string{
		"max-connection-per-server":  strconv.Itoa(c.MaxConnectionPerServer),
		"split":                      strconv.Itoa(c.Split),
		"min-split-size":             c.MinSplitSize,
		"max-overall-download-limit": c.DownloadLimit,
		"all-proxy":                  c.Proxy,
	}
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func clamp(value, low, high int) int {
	return max(low, min(value, high))
}
