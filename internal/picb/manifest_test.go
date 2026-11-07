package picb

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

var runStart = time.Date(2026, 11, 7, 14, 32, 5, 0, time.Local)

func entry(src string, size int64) FileEntry {
	return FileEntry{
		Source: src, Size: size, Modified: time.Date(2026, 11, 7, 9, 15, 42, 0, time.Local),
		Destination: "wedding/original/" + filepath.Base(src), SHA256: "9f86d0", DateSource: DateEXIF,
		CopiedAt: runStart,
	}
}

func newManifest(t *testing.T, dir string) *Manifest {
	t.Helper()
	m, err := CreateManifest(dir, RunHeader{StartedAt: runStart, Source: "/Volumes/A7V", Folder: "wedding", PicbVersion: "0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := newManifest(t, dir)
	a, b := entry("DCIM/100MSDCF/DSC00001.ARW", 100), entry("DCIM/100MSDCF/DSC00002.ARW", 200)
	for _, e := range []FileEntry{a, b} {
		if err := m.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := LoadImported(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []FileEntry{a, b} {
		k := Key{Source: e.Source, Size: e.Size, ModUnix: e.Modified.Unix()}
		if !got[k] {
			t.Errorf("missing %+v in %v", k, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestManifestHeaderFirst(t *testing.T) {
	dir := t.TempDir()
	m := newManifest(t, dir)
	m.Close()
	data, err := os.ReadFile(m.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "---\nkind: run\n") {
		t.Errorf("manifest starts %q", data[:min(len(data), 40)])
	}
}

func TestManifestFinish(t *testing.T) {
	dir := t.TempDir()
	m := newManifest(t, dir)
	if err := m.Append(entry("DCIM/100MSDCF/DSC00001.ARW", 100)); err != nil {
		t.Fatal(err)
	}
	err := m.Finish(SummaryDoc{FinishedAt: runStart, Copied: 1, Bytes: 100, Workers: 3,
		Timings: []Timing{{Phase: "scan", Seconds: 0.04}, {Phase: "copy", Seconds: 1.5, Note: "1 files"}}})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(m.Path())
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var last SummaryDoc
	for {
		var d SummaryDoc
		if dec.Decode(&d) != nil {
			break
		}
		last = d
	}
	if last.Kind != "summary" || len(last.Timings) != 2 || last.Timings[1].Note != "1 files" {
		t.Errorf("last doc = %+v", last)
	}
	got, err := LoadImported(dir)
	if err != nil || len(got) != 1 {
		t.Errorf("LoadImported = %v, %v; want 1 key", got, err)
	}
}

func TestLoadImportedTruncatedTail(t *testing.T) {
	dir := t.TempDir()
	m := newManifest(t, dir)
	m.Append(entry("DCIM/100MSDCF/DSC00001.ARW", 100))
	m.Close()
	f, _ := os.OpenFile(m.Path(), os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("---\nkind: file\nsource: \"DCIM/100MSDCF/DSC0") // crash mid-write
	f.Close()
	got, err := LoadImported(dir)
	if err != nil || len(got) != 1 {
		t.Fatalf("LoadImported = %v, %v; want the 1 good entry", got, err)
	}
}

func TestLoadImportedMultipleManifests(t *testing.T) {
	dir := t.TempDir()
	m1 := newManifest(t, dir)
	m1.Append(entry("DCIM/100MSDCF/DSC00001.ARW", 100))
	m1.Close()
	m2, err := CreateManifest(dir, RunHeader{StartedAt: runStart.Add(time.Hour), Folder: "other"})
	if err != nil {
		t.Fatal(err)
	}
	m2.Append(entry("DCIM/100MSDCF/DSC00002.ARW", 200))
	m2.Close()
	if got, _ := LoadImported(dir); len(got) != 2 {
		t.Errorf("got %v, want union of 2", got)
	}
}

func TestLoadImportedMissingDir(t *testing.T) {
	got, err := LoadImported(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want empty, nil", got, err)
	}
}

func TestManifestNameFromStart(t *testing.T) {
	dir := t.TempDir()
	m1 := newManifest(t, dir)
	m1.Close()
	if !strings.HasSuffix(m1.Path(), "20261107_143205.yaml") {
		t.Errorf("path = %s", m1.Path())
	}
	m2 := newManifest(t, dir) // same second
	m2.Close()
	if m2.Path() == m1.Path() {
		t.Fatal("second manifest reused the first one's name")
	}
	if data, _ := os.ReadFile(m1.Path()); !strings.HasPrefix(string(data), "---\nkind: run\n") {
		t.Error("first manifest was overwritten")
	}
}

func TestKeyOfSecondsPrecision(t *testing.T) {
	f := SourceFile{Rel: "DCIM/a.ARW", Size: 5, ModTime: time.Unix(1000, 999_000_000)}
	if k := KeyOf(f); k != (Key{Source: "DCIM/a.ARW", Size: 5, ModUnix: 1000}) {
		t.Errorf("KeyOf = %+v", k)
	}
}

func TestManifestNameUsesLocalTime(t *testing.T) {
	start := time.Date(2026, 11, 7, 3, 32, 5, 0, time.FixedZone("X", 5*3600))
	m, err := CreateManifest(t.TempDir(), RunHeader{StartedAt: start})
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	if want := start.Local().Format("20060102_150405") + ".yaml"; filepath.Base(m.Path()) != want {
		t.Errorf("name = %s, want %s", filepath.Base(m.Path()), want)
	}
}

func TestManifestAppendGroup(t *testing.T) {
	dir := t.TempDir()
	m := newManifest(t, dir)
	if err := m.Append(entry("PRIVATE/M4ROOT/CLIP/C0001.MP4", 100), entry("PRIVATE/M4ROOT/CLIP/C0001M01.XML", 5)); err != nil {
		t.Fatal(err)
	}
	m.Close()
	if got, _ := LoadImported(dir); len(got) != 2 {
		t.Errorf("got %v, want both group entries", got)
	}
	data, _ := os.ReadFile(m.Path())
	if n := strings.Count(string(data), "---\nkind: file\n"); n != 2 {
		t.Errorf("%d file docs, want 2", n)
	}
}
