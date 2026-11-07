package picb

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Item is one file to copy and where it goes.
type Item struct {
	Src        SourceFile
	Date       time.Time
	DateSource DateSource
	Counter    int
	DestRel    string // relative to destination, slash-separated
}

// Dater dates a file; run passes a closure over CaptureDate, tests pass fakes.
type Dater func(f SourceFile, sidecar *SourceFile) (time.Time, DateSource)

// rank orders a group's members: the first one dates the whole group.
var rank = map[Class]int{ClassRAW: 0, ClassVideo: 1, ClassJPEG: 2, ClassSidecar: 3}

// BuildPlan groups RAW+JPEG pairs and video+sidecar pairs (same dir, same
// base name), drops already-imported files (a sidecar follows its video),
// orders groups by shooting time and numbers them from start. Members of a
// group share a counter.
func BuildPlan(files []SourceFile, imported map[Key]bool, cfg Config, folder string, start int, date Dater) (items []Item, skipped int) {
	all := map[string][]SourceFile{}
	for _, f := range files {
		k := groupKey(f)
		all[k] = append(all[k], f)
	}
	groups := map[string][]SourceFile{}
	for k, m := range all {
		// A sidecar follows its video: copied again whenever a video in its
		// group still needs copying (duplicate at worst), skipped otherwise.
		videoPending := slices.ContainsFunc(m, func(f SourceFile) bool { return f.Class == ClassVideo && !imported[KeyOf(f)] })
		hasVideo := slices.ContainsFunc(m, func(f SourceFile) bool { return f.Class == ClassVideo })
		for _, f := range m {
			skip := imported[KeyOf(f)]
			if f.Class == ClassSidecar && hasVideo {
				skip = !videoPending
			}
			if skip {
				skipped++
				continue
			}
			groups[k] = append(groups[k], f)
		}
	}

	type group struct {
		members []SourceFile
		date    time.Time
		src     DateSource
	}
	var gs []group
	for _, m := range groups {
		slices.SortFunc(m, func(a, b SourceFile) int {
			if r := rank[a.Class] - rank[b.Class]; r != 0 {
				return r
			}
			return strings.Compare(a.Rel, b.Rel)
		})
		var sidecar *SourceFile
		if m[0].Class == ClassVideo {
			for i := range m {
				if m[i].Class == ClassSidecar {
					sidecar = &m[i]
				}
			}
		}
		d, src := date(m[0], sidecar)
		gs = append(gs, group{m, d.In(time.Local), src})
	}
	slices.SortFunc(gs, func(a, b group) int {
		if c := a.date.Compare(b.date); c != 0 {
			return c
		}
		return strings.Compare(a.members[0].Rel, b.members[0].Rel)
	})

	for i, g := range gs {
		stem := g.date.Format(cfg.dateLayout())
		if cfg.IncludeTime {
			stem += "_" + g.date.Format(timeLayout)
		}
		stem += fmt.Sprintf("_%04d", start+i)
		for _, f := range g.members {
			items = append(items, Item{
				Src: f, Date: g.date, DateSource: g.src, Counter: start + i,
				DestRel: path.Join(folder, "original", subdir(f.Class, cfg.SplitRawJPEG), stem+path.Ext(f.Rel)),
			})
		}
	}
	return items, skipped
}

// groupKey pairs files by dir + upper-cased base name; a sidecar
// C0001M01.XML joins C0001.MP4. Photos and videos never pair.
func groupKey(f SourceFile) string {
	base := strings.ToUpper(strings.TrimSuffix(path.Base(f.Rel), path.Ext(f.Rel)))
	kind := "photo"
	if f.Class == ClassVideo || f.Class == ClassSidecar {
		kind = "video"
	}
	if f.Class == ClassSidecar {
		base = strings.TrimSuffix(base, "M01")
	}
	return path.Dir(f.Rel) + "|" + kind + "|" + base
}

func subdir(c Class, split bool) string {
	switch {
	case c == ClassVideo || c == ClassSidecar:
		return "VIDEO"
	case split && c == ClassRAW:
		return "RAW"
	case split && c == ClassJPEG:
		return "JPEG"
	}
	return ""
}

var counterRE = regexp.MustCompile(`_(\d{4,})\.[^.]+$`)

// NextCounter is 1 + the highest trailing _NNNN counter in originalDir and its
// RAW/, JPEG/, VIDEO/ subfolders. Hidden and .partial files don't count.
func NextCounter(originalDir string) (int, error) {
	high := 0
	for _, sub := range []string{"", "RAW", "JPEG", "VIDEO"} {
		entries, err := os.ReadDir(filepath.Join(originalDir, sub))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, err
		}
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || strings.HasPrefix(n, ".") || strings.HasSuffix(n, ".partial") {
				continue
			}
			if m := counterRE.FindStringSubmatch(n); m != nil {
				if v, err := strconv.Atoi(m[1]); err == nil && v > high {
					high = v
				}
			}
		}
	}
	return high + 1, nil
}
