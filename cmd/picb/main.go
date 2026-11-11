// Command picb imports photos and videos from a camera card.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"

	"github.com/tarciosaraiva/pic-bridge/internal/picb"
)

// version is set by release builds with -ldflags "-X main.version=vX.Y.Z".
var version = ""

// versionString falls back to the module version Go records, which covers
// `go install .../cmd/picb@vX.Y.Z` and local builds.
func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "dev"
}

func main() { os.Exit(run()) }

func run() int {
	cfgPath := flag.String("config", "", "config file (default ~/.pic-bridge/config.yaml)")
	clean := flag.Bool("clean", false, "after the import, re-verify every card file and offer to erase the card")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: picb [--config path] [--clean] <folder>\n\nImports the card into <destination>/<folder>/original.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("picb", versionString())
		return 0
	}
	if flag.NArg() < 1 {
		flag.Usage()
		return 2
	}
	folder := flag.Arg(0)
	flag.CommandLine.Parse(flag.Args()[1:]) // also accept flags after the folder
	if flag.NArg() != 0 {
		flag.Usage()
		return 2
	}
	if err := picb.ValidateFolderName(folder); err != nil {
		fmt.Fprintln(os.Stderr, "picb:", err)
		return 2
	}

	path := *cfgPath
	if path == "" {
		var err error
		if path, err = picb.DefaultConfigPath(); err != nil {
			fmt.Fprintln(os.Stderr, "picb:", err)
			return 1
		}
	}
	cfg, err := picb.LoadConfig(path)
	if errors.Is(err, picb.ErrNoConfig) {
		return offerConfig(path)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "picb:", err)
		return 1
	}

	if *clean {
		cfg.VerifySource = false // the clean gate does the second card read
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() { <-ctx.Done(); stop() }() // first Ctrl-C stops cleanly; a second one kills (e.g. a hung card read)
	s, err := picb.Run(ctx, cfg, folder, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "picb:", err)
		return 1
	}
	if *clean {
		return cleanCard(ctx, cfg, s, os.Stdin, isTerminal(), os.Stdout)
	}
	if len(s.Failed) > 0 || s.NotAttempted > 0 || len(s.ScanErrors) > 0 {
		return 1
	}
	return 0
}

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

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// offerConfig handles a missing config: in a terminal it offers to write the
// sample; otherwise it prints where the config goes and what it looks like.
func offerConfig(path string) int {
	if isTerminal() {
		fmt.Printf("No config at %s. Create it with defaults? [y/N] ", path)
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.EqualFold(strings.TrimSpace(answer), "y") {
			if err := picb.WriteSampleConfig(path); err != nil {
				fmt.Fprintln(os.Stderr, "picb:", err)
				return 1
			}
			fmt.Printf("Wrote %s. Check `destination` in it, then run picb again.\n", path)
			return 0
		}
	}
	fmt.Fprintf(os.Stderr, "picb: no config at %s. Create it with:\n\n%s", path, picb.SampleConfig)
	return 1
}
