# Archive — design

Date: 2026-11-14. Issue: #52. Cloud targets: #53.

## Why

Imports pile up in `~/Pictures/Imports` and fill the Mac's disk. The user
wants to move a finished shoot to an external USB hard drive (or any folder
they choose) and free the local space, with the same "verified before
anything is deleted" guarantee that `--clean` gives the card.

Cloud and network targets are wanted later. Using rclone for the copy now
means a cloud target later is a config line (`b2:photos`, `gdrive:photos`),
not new code.

## Goals

- `picb --archive <folder>`: copy `<destination>/<folder>` to every configured
  archive target, verify each copy, then ask once and delete the local folder.
- `archive_targets` config: a list of local paths, plus the special value
  `auto`.
- `auto` finds a plugged-in external hard drive, never an SD card or the
  Mac's own disk, and uses it after the user confirms.
- rclone does the copying and checking.

## Non-goals

- Cloud or network targets in this version. rclone could already reach them,
  but they are not designed, documented or tested here. Config validation
  accepts only local paths and `auto` for now.
- Compression or bundling (zip/tar). Each file is archived as itself. Photo
  and video formats are already compressed; one bad sector should cost one
  file, not the archive; files must open straight from the drive.
- Keeping the local copy (a "backup, don't move" mode). The goal is freeing
  space.
- Restoring from the archive. Finder or `rclone copy` back.
- Linux/Windows `auto` detection. `auto` uses macOS `diskutil`; elsewhere it
  fails with "auto archive target is macOS only". Explicit paths work
  anywhere rclone does. Linux support is issue #7.
- Bundling rclone with picb. The user installs it (`brew install rclone`).

## Anti-goals (failure even if it "works")

- Deleting the local folder when any target is missing, failed to copy, or
  failed its check.
- Deleting without an explicit `y` from a person at a terminal.
- Overwriting or changing a file that already exists in a target.
- `auto` picking an SD card, the internal disk, or the volume that holds the
  destination.
- Deleting anything outside `<destination>/<folder>`.

## User-facing behaviour

```
picb [--config path] --archive <folder>
```

`--archive` does not import; no card is needed. `--archive` with `--clean` is
refused (usage error, exit 2).

1. `<folder>` must pass `ValidateFolderName` and `<destination>/<folder>` must
   exist as a folder. Otherwise error, exit 1.
2. `archive_targets` empty or unset: "no archive_targets in config", exit 1.
3. rclone not on `PATH`: "--archive needs rclone: brew install rclone",
   exit 1.
4. Take the destination lock (`lockDest`), same as an import.
   Then, if `<destination>/<folder>/original` holds any `.*.partial` file (an
   interrupted import), refuse: "<folder> has an unfinished import; run
   `picb <folder>` again first", exit 1. Archive does not read manifests.
5. Resolve targets (see `auto` below). Each target becomes
   `<target>/<folder>`. Targets are counted per drive (`Stat_t.Dev` of the
   target root): two targets on the same drive count once in the prompt, and
   `auto` never offers a drive an explicit target already uses.
   An explicit path whose folder does not exist (drive not plugged in) is an
   error, exit 1, nothing copied.
   An explicit target on the same disk as `destination` is allowed (user
   decision: archiving locally is the user's choice). The only refusal is a
   target whose `<target>/<folder>` is inside, equal to, or contains
   `<destination>/<folder>`: the delete would destroy the copy. Error, exit 1,
   nothing copied.
6. For each target, in config order:
   - `rclone copy --immutable <destination>/<folder> <target>/<folder>`
   - `rclone check <destination>/<folder> <target>/<folder>`
   - Show rclone's progress to the user (`--progress` when stdout is a
     terminal).
   - Either command exits non-zero: record the target as failed, carry on
     with the next target.
   - `rclone check` runs without `--one-way`, so files in `<target>/<folder>`
     that are not in the local folder fail the target. A different shoot with
     the same folder name already on the drive therefore blocks the delete;
     the user renames one of them. A resumed archive (target holds a subset)
     still passes.
7. Any target failed: print "Not deleting <destination>/<folder>:" and each
   failed target with rclone's exit status. Exit 1. Local folder untouched.
8. All passed: prompt
   `<folder> copied and checked on N target(s): <list>. Delete <destination>/<folder>? [y/N] `
   - Not a terminal on stdin: "Not deleting: no terminal", exit 1.
   - Anything but `y`/`Y`: "Kept <destination>/<folder>.", exit 0.
   - `y`: `os.RemoveAll(<destination>/<folder>)`, print
     "Deleted <destination>/<folder>. Archived copies: <list>.", exit 0.
     Remove error: print it, exit 1 (every file is already checked on every
     target).
9. Ctrl-C before the delete: rclone is stopped, nothing is deleted. A re-run
   resumes: `--immutable` skips files already there and identical, and errors
   on files there but different.

