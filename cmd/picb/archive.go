package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
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
	for _, t := range cfg.ArchiveTargets {
		if t == "auto" {
			fmt.Fprintln(out, "picb: auto archive target is not supported yet")
			return 1
		}
		if err := a.AddTarget(t); err != nil {
			fmt.Fprintln(out, "picb:", err)
			return 1
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
	answer, _ := bufio.NewReader(in).ReadString('\n')
	if !strings.EqualFold(strings.TrimSpace(answer), "y") {
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
