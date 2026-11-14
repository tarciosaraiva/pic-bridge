// Package picb imports photos and videos from camera cards.
package picb

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is the user's config.yaml.
type Config struct {
	Source       string `yaml:"source"`
	Destination  string `yaml:"destination"`
	DateFormat   string `yaml:"date_format"`
	IncludeTime  bool   `yaml:"include_time"`
	UseEXIF      bool   `yaml:"use_exif"`
	SplitRawJPEG bool   `yaml:"split_raw_jpeg"`
	Workers      int    `yaml:"workers"`
	VerifySource bool   `yaml:"verify_source"`
	// ArchiveTargets are absolute folders or "auto" (an external drive).
	ArchiveTargets []string `yaml:"archive_targets"`
}

// ErrNoConfig is returned (wrapped) when the config file does not exist.
var ErrNoConfig = errors.New("config file not found")

// SampleConfig is printed when no config file exists.
const SampleConfig = `source: auto                  # default: auto. Or an explicit path to the card root.
destination: ~/Pictures/Imports   # required
date_format: YYYYMMDD         # default YYYYMMDD
include_time: false           # default false; true adds _HHmmss to FILE names only
use_exif: true                # default true; false = use file modified time
split_raw_jpeg: false         # default false; true = RAW/ and JPEG/ subfolders
workers: 2                    # parallel copies; 2 saturated a USB card reader
verify_source: false          # default false; true reads each card file twice (slower, catches bad card reads)
# archive_targets:            # needed for --archive (rclone required)
#   - auto                    # a plugged-in external hard drive, confirmed each run (macOS)
#   - /Volumes/Backup/photos
`

const timeLayout = "150405"

var dateTokens = strings.NewReplacer("YYYY", "2006", "MM", "01", "DD", "02")

// DefaultConfigPath returns $HOME/.pic-bridge/config.yaml.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pic-bridge", "config.yaml"), nil
}

// WriteSampleConfig writes SampleConfig to path, creating its folder.
// It never overwrites an existing file.
func WriteSampleConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(SampleConfig); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// LoadConfig reads path, applies defaults, expands ~ and validates.
func LoadConfig(path string) (Config, error) {
	cfg := Config{Source: "auto", DateFormat: "YYYYMMDD", UseEXIF: true, Workers: 2}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, fmt.Errorf("%w: %s", ErrNoConfig, path)
	}
	if err != nil {
		return cfg, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // a typo like use_exf must not silently fall back to a default
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Destination == "" {
		return cfg, errors.New("config: destination is required")
	}
	if cfg.Source == "" {
		return cfg, errors.New("config: source must be auto or a path")
	}
	for _, tok := range []string{"HH", "mm", "ss"} {
		if strings.Contains(cfg.DateFormat, tok) {
			return cfg, fmt.Errorf("config: date_format %q must not contain time tokens (use include_time)", cfg.DateFormat)
		}
	}
	// Only YYYY/MM/DD plus separators: anything else would reach Go's layout
	// parser (e.g. "1" = month, "Jan") or a path separator.
	rest := strings.NewReplacer("YYYY", "", "MM", "", "DD", "").Replace(cfg.DateFormat)
	if rest == cfg.DateFormat || strings.Trim(rest, "-_. ") != "" ||
		strings.Trim(cfg.DateFormat, "-_. ") != cfg.DateFormat {
		return cfg, fmt.Errorf("config: date_format %q may only use YYYY, MM, DD joined by - _ . or space", cfg.DateFormat)
	}
	// Go's layout parser gives some separator runs meaning (e.g. "__2" = day of
	// year), so check the layout formats a known date exactly as written.
	probe := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	want := strings.NewReplacer("YYYY", "2026", "MM", "11", "DD", "07").Replace(cfg.DateFormat)
	if probe.Format(cfg.dateLayout()) != want {
		return cfg, fmt.Errorf("config: date_format %q is ambiguous; avoid repeated separators", cfg.DateFormat)
	}
	if cfg.Workers < 1 {
		return cfg, errors.New("config: workers must be at least 1")
	}
	if cfg.Source, err = expandHome(cfg.Source); err != nil {
		return cfg, err
	}
	if cfg.Destination, err = expandHome(cfg.Destination); err != nil {
		return cfg, err
	}
	if cfg.Destination, err = filepath.Abs(cfg.Destination); err != nil {
		return cfg, err
	}
	for i, t := range cfg.ArchiveTargets {
		if t == "auto" {
			continue
		}
		if t, err = expandHome(t); err != nil {
			return cfg, err
		}
		// ponytail: rclone remotes ("b2:photos") are rejected until #53.
		if !filepath.IsAbs(t) {
			return cfg, fmt.Errorf("config: archive target %q must be auto or an absolute path", cfg.ArchiveTargets[i])
		}
		cfg.ArchiveTargets[i] = filepath.Clean(t)
	}
	return cfg, nil
}

func expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}

// ValidateFolderName requires a single, non-dot path segment.
func ValidateFolderName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, '/') {
		return fmt.Errorf("folder name %q must be a single folder name (no '/', not '.' or '..')", name)
	}
	return nil
}

func (c Config) dateLayout() string { return dateTokens.Replace(c.DateFormat) }
