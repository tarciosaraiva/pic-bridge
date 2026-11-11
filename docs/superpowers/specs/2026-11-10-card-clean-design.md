# Card clean — design

Date: 2026-11-10. Replaces the wording of issue #1; folds in the need behind #4.

## Why

One `picb` run should take a card from "full" to "ready to shoot again":
import, verify, clean. Today the user formats in the camera afterwards. The
method doesn't matter to them; one process doing all of it does.

Cleaning removes the card as a second copy, so "verified" has to mean the
destination really holds the same bytes as the card. That is why cleaning
carries a stronger check than a normal import.

## Goals

- `--clean` flag: after the import, verify everything and, if it all checks
  out, ask once and erase the card's contents.
- `verify_source` config option (default `false`): read each card file twice
  during a normal import (issue #4).
- Never erase unless every file on the card that `picb` is responsible for is
  provably in the destination.
- Same behaviour on macOS and Linux, without root, without new dependencies or
  external tools.

## Non-goals

- A config key for cleaning. Cleaning is per run, by flag only.
- A real format (rebuilding the file system). Formatting needs `diskutil` on
  macOS and root (`mkfs.exfat`) or D-Bus (udisks2) on Linux. Erasing the
  contents gives the camera the same empty card. A card with a damaged file
  system still needs an in-camera format.
- Deleting only the imported files. That leaves Sony's `PRIVATE/` database out
  of step with the card; the camera then asks to repair it. Everything goes.
- Protecting files outside `DCIM/` and `PRIVATE/M4ROOT/CLIP/`. The card is
  used only with the camera; camera database, thumbnails and settings files
  there are erased, same as an in-camera format. (User decision.)
- Linux card auto-detection (`source: auto` under `/media/$USER`,
  `/run/media/$USER`) and Linux release builds. Separate issue; on Linux,
  cleaning works with an explicit `source:`.
- Windows. `--clean` is refused there with "not supported on this OS".
- Ejecting the card after cleaning.

## Anti-goals (failure even if it "works")

- Erasing when any card file is not verified in the destination.
- Erasing without an explicit `y` from a person at a terminal.
- Erasing something that is not a camera card, or a volume that holds the
  destination.

## User-facing behaviour

```
picb [--config path] [--clean] <folder>
```

Without `--clean`: unchanged from today, plus `verify_source` if set.

With `--clean`:

1. Import runs as normal (copy-time `verify_source` read is skipped; the gate
   below does the second read for every file).
2. If the import had any failed or not-attempted file, or was stopped with
   Ctrl-C: print "Not cleaning: …", exit 1. No gate, no prompt.
   Same if the scan could not read a card folder or file: the scan carries
   on, lists each one in the run output and in the manifest summary
   (`scan_errors`), and the run exits 1 with or without `--clean`. The gate
   only checks scanned files, so an incomplete scan must never reach it.
3. Gate runs (see below), printing progress like the copy does.
4. Gate fails: print "Not cleaning the card:" and every blocking file with its
   reason. Exit 1. Card untouched.
5. Gate passes: prompt
   `All N files verified against <destination>. Erase everything on card <source>? [y/N] `
   - Not a terminal on stdin: do not prompt; print "Not cleaning: no terminal",
     exit 1.
   - Anything but `y`/`Y`: print "Card left as is.", exit 0.
   - `y`: erase (see below), print "Card <source> erased. Eject it before
     removing it.", exit 0. Erase error: print what could not be removed,
     exit 1 (nothing is lost: every file was verified).
6. Ctrl-C at any point before the erase starts: no erase.

The gate also runs when this run copied nothing (re-run on an
already-imported card), so `--clean` works as a "verify and erase" pass.

## The gate

Inputs: the card root, the scan result (files + ignored) from this run, and
every manifest in `<destination>/.pic-bridge/manifests/`.

Refuse if any of these hold:

1. **Not a card.** The source must be a mount point (its device ID differs
   from its parent folder's, via `os.Stat` + `syscall.Stat_t.Dev`), must not be
   `/`, and must contain a `DCIM/` folder. Holds for `source: auto` and an
   explicit `source:`.
2. **Destination on the card.** The destination folder must be on a different
   device from the source. Otherwise erasing would destroy the imports.
3. **Ignored files inside the scanned folders.** Unknown extensions or
   orphan sidecars under `DCIM/` or `PRIVATE/M4ROOT/CLIP/` were not imported,
   so erasing would lose them (e.g. a new camera writing `.DNG`). List them.
4. **No manifest entry.** A scanned card file with no `kind: file` entry whose
   source + size + modified time match, across all manifests (not only this
   run's — files skipped as already imported live in older manifests).
5. **Card re-read mismatch.** Re-read and SHA-256 the card file; it must equal
   the entry's `sha256`. This is the second read of #4, applied to every file
   including ones imported on earlier runs.
6. **Destination mismatch.** `<destination>/<entry.destination>` must exist and
   its SHA-256 must equal the entry's `sha256`.

If several entries match one card file (possible after an exFAT mtime shift
causes a re-import), the file passes if any one entry passes 5 and 6.

Files outside the two scanned folders are not checked (non-goal above).

### Cost

Check 5 reads the whole card again: roughly as long as the copy (11.2 GB at
the measured card speed). Check 6 reads the destination, which is an SSD and
much faster. Use the same `cfg.Workers` pool as the copy; the card is still
the bottleneck.

### Known ceiling

For files copied in this same run, check 6 may be served from the page cache
rather than the disk — the same open question as #27. Check 5 still catches
a bad card read and check 6 still catches a missing or replaced file; only a
silent bad *write* in this run could slip through. #27 stays open and should
be settled before or alongside this work.

The "not a card" check is weaker than a removable-media check: any mounted
volume with a `DCIM/` folder, whose scanned files are all verified, passes.
The prompt naming the source path is the last guard.

## Erase

- `os.RemoveAll` on each entry in the card root. Standard library only.
- Skip dot-entries at the card root (`.Spotlight-V100`, `.fseventsd`,
  `.Trashes` and similar). The OS creates and may protect them; the camera
  ignores them. Hidden files inside folders go with their folder.
- Keep going after an error; collect every path that could not be removed and
  report them.
- Build-tagged per OS like `nocache_*.go`: the mount-point check uses
  `syscall.Stat_t`, so unix-only; other OSes get a stub returning "not
  supported on this OS".

## `verify_source` (normal import)

- Config: `verify_source: false` (default). Added to the sample config.
- When true and `--clean` is not set: after the copy's verify, re-read the card
  file and compare its SHA-256 with the copy-time hash. Mismatch fails the
  group like any other copy error (temp removed, nothing recorded).
- Share the hash helper with the gate.

## Docs and issues (after this design is approved)

- Main spec `2026-11-07-pic-bridge-design.md`: move "Wiping the card (#1)" out
  of non-goals with a pointer here; same for `verify_source` (#4).
- Issue #1: rewrite to "flag + prompt, all manifests, erase contents".
- Issue #4: note `--clean` covers it during cleaning; config stays for
  normal imports.
- New `later` issue: Linux support (auto-detect under `/media/$USER` and
  `/run/media/$USER`, Linux release builds).
- README: document `--clean` and `verify_source`.

## Left to the builder

- Package layout (gate and erase in `internal/picb`, prompt in `cmd/picb`
  like `offerConfig`), function names, output wording beyond the strings above.
- Whether `LoadImported` grows to return entries or a sibling loader is added
  (the gate needs `sha256` and `destination`, which it drops today).
- Test strategy: the gate and erase are unit-testable on temp folders. The
  mount-point check needs a hook (as `copy.go` does for `link`/`rehash`) so
  a temp folder can pose as a card. One manual run on a real card before
  release.

## Decided by Claude (user to check)

- Ignored files inside scanned folders block the erase.
- With `--clean`, the second card read happens in the gate, not during copy.
- Gate refusal, "no terminal" and erase errors exit 1; user answering No
  exits 0.
- Mount point + `DCIM/` + destination-on-another-device as the card check.
- Root dot-entries are left in place.
