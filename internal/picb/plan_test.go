package picb

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func at(h, m int) time.Time { return time.Date(2026, 11, 7, h, m, 0, 0, time.Local) }

// sf builds a SourceFile; class comes from the extension like Scan does.
func sf(rel string) SourceFile {
	c, ok := extClass[strings.ToUpper(filepath.Ext(rel))]
	if !ok {
		c = ClassSidecar // .XML
	}
	return SourceFile{Rel: rel, Size: int64(len(rel)), ModTime: time.Unix(1000, 0), Class: c}
}

// fakeDater returns dates by Rel.
func fakeDater(dates map[string]time.Time) Dater {
	return func(f SourceFile, _ *SourceFile) (time.Time, DateSource) {
		return dates[f.Rel], DateEXIF
	}
}

var splitCfg = Config{DateFormat: "YYYYMMDD", SplitRawJPEG: true}
var flatCfg = Config{DateFormat: "YYYYMMDD"}

func dests(items []Item) map[string]string {
	m := map[string]string{}
	for _, it := range items {
		m[it.Src.Rel] = it.DestRel
	}
	return m
}

func wantDests(t *testing.T, items []Item, want map[string]string) {
	t.Helper()
	got := dests(items)
	if len(got) != len(want) {
		t.Errorf("got %d items %v, want %d", len(got), got, len(want))
	}
	for rel, d := range want {
		if got[rel] != d {
			t.Errorf("%s → %q, want %q", rel, got[rel], d)
		}
	}
}

func TestPlanBasicNames(t *testing.T) {
	files := []SourceFile{sf("DCIM/100MSDCF/DSC00001.ARW"), sf("DCIM/100MSDCF/DSC00002.ARW")}
	d := fakeDater(map[string]time.Time{files[0].Rel: at(9, 0), files[1].Rel: at(10, 0)})
	items, _ := BuildPlan(files, nil, flatCfg, "wedding", 1, d)
	wantDests(t, items, map[string]string{
		files[0].Rel: "wedding/original/20261107_0001.ARW",
		files[1].Rel: "wedding/original/20261107_0002.ARW",
	})
}

func TestPlanShootingOrder(t *testing.T) {
	a, b, c := sf("DCIM/100MSDCF/DSC00001.ARW"), sf("DCIM/100MSDCF/DSC00002.ARW"), sf("DCIM/100MSDCF/DSC00003.ARW")
	d := fakeDater(map[string]time.Time{a.Rel: at(10, 0), b.Rel: at(9, 0), c.Rel: at(10, 0)})
	items, _ := BuildPlan([]SourceFile{c, b, a}, nil, flatCfg, "w", 1, d)
	wantDests(t, items, map[string]string{
		b.Rel: "w/original/20261107_0001.ARW",
		a.Rel: "w/original/20261107_0002.ARW", // same time as c: Rel tie-break
		c.Rel: "w/original/20261107_0003.ARW",
	})
}

func TestPlanPairShareCounter(t *testing.T) {
	r, j := sf("DCIM/100MSDCF/DSC01234.ARW"), sf("DCIM/100MSDCF/DSC01234.JPG")
	d := fakeDater(map[string]time.Time{r.Rel: at(9, 0), j.Rel: at(9, 1)})
	items, _ := BuildPlan([]SourceFile{r, j}, nil, splitCfg, "w", 1, d)
	wantDests(t, items, map[string]string{
		r.Rel: "w/original/RAW/20261107_0001.ARW",
		j.Rel: "w/original/JPEG/20261107_0001.JPG",
	})
	for _, it := range items {
		if !it.Date.Equal(at(9, 0)) {
			t.Errorf("%s date %v, want RAW date", it.Src.Rel, it.Date)
		}
	}
}

func TestPlanPairsCaseInsensitive(t *testing.T) {
	r, j := sf("DCIM/100MSDCF/DSC01234.ARW"), sf("DCIM/100MSDCF/dsc01234.jpg")
	d := fakeDater(map[string]time.Time{r.Rel: at(9, 0), j.Rel: at(9, 0)})
	items, _ := BuildPlan([]SourceFile{r, j}, nil, splitCfg, "w", 1, d)
	wantDests(t, items, map[string]string{
		r.Rel: "w/original/RAW/20261107_0001.ARW",
		j.Rel: "w/original/JPEG/20261107_0001.jpg",
	})
}

func TestPlanPairOnlySameDir(t *testing.T) {
	r, j := sf("DCIM/100MSDCF/DSC00001.ARW"), sf("DCIM/101MSDCF/DSC00001.JPG")
	d := fakeDater(map[string]time.Time{r.Rel: at(9, 0), j.Rel: at(9, 0)})
	items, _ := BuildPlan([]SourceFile{r, j}, nil, splitCfg, "w", 1, d)
	wantDests(t, items, map[string]string{
		r.Rel: "w/original/RAW/20261107_0001.ARW",
		j.Rel: "w/original/JPEG/20261107_0002.JPG",
	})
}

