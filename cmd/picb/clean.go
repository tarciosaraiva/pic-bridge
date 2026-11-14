package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/tarciosaraiva/pic-bridge/internal/picb"
)

// cleanCard runs after an import with --clean: it erases the card only when
// the import was complete, every card file is verified in the destination,
// and a person at a terminal answers y. It returns the exit code.
func cleanCard(ctx context.Context, cfg picb.Config, s picb.Summary, in io.Reader, tty bool, out io.Writer) int {
	refuse := func(why string) int {
		fmt.Fprintln(out, "Not cleaning:", why)
		return 1
	}
	// The gate only checks scanned files: an incomplete scan must never reach it.
	if len(s.ScanErrors) > 0 {
		return refuse("the card could not be read completely.")
	}
	if len(s.Failed) > 0 || s.NotAttempted > 0 {
		return refuse("the import did not finish cleanly.")
	}
	if ctx.Err() != nil {
		return refuse("stopped.")
	}
	blockers, err := picb.CheckClean(ctx, cfg, s.Source, s.Files, s.Ignored, out)
	if ctx.Err() != nil {
		return refuse("stopped.")
	}
	if err != nil {
		return refuse(err.Error())
	}
	if len(blockers) > 0 {
		fmt.Fprintln(out, "Not cleaning the card:")
		for _, b := range blockers {
			fmt.Fprintf(out, "  %s: %s\n", b.Path, b.Reason)
		}
		return 1
	}
	if !tty {
		return refuse("no terminal")
	}
	fmt.Fprintf(out, "All %d files verified against %s. Erase everything on card %s? [y/N] ", len(s.Files), cfg.Destination, s.Source)
	answer, _ := bufio.NewReader(in).ReadString('\n')
	if !strings.EqualFold(strings.TrimSpace(answer), "y") {
		fmt.Fprintln(out, "Card left as is.")
		return 0
	}
	if ctx.Err() != nil {
		return refuse("stopped.")
	}
	if fails := picb.EraseCard(s.Source); len(fails) > 0 {
		fmt.Fprintln(out, "Could not remove:")
		for _, f := range fails {
			fmt.Fprintln(out, " ", f)
		}
		return 1
	}
	fmt.Fprintf(out, "Card %s erased. Eject it before removing it.\n", s.Source)
	return 0
}
