package picb

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// touch creates root/rel (and parents) with a little content.
func touch(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFindCardRoot(t *testing.T) {
	vols := t.TempDir()
	touch(t, vols, "A7V/DCIM/100MSDCF/DSC00001.ARW", "Backup/notes.txt")
	got, err := FindCardRoot(vols)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(vols, "A7V"); got != want {
		t.Errorf("FindCardRoot = %q, want %q", got, want)
	}

	touch(t, vols, "EOS/DCIM/100CANON/IMG_0001.CR3")
	if _, err := FindCardRoot(vols); err == nil {
		t.Error("two cards: want error")
	}

	if _, err := FindCardRoot(t.TempDir()); err == nil {
		t.Error("no card: want error")
	}
}

func classes(files []SourceFile) map[string]Class {
	m := map[string]Class{}
	for _, f := range files {
		m[f.Rel] = f.Class
	}
	return m
}

func TestScanClassifies(t *testing.T) {
	root := t.TempDir()
	touch(t, root,
		"DCIM/100MSDCF/DSC00001.ARW",
		"DCIM/100MSDCF/DSC00001.JPG",
		"DCIM/100MSDCF/DSC00002.CR3",
		"DCIM/100CANON/MVI_0001.MOV",
		"PRIVATE/M4ROOT/CLIP/C0001.MP4",
		"PRIVATE/M4ROOT/CLIP/C0001M01.XML",
		"DCIM/100MSDCF/notes.txt",
	)
	files, ignored, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Class{
		"DCIM/100MSDCF/DSC00001.ARW":       ClassRAW,
		"DCIM/100MSDCF/DSC00001.JPG":       ClassJPEG,
		"DCIM/100MSDCF/DSC00002.CR3":       ClassRAW,
		"DCIM/100CANON/MVI_0001.MOV":       ClassVideo,
		"PRIVATE/M4ROOT/CLIP/C0001.MP4":    ClassVideo,
		"PRIVATE/M4ROOT/CLIP/C0001M01.XML": ClassSidecar,
	}
	if got := classes(files); !maps.Equal(got, want) {
		t.Errorf("classes = %v, want %v", got, want)
	}
	if !slices.Equal(ignored, []string{"DCIM/100MSDCF/notes.txt"}) {
		t.Errorf("ignored = %v", ignored)
	}
	if !slices.IsSortedFunc(files, func(a, b SourceFile) int { return strings.Compare(a.Rel, b.Rel) }) {
		t.Error("files not sorted by Rel")
	}
	f := files[slices.IndexFunc(files, func(f SourceFile) bool { return f.Rel == "DCIM/100MSDCF/DSC00001.ARW" })]
	if f.Size != int64(len(f.Rel)) || f.Abs != filepath.Join(root, "DCIM/100MSDCF/DSC00001.ARW") || f.ModTime.IsZero() {
		t.Errorf("metadata wrong: %+v", f)
	}
}

func TestScanSidecarWithoutVideoIgnored(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "PRIVATE/M4ROOT/CLIP/C0009M01.XML")
	files, ignored, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 || !slices.Equal(ignored, []string{"PRIVATE/M4ROOT/CLIP/C0009M01.XML"}) {
		t.Errorf("files=%v ignored=%v", files, ignored)
	}
}

func TestScanSkipsHidden(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "DCIM/100MSDCF/._DSC00001.ARW")
	files, ignored, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 || len(ignored) != 0 {
		t.Errorf("files=%v ignored=%v", files, ignored)
	}
}

func TestScanCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "DCIM/100MSDCF/dsc00003.arw", "PRIVATE/M4ROOT/CLIP/C0002.Mp4", "PRIVATE/M4ROOT/CLIP/c0002m01.xml")
	files, _, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Class{
		"DCIM/100MSDCF/dsc00003.arw":       ClassRAW,
		"PRIVATE/M4ROOT/CLIP/C0002.Mp4":    ClassVideo,
		"PRIVATE/M4ROOT/CLIP/c0002m01.xml": ClassSidecar,
	}
	if got := classes(files); !maps.Equal(got, want) {
		t.Errorf("classes = %v, want %v", got, want)
	}
}

func TestScanNoDCIMOrClip(t *testing.T) {
	files, ignored, err := Scan(t.TempDir())
	if err != nil || len(files) != 0 || len(ignored) != 0 {
		t.Errorf("files=%v ignored=%v err=%v", files, ignored, err)
	}
}
