package picb

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func box(typ string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	b := binary.BigEndian.AppendUint32(nil, uint32(8+len(body)))
	return append(append(b, typ...), body...)
}

// mvhdBytes builds ftyp + moov{mvhd} with the given creation time.
func mvhdBytes(created time.Time, version byte) []byte {
	secs := uint64(created.Sub(mp4Epoch) / time.Second)
	p := []byte{version, 0, 0, 0}
	if version == 1 {
		p = binary.BigEndian.AppendUint64(p, secs) // creation
		p = binary.BigEndian.AppendUint64(p, secs) // modification
	} else {
		p = binary.BigEndian.AppendUint32(p, uint32(secs))
		p = binary.BigEndian.AppendUint32(p, uint32(secs))
	}
	p = append(p, make([]byte, 80)...) // timescale, duration, rest (unused)
	return append(box("ftyp", []byte("isom\x00\x00\x02\x00")), box("moov", box("mvhd", p))...)
}

func TestParseMvhdV0(t *testing.T) {
	want := time.Date(2026, 11, 7, 3, 15, 42, 0, time.UTC)
	got, err := parseMvhd(bytes.NewReader(mvhdBytes(want, 0)))
	if err != nil || !got.Equal(want) {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}

func TestParseMvhdV1(t *testing.T) {
	want := time.Date(2026, 11, 7, 3, 15, 42, 0, time.UTC)
	got, err := parseMvhd(bytes.NewReader(mvhdBytes(want, 1)))
	if err != nil || !got.Equal(want) {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}

func TestParseMvhdMissing(t *testing.T) {
	if _, err := parseMvhd(bytes.NewReader(box("ftyp", []byte("isom")))); err == nil {
		t.Fatal("want error without moov/mvhd")
	}
}

const sonyXML = `<?xml version="1.0" encoding="UTF-8"?>
<NonRealTimeMeta xmlns="urn:schemas-professionalDisc:nonRealTimeMeta:ver.2.00">
	<CreationDate value="2026-11-07T14:15:42+11:00"/>
</NonRealTimeMeta>`

func TestParseSonyXML(t *testing.T) {
	want := time.Date(2026, 11, 7, 3, 15, 42, 0, time.UTC)
	got, err := parseSonyXML(strings.NewReader(sonyXML))
	if err != nil || !got.Equal(want) {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}

// fileAt writes data to dir/name with the given mtime and returns its SourceFile.
func fileAt(t *testing.T, dir, name string, data []byte, cls Class, mod time.Time) SourceFile {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
	return SourceFile{Rel: name, Abs: p, Size: int64(len(data)), ModTime: mod, Class: cls}
}

var mtime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)

func TestCaptureDateFallsBackToMtime(t *testing.T) {
	f := fileAt(t, t.TempDir(), "DSC00001.JPG", []byte("not a jpeg"), ClassJPEG, mtime)
	got, src := CaptureDate(f, nil, true)
	if src != DateMtime || !got.Equal(mtime) || got.Location() != time.Local {
		t.Fatalf("got %v %v, want %v mtime", got, src, mtime)
	}
}

func TestCaptureDateUseMetaFalse(t *testing.T) {
	dir := t.TempDir()
	v := fileAt(t, dir, "C0001.MP4", mvhdBytes(time.Date(2026, 11, 7, 3, 0, 0, 0, time.UTC), 0), ClassVideo, mtime)
	x := fileAt(t, dir, "C0001M01.XML", []byte(sonyXML), ClassSidecar, mtime)
	if _, src := CaptureDate(v, &x, false); src != DateMtime {
		t.Fatalf("src = %v, want mtime", src)
	}
}

func TestCaptureDateVideoPrefersXML(t *testing.T) {
	dir := t.TempDir()
	v := fileAt(t, dir, "C0001.MP4", mvhdBytes(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), 0), ClassVideo, mtime)
	x := fileAt(t, dir, "C0001M01.XML", []byte(sonyXML), ClassSidecar, mtime)
	got, src := CaptureDate(v, &x, true)
	want := time.Date(2026, 11, 7, 3, 15, 42, 0, time.UTC)
	if src != DateXML || !got.Equal(want) || got.Location() != time.Local {
		t.Fatalf("got %v %v, want %v xml (local)", got, src, want)
	}
}

func TestCaptureDateVideoMvhd(t *testing.T) {
	want := time.Date(2026, 11, 7, 3, 0, 0, 0, time.UTC)
	v := fileAt(t, t.TempDir(), "MVI_0001.MOV", mvhdBytes(want, 0), ClassVideo, mtime)
	got, src := CaptureDate(v, nil, true)
	if src != DateMvhd || !got.Equal(want) || got.Location() != time.Local {
		t.Fatalf("got %v %v, want %v mvhd (local)", got, src, want)
	}
}

// Run against real camera files: PICB_SAMPLE_DIR=/path go test -run EXIFSample -v
func TestCaptureDateEXIFSample(t *testing.T) {
	dir := os.Getenv("PICB_SAMPLE_DIR")
	if dir == "" {
		t.Skip("PICB_SAMPLE_DIR not set")
	}
	files, _, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no camera files under %s (expects DCIM/ or PRIVATE/M4ROOT/CLIP/)", dir)
	}
	for _, f := range files {
		got, src := CaptureDate(f, nil, true)
		t.Logf("%s → %s (%s; file mtime %s)", f.Rel, got.Format(time.RFC3339), src, f.ModTime.Format(time.RFC3339))
		switch {
		case f.Class == ClassRAW || f.Class == ClassJPEG:
			if src != DateEXIF || got.Year() < 2000 {
				t.Errorf("%s: got %v from %s, want exif", f.Rel, got, src)
			}
		case f.Class == ClassVideo:
			if src == DateMtime || got.Year() < 2000 {
				t.Errorf("%s: got %v from %s, want container metadata", f.Rel, got, src)
			}
		}
	}
}

func TestParseMvhdRejectsOverflow(t *testing.T) {
	p := binary.BigEndian.AppendUint64([]byte{1, 0, 0, 0}, 1<<40) // corrupt v1 creation time
	p = binary.BigEndian.AppendUint64(p, 1<<40)
	data := append(box("ftyp", []byte("isom")), box("moov", box("mvhd", append(p, make([]byte, 80)...)))...)
	if got, err := parseMvhd(bytes.NewReader(data)); err == nil {
		t.Fatalf("got %v, want error for out-of-range creation time", got)
	}
}

func TestWithFileRecoversPanic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := withFile(p, func(*os.File) (time.Time, error) { panic("decoder bug") })
	if err == nil {
		t.Fatal("want error from recovered panic")
	}
}
