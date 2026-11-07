# pic-bridge

`picb` imports photos and videos from a camera SD card into a folder you
name, with date-based file names, verifying every copy and recording a manifest of what was imported.

Status: version 1 in progress. See [the version 1 design](docs/superpowers/specs/2026-11-07-pic-bridge-design.md).

## Usage (planned)

1. Create `~/.pic-bridge/config.yaml`:

   ```yaml
   destination: ~/Pictures/Imports
   date_format: YYYYMMDD
   include_time: false
   use_exif: true
   split_raw_jpeg: false
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

RAW (`.ARW`, `.CR2`, `.CR3`), JPEG, video (`.MP4`, `.MOV`) plus Sony `.XML` sidecars.
