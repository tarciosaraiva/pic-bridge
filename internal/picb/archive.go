package picb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Archive is one --archive run: <destination>/<folder> copied to each target
// with rclone, checked, and only then offered for deletion. It holds the
// destination lock until Close.
type Archive struct {
	Src     string   // <destination>/<folder>
	Targets []string // <target>/<folder>, config order, no duplicates
	folder  string
	devs    map[uint64]bool // drives the targets are on
	unlock  func()
}

// TargetFailure is a target whose copy or check failed.
type TargetFailure struct{ Target, Reason string }

// OpenArchive checks that folder can be archived and takes the destination
// lock. The caller must Close it.
func OpenArchive(cfg Config, folder string) (*Archive, error) {
	if err := ValidateFolderName(folder); err != nil {
		return nil, err
	}
	src := filepath.Join(cfg.Destination, folder)
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", src)
	}
	if len(cfg.ArchiveTargets) == 0 {
		return nil, errors.New("no archive_targets in config")
	}
	if _, err := exec.LookPath("rclone"); err != nil {
		return nil, errors.New("--archive needs rclone: brew install rclone")
	}
	if err := os.MkdirAll(filepath.Join(cfg.Destination, ".pic-bridge"), 0o755); err != nil {
		return nil, err
	}
	unlock, err := lockDest(cfg.Destination)
	if err != nil {
		return nil, err
	}
	if len(partials(filepath.Join(src, "original"))) > 0 {
		unlock()
		return nil, fmt.Errorf("%s has an unfinished import; run `picb %s` again first", folder, folder)
	}
	return &Archive{Src: src, folder: folder, devs: map[uint64]bool{}, unlock: unlock}, nil
}

// Close releases the destination lock.
func (a *Archive) Close() { a.unlock() }

// Drives is how many separate drives the targets are on.
func (a *Archive) Drives() int { return len(a.devs) }

// UsesDrive reports whether a target is already on root's drive.
func (a *Archive) UsesDrive(root string) bool {
	dev, err := deviceID(root)
	return err == nil && a.devs[dev]
}

// AddTarget adds <root>/<folder>. root must be an existing folder, and the
// target must not overlap the source: deleting the source would destroy it.
func (a *Archive) AddTarget(root string) error {
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return fmt.Errorf("archive target %s is not a folder (is the drive plugged in?)", root)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realSrc, err := filepath.EvalSymlinks(a.Src)
	if err != nil {
		return err
	}
	dir := filepath.Join(realRoot, a.folder)
	if within(dir, realSrc) || within(realSrc, dir) {
		return fmt.Errorf("archive target %s overlaps %s", dir, a.Src)
	}
	dev, err := deviceID(realRoot)
	if err != nil {
		return err
	}
	a.devs[dev] = true
	dir = filepath.Join(root, a.folder)
	for _, t := range a.Targets {
		if t == dir {
			return nil
		}
	}
	a.Targets = append(a.Targets, dir)
	return nil
}

// within reports whether p is base or inside it. Both must be clean.
func within(p, base string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// Copy runs rclone copy then rclone check for each target, in order, and
// carries on past a failed target. progress adds rclone's --progress.
// It stops early only when ctx is cancelled; check ctx.Err() before trusting
// an empty result.
func (a *Archive) Copy(ctx context.Context, out io.Writer, progress bool) []TargetFailure {
	// .DS_Store: Finder writes one into a target folder that someone opens,
	// which would fail the next check for no real reason. ._*: macOS stores
	// extended attributes (com.apple.provenance) as AppleDouble files on
	// ExFAT/FAT drives.
	// --config "": local paths need no rclone config; skips its "not found"
	// notice. ponytail: drop when remote targets land (#53).
	// --modify-window 2s: ExFAT keeps times to 10ms and FAT to 2s, so a
	// re-run would see every file as modified. Content is still compared
	// by rclone check.
	common := []string{"--config", "", "--modify-window", "2s", "--exclude", ".DS_Store", "--exclude", "._*"}
	if progress {
		common = append(common, "--progress")
	}
	var fails []TargetFailure
	for _, t := range a.Targets {
		fmt.Fprintf(out, "Archiving %s to %s\n", a.Src, t)
		// --immutable: never change a file already on the target.
		// ponytail: transfers 2, the drive is the bottleneck; tune if a fast SSD target shows otherwise.
		copyArgs := append([]string{"copy", "--immutable", "--transfers", "2"}, common...)
		// Without --one-way, files on the target that are not in Src fail it.
		checkArgs := append([]string{"check"}, common...)
		for _, args := range [][]string{copyArgs, checkArgs} {
			if err := rclone(ctx, out, append(args, a.Src, t)...); err != nil {
				if ctx.Err() != nil {
					return fails
				}
				fails = append(fails, TargetFailure{t, fmt.Sprintf("rclone %s: %v", args[0], err)})
				break
			}
		}
	}
	return fails
}

func rclone(ctx context.Context, out io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "rclone", args...)
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}

// Delete removes the archived folder. Call it only when Copy returned no
// failures and a person said yes.
func (a *Archive) Delete() error { return os.RemoveAll(a.Src) }