func TestPlanVideoAndSidecar(t *testing.T) {
	v, x := sf("PRIVATE/M4ROOT/CLIP/C0001.MP4"), sf("PRIVATE/M4ROOT/CLIP/C0001M01.XML")
	var gotSidecar *SourceFile
	d := func(f SourceFile, s *SourceFile) (time.Time, DateSource) {
		if f.Rel == v.Rel {
			gotSidecar = s
		}
		return at(9, 0), DateXML
	}
	for _, cfg := range []Config{splitCfg, flatCfg} {
		items, _ := BuildPlan([]SourceFile{x, v}, nil, cfg, "w", 1, d)
		wantDests(t, items, map[string]string{
			v.Rel: "w/original/VIDEO/20261107_0001.MP4",
			x.Rel: "w/original/VIDEO/20261107_0001.XML",
		})
		if gotSidecar == nil || gotSidecar.Rel != x.Rel {
			t.Errorf("video dated without its sidecar: %v", gotSidecar)
		}
	}
}

func TestPlanIncludeTime(t *testing.T) {
	f := sf("DCIM/100MSDCF/DSC00001.ARW")
	cfg := flatCfg
	cfg.IncludeTime = true
	d := fakeDater(map[string]time.Time{f.Rel: time.Date(2026, 11, 7, 14, 32, 5, 0, time.Local)})
	items, _ := BuildPlan([]SourceFile{f}, nil, cfg, "w", 1, d)
	wantDests(t, items, map[string]string{f.Rel: "w/original/20261107_143205_0001.ARW"})
}

func TestPlanMultiDay(t *testing.T) {
	a, b := sf("DCIM/100MSDCF/DSC00001.ARW"), sf("DCIM/100MSDCF/DSC00002.ARW")
	d := fakeDater(map[string]time.Time{a.Rel: at(23, 59), b.Rel: time.Date(2026, 11, 8, 0, 1, 0, 0, time.Local)})
	items, _ := BuildPlan([]SourceFile{a, b}, nil, flatCfg, "w", 1, d)
	wantDests(t, items, map[string]string{
		a.Rel: "w/original/20261107_0001.ARW",
		b.Rel: "w/original/20261108_0002.ARW",
	})
}

func TestPlanStartCounter(t *testing.T) {
	f := sf("DCIM/100MSDCF/DSC00001.ARW")
	items, _ := BuildPlan([]SourceFile{f}, nil, flatCfg, "w", 42, fakeDater(map[string]time.Time{f.Rel: at(9, 0)}))
	wantDests(t, items, map[string]string{f.Rel: "w/original/20261107_0042.ARW"})
}

func TestPlanCounterPast9999(t *testing.T) {
	f := sf("DCIM/100MSDCF/DSC00001.ARW")
	items, _ := BuildPlan([]SourceFile{f}, nil, flatCfg, "w", 10000, fakeDater(map[string]time.Time{f.Rel: at(9, 0)}))
	wantDests(t, items, map[string]string{f.Rel: "w/original/20261107_10000.ARW"})
}

func TestPlanSkipsImported(t *testing.T) {
	a, b, c := sf("DCIM/100MSDCF/DSC00001.ARW"), sf("DCIM/100MSDCF/DSC00002.ARW"), sf("DCIM/100MSDCF/DSC00003.ARW")
	d := fakeDater(map[string]time.Time{a.Rel: at(9, 0), b.Rel: at(10, 0), c.Rel: at(11, 0)})
	items, skipped := BuildPlan([]SourceFile{a, b, c}, map[Key]bool{KeyOf(a): true}, flatCfg, "w", 5, d)
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	wantDests(t, items, map[string]string{
		b.Rel: "w/original/20261107_0005.ARW",
		c.Rel: "w/original/20261107_0006.ARW",
	})
}

func TestPlanSkipsPairMemberIndividually(t *testing.T) {
	r, j := sf("DCIM/100MSDCF/DSC01234.ARW"), sf("DCIM/100MSDCF/DSC01234.JPG")
	d := fakeDater(map[string]time.Time{r.Rel: at(9, 0), j.Rel: at(9, 0)})
	items, skipped := BuildPlan([]SourceFile{r, j}, map[Key]bool{KeyOf(r): true}, splitCfg, "w", 3, d)
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	wantDests(t, items, map[string]string{j.Rel: "w/original/JPEG/20261107_0003.JPG"})
}

func TestPlanNoSplit(t *testing.T) {
	r, j := sf("DCIM/100MSDCF/DSC01234.ARW"), sf("DCIM/100MSDCF/DSC01234.JPG")
	d := fakeDater(map[string]time.Time{r.Rel: at(9, 0), j.Rel: at(9, 0)})
	items, _ := BuildPlan([]SourceFile{r, j}, nil, flatCfg, "w", 1, d)
	wantDests(t, items, map[string]string{
		r.Rel: "w/original/20261107_0001.ARW",
		j.Rel: "w/original/20261107_0001.JPG",
	})
}

