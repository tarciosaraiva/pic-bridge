package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tarciosaraiva/pic-bridge/internal/picb"
)

const firstDest = "wedding/original/20261107_0001.ARW"

// importedCard imports a two-photo temp card and makes it look like a
// mounted volume, with the destination on another disk.
func importedCard(t *testing.T) (picb.Config, picb.Summary) {
	t.Helper()
	card := t.TempDir()
	shot := []string{"DSC00001.ARW", "DSC00002.ARW"}
	for i, name := range shot {
		p := filepath.Join(card, "DCIM", "100MSDCF", name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("data:"+name), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := time.Date(2026, 11, 7, 9+i, 0, 0, 0, time.Local)
		os.Chtimes(p, mt, mt)
	}
	cfg := picb.Config{Source: card, Destination: t.TempDir(), DateFormat: "YYYYMMDD", Workers: 2}
	s, err := picb.Run(context.Background(), cfg, "wedding", io.Discard)
	if err != nil || s.Copied != len(shot) {
		t.Fatalf("import: copied %d, %v", s.Copied, err)
	}
	t.Cleanup(picb.SetDeviceIDForTest(func(p string) (uint64, error) {
		switch p {
		case card:
			return 1, nil
		case cfg.Destination:
			return 3, nil
		}
		return 2, nil
	}))
	return cfg, s
}

// cardLeft lists what is in the card root.
func cardLeft(t *testing.T, card string) []string {
	t.Helper()
	ents, err := os.ReadDir(card)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

func wantIntact(t *testing.T, card string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(card, "DCIM", "100MSDCF", "DSC00001.ARW")); err != nil {
		t.Fatalf("card was touched: %v", err)
	}
}

func wantRefused(t *testing.T, code int, out, line string) {
	t.Helper()
	if code != 1 || !strings.Contains(out, line+"\n") {
		t.Fatalf("exit %d, output %q; want exit 1 and %q", code, out, line)
	}
	if strings.Contains(out, "[y/N]") {
		t.Fatalf("prompted despite refusal: %q", out)
	}
}

func TestCleanCardRefusesAfterFailure(t *testing.T) {
	cfg, s := importedCard(t)
	s.Failed = map[string]string{"DCIM/100MSDCF/DSC00003.ARW": "boom"}
	var out bytes.Buffer
	code := cleanCard(context.Background(), cfg, s, strings.NewReader("y\n"), true, &out)
	wantRefused(t, code, out.String(), "Not cleaning: the import did not finish cleanly.")
	wantIntact(t, s.Source)
}

func TestCleanCardRefusesNotAttempted(t *testing.T) {
	cfg, s := importedCard(t)
	s.NotAttempted = 1
	var out bytes.Buffer
	code := cleanCard(context.Background(), cfg, s, strings.NewReader("y\n"), true, &out)
	wantRefused(t, code, out.String(), "Not cleaning: the import did not finish cleanly.")
	wantIntact(t, s.Source)
}

func TestCleanCardRefusesWhenStopped(t *testing.T) {
	cfg, s := importedCard(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	code := cleanCard(ctx, cfg, s, strings.NewReader("y\n"), true, &out)
	wantRefused(t, code, out.String(), "Not cleaning: stopped.")
	wantIntact(t, s.Source)
}

func TestCleanCardNoTerminal(t *testing.T) {
	cfg, s := importedCard(t)
	var out bytes.Buffer
	code := cleanCard(context.Background(), cfg, s, strings.NewReader("y\n"), false, &out)
	wantRefused(t, code, out.String(), "Not cleaning: no terminal")
	wantIntact(t, s.Source)
}

func TestCleanCardAnswerNo(t *testing.T) {
	for _, in := range []string{"n\n", "", "yes\n", "\n"} {
		cfg, s := importedCard(t)
		var out bytes.Buffer
		code := cleanCard(context.Background(), cfg, s, strings.NewReader(in), true, &out)
		if code != 0 || !strings.HasSuffix(out.String(), "Card left as is.\n") {
			t.Fatalf("answer %q: exit %d, output %q", in, code, out.String())
		}
		wantIntact(t, s.Source)
	}
}

func TestCleanCardErases(t *testing.T) {
	for _, in := range []string{"y\n", "Y\n"} {
		cfg, s := importedCard(t)
		os.Mkdir(filepath.Join(s.Source, ".Trashes"), 0o755)
		var out bytes.Buffer
		code := cleanCard(context.Background(), cfg, s, strings.NewReader(in), true, &out)
		want := "All 2 files verified against " + cfg.Destination + ". Erase everything on card " + s.Source + "? [y/N] " +
			"Card " + s.Source + " erased. Eject it before removing it.\n"
		if code != 0 || !strings.HasSuffix(out.String(), want) {
			t.Fatalf("answer %q: exit %d, output %q; want suffix %q", in, code, out.String(), want)
		}
		if left := cardLeft(t, s.Source); len(left) != 1 || left[0] != ".Trashes" {
			t.Fatalf("card still has %v", left)
		}
		if _, err := os.Stat(filepath.Join(cfg.Destination, filepath.FromSlash(firstDest))); err != nil {
			t.Fatalf("destination lost: %v", err)
		}
	}
}

func TestCleanCardBlockersListed(t *testing.T) {
	cfg, s := importedCard(t)
	os.Remove(filepath.Join(cfg.Destination, filepath.FromSlash(firstDest)))
	var out bytes.Buffer
	code := cleanCard(context.Background(), cfg, s, strings.NewReader("y\n"), true, &out)
	wantRefused(t, code, out.String(), "Not cleaning the card:\n  DCIM/100MSDCF/DSC00001.ARW: destination "+firstDest+" missing")
	wantIntact(t, s.Source)
}

// cancelOnRead answers "y" but Ctrl-C lands while the prompt is waiting.
type cancelOnRead struct{ cancel context.CancelFunc }

func (c cancelOnRead) Read(p []byte) (int, error) {
	c.cancel()
	return copy(p, "y\n"), nil
}

func TestCleanCardStoppedAtPrompt(t *testing.T) {
	cfg, s := importedCard(t)
	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	code := cleanCard(ctx, cfg, s, cancelOnRead{cancel}, true, &out)
	if code != 1 || !strings.HasSuffix(out.String(), "Not cleaning: stopped.\n") {
		t.Fatalf("exit %d, output %q", code, out.String())
	}
	wantIntact(t, s.Source)
}

func TestCleanCardEraseFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores folder permissions")
	}
	cfg, s := importedCard(t)
	dcim := filepath.Join(s.Source, "DCIM")
	os.Chmod(dcim, 0o555)
	t.Cleanup(func() { os.Chmod(dcim, 0o755) })
	var out bytes.Buffer
	code := cleanCard(context.Background(), cfg, s, strings.NewReader("y\n"), true, &out)
	if code != 1 || !strings.Contains(out.String(), "Could not remove:\n  "+dcim+": ") {
		t.Fatalf("exit %d, output %q", code, out.String())
	}
}

func TestCleanCardRefusesAfterScanError(t *testing.T) {
	cfg, s := importedCard(t)
	s.ScanErrors = []string{"DCIM/101MSDCF: permission denied"}
	var out bytes.Buffer
	code := cleanCard(context.Background(), cfg, s, strings.NewReader("y\n"), true, &out)
	wantRefused(t, code, out.String(), "Not cleaning: the card could not be read completely.")
	wantIntact(t, s.Source)
}
