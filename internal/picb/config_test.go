package picb

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadConfigDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := LoadConfig(writeConfig(t, "destination: ~/Pictures/Imports\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source != "auto" {
		t.Errorf("Source = %q, want auto", cfg.Source)
	}
	if cfg.DateFormat != "YYYYMMDD" {
		t.Errorf("DateFormat = %q, want YYYYMMDD", cfg.DateFormat)
	}
	if !cfg.UseEXIF {
		t.Error("UseEXIF = false, want true")
	}
	if cfg.IncludeTime {
		t.Error("IncludeTime = true, want false")
	}
	if cfg.SplitRawJPEG {
		t.Error("SplitRawJPEG = true, want false")
	}
	if cfg.Workers != 3 {
		t.Errorf("Workers = %d, want 3", cfg.Workers)
	}
	if want := filepath.Join(home, "Pictures/Imports"); cfg.Destination != want {
		t.Errorf("Destination = %q, want %q", cfg.Destination, want)
	}
}

func TestLoadConfigExplicitFalse(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, "destination: /tmp/x\nuse_exif: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UseEXIF {
		t.Error("UseEXIF = true, want false")
	}
}

func TestLoadConfigMissingDestination(t *testing.T) {
	if _, err := LoadConfig(writeConfig(t, "use_exif: true\n")); err == nil {
		t.Fatal("want error for missing destination")
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if !errors.Is(err, ErrNoConfig) {
		t.Fatalf("err = %v, want ErrNoConfig", err)
	}
}

func TestDateLayout(t *testing.T) {
	for in, want := range map[string]string{
		"YYYYMMDD":   "20060102",
		"YYYY-MM-DD": "2006-01-02",
		"DD.MM.YYYY": "02.01.2006",
	} {
		if got := (Config{DateFormat: in}).dateLayout(); got != want {
			t.Errorf("dateLayout(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadConfigRejectsTimeTokens(t *testing.T) {
	if _, err := LoadConfig(writeConfig(t, "destination: /tmp/x\ndate_format: YYYYMMDD_HHmm\n")); err == nil {
		t.Fatal("want error for time tokens in date_format")
	}
}

func TestValidateFolderName(t *testing.T) {
	for _, ok := range []string{"wedding", "2026 trip"} {
		if err := ValidateFolderName(ok); err != nil {
			t.Errorf("ValidateFolderName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "a/b"} {
		if err := ValidateFolderName(bad); err == nil {
			t.Errorf("ValidateFolderName(%q) = nil, want error", bad)
		}
	}
}

func TestLoadConfigRejectsBadDateFormat(t *testing.T) {
	for _, df := range []string{`""`, "../YYYY", "a/YYYYMMDD", `YYYY\MM`, "YYYYmm", "YYYYss"} {
		if _, err := LoadConfig(writeConfig(t, "destination: /tmp/x\ndate_format: "+df+"\n")); err == nil {
			t.Errorf("date_format %q: want error", df)
		}
	}
}

func TestLoadConfigRejectsEmptySource(t *testing.T) {
	if _, err := LoadConfig(writeConfig(t, "destination: /tmp/x\nsource: \"\"\n")); err == nil {
		t.Fatal("want error for empty source")
	}
}

// A key with no value is YAML null, which keeps the default.
func TestLoadConfigBlankKeyKeepsDefault(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, "destination: /tmp/x\nsource:\ndate_format:\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source != "auto" || cfg.DateFormat != "YYYYMMDD" {
		t.Errorf("Source=%q DateFormat=%q, want defaults", cfg.Source, cfg.DateFormat)
	}
}

func TestLoadConfigRejectsUnknownKey(t *testing.T) {
	if _, err := LoadConfig(writeConfig(t, "destination: /tmp/x\nuse_exf: false\n")); err == nil {
		t.Fatal("want error for unknown key use_exf")
	}
}

func TestLoadConfigRejectsZeroWorkers(t *testing.T) {
	if _, err := LoadConfig(writeConfig(t, "destination: /tmp/x\nworkers: 0\n")); err == nil {
		t.Fatal("want error for workers: 0")
	}
}

func TestLoadConfigExpandsSourceAndAbsDestination(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := LoadConfig(writeConfig(t, "destination: rel/dir\nsource: ~/card\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "card"); cfg.Source != want {
		t.Errorf("Source = %q, want %q", cfg.Source, want)
	}
	if !filepath.IsAbs(cfg.Destination) {
		t.Errorf("Destination %q not absolute", cfg.Destination)
	}
}

func TestLoadConfigDateFormatOnlyTokensAndSeparators(t *testing.T) {
	for _, bad := range []string{"YYYY1", "YYYY-Jan", "YYYY_MMM", "---", "YYYYMMDDx"} {
		if _, err := LoadConfig(writeConfig(t, "destination: /tmp/x\ndate_format: "+bad+"\n")); err == nil {
			t.Errorf("date_format %q: want error", bad)
		}
	}
	for _, ok := range []string{"YYYY-MM-DD", "DD.MM.YYYY", "YYYY_MM_DD", "YYYY MM DD", "YYYYMM"} {
		if _, err := LoadConfig(writeConfig(t, "destination: /tmp/x\ndate_format: "+ok+"\n")); err != nil {
			t.Errorf("date_format %q: %v", ok, err)
		}
	}
}

func TestLoadConfigDateFormatRejectsLiteralDigits(t *testing.T) {
	if _, err := LoadConfig(writeConfig(t, "destination: /tmp/x\ndate_format: 01YYYY\n")); err == nil {
		t.Fatal("want error for literal digits")
	}
}

func TestLoadConfigDateFormatEdges(t *testing.T) {
	// Leading/trailing separators (".YYYY" would create hidden files) and
	// layouts where Go's parser reads "__2"/"_2" as day-of-year/padded day.
	for _, bad := range []string{".YYYYMMDD", "YYYYMMDD ", "_YYYYMMDD", "MM__YYYY", "MM_DD__YYYY"} {
		if _, err := LoadConfig(writeConfig(t, "destination: /tmp/x\ndate_format: \""+bad+"\"\n")); err == nil {
			t.Errorf("date_format %q: want error", bad)
		}
	}
}
