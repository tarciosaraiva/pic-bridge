package picb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	firstRel  = "DCIM/100MSDCF/DSC00001.ARW"
	firstDest = "wedding/original/20261107_0001.ARW"
)

// fakeDevices makes card look like a mounted volume and dest like another disk.
func fakeDevices(t *testing.T, card, dest string) {
	t.Helper()
	withDevices(t, func(p string) (uint64, error) {
		switch {
		case p == card:
			return 1, nil
		case p == filepath.Dir(card):
			return 2, nil
		case p == dest || strings.HasPrefix(p, dest+string(filepath.Separator)):
			return 3, nil
		}
		return 0, fmt.Errorf("fakeDevices: unexpected path %s", p)
	})
}

func withDevices(t *testing.T, f func(string) (uint64, error)) {
	t.Helper()
	old := deviceID
	deviceID = f
	t.Cleanup(func() { deviceID = old })
}

// importedCard imports happyCard and fakes the devices so the gate can pass.
func importedCard(t *testing.T) (Config, Summary) {
	t.Helper()
	cfg := runCfg(fakeCard(t, happyCard), t)
	s := mustRun(t, cfg)
	fakeDevices(t, s.Source, cfg.Destination)
	return cfg, s
}

func gate(t *testing.T, cfg Config, src string, files []SourceFile, ignored []string) []Blocker {
	t.Helper()
	b, err := CheckClean(context.Background(), cfg, src, files, ignored, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func wantBlockers(t *testing.T, got []Blocker, want ...Blocker) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("blockers = %+v, want %+v", got, want)
	}
}

func TestRunSummarySourceAndFiles(t *testing.T) {
	card := fakeCard(t, happyCard)
	s := mustRun(t, runCfg(card, t))
	if s.Source != card || len(s.Files) != 5 {
		t.Fatalf("Source = %q, %d files; want %q, 5", s.Source, len(s.Files), card)
	}
}

func TestCheckCleanPasses(t *testing.T) {
	cfg, s := importedCard(t)
	var out bytes.Buffer
	b, err := CheckClean(context.Background(), cfg, s.Source, s.Files, s.Ignored, &out)
	if err != nil {
		t.Fatal(err)
	}
	wantBlockers(t, b)
	if !strings.Contains(out.String(), "Verifying [5/5] ") {
		t.Errorf("progress = %q", out.String())
	}
}

func TestCheckCleanRoot(t *testing.T) {
	cfg := runCfg("/", t)
	wantBlockers(t, gate(t, cfg, "/", nil, nil), Blocker{"/", "is the filesystem root"})
}

func TestCheckCleanNotMounted(t *testing.T) {
	cfg, s := importedCard(t)
	withDevices(t, func(string) (uint64, error) { return 1, nil })
	wantBlockers(t, gate(t, cfg, s.Source, s.Files, s.Ignored), Blocker{s.Source, "not a mounted volume"})
}

// A symlink to a folder on another disk has a different device from its
// parent, but it is not a volume.
func TestCheckCleanSymlinkSource(t *testing.T) {
	cfg, s := importedCard(t)
	link := filepath.Join(t.TempDir(), "card")
	if err := os.Symlink(s.Source, link); err != nil {
		t.Fatal(err)
	}
	fakeDevices(t, link, cfg.Destination)
	wantBlockers(t, gate(t, cfg, link, s.Files, s.Ignored), Blocker{link, "not a mounted volume"})
}

func TestCheckCleanSourceTrailingSlash(t *testing.T) {
	cfg, s := importedCard(t)
	wantBlockers(t, gate(t, cfg, s.Source+string(filepath.Separator), s.Files, s.Ignored))
}

// extraEntry writes one more manifest (sorting before the import's) with an
// entry for the same card file as s.Files[0].
func extraEntry(t *testing.T, cfg Config, f SourceFile, sha, dest string) {
	t.Helper()
	m, err := CreateManifest(filepath.Join(cfg.Destination, ".pic-bridge", "manifests"), RunHeader{StartedAt: runStart})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Append(FileEntry{Source: f.Rel, Size: f.Size, Modified: f.ModTime, Destination: dest, SHA256: sha}); err != nil {
		t.Fatal(err)
	}
	m.Close()
}

// After a re-import one card file has several entries: it passes when any
// one of them matches the card and its destination.
func TestCheckCleanAnyEntryPasses(t *testing.T) {
	cfg, s := importedCard(t)
	f := s.Files[0]
	sum, err := hashFile(f.Abs)
	if err != nil || f.Rel != firstRel {
		t.Fatal(f.Rel, err)
	}
	extraEntry(t, cfg, f, "stale", "wedding/original/gone.ARW")
	extraEntry(t, cfg, f, sum, "wedding/original/gone.ARW")
	wantBlockers(t, gate(t, cfg, s.Source, s.Files, s.Ignored))

	os.Remove(filepath.Join(cfg.Destination, filepath.FromSlash(firstDest)))
	wantBlockers(t, gate(t, cfg, s.Source, s.Files, s.Ignored), Blocker{firstRel, "destination " + firstDest + " missing"})
}

