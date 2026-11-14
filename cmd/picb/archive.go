package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/tarciosaraiva/pic-bridge/internal/picb"
)

// archiveFolder runs --archive: copy and check <destination>/<folder> on every
// archive target, then delete it only when all passed and a person at a
// terminal answers y. tty is stdin; progress is stdout being a terminal.
// It returns the exit code.
func archiveFolder(ctx context.Context, cfg picb.Config, folder string, in io.Reader, tty, progress bool, out io.Writer) int {
	a, err := picb.OpenArchive(cfg, folder)
	if err != nil {
		fmt.Fprintln(out, "picb:", err)
		return 1
	}
	defer a.Close()
	r := bufio.NewReader(in) // shared by both prompts
	auto := false
	for _, t := range cfg.ArchiveTargets {
		if t == "auto" {
			auto = true
			continue
		}
		if err := a.AddTarget(t); err != nil {
			fmt.Fprintln(out, "picb:", err)
			return 1
		}
	}
	// After the explicit targets, so auto skips their drives. The auto
	// target is therefore copied last, whatever its place in the config.
	if auto {
		if code, ok := addAutoDrive(a, r, tty, out); !ok {
			return code
		}
	}

	fails := a.Copy(ctx, out, progress)
	refuse := func(why string) int {
		fmt.Fprintln(out, "Not deleting:", why)
		return 1
	}
	if ctx.Err() != nil {
		return refuse("stopped.")
	}
	if len(fails) > 0 {
		fmt.Fprintf(out, "Not deleting %s:\n", a.Src)
		for _, f := range fails {
			fmt.Fprintf(out, "  %s: %s\n", f.Target, f.Reason)
		}
		return 1
	}
	if !tty {
		return refuse("no terminal")
	}
	list := strings.Join(a.Targets, ", ")
	fmt.Fprintf(out, "%s copied and checked on %d target(s): %s. Delete %s? [y/N] ", folder, a.Drives(), list, a.Src)
	if !strings.EqualFold(readAnswer(r), "y") {
		fmt.Fprintf(out, "Kept %s.\n", a.Src)
		return 0
	}
	if ctx.Err() != nil {
		return refuse("stopped.")
	}
	if err := a.Delete(); err != nil {
		fmt.Fprintln(out, "picb:", err)
		return 1
	}
	fmt.Fprintf(out, "Deleted %s. Archived copies: %s.\n", a.Src, list)
	return 0
}

// addAutoDrive asks which external drive the auto target means and adds it.
// ok is false when the run must stop, with code its exit code.
func addAutoDrive(a *picb.Archive, r *bufio.Reader, tty bool, out io.Writer) (code int, ok bool) {
	drives, err := a.ArchiveDrives()
	switch {
	case err != nil:
		fmt.Fprintln(out, "picb:", err)
		return 1, false
	case len(drives) == 0:
		fmt.Fprintln(out, "picb: auto: no external hard drive found")
		return 1, false
	case !tty:
		fmt.Fprintln(out, "picb: auto needs a terminal to confirm")
		return 1, false
	}
	root := pickDrive(drives, r, out)
	if root == "" {
		fmt.Fprintln(out, "Not archived.")
		return 0, false
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		fmt.Fprintln(out, "picb:", err)
		return 1, false
	}
	if err := a.AddTarget(root); err != nil {
		fmt.Fprintln(out, "picb:", err)
		return 1, false
	}
	return 0, true
}

// pickDrive asks for one of drives; "" means none.
func pickDrive(drives []string, r *bufio.Reader, out io.Writer) string {
	if len(drives) == 1 {
		fmt.Fprintf(out, "Archive to %s? [y/N] ", drives[0])
		if strings.EqualFold(readAnswer(r), "y") {
			return drives[0]
		}
		return ""
	}
	for i, d := range drives {
		fmt.Fprintf(out, "  %d) %s\n", i+1, d)
	}
	fmt.Fprintf(out, "Archive to which drive? [1-%d, Enter for none] ", len(drives))
	n, err := strconv.Atoi(readAnswer(r))
	if err != nil || n < 1 || n > len(drives) {
		return ""
	}
	return drives[n-1]
}

func readAnswer(r *bufio.Reader) string {
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}
