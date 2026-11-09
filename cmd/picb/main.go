// Command picb imports photos and videos from a camera card.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/tarciosaraiva/pic-bridge/internal/picb"
)

func main() { os.Exit(run()) }

func run() int {
	cfgPath := flag.String("config", "", "config file (default ~/.pic-bridge/config.yaml)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: picb [--config path] <folder>\n\nImports the card into <destination>/<folder>/original.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() { <-ctx.Done(); stop() }() // first Ctrl-C stops cleanly; a second one kills (e.g. a hung card read)
	s, err := picb.Run(ctx, cfg, folder, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "picb:", err)
		return 1
	}
	if len(s.Failed) > 0 || s.NotAttempted > 0 {
		return 1
	}
	return 0
}

// offerConfig handles a missing config: in a terminal it offers to write the
// sample; otherwise it prints where the config goes and what it looks like.
func offerConfig(path string) int {
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
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
