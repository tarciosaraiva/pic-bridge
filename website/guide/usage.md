# Usage

## Importing a card

Insert the card and run:

```sh
picb wedding
```

Result:

- `~/Pictures/Imports/wedding/original/20261107_0001.ARW`
- videos under `original/VIDEO/`
- an empty `wedding/processed/` for your edits
- a manifest in `~/Pictures/Imports/.pic-bridge/manifests/`

Every copy is written to a temp name, checked with SHA-256, then renamed. An
existing file is never overwritten. Re-running skips files already imported.

## Cleaning the card

```sh
picb --clean wedding
```

`picb` imports as above, then re-reads every file on the card and its copy in
the destination and compares both with the manifest. If everything matches it
asks once, and on `y` erases the card's contents.

- Nothing is erased if any file failed to import, is not verified, or is of an
  unknown type, or if any card folder or file could not be read during the
  scan. `picb` lists what blocked it and exits 1.
- It works on a card imported earlier too: nothing is copied, everything is
  verified, then it asks.
- Known limit: macOS will not delete files the camera marked read-only (Sony's
  `PRIVATE/SONY/SONYCARD.IND`). `picb` erases everything else, lists them under
  `Could not remove:` and exits 1. No photo is lost; the camera rewrites or
  reuses the file.
- macOS and Linux only. Eject the card before removing it.

See [the card clean design](https://github.com/tarciosaraiva/pic-bridge/blob/main/docs/superpowers/specs/2026-11-10-card-clean-design.md).

## Archiving a folder

```sh
picb --archive wedding
```

Moves a finished folder (`original/` and `processed/`) off the Mac to an
external drive. It does not import and needs no card. It needs
[rclone](https://rclone.org) and
[`archive_targets`](./configuration#archive-targets) in the config.

Each target gets `<target>/wedding`, copied with `rclone copy --immutable` and
checked with `rclone check`. If every target passes, `picb` asks once and on
`y` deletes the local folder.

- Nothing is deleted if a target is missing, fails its copy or its check, or
  holds files the local folder does not. `picb` lists the failed targets and
  exits 1. Run it again: files already archived are skipped.
- A file that already exists on a target is never changed.
- `.DS_Store` files (Finder metadata) are not archived, and the check ignores
  them on the drive.
- ExFAT and FAT drives work, with two adjustments:
  - These filesystems can't store macOS extended attributes, so macOS writes
    them as hidden `._<name>` files. `picb` doesn't archive them, and the check
    ignores them on the drive.
  - They store file times less precisely (ExFAT to 10 ms, FAT to 2 s), so a
    re-run would see every file as changed. Times within 2 seconds count as
    equal.

  Neither weakens the check: `rclone check` reads every file in full on both
  sides and compares checksums.
- Clean the card before archiving its folder: `--clean` checks the destination
  copy, so it fails once the folder is gone.

See [the archive design](https://github.com/tarciosaraiva/pic-bridge/blob/main/docs/superpowers/specs/2026-11-14-archive-design.md).

## Supported files

RAW (`.ARW`, `.CR2`, `.CR3`), JPEG, HEIF (`.HIF`, `.HEIF`, `.HEIC`), video
(`.MP4`, `.MOV`) plus Sony `.XML` sidecars.
