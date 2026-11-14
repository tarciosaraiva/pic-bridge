# pic-bridge

`picb` imports photos and videos from a camera SD card into a folder you
name, with date-based file names, verifying every copy and recording a manifest of what was imported.

Status: version 1 released ([v1.0.0](https://github.com/tarciosaraiva/pic-bridge/releases/tag/v1.0.0)). See [the version 1 design](docs/superpowers/specs/2026-11-07-pic-bridge-design.md).

## Install

Download the tarball for your Mac (`arm64` for Apple Silicon, `amd64` for
Intel) from the [latest release](https://github.com/tarciosaraiva/pic-bridge/releases/latest),
or with Go 1.27+:

```sh
go install github.com/tarciosaraiva/pic-bridge/cmd/picb@latest
```

## Usage

1. Create `~/.pic-bridge/config.yaml` (or run `picb <folder>` once and answer
   `y` to create it with defaults):

   ```yaml
   destination: ~/Pictures/Imports
   date_format: YYYYMMDD
   include_time: false
   use_exif: true
   split_raw_jpeg: false
   workers: 2
   verify_source: false   # true reads each card file twice (slower, catches bad card reads)
   ```

2. Insert the card and run `picb <folder-name>`, e.g. `picb wedding`
   (`--config <path>` to use another config file).

Result: `~/Pictures/Imports/wedding/original/20261107_0001.ARW`, videos
under `original/VIDEO/`, an empty `wedding/processed/` for your edits, and a manifest in `~/Pictures/Imports/.pic-bridge/manifests/`.
Re-running skips files already imported.

### Cleaning the card

`picb --clean <folder-name>` imports as above, then re-reads every file on the
card and its copy in the destination and compares both with the manifest. If
everything matches it asks once, and on `y` erases the card's contents (see
[the card clean design](docs/superpowers/specs/2026-11-10-card-clean-design.md)).

- Nothing is erased if any file failed to import, is not verified, or is of an
  unknown type, or if any card folder or file could not be read during the
  scan; `picb` lists what blocked it and exits 1.
- It works on a card imported earlier too: nothing is copied, everything is
  verified, then it asks.
- Known limit: macOS will not delete files the camera marked read-only
  (Sony's `PRIVATE/SONY/SONYCARD.IND`). `picb` erases everything else, lists
  them under `Could not remove:` and exits 1. No photo is lost; the camera
  rewrites or reuses the file.
- macOS and Linux only. Eject the card before removing it.

### Archiving a folder

`picb --archive <folder-name>` moves a finished folder (`original/` and
`processed/`) off the Mac to an external drive. It does not import and needs no
card (see [the archive design](docs/superpowers/specs/2026-11-14-archive-design.md)).

1. Install [rclone](https://rclone.org): `brew install rclone`.
2. Add targets to the config:

   ```yaml
   archive_targets:
     - /Volumes/Backup/photos
   ```

3. Run `picb --archive wedding`. Each target gets `<target>/wedding`, copied
   with `rclone copy --immutable` and checked with `rclone check`. If every
   target passes, `picb` asks once and on `y` deletes the local folder.

- Nothing is deleted if a target is missing, fails its copy or its check, or
  holds files the local folder does not. `picb` lists the failed targets and
  exits 1. Run it again: files already archived are skipped.
- A file that already exists on a target is never changed.
- Clean the card before archiving its folder: `--clean` checks the
  destination copy, so it fails once the folder is gone.
- Targets must be absolute local paths. Network shares and cloud remotes are
  not supported yet (#53).

## Build and test

Needs Go 1.27+.

```sh
make build   # builds ./picb
make test    # go vet + tests with the race detector
make clean   # removes ./picb
```

## Supported files

RAW (`.ARW`, `.CR2`, `.CR3`), JPEG, HEIF (`.HIF`, `.HEIF`, `.HEIC`), video (`.MP4`, `.MOV`) plus Sony `.XML` sidecars.

## License

Copyright 2026 Tarcio Saraiva. Licensed under the [Apache License 2.0](LICENSE).
