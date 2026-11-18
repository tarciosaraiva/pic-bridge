# Configuration

`picb` reads `~/.pic-bridge/config.yaml`. Use `--config <path>` to read another
file.

If the file does not exist, run `picb <folder>` once and answer `y` to create it
with defaults.

```yaml
source: auto
destination: ~/Pictures/Imports
date_format: YYYYMMDD
include_time: false
use_exif: true
split_raw_jpeg: false
workers: 2
verify_source: false
# archive_targets:
#   - auto
#   - /Volumes/Backup/photos
```

Unknown keys are an error, so a typo never falls back to a default silently.

## Keys

| Key | Default | Meaning |
| --- | --- | --- |
| `source` | `auto` | The card root. `auto` finds the card, or set a path. |
| `destination` | none, required | Folder that holds every import. `~` is expanded. |
| `date_format` | `YYYYMMDD` | File name date. Only `YYYY`, `MM`, `DD` joined by `-`, `_`, `.` or a space. |
| `include_time` | `false` | `true` adds `_HHmmss` to file names (not folder names). |
| `use_exif` | `true` | Date from EXIF. `false` uses the file's modified time. |
| `split_raw_jpeg` | `false` | `true` puts RAW and JPEG files in `RAW/` and `JPEG/` subfolders. |
| `workers` | `2` | Parallel copies. 2 saturates a USB card reader. |
| `verify_source` | `false` | `true` reads each card file twice. Slower, but catches bad card reads. |
| `archive_targets` | none | Needed only for [`--archive`](./usage#archiving-a-folder). |

## Archive targets

```yaml
archive_targets:
  - auto                  # a plugged-in external hard drive, confirmed each run
  - /Volumes/Backup/photos
```

- `auto` (macOS only) offers each plugged-in external hard drive, never an SD
  card or the Mac's own disk. It archives to `/Volumes/<Name>/pic-bridge` after
  you confirm.
- Other targets must be absolute local paths. Network shares and cloud remotes
  are not supported yet
  ([#53](https://github.com/tarciosaraiva/pic-bridge/issues/53)).

Next: [Usage](./usage).
