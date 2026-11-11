package picb

import (
	"bytes"
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

// fakeCard writes rel → mtime files under a temp card root.
func fakeCard(t *testing.T, files map[string]time.Time) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "DCIM"), 0o755)
	for rel, mt := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("data:"+rel), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(p, mt, mt)
	}
	return root
}

var happyCard = map[string]time.Time{
	"DCIM/100MSDCF/DSC00001.ARW":       at(9, 0),
	"DCIM/100MSDCF/DSC00002.ARW":       at(10, 0),
	"DCIM/100MSDCF/DSC00002.JPG":       at(10, 0),
	"PRIVATE/M4ROOT/CLIP/C0001.MP4":    at(11, 0),
	"PRIVATE/M4ROOT/CLIP/C0001M01.XML": at(11, 0),
}

var happyDests = map[string]string{
	"DCIM/100MSDCF/DSC00001.ARW":       "wedding/original/20261107_0001.ARW",
	"DCIM/100MSDCF/DSC00002.ARW":       "wedding/original/20261107_0002.ARW",
	"DCIM/100MSDCF/DSC00002.JPG":       "wedding/original/20261107_0002.JPG",
	"PRIVATE/M4ROOT/CLIP/C0001.MP4":    "wedding/original/VIDEO/20261107_0003.MP4",
	"PRIVATE/M4ROOT/CLIP/C0001M01.XML": "wedding/original/VIDEO/20261107_0003.XML",
}

func runCfg(card string, t *testing.T) Config {
	return Config{Source: card, Destination: t.TempDir(), DateFormat: "YYYYMMDD", Workers: 2}
}

