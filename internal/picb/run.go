package picb

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

const Version = "0.1.0"

// Summary is what one import did.
type Summary struct {
	Copied, Skipped int
	NotAttempted    int // planned but never started (Ctrl-C)
	Bytes           int64
	Failed          map[string]string // card Rel → reason
	Ignored         []string
	MtimeFallback   []string
	ManifestPath    string
	Timings         []Timing
	Source          string       // resolved card root
	Files           []SourceFile // the scan of Source
	ScanErrors      []string     // "<rel>: <error>" for card entries the scan could not read; Files is incomplete when set
}

var volumesDir = "/Volumes"

// Run imports the card into <destination>/<folder>/original. The error is
// only for setup failures; per-file failures land in Summary.Failed.
func Run(ctx context.Context, cfg Config, folder string, out io.Writer) (Summary, error) {
	start := time.Now()
	s := Summary{Failed: map[string]string{}}
	src := cfg.Source
	if src == "auto" {
		var err error
		if src, err = FindCardRoot(volumesDir); err != nil {
			return s, err
		}
	}
	// A mistyped source must not look like an empty card.
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return s, fmt.Errorf("source %s is not a readable folder (is the card mounted?)", src)
	}
	origDir := filepath.Join(cfg.Destination, folder, "original")
	manDir := filepath.Join(cfg.Destination, ".pic-bridge", "manifests")
	for _, d := range []string{origDir, filepath.Join(cfg.Destination, folder, "processed"), manDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return s, err
		}
	}
	unlock, err := lockDest(cfg.Destination)
	if err != nil {
		return s, err
	}
	defer unlock()
	if err := removePartials(origDir); err != nil {
		return s, err
	}

	t := time.Now()
	files, ignored, scanErrs, err := Scan(src)
	if err != nil {
		return s, err
	}
	s.Source, s.Files, s.Ignored, s.ScanErrors = src, files, ignored, scanErrs
	s.Timings = append(s.Timings, Timing{Phase: "scan", Seconds: since(t), Note: fmt.Sprintf("%d files", len(files))})

	t = time.Now()
	imported, err := LoadImported(manDir)
	if err != nil {
		return s, err
	}
	s.Timings = append(s.Timings, Timing{Phase: "manifests", Seconds: since(t), Note: fmt.Sprintf("%d imported", len(imported))})

	// Dates are read lazily, once per group, inside BuildPlan; time them apart.
	var dateTime time.Duration
	bySrc := map[DateSource]int{}
	dater := func(f SourceFile, sidecar *SourceFile) (time.Time, DateSource) {
		t0 := time.Now()
		d, ds := CaptureDate(f, sidecar, cfg.UseEXIF)
		dateTime += time.Since(t0)
		bySrc[ds]++
		if ds == DateMtime && cfg.UseEXIF {
			s.MtimeFallback = append(s.MtimeFallback, f.Rel)
		}
		return d, ds
	}
	t = time.Now()
	next, err := NextCounter(origDir)
	if err != nil {
		return s, err
	}
	items, skipped := BuildPlan(files, imported, cfg, folder, next, dater)
	s.Skipped = skipped
	planTime := time.Since(t) - dateTime
	var notes []string
	for _, k := range slices.Sorted(maps.Keys(bySrc)) {
		notes = append(notes, fmt.Sprintf("%s=%d", k, bySrc[k]))
	}
	s.Timings = append(s.Timings,
		Timing{Phase: "dates", Seconds: dateTime.Seconds(), Note: strings.Join(notes, " ")},
		Timing{Phase: "plan", Seconds: planTime.Seconds(), Note: fmt.Sprintf("%d to copy, %d skipped", len(items), skipped)})

	var total int64
	photos, videos := 0, 0
	for _, it := range items {
		total += it.Src.Size
		switch it.Src.Class {
		case ClassRAW, ClassJPEG:
			photos++
		case ClassVideo:
			videos++
		}
	}
	fmt.Fprintf(out, "Importing %s, %s (%.1f MB) from %s into %s; %d already imported.\n",
		plural(photos, "photo"), plural(videos, "video"), float64(total)/1e6, src, filepath.Join(cfg.Destination, folder), skipped)

	for _, it := range items {
		if err := os.MkdirAll(filepath.Join(cfg.Destination, filepath.FromSlash(path.Dir(it.DestRel))), 0o755); err != nil {
			return s, err
		}
	}
	// From here on every path Finishes the manifest.
	m, err := CreateManifest(manDir, RunHeader{StartedAt: start, Source: src, Folder: folder, PicbVersion: Version, Config: cfg})
	if err != nil {
		return s, err
	}
	s.ManifestPath = m.Path()

	t = time.Now()
	read, verify := copyGroups(ctx, cfg, m, groupItems(items), len(items), &s, out)
	copyTime := since(t)
	s.NotAttempted = len(items) - s.Copied - len(s.Failed)
	s.Timings = append(s.Timings,
		Timing{Phase: "copy", Seconds: copyTime, Note: fmt.Sprintf("%d files, %d bytes, %.1f MB/s", s.Copied, s.Bytes, float64(s.Bytes)/max(copyTime, 1e-9)/1e6)},
		Timing{Phase: "copy.read", Seconds: read.Seconds(), Note: "summed across workers"},
		Timing{Phase: "copy.verify", Seconds: verify.Seconds(), Note: "summed across workers"},
		Timing{Phase: "total", Seconds: since(start)})

	err = m.Finish(SummaryDoc{FinishedAt: time.Now(), Copied: s.Copied, Skipped: s.Skipped, Failed: len(s.Failed), NotAttempted: s.NotAttempted,
		Bytes: s.Bytes, Workers: cfg.Workers, ScanErrors: s.ScanErrors, Timings: s.Timings})
	printSummary(out, s)
	return s, err
}

