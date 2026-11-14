package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarciosaraiva/pic-bridge/internal/picb"
)

// fakeRclone puts an rclone on PATH that copies with cp and checks with
// diff -r. A target path containing "broken" fails its copy. Each call is
// appended to the returned log file.
func fakeRclone(t *testing.T) (log string) {
	t.Helper()
	bin := t.TempDir()
	log = filepath.Join(bin, "calls")
	script := `#!/bin/sh
echo "$@" >> "` + log + `"
cmd=$1; shift
while [ $# -gt 2 ]; do shift; done
case "$2" in *broken*) exit 3;; esac
case $cmd in
copy) mkdir -p "$2" && cp -R "$1/." "$2/";;
check) diff -r "$1" "$2" >/dev/null;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "rclone"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// shoot makes <dest>/wedding with one photo and returns the config.
func shoot(t *testing.T, targets ...string) picb.Config {
	t.Helper()
	dest := t.TempDir()
	p := filepath.Join(dest, "wedding", "original", "20261107_0001.ARW")
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte("photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	return picb.Config{Destination: dest, ArchiveTargets: targets}
}

func archive(cfg picb.Config, answer string, tty bool) (int, string) {
	var out bytes.Buffer
	code := archiveFolder(context.Background(), cfg, "wedding", strings.NewReader(answer), tty, false, &out)
	return code, out.String()
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestArchiveAllPassDeletes(t *testing.T) {
	fakeRclone(t)
	a, b := t.TempDir(), t.TempDir()
	cfg := shoot(t, a, b)
	code, out := archive(cfg, "y\n", true)
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if exists(filepath.Join(cfg.Destination, "wedding")) {
		t.Error("local folder kept")
	}
	for _, d := range []string{a, b} {
		if !exists(filepath.Join(d, "wedding", "original", "20261107_0001.ARW")) {
			t.Errorf("%s: no archived copy", d)
		}
	}
}

func TestArchiveAnswerNoKeeps(t *testing.T) {
	fakeRclone(t)
	cfg := shoot(t, t.TempDir())
	if code, out := archive(cfg, "\n", true); code != 0 || !strings.Contains(out, "Kept") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !exists(filepath.Join(cfg.Destination, "wedding")) {
		t.Error("local folder deleted")
	}
}

func TestArchiveNoTerminalKeeps(t *testing.T) {
	fakeRclone(t)
	cfg := shoot(t, t.TempDir())
	if code, out := archive(cfg, "y\n", false); code != 1 || !strings.Contains(out, "no terminal") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !exists(filepath.Join(cfg.Destination, "wedding")) {
		t.Error("local folder deleted")
	}
}

func TestArchiveFailedTargetKeepsAndCarriesOn(t *testing.T) {
	fakeRclone(t)
	broken := filepath.Join(t.TempDir(), "broken")
	os.Mkdir(broken, 0o755)
	good := t.TempDir()
	cfg := shoot(t, broken, good)
	code, out := archive(cfg, "y\n", true)
	if code != 1 || !strings.Contains(out, "Not deleting") || !strings.Contains(out, "exit status 3") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !exists(filepath.Join(cfg.Destination, "wedding")) {
		t.Error("local folder deleted")
	}
	if !exists(filepath.Join(good, "wedding")) {
		t.Error("stopped after the failed target")
	}
}

func TestArchiveExtraFileOnTargetFails(t *testing.T) {
	fakeRclone(t)
	target := t.TempDir()
	os.MkdirAll(filepath.Join(target, "wedding"), 0o755)
	os.WriteFile(filepath.Join(target, "wedding", "other-shoot.ARW"), []byte("x"), 0o644)
	cfg := shoot(t, target)
	if code, out := archive(cfg, "y\n", true); code != 1 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !exists(filepath.Join(cfg.Destination, "wedding")) {
		t.Error("local folder deleted")
	}
}

func TestArchiveMissingTargetCopiesNothing(t *testing.T) {
	log := fakeRclone(t)
	cfg := shoot(t, t.TempDir(), filepath.Join(t.TempDir(), "unplugged"))
	if code, out := archive(cfg, "y\n", true); code != 1 || !strings.Contains(out, "plugged in") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if exists(log) {
		t.Error("rclone ran")
	}
}

func TestArchiveTargetOverlappingSourceRefused(t *testing.T) {
	log := fakeRclone(t)
	cfg := shoot(t)
	cfg.ArchiveTargets = []string{cfg.Destination}
	if code, out := archive(cfg, "y\n", true); code != 1 || !strings.Contains(out, "overlaps") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if exists(log) {
		t.Error("rclone ran")
	}
}

func TestArchiveCountsTargetsPerDrive(t *testing.T) {
	fakeRclone(t)
	drive := t.TempDir()
	a, b := filepath.Join(drive, "a"), filepath.Join(drive, "b")
	os.Mkdir(a, 0o755)
	os.Mkdir(b, 0o755)
	cfg := shoot(t, a, b, a)
	code, out := archive(cfg, "n\n", true)
	if code != 0 || !strings.Contains(out, "on 1 target(s)") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if strings.Count(out, "Archiving ") != 2 {
		t.Errorf("want 2 copies (duplicate path once)\n%s", out)
	}
}

func TestArchiveRefusesUnfinishedImport(t *testing.T) {
	fakeRclone(t)
	cfg := shoot(t, t.TempDir())
	os.WriteFile(filepath.Join(cfg.Destination, "wedding", "original", ".x.ARW.123.partial"), nil, 0o644)
	if code, out := archive(cfg, "y\n", true); code != 1 || !strings.Contains(out, "unfinished import") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestArchiveSetupErrors(t *testing.T) {
	fakeRclone(t)
	cfg := shoot(t)
	if code, out := archive(cfg, "", true); code != 1 || !strings.Contains(out, "no archive_targets") {
		t.Errorf("no targets: code %d\n%s", code, out)
	}
	cfg.ArchiveTargets = []string{t.TempDir()}
	t.Setenv("PATH", t.TempDir())
	if code, out := archive(cfg, "", true); code != 1 || !strings.Contains(out, "brew install rclone") {
		t.Errorf("no rclone: code %d\n%s", code, out)
	}
	var buf bytes.Buffer
	if code := archiveFolder(context.Background(), cfg, "nope", strings.NewReader(""), true, false, &buf); code != 1 {
		t.Errorf("missing folder: code %d\n%s", code, buf.String())
	}
}

func TestPickDrive(t *testing.T) {
	one := []string{"/Volumes/A/pic-bridge"}
	two := []string{"/Volumes/A/pic-bridge", "/Volumes/B/pic-bridge"}
	for _, c := range []struct {
		drives []string
		answer string
		want   string
	}{
		{one, "y\n", one[0]},
		{one, "\n", ""},
		{two, "2\n", two[1]},
		{two, "\n", ""},
		{two, "3\n", ""},
		{two, "x\n", ""},
	} {
		var out bytes.Buffer
		if got := pickDrive(c.drives, bufio.NewReader(strings.NewReader(c.answer)), &out); got != c.want {
			t.Errorf("%d drives, answer %q: got %q, want %q", len(c.drives), c.answer, got, c.want)
		}
	}
}

func TestArchiveExcludesMacMetadata(t *testing.T) {
	log := fakeRclone(t)
	cfg := shoot(t, t.TempDir())
	archive(cfg, "n\n", true)
	calls, _ := os.ReadFile(log)
	for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		if !strings.Contains(line, "--exclude ._*") || !strings.Contains(line, "--exclude .DS_Store") {
			t.Errorf("rclone call without metadata excludes: %s", line)
		}
	}
}