The whole `<folder>` goes: `original/` and `processed/` (the user's edits).

## `auto`

On macOS, list the entries in `/Volumes` (`volumesDir`, as the card
auto-detect uses) and run `diskutil info -plist <volume>` on each. Keep a
volume only if all hold:

- `Internal` is false (excludes the Mac's disk and the built-in SD slot).
- `RemovableMedia` is false (excludes SD cards in a USB card reader).
- `BusProtocol` is not `Secure Digital` and not `Disk Image`.
- It is not on the same device as `destination` (`syscall.Stat_t.Dev`).

Parse the plist with `encoding/xml` (standard library); only those keys are
needed.

Then:

- One match: prompt `Archive to /Volumes/<Name>/pic-bridge? [y/N] `. No → exit
  0 with "Not archived.", nothing copied.
- Several: list them numbered, prompt for a number; empty or invalid → exit 0,
  nothing copied.
- None: "auto: no external hard drive found", exit 1.
- Not a terminal on stdin: "auto needs a terminal to confirm", exit 1.

The archive root on the chosen drive is `/Volumes/<Name>/pic-bridge/`; the
shoot lands in `/Volumes/<Name>/pic-bridge/<folder>/`. The `auto` confirmation
happens before any copy, so step 8's delete prompt is a second, separate `y`.

Measured on the user's Mac (2026-11-14):

| Volume | Internal | RemovableMedia | BusProtocol | auto |
|---|---|---|---|---|
| USB hard drive | false | false | USB | picked |
| SD card, built-in slot | true | true | Secure Digital | skipped |
| Macintosh HD | true | false | Apple Fabric | skipped |

Not measured: SD card in a USB card reader (expected `RemovableMedia: true`).
Test by hand before release.

## Config

```yaml
archive_targets:          # optional; needed for --archive
  - auto                  # a plugged-in external hard drive, confirmed each run
  - /Volumes/Backup/photos
```

- `~` expanded like `destination`.
- Each entry is `auto` or an absolute path. Anything else (relative path,
  `name:` rclone remote) is a config error for now.
- Added to `SampleConfig`, commented out.

## Manifests

Manifests stay in `<destination>/.pic-bridge/manifests/`, so a re-import of
the same card still skips files already imported, even after their folder was
archived.

Known consequence: `--clean` on a card whose folder was already archived
fails the gate (destination file missing). Clean the card before archiving.
README says so.

## Verification

`rclone check` between two local paths compares size and a hash it computes
on both sides, reading both in full. picb's SHA-256 manifest is not used by
the archive; the source of truth is the local folder at archive time.

Known ceiling: rclone's read of the target may come from the page cache
right after the copy, the same open question as #27. A silent bad write to
the drive could slip through. Upgrade path: `rclone check --download` or an
unmount/remount before checking; decide after #27.

## Package layout

- `internal/picb/archive.go`: target resolution, running rclone (via
  `os/exec`, behind a function variable so tests can fake it), the gate.
- `internal/picb/volumes_darwin.go` / `volumes_other.go`: `auto` detection
  (`diskutil` plist parsing); other OSes return the "macOS only" error.
- `cmd/picb/main.go`: `--archive` flag and the prompts, like `--clean`.

## Tests

- Config: `archive_targets` parsing, `~` expansion, rejection of relative
  paths and remotes.
- `auto`: plist parsing from saved `diskutil` output fixtures (the three
  measured volumes above, plus a USB card reader fixture once captured).
- Archive flow with a fake rclone: all pass → prompt → delete; one target
  fails → nothing deleted; missing explicit target → nothing copied; same
  target twice counted once.
- One manual run on the real USB drive and an SD card before release.

## Docs and issues (after this design is approved)

- New issue: "Archive a folder to an external drive with rclone" — the PR
  links it.
- New `later` issue: cloud/network archive targets (rclone remotes; Google
  Drive 5 TB plan, B2 or Glacier Deep Archive), priority-two.
- README: `--archive`, `archive_targets`, rclone install, "clean before
  archive".
- Main spec: pointer to this design.

## Left to the builder

- Function names, output wording beyond the strings above.
- Exact rclone flags beyond `--immutable` (e.g. `--checkers`, `--transfers`;
  keep transfers small, the drive is the bottleneck).
- How rclone's progress is passed through.

## Decided by Claude (user to check)

- Archive is a move: delete happens only after all targets pass and a `y`.
- Archive root on an `auto` drive is `/Volumes/<Name>/pic-bridge/`.
- `auto` asks before copying; delete asks again after checking.
- A configured explicit target that is not mounted fails the whole run
  rather than being skipped.
- `--archive` and `--clean` are not combined.
- Remotes are rejected in config until the cloud issue lands.
- Extra files in `<target>/<folder>` fail that target (no `--one-way`).
- Targets counted per drive; `auto` skips drives an explicit target uses.
- Leftover `.partial` files from an interrupted import block the archive.
- A target overlapping `<destination>/<folder>` is refused (data-loss guard,
  not a policy on local targets).