func TestCheckCleanNoDCIM(t *testing.T) {
	card := t.TempDir()
	cfg := runCfg(card, t)
	fakeDevices(t, card, cfg.Destination)
	wantBlockers(t, gate(t, cfg, card, nil, nil), Blocker{card, "no DCIM folder"})
}

func TestCheckCleanDestinationOnCard(t *testing.T) {
	cfg, s := importedCard(t)
	withDevices(t, func(p string) (uint64, error) {
		if p == filepath.Dir(s.Source) {
			return 2, nil
		}
		return 1, nil
	})
	wantBlockers(t, gate(t, cfg, s.Source, s.Files, s.Ignored), Blocker{s.Source, "destination is on the card"})
}

func TestCheckCleanIgnoredBlocks(t *testing.T) {
	card := fakeCard(t, happyCard)
	os.WriteFile(filepath.Join(card, "DCIM", "100MSDCF", "X.DNG"), []byte("raw"), 0o644)
	cfg := runCfg(card, t)
	s := mustRun(t, cfg)
	fakeDevices(t, card, cfg.Destination)
	wantBlockers(t, gate(t, cfg, s.Source, s.Files, s.Ignored), Blocker{"DCIM/100MSDCF/X.DNG", "not imported (unknown file)"})
}

func TestCheckCleanCardFileChanged(t *testing.T) {
	cfg, s := importedCard(t)
	p := filepath.Join(s.Source, filepath.FromSlash(firstRel))
	os.WriteFile(p, []byte("shot again, longer"), 0o644)
	os.Chtimes(p, at(15, 0), at(15, 0))
	files, ignored, _, err := Scan(s.Source)
	if err != nil {
		t.Fatal(err)
	}
	wantBlockers(t, gate(t, cfg, s.Source, files, ignored), Blocker{firstRel, "no manifest entry"})
}

func TestCheckCleanCardBadRead(t *testing.T) {
	cfg, s := importedCard(t)
	p := filepath.Join(s.Source, filepath.FromSlash(firstRel))
	os.WriteFile(p, []byte(strings.Repeat("X", len("data:"+firstRel))), 0o644)
	os.Chtimes(p, happyCard[firstRel], happyCard[firstRel])
	wantBlockers(t, gate(t, cfg, s.Source, s.Files, s.Ignored), Blocker{firstRel, "card re-read does not match the manifest"})
}

func TestCheckCleanAfterReRun(t *testing.T) {
	cfg, _ := importedCard(t)
	s := mustRun(t, cfg)
	if s.Copied != 0 || s.Skipped != 5 {
		t.Fatalf("second run copied %d skipped %d", s.Copied, s.Skipped)
	}
	wantBlockers(t, gate(t, cfg, s.Source, s.Files, s.Ignored))
}

func TestCheckCleanDestMissing(t *testing.T) {
	cfg, s := importedCard(t)
	os.Remove(filepath.Join(cfg.Destination, filepath.FromSlash(firstDest)))
	wantBlockers(t, gate(t, cfg, s.Source, s.Files, s.Ignored), Blocker{firstRel, "destination " + firstDest + " missing"})
}

func TestCheckCleanDestEdited(t *testing.T) {
	cfg, s := importedCard(t)
	os.WriteFile(filepath.Join(cfg.Destination, filepath.FromSlash(firstDest)), []byte("edited"), 0o644)
	wantBlockers(t, gate(t, cfg, s.Source, s.Files, s.Ignored), Blocker{firstRel, "destination " + firstDest + " does not match the manifest"})
}

func TestEraseCard(t *testing.T) {
	root := fakeCard(t, happyCard)
	os.WriteFile(filepath.Join(root, "DCIM", ".hidden"), nil, 0o644)
	os.Mkdir(filepath.Join(root, ".Trashes"), 0o755)
	if fails := EraseCard(root); fails != nil {
		t.Fatal(fails)
	}
	ents, _ := os.ReadDir(root)
	if len(ents) != 1 || ents[0].Name() != ".Trashes" {
		t.Fatalf("left %v", ents)
	}
}

func TestEraseCardKeepsGoing(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores folder permissions")
	}
	root := t.TempDir()
	for _, rel := range []string{"A/locked.txt", "B/y", "DCIM/x"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, nil, 0o644)
	}
	a := filepath.Join(root, "A")
	os.Chmod(a, 0o555)
	t.Cleanup(func() { os.Chmod(a, 0o755) })
	fails := EraseCard(root)
	if len(fails) != 1 || !strings.HasPrefix(fails[0], a) {
		t.Fatalf("fails = %v, want one naming %s", fails, a)
	}
	ents, _ := os.ReadDir(root)
	if len(ents) != 1 || ents[0].Name() != "A" {
		t.Fatalf("left %v, want only A", ents)
	}
}

func TestEraseCardUnreadableRoot(t *testing.T) {
	if fails := EraseCard(filepath.Join(t.TempDir(), "gone")); len(fails) != 1 {
		t.Fatalf("fails = %v, want the read error", fails)
	}
}

func TestCheckCleanCancelled(t *testing.T) {
	cfg, s := importedCard(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CheckClean(ctx, cfg, s.Source, s.Files, s.Ignored, io.Discard); err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
