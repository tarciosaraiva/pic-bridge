package picb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// deviceID is a test hook: a temp folder can pose as a mounted card.
var deviceID = deviceOf

// SetDeviceIDForTest swaps the device lookup so cmd/picb tests can pass a
// temp folder off as a card. Call restore when done.
// ponytail: exported test seam; move gate tests to an internal package if more seams pile up.
func SetDeviceIDForTest(f func(string) (uint64, error)) (restore func()) {
	old := deviceID
	deviceID = f
	return func() { deviceID = old }
}

// Blocker is one reason the card must not be erased. Path is card-relative
// for a file, or the card root for a problem with the volume itself.
type Blocker struct{ Path, Reason string }

// CheckClean is the gate in front of EraseCard: it returns a Blocker for
// everything that makes erasing src unsafe. files and ignored are the scan of
// src. The error is for setup problems only (unsupported OS, unreadable
// manifests, ctx cancelled); callers must not erase on either.
func CheckClean(ctx context.Context, cfg Config, src string, files []SourceFile, ignored []string, out io.Writer) ([]Blocker, error) {
	if reason, err := volumeProblem(cfg, src); err != nil {
		return nil, err
	} else if reason != "" {
		return []Blocker{{src, reason}}, nil
	}
	entries, err := LoadEntries(filepath.Join(cfg.Destination, ".pic-bridge", "manifests"))
	if err != nil {
		return nil, err
	}
	var blockers []Blocker
	for _, rel := range ignored {
		blockers = append(blockers, Blocker{rel, "not imported (unknown file)"})
	}

	var mu sync.Mutex
	done := 0
	ch := make(chan SourceFile)
	var wg sync.WaitGroup
	for range cfg.Workers {
		wg.Go(func() {
			for f := range ch {
				reason := fileProblem(cfg.Destination, f, entries[KeyOf(f)])
				mu.Lock()
				done++
				fmt.Fprintf(out, "Verifying [%d/%d] %s\n", done, len(files), f.Rel)
				if reason != "" {
					blockers = append(blockers, Blocker{f.Rel, reason})
				}
				mu.Unlock()
			}
		})
	}
feed:
	for _, f := range files {
		select {
		case ch <- f:
		case <-ctx.Done():
			break feed
		}
	}
	close(ch)
	wg.Wait()
	// ponytail: a hash in flight finishes before Ctrl-C is seen; pass ctx into
	// hashFile if that wait matters on big videos.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(blockers, func(i, j int) bool { return blockers[i].Path < blockers[j].Path })
	return blockers, nil
}

// EraseCard removes everything in root except root-level dot-entries
// (.Trashes, .Spotlight-V100: the OS owns them, the camera ignores them).
// It keeps going after an error and returns "<path>: <error>" for each entry
// it could not remove. Call it only after CheckClean returned no blockers.
func EraseCard(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", root, err)}
	}
	var fails []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(root, e.Name())
		if err := os.RemoveAll(p); err != nil {
			fails = append(fails, fmt.Sprintf("%s: %v", p, err))
		}
	}
	return fails
}

// volumeProblem says why src is not a camera card that is safe to erase:
// it must be a mounted volume with a DCIM folder, and must not hold the
// destination. Empty means no problem.
func volumeProblem(cfg Config, src string) (string, error) {
	if filepath.Clean(src) == "/" {
		return "is the filesystem root", nil
	}
	// A symlink to a folder on another disk would pass the device check below.
	if fi, err := os.Lstat(src); err != nil {
		return "", err
	} else if fi.Mode()&fs.ModeSymlink != 0 {
		return "not a mounted volume", nil
	}
	card, err := deviceID(src)
	if err != nil {
		return "", err
	}
	parent, err := deviceID(filepath.Dir(src))
	if err != nil {
		return "", err
	}
	if card == parent {
		return "not a mounted volume", nil
	}
	if fi, err := os.Stat(filepath.Join(src, "DCIM")); err != nil || !fi.IsDir() {
		return "no DCIM folder", nil
	}
	dest, err := deviceID(cfg.Destination)
	if err != nil {
		return "", err
	}
	if dest == card {
		return "destination is on the card", nil
	}
	return "", nil
}

// fileProblem re-reads card file f and the destination copy its manifest
// entries name. f passes when any one entry matches both. Empty means verified.
func fileProblem(destination string, f SourceFile, entries []FileEntry) string {
	if len(entries) == 0 {
		return "no manifest entry"
	}
	sum, err := hashFile(f.Abs)
	if err != nil {
		return "card re-read failed: " + err.Error()
	}
	reason := "card re-read does not match the manifest"
	for _, e := range entries {
		if e.SHA256 != sum {
			continue
		}
		got, err := hashFile(filepath.Join(destination, filepath.FromSlash(e.Destination)))
		switch {
		case err == nil && got == sum:
			return ""
		case errors.Is(err, fs.ErrNotExist):
			reason = "destination " + e.Destination + " missing"
		case err != nil:
			reason = "destination " + e.Destination + ": " + err.Error()
		default:
			reason = "destination " + e.Destination + " does not match the manifest"
		}
	}
	return reason
}
