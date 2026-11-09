# pic-bridge

`picb` imports photos and videos from a camera SD card into a folder you
name, with date-based file names, verifying every copy and recording a manifest of what was imported.

Status: version 1 in progress. See [the version 1 design](docs/superpowers/specs/2026-11-07-pic-bridge-design.md).

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
   ```

2. Insert the card and run `picb <folder-name>`, e.g. `picb wedding`
   (`--config <path>` to use another config file).

Result: `~/Pictures/Imports/wedding/original/20261107_0001.ARW`, videos
under `original/VIDEO/`, an empty `wedding/processed/` for your edits, and a manifest in `~/Pictures/Imports/.pic-bridge/manifests/`.
Re-running skips files already imported.

## Build and test

Needs Go 1.27+.

```sh
make build   # builds ./picb
make test    # go vet + tests with the race detector
make clean   # removes ./picb
```

## Supported files

RAW (`.ARW`, `.CR2`, `.CR3`), JPEG, HEIF (`.HIF`, `.HEIF`, `.HEIC`), video (`.MP4`, `.MOV`) plus Sony `.XML` sidecars.