func TestNextCounter(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "20261107_0007.ARW", "RAW/20261107_0012.ARW", "VIDEO/20261107_0003.MP4")
	if n, err := NextCounter(dir); err != nil || n != 13 {
		t.Errorf("NextCounter = %d, %v; want 13", n, err)
	}
	if n, err := NextCounter(filepath.Join(dir, "nope")); err != nil || n != 1 {
		t.Errorf("missing dir: NextCounter = %d, %v; want 1", n, err)
	}
}

func TestNextCounterIgnoresPartial(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "20261107_0002.ARW", ".20261107_0099.ARW.partial", "JPEG/20261107_0098.JPG.partial")
	if n, err := NextCounter(dir); err != nil || n != 3 {
		t.Errorf("NextCounter = %d, %v; want 3", n, err)
	}
}

func TestPlanVideoImportedSkipsSidecar(t *testing.T) {
	v, x := sf("PRIVATE/M4ROOT/CLIP/C0001.MP4"), sf("PRIVATE/M4ROOT/CLIP/C0001M01.XML")
	d := fakeDater(map[string]time.Time{v.Rel: at(9, 0), x.Rel: at(9, 0)})
	items, skipped := BuildPlan([]SourceFile{v, x}, map[Key]bool{KeyOf(v): true}, flatCfg, "w", 1, d)
	if len(items) != 0 || skipped != 2 {
		t.Errorf("items %v, skipped %d; want none, 2", dests(items), skipped)
	}
}

func TestPlanSidecarImportedVideoNot(t *testing.T) {
	v, x := sf("PRIVATE/M4ROOT/CLIP/C0001.MP4"), sf("PRIVATE/M4ROOT/CLIP/C0001M01.XML")
	d := fakeDater(map[string]time.Time{v.Rel: at(9, 0), x.Rel: at(9, 0)})
	items, skipped := BuildPlan([]SourceFile{v, x}, map[Key]bool{KeyOf(x): true}, flatCfg, "w", 1, d)
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0 (video not imported: copy both)", skipped)
	}
	wantDests(t, items, map[string]string{
		v.Rel: "w/original/VIDEO/20261107_0001.MP4",
		x.Rel: "w/original/VIDEO/20261107_0001.XML",
	})
}

func TestPlanSameBaseVideosNotLost(t *testing.T) {
	mp4, mov := sf("PRIVATE/M4ROOT/CLIP/C0001.MP4"), sf("PRIVATE/M4ROOT/CLIP/C0001.MOV")
	d := fakeDater(map[string]time.Time{mp4.Rel: at(9, 0), mov.Rel: at(9, 0)})
	items, skipped := BuildPlan([]SourceFile{mp4, mov}, map[Key]bool{KeyOf(mp4): true}, flatCfg, "w", 1, d)
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if got := dests(items); got[mov.Rel] == "" {
		t.Errorf("unimported MOV not planned: %v", got)
	}
}

func TestPlanCounterField(t *testing.T) {
	r, j, a := sf("DCIM/100MSDCF/DSC01234.ARW"), sf("DCIM/100MSDCF/DSC01234.JPG"), sf("DCIM/100MSDCF/DSC01235.ARW")
	d := fakeDater(map[string]time.Time{r.Rel: at(9, 0), j.Rel: at(9, 0), a.Rel: at(10, 0)})
	items, _ := BuildPlan([]SourceFile{r, j, a}, nil, splitCfg, "w", 7, d)
	got := map[string]int{}
	for _, it := range items {
		got[it.Src.Rel] = it.Counter
	}
	if got[r.Rel] != 7 || got[j.Rel] != 7 || got[a.Rel] != 8 {
		t.Errorf("counters = %v, want pair 7, single 8", got)
	}
}

func TestPlanVideoNameEndingM01(t *testing.T) {
	a, b := sf("PRIVATE/M4ROOT/CLIP/ABC.MP4"), sf("PRIVATE/M4ROOT/CLIP/ABCM01.MP4")
	d := fakeDater(map[string]time.Time{a.Rel: at(9, 0), b.Rel: at(10, 0)})
	items, _ := BuildPlan([]SourceFile{a, b}, nil, flatCfg, "w", 1, d)
	wantDests(t, items, map[string]string{
		a.Rel: "w/original/VIDEO/20261107_0001.MP4",
		b.Rel: "w/original/VIDEO/20261107_0002.MP4",
	})
}

func TestPlanNamesInLocalTime(t *testing.T) {
	f := sf("DCIM/100MSDCF/DSC00001.ARW")
	shot := time.Date(2026, 11, 7, 23, 0, 0, 0, time.FixedZone("X", -12*3600))
	items, _ := BuildPlan([]SourceFile{f}, nil, flatCfg, "w", 1, fakeDater(map[string]time.Time{f.Rel: shot}))
	want := "w/original/" + shot.In(time.Local).Format("20060102") + "_0001.ARW"
	wantDests(t, items, map[string]string{f.Rel: want})
}
