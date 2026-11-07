# pic-bridge — Version 1 Design

Date: 2026-11-07
Command: `picb`
Repo: https://github.com/tarciosaraiva/pic-bridge (project board #1)

## Intent

Importing a camera card today is manual: insert the SD card into a reader,
copy 200+ files (~50 MB each, 10+ GB per card) by hand, build the folder
structure by hand. The user shoots a Sony A7 V (previously a Canon EOS).

`picb` makes this one step: insert card, run `picb <folder-name>`, done.
Everything else comes from a config file.

```
picb wedding                         # config from ~/.pic-bridge/config.yaml
picb --config ~/other.yaml wedding   # explicit config path
```

`<folder-name>` is required; without it, print usage and exit non-zero.

Success: every photo and video on the card ends up in the destination,
renamed and foldered consistently, each copy verified, with a manifest
recording exactly what was imported.

## Goals (version 1)

- Import photos (RAW + JPEG) and videos from the card to the destination.
- Put every file in the run's named folder, renamed by capture date + counter.
- Optionally split RAW and JPEG into separate subfolders; videos always separate.
- Verify every copy with SHA-256.
- Write an import manifest incrementally, one entry per verified file.
- Never lose or overwrite data.

## Non-goals (version 1) — tracked as issues

- Wiping the card (#1).
- The interactive re-import flow (already-imported card → continue/stop/wipe
  prompts) (#2). The basic skip of already-imported files IS in v1 (see
  "Skipping already-imported files").
- Comparing run settings with the previous run (#3). The manifest stores the
  config snapshot now so this needs no format change later.
- Double-reading the source to catch bad card reads (`verify_source`) (#4).
- Auto-running on card insert. The tool is run by hand in a terminal.
- Windows/Linux card auto-detection. Target is macOS; code should not go out of
  its way to block other platforms.

## Anti-goals (failure even if it "works")

- Any path that overwrites an existing file in the destination.
- Counting a file as imported before its copy is verified.
- A half-copied file left at its final name in the destination.
- A crash that leaves the manifest unreadable for entries written before the crash.

## Constraints

- Language: Go (latest stable; go1.27.3 at time of writing).
- Bottleneck is card read speed (~90–300 MB/s), not CPU. Concurrency is a
  small fixed pool of copy workers; more workers just contend for the card.
- Card is read once per file (no double read in v1).

## Configuration

Location: `$HOME/.pic-bridge/config.yaml` by default, or the path given with
`--config`.

```yaml
source: auto                  # default: auto. Or an explicit path to the card root.
destination: ~/Pictures/Imports   # required
date_format: YYYYMMDD         # default YYYYMMDD
include_time: false           # default false; true adds _HHmmss to FILE names only
use_exif: true                # default true; false = use file modified time
split_raw_jpeg: false         # default false; true = RAW/ and JPEG/ subfolders
```

Field names are the builder's call if a clearer name exists; semantics are fixed.

- `source: auto` = card root of a mounted volume under `/Volumes/*` that
  contains `DCIM`. If none found: error. If more than one found: ask the user
  to pick (or error — builder's call).
- `date_format` uses user-facing tokens `YYYY MM DD HH mm ss`, translated to a
  Go time layout internally. `date_format` must contain only date tokens
  (time comes from `include_time`); reject `HH mm ss` there.
- `~` in paths expands to `$HOME`.
- `<folder-name>` must be a single path segment (no `/`, not `.` or `..`).
- Missing config file: offer to create it with defaults (`[y/N]`). Yes → write the sample config, tell the user to check `destination`, exit 0. No, or not running in a terminal → print where it is expected and a sample; exit non-zero.

## What gets imported

Scanned under the card root:

- `DCIM/**` — photos (and Canon videos).
- `PRIVATE/M4ROOT/CLIP/**` — Sony videos and their `.XML` sidecars.

File classes (case-insensitive extensions):

| Class | Extensions |
|-------|-----------|
| RAW   | `.ARW`, `.CR2`, `.CR3` |
| JPEG  | `.JPG`, `.JPEG` |
| VIDEO | `.MP4`, `.MOV` |
| Sidecar | `.XML` next to a video with the same base name (Sony `C0001M01.XML` ↔ `C0001.MP4`) |

Anything else is ignored and listed in the end summary. (Adding HEIF/HIF is
the builder's call if trivial.)

## Capture date

- Photos with `use_exif: true`: EXIF `DateTimeOriginal`.
- Videos with `use_exif: true`: Sony `.XML` sidecar `CreationDate` (carries a
  zone offset) when present; otherwise MP4/MOV container creation time (`mvhd`),
  treated as UTC. Known risk (unverified): some Canon models may write local
  time into `mvhd`, which would shift near-midnight Canon clips by the UTC
  offset. Accepted for v1 since the current camera is Sony; `date_source`
  records `xml`, `mvhd`, `exif` or `mtime` so it can be audited.
  Dates affect file names only, never folders, so a wrong date can't scatter
  files across folders.
- `use_exif: false`, or metadata missing/unreadable: file modified time.
- Summary lists every file that fell back to modified time.
- Time zone: all dates converted to the Mac's local time before naming.
  EXIF `DateTimeOriginal` has no zone (use `OffsetTimeOriginal` if present,
  else treat as local). MP4 `mvhd` is UTC — convert to local. This matters:
  it decides the date in the file name near midnight.

## Naming

Folder: `<destination>/<folder-name>/` — the name passed on the command line.
When the import starts (before any file is copied), create two subfolders:

- `original/` — every imported file goes here, whatever its date.
- `processed/` — left empty for the user's edited output later. picb never
  writes to it or reads from it; if it already exists, leave it untouched.

Both are created even if the run then finds nothing to copy.

File: `<date>[_<time>]_<counter>.<ext>`

Examples (`picb wedding`):

```
<destination>/wedding/original/20261107_0001.ARW                 # split off, time off
<destination>/wedding/original/20261107_143205_0001.ARW          # include_time: true
<destination>/wedding/original/RAW/20261107_0001.ARW             # split_raw_jpeg: true
<destination>/wedding/original/JPEG/20261107_0001.JPG
<destination>/wedding/original/VIDEO/20261107_0003.MP4           # videos always in VIDEO/
<destination>/wedding/original/VIDEO/20261107_0003.XML           # sidecar shares clip's name
<destination>/wedding/original/20261108_0004.ARW                 # shot after midnight: same folder
```

Without split, RAW and JPEG sit together in `original/` (videos still in `VIDEO/`).

Counter rules:

- 4 digits, zero padded, starting at `0001` per `original/` folder. (Grow past 9999
  naturally, e.g. `10000`.)
- One counter per folder for the whole run, across all dates, photos and videos.
- Assigned in shooting order (capture date, then original filename as tie-break).
- RAW+JPEG pairs (same original base name in the same card folder, e.g.
  `DSC01234.ARW` + `DSC01234.JPG`) share one counter number; if their dates
  disagree, the RAW's date wins. A video and its sidecar share one counter number.
- Counter kept even when time is included (bursts share a second).
- Importing into an existing folder continues from the highest counter
  already present in its `original/`, including the `RAW/`, `JPEG/` and `VIDEO/`
  subfolders (scan existing names; ignore `.partial` leftovers), never restarting.
- Counters are assigned up front, before copying starts, so concurrent workers
  never race for numbers.
- Final safety net: if a target name exists anyway, do not overwrite — fail
  that file and report it.

## Skipping already-imported files

Before assigning counters, read every manifest in
`<destination>/.pic-bridge/manifests/` and skip any card file whose
original path (relative to card root) + size + modified time match a
`kind: file` entry. Uses the folder listing only; no file is opened.
This makes re-running after Ctrl-C, an unplugged card or failed files safe.
Skipped files count in the summary. Matching is across all folders, so
re-running the same card under a different folder name also skips.
Known ceiling: if a card reader shifts modified times (exFAT time-zone
handling), files look new and get copied again — duplicates, not loss.

## Copy and verify

Per file:

1. Copy to a temporary name in the target folder (e.g. `.<name>.partial`).
2. While copying, feed bytes to SHA-256 (`io.MultiWriter`) — one card read.
3. fsync, then re-read the temp file from the destination disk and hash it.
4. Hashes match → move temp to final name without ever replacing an existing
   file (plain `os.Rename` overwrites on macOS; use `os.Link` + remove temp,
   which fails if the target exists). Preserve source modified time,
   then append the manifest entry. Mismatch or any error → delete temp,
   record failure, continue with other files.

Ctrl-C: stop handing out new files, let in-flight copies finish or clean up
their temp files, flush manifest, print summary.

Worker count: config key `workers`. Default set from a timed comparison of
1–4 workers on the user's real card and reader (smallest count within ~5% of
the best throughput). Adaptive worker count is deferred (#5).

## Manifest

One manifest per import run:
`<destination>/.pic-bridge/manifests/<YYYYMMDD_HHmmss>.yaml` (run start time).

Format: YAML, UTF-8, multi-document stream. First document is the run header;
one document per verified file follows, appended and fsynced as each file
completes. A crash can only damage the last document; readers decode
documents in order and discard a trailing broken one.

```yaml
---
kind: run
started_at: 2026-11-07T14:32:05+11:00
source: /Volumes/A7V/
folder: wedding
picb_version: 0.1.0
config:            # full snapshot of the effective config for this run
  destination: /Users/tarcio/Pictures/Imports
  date_format: YYYYMMDD
  include_time: false
  use_exif: true
  split_raw_jpeg: true
---
kind: file
source: DCIM/100MSDCF/DSC01234.ARW      # relative to card root
size: 52428800
modified: 2026-11-07T09:15:42+11:00     # source file modified time
destination: wedding/original/RAW/20261107_0001.ARW   # relative to destination
sha256: 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
date_source: exif                       # exif | xml | mvhd | mtime
copied_at: 2026-11-07T14:32:09+11:00
```

Failed files are not written as `kind: file`. A closing `kind: summary`
document records counts, bytes, worker count and per-phase timings (also
written on Ctrl-C; absent only after a crash). Readers ignore it for matching.

Original name + size + modified time are stored because they are the future
matching key for re-import (#2), readable from a folder listing without
opening files.

## Interaction and output

- Normal run is non-interactive: read config, scan, show plan summary
  (N photos, N videos, total size, destination), copy with progress, print
  final summary.
- Whether to ask "proceed? [Y/n]" after the plan summary is the builder's call.
- Final summary: copied, failed (with reasons), ignored files, files that used
  modified time instead of metadata, manifest path.
- Exit code non-zero if any file failed.
- Timings table after the summary: `scan`, `manifests`, `dates`, `plan`,
  `copy` (with MB/s), `copy.read` and `copy.verify` (summed across workers),
  `total`. `dates` measures metadata-read overhead on every run (input for #2);
  read vs verify shows whether the card is the bottleneck.

## Libraries

Checked 2026-11-07:

- EXIF: `github.com/evanoberholster/imagemeta` v1.1.0 — docs list ARW, CR2,
  CR3, JPEG support. Not yet verified against real A7 V files: builder should
  test on sample ARW/JPEG from the user's card early.
- MP4/MOV creation time: `github.com/abema/go-mp4` v1.7.4, or hand-parse the
  `mvhd` box — builder's call.
- YAML: `go.yaml.in/yaml/v3` v3.0.6 (maintained successor of `gopkg.in/yaml.v3`).
- SHA-256, file I/O, concurrency: Go standard library.

## Left to the builder

- Package layout, CLI framework (stdlib `flag` preferred), progress display.
- Exact worker count default.
- Field-name polish in config, as long as semantics hold.
- Error message wording.
- Test strategy; must include a test for counter assignment (pairs, multi-day,
  continuing an existing folder), skipping against manifests, and the
  copy-verify-manifest path.