// copyGroups copies each group with cfg.Workers workers. A group is recorded
// in the manifest only when every member is verified and in place.
func copyGroups(ctx context.Context, cfg Config, m *Manifest, groups [][]Item, n int, s *Summary, out io.Writer) (read, verify time.Duration) {
	var mu sync.Mutex
	done := 0
	ch := make(chan []Item)
	var wg sync.WaitGroup
	for range cfg.Workers {
		wg.Go(func() {
			for g := range ch {
				var entries []FileEntry
				var stats []CopyStats
				var failErr error
				cctx := ctx
				for _, it := range g {
					st, err := CopyVerify(cctx, it.Src.Abs, filepath.Join(cfg.Destination, filepath.FromSlash(it.DestRel)), it.Src.ModTime, cfg.VerifySource)
					mu.Lock()
					read += st.Read
					verify += st.Verify
					mu.Unlock()
					if err != nil {
						failErr = err
						break
					}
					// Once a member is in place the group finishes even after
					// Ctrl-C, so no unrecorded file is left in original/.
					cctx = context.WithoutCancel(ctx)
					stats = append(stats, st)
					entries = append(entries, FileEntry{Source: it.Src.Rel, Size: it.Src.Size, Modified: it.Src.ModTime,
						Destination: it.DestRel, SHA256: st.SHA256, DateSource: it.DateSource, CopiedAt: time.Now()})
				}
				if failErr == nil {
					failErr = m.Append(entries...)
				}
				mu.Lock()
				if failErr != nil {
					for _, it := range g {
						s.Failed[it.Src.Rel] = failErr.Error()
					}
				} else {
					for i, it := range g {
						done++
						s.Copied++
						s.Bytes += stats[i].Bytes
						fmt.Fprintf(out, "[%d/%d] %s\n", done, n, it.DestRel)
					}
				}
				mu.Unlock()
			}
		})
	}
feed:
	for _, g := range groups {
		if ctx.Err() != nil { // select picks randomly when both are ready
			break
		}
		select {
		case ch <- g:
		case <-ctx.Done():
			break feed
		}
	}
	close(ch)
	wg.Wait()
	return read, verify
}

// groupItems splits BuildPlan's output (members of a group are adjacent)
// into groups by Counter.
func groupItems(items []Item) [][]Item {
	var gs [][]Item
	for i, it := range items {
		if i == 0 || it.Counter != items[i-1].Counter {
			gs = append(gs, nil)
		}
		gs[len(gs)-1] = append(gs[len(gs)-1], it)
	}
	return gs
}

// removePartials deletes crash leftovers (.*.partial) in original/ and its
// RAW/, JPEG/, VIDEO/ subfolders.
func removePartials(origDir string) error {
	for _, p := range partials(origDir) {
		if err := os.Remove(p); err != nil {
			return err
		}
	}
	return nil
}

// partials lists the .*.partial files in original/ and its subfolders.
func partials(origDir string) []string {
	var ps []string
	for _, sub := range []string{"", "RAW", "JPEG", "VIDEO"} {
		m, _ := filepath.Glob(filepath.Join(origDir, sub, ".*.partial"))
		ps = append(ps, m...)
	}
	return ps
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func since(t time.Time) float64 { return time.Since(t).Seconds() }

func printSummary(out io.Writer, s Summary) {
	fmt.Fprintf(out, "\nCopied %d, skipped %d, failed %d.\n", s.Copied, s.Skipped, len(s.Failed))
	if s.NotAttempted > 0 {
		fmt.Fprintf(out, "Stopped early: %d files not attempted; run again to import them.\n", s.NotAttempted)
	}
	for _, rel := range slices.Sorted(maps.Keys(s.Failed)) {
		fmt.Fprintf(out, "  FAILED %s: %s\n", rel, s.Failed[rel])
	}
	if n := len(s.ScanErrors); n > 0 {
		noun := "entries"
		if n == 1 {
			noun = "entry"
		}
		fmt.Fprintf(out, "Could not read %d card %s; nothing under it was imported:\n", n, noun)
		for _, e := range s.ScanErrors {
			fmt.Fprintf(out, "  %s\n", e)
		}
	}
	if len(s.Ignored) > 0 {
		fmt.Fprintf(out, "Ignored %d unknown files: %s\n", len(s.Ignored), strings.Join(s.Ignored, ", "))
	}
	if len(s.MtimeFallback) > 0 {
		fmt.Fprintf(out, "No capture date, used modified time: %s\n", strings.Join(s.MtimeFallback, ", "))
	}
	fmt.Fprintf(out, "Manifest: %s\n\n", s.ManifestPath)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "phase\tseconds\tnote")
	for _, t := range s.Timings {
		fmt.Fprintf(tw, "%s\t%.3f\t%s\n", t.Phase, t.Seconds, t.Note)
	}
	tw.Flush()
}