func mustRun(t *testing.T, cfg Config) Summary {
	t.Helper()
	s, err := Run(context.Background(), cfg, "wedding", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// manifestDocs decodes every document of the only manifest under dest.
func manifestDocs(t *testing.T, dest string) []map[string]any {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(dest, ".pic-bridge", "manifests", "*.yaml"))
	if len(paths) == 0 {
		t.Fatal("no manifest")
	}
	data, _ := os.ReadFile(paths[len(paths)-1])
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var docs []map[string]any
	for {
		var d map[string]any
		if dec.Decode(&d) != nil {
			return docs
		}
		docs = append(docs, d)
	}
}

func kinds(docs []map[string]any) []string {
	var k []string
	for _, d := range docs {
		k = append(k, d["kind"].(string))
	}
	return k
}

func noPartialsUnder(t *testing.T, dir string) {
	t.Helper()
	filepath.WalkDir(dir, func(p string, _ os.DirEntry, _ error) error {
		if strings.HasSuffix(p, ".partial") {
			t.Errorf("leftover %s", p)
		}
		return nil
	})
}

func TestRunHappyPath(t *testing.T) {
	card := fakeCard(t, happyCard)
	cfg := runCfg(card, t)
	s := mustRun(t, cfg)
	if s.Copied != 5 || len(s.Failed) != 0 {
		t.Fatalf("summary = %+v", s)
	}
	for rel, dest := range happyDests {
		got, err := os.ReadFile(filepath.Join(cfg.Destination, dest))
		if err != nil || string(got) != "data:"+rel {
			t.Errorf("%s → %s: %q, %v", rel, dest, got, err)
		}
	}
	if e, err := os.ReadDir(filepath.Join(cfg.Destination, "wedding", "processed")); err != nil || len(e) != 0 {
		t.Errorf("processed/ = %v, %v; want empty dir", e, err)
	}
	want := []string{"run", "file", "file", "file", "file", "file", "summary"}
	if k := kinds(manifestDocs(t, cfg.Destination)); !slices.Equal(k, want) {
		t.Errorf("manifest kinds = %v", k)
	}
}

func TestRunEmptyCard(t *testing.T) {
	cfg := runCfg(fakeCard(t, nil), t)
	s := mustRun(t, cfg)
	if s.Copied != 0 {
		t.Errorf("copied = %d", s.Copied)
	}
	for _, d := range []string{"original", "processed"} {
		if _, err := os.Stat(filepath.Join(cfg.Destination, "wedding", d)); err != nil {
			t.Error(err)
		}
	}
	if k := kinds(manifestDocs(t, cfg.Destination)); !slices.Equal(k, []string{"run", "summary"}) {
		t.Errorf("manifest kinds = %v", k)
	}
}

func TestRunSkipsSecondTime(t *testing.T) {
	cfg := runCfg(fakeCard(t, happyCard), t)
	mustRun(t, cfg)
	s := mustRun(t, cfg)
	if s.Copied != 0 || s.Skipped != 5 {
		t.Errorf("second run copied %d skipped %d; want 0, 5", s.Copied, s.Skipped)
	}
	n := 0
	filepath.WalkDir(filepath.Join(cfg.Destination, "wedding", "original"), func(_ string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if n != 5 {
		t.Errorf("%d files in original/, want 5", n)
	}
}

func TestRunContinuesCounter(t *testing.T) {
	card := fakeCard(t, happyCard)
	cfg := runCfg(card, t)
	mustRun(t, cfg)
	p := filepath.Join(card, "DCIM", "100MSDCF", "DSC00003.ARW")
	os.WriteFile(p, []byte("new"), 0o644)
	os.Chtimes(p, at(12, 0), at(12, 0))
	if s := mustRun(t, cfg); s.Copied != 1 {
		t.Fatalf("copied = %d", s.Copied)
	}
	if _, err := os.Stat(filepath.Join(cfg.Destination, "wedding", "original", "20261107_0004.ARW")); err != nil {
		t.Error(err)
	}
}

func TestRunReportsFailure(t *testing.T) {
	card := fakeCard(t, happyCard)
	bad := filepath.Join(card, "DCIM", "100MSDCF", "DSC00001.ARW")
	os.Chmod(bad, 0)
	t.Cleanup(func() { os.Chmod(bad, 0o644) })
	cfg := runCfg(card, t)
	s := mustRun(t, cfg)
	if _, ok := s.Failed["DCIM/100MSDCF/DSC00001.ARW"]; !ok || s.Copied != 4 {
		t.Fatalf("failed %v copied %d; want DSC00001 failed, 4 copied", s.Failed, s.Copied)
	}
	imported, _ := LoadImported(filepath.Join(cfg.Destination, ".pic-bridge", "manifests"))
	for k := range imported {
		if k.Source == "DCIM/100MSDCF/DSC00001.ARW" {
			t.Error("failed file is in the manifest")
		}
	}
}

func TestRunGroupFailureRecordsNothing(t *testing.T) {
	card := fakeCard(t, happyCard)
	bad := filepath.Join(card, "PRIVATE", "M4ROOT", "CLIP", "C0001M01.XML")
	os.Chmod(bad, 0)
	t.Cleanup(func() { os.Chmod(bad, 0o644) })
	cfg := runCfg(card, t)
	s := mustRun(t, cfg)
	for _, rel := range []string{"PRIVATE/M4ROOT/CLIP/C0001.MP4", "PRIVATE/M4ROOT/CLIP/C0001M01.XML"} {
		if _, ok := s.Failed[rel]; !ok {
			t.Errorf("%s not in Failed %v", rel, s.Failed)
		}
	}
	imported, _ := LoadImported(filepath.Join(cfg.Destination, ".pic-bridge", "manifests"))
	if len(imported) != 3 {
		t.Errorf("manifest has %d files, want the 3 photos only", len(imported))
	}
}

func TestRunCancel(t *testing.T) {
	cfg := runCfg(fakeCard(t, happyCard), t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err := Run(ctx, cfg, "wedding", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if s.Copied != 0 || len(s.Failed) != 0 || s.NotAttempted != 5 {
		t.Errorf("copied %d failed %d not attempted %d; want 0, 0, 5", s.Copied, len(s.Failed), s.NotAttempted)
	}
	noPartialsUnder(t, cfg.Destination)
	if _, err := LoadImported(filepath.Join(cfg.Destination, ".pic-bridge", "manifests")); err != nil {
		t.Error(err)
	}
	docs := manifestDocs(t, cfg.Destination)
	if last := docs[len(docs)-1]; last["kind"] != "summary" || last["not_attempted"] != 5 {
		t.Errorf("last doc = %v, want summary with not_attempted: 5", last)
	}
}

func TestRunProcessedUntouched(t *testing.T) {
	cfg := runCfg(fakeCard(t, happyCard), t)
	edit := filepath.Join(cfg.Destination, "wedding", "processed", "edit.tif")
	os.MkdirAll(filepath.Dir(edit), 0o755)
	os.WriteFile(edit, []byte("mine"), 0o644)
	mustRun(t, cfg)
	if got, _ := os.ReadFile(edit); string(got) != "mine" {
		t.Errorf("edit.tif = %q", got)
	}
}

func TestRunRemovesStalePartials(t *testing.T) {
	cfg := runCfg(fakeCard(t, happyCard), t)
	stale := filepath.Join(cfg.Destination, "wedding", "original", "RAW", ".20261107_0001.ARW.123.partial")
	os.MkdirAll(filepath.Dir(stale), 0o755)
	os.WriteFile(stale, []byte("crash"), 0o644)
	mustRun(t, cfg)
	noPartialsUnder(t, cfg.Destination)
}

func TestRunTimings(t *testing.T) {
	cfg := runCfg(fakeCard(t, happyCard), t)
	s := mustRun(t, cfg)
	want := []string{"scan", "manifests", "dates", "plan", "copy", "copy.read", "copy.verify", "total"}
	var got []string
	for _, tm := range s.Timings {
		got = append(got, tm.Phase)
	}
	if !slices.Equal(got, want) {
		t.Errorf("phases = %v", got)
	}
	docs := manifestDocs(t, cfg.Destination)
	last := docs[len(docs)-1]
	if last["kind"] != "summary" || last["copied"] != 5 || last["workers"] != cfg.Workers || len(last["timings"].([]any)) != len(want) {
		t.Errorf("summary doc = %v", last)
	}
}

func TestRunMissingSource(t *testing.T) {
	cfg := runCfg(filepath.Join(t.TempDir(), "typo"), t)
	if _, err := Run(context.Background(), cfg, "wedding", io.Discard); err == nil {
		t.Fatal("want setup error for a source that does not exist")
	}
	if _, err := os.Stat(filepath.Join(cfg.Destination, "wedding")); err == nil {
		t.Error("created folders for a run that could not start")
	}
}

// Ctrl-C lands while a pair's RAW is being verified: the pair must finish
// and be recorded, nothing on disk may be left unrecorded, and a resume run
// must import the rest without duplicates.
func TestRunCancelMidGroup(t *testing.T) {
	cfg := runCfg(fakeCard(t, happyCard), t)
	cfg.Workers = 1
	ctx, cancel := context.WithCancel(context.Background())
	orig := rehash
	rehash = func(p string) (string, error) {
		if strings.Contains(p, "_0002.ARW") {
			cancel()
		}
		return orig(p)
	}
	t.Cleanup(func() { rehash = orig })

	s, err := Run(ctx, cfg, "wedding", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Failed) != 0 || s.Copied != 3 || s.NotAttempted != 2 {
		t.Errorf("run 1: copied %d failed %v not attempted %d; want 3, none, 2", s.Copied, s.Failed, s.NotAttempted)
	}
	onDisk := func() []string {
		var fs []string
		filepath.WalkDir(filepath.Join(cfg.Destination, "wedding", "original"), func(p string, d os.DirEntry, _ error) error {
			if !d.IsDir() {
				r, _ := filepath.Rel(cfg.Destination, p)
				fs = append(fs, filepath.ToSlash(r))
			}
			return nil
		})
		slices.Sort(fs)
		return fs
	}
	recorded := func() []string {
		var fs []string
		paths, _ := filepath.Glob(filepath.Join(cfg.Destination, ".pic-bridge", "manifests", "*.yaml"))
		for _, p := range paths {
			data, _ := os.ReadFile(p)
			dec := yaml.NewDecoder(bytes.NewReader(data))
			for {
				var e FileEntry
				if dec.Decode(&e) != nil {
					break
				}
				if e.Kind == "file" {
					fs = append(fs, e.Destination)
				}
			}
		}
		slices.Sort(fs)
		return fs
	}
	if d, r := onDisk(), recorded(); !slices.Equal(d, r) {
		t.Errorf("run 1: on disk %v but recorded %v", d, r)
	}

	rehash = orig
	if s := mustRun(t, cfg); s.Copied != 2 || len(s.Failed) != 0 {
		t.Errorf("resume: copied %d failed %v; want 2, none", s.Copied, s.Failed)
	}
	want := slices.Sorted(maps.Values(happyDests))
	if d := onDisk(); !slices.Equal(d, want) {
		t.Errorf("after resume on disk %v, want exactly %v", d, want)
	}
}

func TestRunPlanLineSplitsPhotosAndVideos(t *testing.T) {
	cfg := runCfg(fakeCard(t, happyCard), t)
	var out bytes.Buffer
	if _, err := Run(context.Background(), cfg, "wedding", &out); err != nil {
		t.Fatal(err)
	}
	if line, _, _ := strings.Cut(out.String(), "\n"); !strings.HasPrefix(line, "Importing 3 photos, 1 video (") {
		t.Errorf("plan line = %q", line)
	}
}

// A folder the scan could not read: the rest is imported, the problem is
// printed and recorded in the manifest.
func TestRunReportsScanErrors(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores folder permissions")
	}
	card := fakeCard(t, happyCard)
	locked := filepath.Join(card, "DCIM", "100MSDCF")
	os.Chmod(locked, 0)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	cfg := runCfg(card, t)
	var out bytes.Buffer
	s, err := Run(context.Background(), cfg, "wedding", &out)
	if err != nil {
		t.Fatal(err)
	}
	if s.Copied != 2 || len(s.ScanErrors) != 1 || !strings.HasPrefix(s.ScanErrors[0], "DCIM/100MSDCF: ") {
		t.Fatalf("copied %d, scan errors %v; want 2 and one for DCIM/100MSDCF", s.Copied, s.ScanErrors)
	}
	if !strings.Contains(out.String(), "Could not read 1 card entry; nothing under it was imported:\n  DCIM/100MSDCF: ") {
		t.Errorf("output = %q", out.String())
	}
	docs := manifestDocs(t, cfg.Destination)
	if logged, _ := docs[len(docs)-1]["scan_errors"].([]any); len(logged) != 1 {
		t.Errorf("summary doc = %v, want 1 scan_errors entry", docs[len(docs)-1])
	}
}
