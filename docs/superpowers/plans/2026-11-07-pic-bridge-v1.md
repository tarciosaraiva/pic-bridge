# pic-bridge v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `picb <folder-name>` copies every photo/video from a mounted camera card into `<destination>/<folder-name>/original/`, renamed `<date>[_<time>]_<counter>.<ext>`, SHA-256 verified, with an incremental YAML manifest and skipping of already-imported files.

**Architecture:** One Go module. `cmd/picb/main.go` is a thin CLI (stdlib `flag`). All logic lives in `internal/picb`, split by responsibility: config, scan, dates, manifest, plan (grouping + counters + names, pure), copy (copy-verify-link), run (orchestration + worker pool). Plan is computed fully before any copy starts; workers only execute it.

**Tech Stack:** Go 1.27.3, `github.com/evanoberholster/imagemeta` v1.1.0 (EXIF), `go.yaml.in/yaml/v3` v3.0.6 (config + manifest). `mvhd` and Sony XML parsed by hand with stdlib (`encoding/binary`, `encoding/xml`).

**Spec:** `docs/superpowers/specs/2026-11-07-pic-bridge-design.md`

## Global Constraints

- Module path: `github.com/tarciosaraiva/pic-bridge`; binary name `picb`; `const Version = "0.1.0"`.
- Default config path: `$HOME/.pic-bridge/config.yaml`; override with `--config <path>`.
- Config defaults: `source: auto`, `date_format: YYYYMMDD`, `include_time: false`, `use_exif: true`, `split_raw_jpeg: false`, `workers: 3` (provisional — final default set from the timed check in Task 7 Step 6). `destination` required.
- Output layout: `<destination>/<folder>/original/` (+ `RAW/`, `JPEG/` when split; `VIDEO/` always) and empty `<destination>/<folder>/processed/`, both created at run start.
- File name: `<date>[_<HHmmss>]_<counter>.<ext>`; counter 4-digit zero padded, `%04d`; extension keeps its original case as on the card.
- Manifests: `<destination>/.pic-bridge/manifests/<YYYYMMDD_HHmmss>.yaml`, multi-document YAML, fsync after every document.
- Never overwrite a destination file. Never leave a `.partial` at a final name. Never write a manifest entry before verification.
- No new dependencies beyond the two above. Tests use stdlib `testing` only.
- **Do not commit** unless the user explicitly says so. Commit steps below are run only on instruction; end messages with `Co-Authored by Claude 🤖`.

## Review Focus

1. **Ctrl-C mid-copy** — no `.partial` left, manifest decodes, summary printed, exit non-zero. Test: Task 7 `TestRunCancel`.
2. **Leftover `.partial` from a crashed run** in `original/` — ignored by counter scan; new run completes. Test: Task 5 `TestNextCounterIgnoresPartial`, Task 6 `TestCopyVerifyOverwritesStalePartial`.
3. **Copy error mid-file (disk full, card pulled)** — temp removed, file reported failed, other files continue, exit non-zero. Test: Task 6 `TestCopyVerifySourceError`, Task 7 `TestRunReportsFailure`.
4. **Empty card / everything already imported** — folders + manifest header still created, exit 0, summary says 0 copied. Test: Task 7 `TestRunEmptyCard`, `TestRunSkipsSecondTime`.
5. **Mixed-case names** (`DSC01234.ARW` + `dsc01234.jpg`, `.Mp4`) — classified and paired case-insensitively. Test: Task 2 `TestScanCaseInsensitive`, Task 5 `TestPlanPairsCaseInsensitive`.

---

## File Structure

```
go.mod
cmd/picb/main.go                 CLI: flags, usage, exit codes
internal/picb/config.go          Config, LoadConfig, DefaultConfigPath, ValidateFolderName, dateLayout
internal/picb/scan.go            Class, SourceFile, FindCardRoot, Scan
internal/picb/dates.go           DateSource, CaptureDate, parseMvhd, parseSonyXML
internal/picb/manifest.go        RunHeader, FileEntry, Manifest writer, LoadImported, Key
internal/picb/plan.go            Item, BuildPlan, NextCounter
internal/picb/copy.go            CopyVerify
internal/picb/run.go             Run, Summary
internal/picb/*_test.go          one test file per source file
```

---

### Task 1: Module and config

**Files:**
- Create: `go.mod`, `internal/picb/config.go`, `internal/picb/config_test.go`

**Interfaces:**
- Produces:
  - `type Config struct { Source, Destination, DateFormat string; IncludeTime, UseEXIF, SplitRawJPEG bool; Workers int }` with yaml tags `source, destination, date_format, include_time, use_exif, split_raw_jpeg, workers`.
  - `func DefaultConfigPath() (string, error)` → `$HOME/.pic-bridge/config.yaml`
  - `func LoadConfig(path string) (Config, error)` — applies defaults, expands leading `~/` in `Source`/`Destination`, makes `Destination` absolute, validates.
  - `var ErrNoConfig = errors.New(...)` wrapped when file missing.
  - `const SampleConfig string` — the YAML block from the spec's Configuration section.
  - `func ValidateFolderName(name string) error`
  - `func (c Config) dateLayout() string` (Go layout from `DateFormat`) and `const timeLayout = "150405"`.

- [ ] **Step 1: `go mod init github.com/tarciosaraiva/pic-bridge`; `go get go.yaml.in/yaml/v3@v3.0.6`**

- [ ] **Step 2: Write failing tests** in `config_test.go`:
  - `TestLoadConfigDefaults`: file containing only `destination: ~/Pictures/Imports` → `Source=="auto"`, `DateFormat=="YYYYMMDD"`, `UseEXIF==true`, `IncludeTime==false`, `SplitRawJPEG==false`, `Workers==3`, `Destination==filepath.Join(home,"Pictures/Imports")` (set `HOME` via `t.Setenv`).
  - `TestLoadConfigExplicitFalse`: `use_exif: false` stays false (defaults must not clobber explicit values — use pointer or pre-filled struct before unmarshal).
  - `TestLoadConfigMissingDestination` → error.
  - `TestLoadConfigMissingFile` → `errors.Is(err, ErrNoConfig)`.
  - `TestDateLayout`: table `YYYYMMDD→"20060102"`, `YYYY-MM-DD→"2006-01-02"`, `DD.MM.YYYY→"02.01.2006"`.
  - `TestLoadConfigRejectsTimeTokens`: `date_format: YYYYMMDD_HHmm` → error.
  - `TestValidateFolderName`: ok `wedding`, `2026 trip`; error `""`, `.`, `..`, `a/b`.

- [ ] **Step 3: Run** `go test ./internal/picb/ -run Config -v` — FAIL (undefined).

- [ ] **Step 4: Implement `config.go`.** Token translation via `strings.NewReplacer("YYYY","2006","MM","01","DD","02")`; reject if `DateFormat` contains `HH`, `mm` or `ss`. Unmarshal into a `Config` pre-filled with defaults so explicit `false` survives.

- [ ] **Step 5: Run** `go test ./internal/picb/ -v` — PASS.

- [ ] **Step 6: Commit (only on user instruction)** `feat: config loading and validation`

---

### Task 2: Card detection and scan

**Files:**
- Create: `internal/picb/scan.go`, `internal/picb/scan_test.go`

**Interfaces:**
- Produces:
  - `type Class int` with `ClassRAW, ClassJPEG, ClassVideo, ClassSidecar`.
  - `type SourceFile struct { Rel string /* slash-separated, relative to card root */; Abs string; Size int64; ModTime time.Time; Class Class }`
  - `func FindCardRoot(volumesDir string) (string, error)` — returns the single child of `volumesDir` containing a `DCIM` dir; error naming candidates if 0 or >1 (my call: error, tell user to set `source`).
  - `func Scan(root string) (files []SourceFile, ignored []string, err error)` — walks `DCIM/` and `PRIVATE/M4ROOT/CLIP/` (each optional), sorted by `Rel`.

- [ ] **Step 1: Write failing tests** (build fake cards in `t.TempDir()`):
  - `TestFindCardRoot`: one volume with `DCIM` + one without → returns the one with. Two with `DCIM` → error. None → error.
  - `TestScanClassifies`: files `DCIM/100MSDCF/DSC00001.ARW`, `DSC00001.JPG`, `DSC00002.CR3`, `DCIM/100CANON/MVI_0001.MOV`, `PRIVATE/M4ROOT/CLIP/C0001.MP4`, `C0001M01.XML`, `DCIM/100MSDCF/notes.txt` → classes RAW, JPEG, RAW, Video, Video, Sidecar; `ignored == ["DCIM/100MSDCF/notes.txt"]`.
  - `TestScanSidecarWithoutVideoIgnored`: `PRIVATE/M4ROOT/CLIP/C0009M01.XML` with no `C0009.MP4` → in `ignored`.
  - `TestScanSkipsHidden`: `DCIM/100MSDCF/._DSC00001.ARW` (macOS resource fork) not in files or ignored.
  - `TestScanCaseInsensitive`: `dsc00003.arw`, `C0002.Mp4` classified RAW / Video.
  - `TestScanNoDCIMOrClip`: empty root → no files, no error.

- [ ] **Step 2: Run** `go test ./internal/picb/ -run 'Scan|FindCard' -v` — FAIL.

- [ ] **Step 3: Implement `scan.go`** with `filepath.WalkDir`; extension via `strings.ToUpper(filepath.Ext)`. Sidecar rule: `XML` whose base (minus ext) is `<videoBase>M01` (case-insensitive) for a video in the same dir. Skip names starting with `.`.

- [ ] **Step 4: Run** — PASS.

- [ ] **Step 5: Commit (only on user instruction)** `feat: card detection and file scan`

---

### Task 3: Capture dates

**Files:**
- Create: `internal/picb/dates.go`, `internal/picb/dates_test.go`

**Interfaces:**
- Consumes: `SourceFile`, `ClassRAW/JPEG/Video` (Task 2).
- Produces:
  - `type DateSource string` with `DateEXIF="exif"`, `DateXML="xml"`, `DateMvhd="mvhd"`, `DateMtime="mtime"`.
  - `func CaptureDate(f SourceFile, sidecar *SourceFile, useMeta bool) (time.Time, DateSource)` — always returns local time; falls back to `f.ModTime` + `DateMtime`. Never errors.
  - `func parseMvhd(r io.ReadSeeker) (time.Time, error)` and `func parseSonyXML(r io.Reader) (time.Time, error)` (unexported, tested directly).

- [ ] **Step 1: `go get github.com/evanoberholster/imagemeta@v1.1.0`; check `go doc github.com/evanoberholster/imagemeta/meta/exif Exif` for the `DateTimeOriginal` type and any offset field.**

- [ ] **Step 2: Write failing tests:**
  - `TestParseMvhdV0`: build bytes `ftyp` box (16 bytes) + `moov` box containing `mvhd` v0 with creation time = seconds since 1904-01-01 UTC for `2026-11-07T03:15:42Z` → returns that instant.
  - `TestParseMvhdV1`: same with version 1 (64-bit times).
  - `TestParseMvhdMissing`: `ftyp` only → error.
  - `TestParseSonyXML`: `<NonRealTimeMeta><CreationDate value="2026-11-07T14:15:42+11:00"/></NonRealTimeMeta>` → `2026-11-07T03:15:42Z` instant.
  - `TestCaptureDateFallsBackToMtime`: JPEG file with garbage bytes → `(ModTime, DateMtime)`.
  - `TestCaptureDateUseMetaFalse`: valid sidecar present but `useMeta=false` → `DateMtime`.
  - `TestCaptureDateVideoPrefersXML`: video file with valid `mvhd` + sidecar with different valid time → sidecar time, `DateXML`.
  - `TestCaptureDateEXIFSample`: skipped unless `PICB_SAMPLE_DIR` set; for each `.ARW/.JPG/.CR3` there, asserts `DateEXIF` and year ≥ 2000. (Run manually against the user's card samples — spec requires this early.)

- [ ] **Step 3: Run** `go test ./internal/picb/ -run 'Mvhd|SonyXML|CaptureDate' -v` — FAIL.

- [ ] **Step 4: Implement `dates.go`.** `parseMvhd`: iterate top-level boxes (32-bit size, 64-bit if size==1) to `moov`, then child boxes to `mvhd`; v0 reads uint32 creation at offset 4 after version/flags, v1 uint64; epoch `time.Date(1904,1,1,0,0,0,0,time.UTC)`. EXIF: `imagemeta.Decode`; if `OffsetTimeOriginal` available use it, else interpret wall clock in `time.Local`. Return `.In(time.Local)` for every source.

- [ ] **Step 5: Run** — PASS. Then ask the user for sample files and run `PICB_SAMPLE_DIR=<dir> go test ./internal/picb/ -run EXIFSample -v`; report result before moving on.

- [ ] **Step 6: Commit (only on user instruction)** `feat: capture date from exif, sony xml, mvhd`

---

### Task 4: Manifest

**Files:**
- Create: `internal/picb/manifest.go`, `internal/picb/manifest_test.go`

**Interfaces:**
- Consumes: `Config` (Task 1), `DateSource` (Task 3).
- Produces:
  - `type RunHeader struct { Kind string \`yaml:"kind"\`; StartedAt time.Time \`yaml:"started_at"\`; Source, Folder, PicbVersion string; Config Config }` (yaml keys per spec: `source, folder, picb_version, config`).
  - `type FileEntry struct { Kind string; Source string; Size int64; Modified time.Time; Destination, SHA256 string; DateSource DateSource; CopiedAt time.Time }` (keys `kind, source, size, modified, destination, sha256, date_source, copied_at`).
  - `type Manifest struct` with `func CreateManifest(manifestsDir string, h RunHeader) (*Manifest, error)`, `func (m *Manifest) Append(es ...FileEntry) error` (goroutine-safe, mutex; all entries in one write + fsync), `func (m *Manifest) Close() error`, `func (m *Manifest) Path() string`.
  - `type Key struct { Source string; Size int64; ModUnix int64 }`; `func KeyOf(f SourceFile) Key` (seconds precision).
  - `func LoadImported(manifestsDir string) (map[Key]bool, error)` — missing dir → empty map, nil. Ignores `kind: run` and `kind: summary` docs.
  - `type Timing struct { Phase string \`yaml:"phase"\`; Seconds float64 \`yaml:"seconds"\`; Note string \`yaml:"note,omitempty"\` }`
  - `type SummaryDoc struct { Kind string; FinishedAt time.Time; Copied, Skipped, Failed int; Bytes int64; Workers int; Timings []Timing }` (keys `kind: summary, finished_at, copied, skipped, failed, bytes, workers, timings`).
  - `func (m *Manifest) Finish(s SummaryDoc) error` — appends the summary doc (sets `Kind="summary"`), syncs, closes.

- [ ] **Step 1: Write failing tests:**
  - `TestManifestRoundTrip`: create, append 2 entries, close; `LoadImported` contains both keys.
  - `TestManifestFinish`: append 1 entry, `Finish` with 2 timings → last doc decodes as `kind: summary` with both timings; `LoadImported` still returns exactly 1 key.
  - `TestManifestHeaderFirst`: file starts with `---\nkind: run\n`.
  - `TestLoadImportedTruncatedTail`: write a manifest, append garbage `---\nkind: file\nsource: DCIM/x` (no newline, broken) → earlier entries still loaded, no error.
  - `TestLoadImportedMultipleManifests`: two files → union.
  - `TestLoadImportedMissingDir` → empty, nil.
  - `TestManifestNameFromStart`: `StartedAt` 2026-11-07 14:32:05 local → `Path()` ends `20261107_143205.yaml`; second manifest with same second → does not overwrite (append `_2`, open with `O_EXCL`).

- [ ] **Step 2: Run** `go test ./internal/picb/ -run 'Manifest|LoadImported' -v` — FAIL.

- [ ] **Step 3: Implement `manifest.go`.** Each document written as `"---\n" + yaml.Marshal(v)` then `f.Sync()`. Reader: `yaml.NewDecoder`, loop `Decode` into `map[string]any`-free struct with `Kind`; stop on first error after ≥0 good docs (treat as broken tail); only `kind: file` counts.

- [ ] **Step 4: Run** — PASS.

- [ ] **Step 5: Commit (only on user instruction)** `feat: incremental yaml manifest`

---

### Task 5: Plan — grouping, counters, names

**Files:**
- Create: `internal/picb/plan.go`, `internal/picb/plan_test.go`

**Interfaces:**
- Consumes: `SourceFile`, `Class*` (Task 2); `DateSource`, `CaptureDate` (Task 3); `Config.dateLayout`, `timeLayout` (Task 1); `Key`, `KeyOf` (Task 4).
- Produces:
  - `type Item struct { Src SourceFile; Date time.Time; DateSource DateSource; Counter int; DestRel string /* relative to destination, slash-separated */ }`
  - `type Dater func(f SourceFile, sidecar *SourceFile) (time.Time, DateSource)` — run passes a closure over `CaptureDate`; tests pass fakes.
  - `func NextCounter(originalDir string) (int, error)` — 1 + highest trailing `_NNNN` before the extension across `originalDir`, `RAW/`, `JPEG/`, `VIDEO/`; ignores names starting `.` and ending `.partial`; missing dir → 1.
  - `func BuildPlan(files []SourceFile, imported map[Key]bool, cfg Config, folder string, start int, date Dater) (items []Item, skipped int)`

- [ ] **Step 1: Write failing tests** (pure, fake `Dater` returning fixed times by `Rel`):
  - `TestPlanBasicNames`: two RAWs at 09:00 and 10:00 on 2026-11-07, `start=1` → `wedding/original/20261107_0001.ARW`, `..._0002.ARW`.
  - `TestPlanShootingOrder`: input order reversed → still 09:00 gets `0001`. Same time → tie-break by `Rel`.
  - `TestPlanPairShareCounter`: `DSC01234.ARW` 09:00 + `DSC01234.JPG` 09:01, split on → `original/RAW/20261107_0001.ARW` and `original/JPEG/20261107_0001.JPG` (JPEG uses RAW date).
  - `TestPlanPairsCaseInsensitive`: `DSC01234.ARW` + `dsc01234.jpg` → same counter.
  - `TestPlanPairOnlySameDir`: `100MSDCF/DSC00001.ARW` + `101MSDCF/DSC00001.JPG` → different counters.
  - `TestPlanVideoAndSidecar`: `C0001.MP4` + `C0001M01.XML` → `original/VIDEO/20261107_0001.MP4`, `original/VIDEO/20261107_0001.XML`, regardless of split.
  - `TestPlanIncludeTime`: 14:32:05 → `20261107_143205_0001.ARW`.
  - `TestPlanMultiDay`: 23:59 on 7th and 00:01 on 8th → `20261107_0001`, `20261108_0002`, same folder.
  - `TestPlanStartCounter`: `start=42` → first is `_0042`.
  - `TestPlanCounterPast9999`: `start=10000` → `_10000`.
  - `TestPlanSkipsImported`: one file's `KeyOf` in `imported` → not in items, `skipped==1`, others numbered from `start` with no gap.
  - `TestPlanSkipsPairMemberIndividually`: ARW imported, JPG not → JPG planned (gets its own new counter).
  - `TestPlanNoSplit`: RAW and JPEG both directly in `original/`.
  - `TestNextCounter`: dir with `20261107_0007.ARW`, `RAW/20261107_0012.ARW`, `VIDEO/20261107_0003.MP4` → 13; missing dir → 1.
  - `TestNextCounterIgnoresPartial`: `.20261107_0099.ARW.partial` present → ignored.

- [ ] **Step 2: Run** `go test ./internal/picb/ -run 'Plan|NextCounter' -v` — FAIL.

- [ ] **Step 3: Implement `plan.go`.** Filter imported first. Group: key = dir + upper(base) for RAW/JPEG; video + its sidecar (`<base>M01`). Group date = RAW (or video) member's date, else the single member's. Sort groups by (date, primary Rel). Assign `start, start+1, …` per group; every member gets the same name stem with its own extension and subfolder.

- [ ] **Step 4: Run** — PASS.

- [ ] **Step 5: Commit (only on user instruction)** `feat: import plan with counters and pairing`

---

### Task 6: Copy and verify

**Files:**
- Create: `internal/picb/copy.go`, `internal/picb/copy_test.go`

**Interfaces:**
- Produces: `type CopyStats struct { SHA256 string; Bytes int64; Read, Verify time.Duration }`, `func CopyVerify(ctx context.Context, srcPath, dstPath string, modTime time.Time) (CopyStats, error)` and `var ErrExists = errors.New("destination exists")`. `Read` = time spent in the card→temp copy; `Verify` = sync + re-hash of temp.

- [ ] **Step 1: Write failing tests:**
  - `TestCopyVerifyOK`: 1 MB random file → dst bytes equal, `stats.SHA256` equals `sha256` of source, `stats.Bytes == 1<<20`, `stats.Read > 0`, `stats.Verify > 0`, dst mtime == `modTime` (second precision), no `.partial` in dir.
  - `TestCopyVerifyRefusesExisting`: dst exists with content "keep" → `errors.Is(err, ErrExists)`, content still "keep", no `.partial`.
  - `TestCopyVerifyIgnoresStalePartial`: `.<name>.partial` already present → success, stale file untouched (Run cleans those).
  - `TestCopyVerifySourceError`: src path missing → error, no dst, no `.partial`.
  - `TestCopyVerifyCancelled`: cancelled ctx → error, no dst, no `.partial`.

- [ ] **Step 2: Run** `go test ./internal/picb/ -run CopyVerify -v` — FAIL.

- [ ] **Step 3: Implement `copy.go`.** Steps exactly as spec "Copy and verify": temp `filepath.Join(dir, "."+base+".partial")` opened `O_CREATE|O_TRUNC|O_WRONLY`; `io.Copy` through a ctx-checking reader into `io.MultiWriter(tmp, hasher)`; `Sync`; re-open temp, hash, compare; `os.Chtimes` on temp; `os.Link(tmp, dst)` (map `os.ErrExist` → `ErrExists`); remove temp. `defer` removes temp on every error path.

- [ ] **Step 4: Run** — PASS.

- [ ] **Step 5: Commit (only on user instruction)** `feat: verified copy without overwrite`

---

### Task 7: Run orchestration and CLI

**Files:**
- Create: `internal/picb/run.go`, `internal/picb/run_test.go`, `cmd/picb/main.go`

**Interfaces:**
- Consumes: everything above.
- Produces:
  - `type Summary struct { Copied, Skipped int; Bytes int64; Failed map[string]string /* Rel → reason */; Ignored []string; MtimeFallback []string; ManifestPath string; Timings []Timing }`
  - Timing phases, in this order: `scan`, `manifests`, `dates`, `plan`, `copy`, `copy.read`, `copy.verify`, `total`. `copy.read`/`copy.verify` are sums of `CopyStats` across workers (note says "summed across workers"); `copy` note carries files, bytes and MB/s (`Bytes / copy seconds / 1e6`); `dates` note carries counts per `DateSource`.
  - `func Run(ctx context.Context, cfg Config, folder string, out io.Writer) (Summary, error)` — `error` only for setup failures (no card, destination unwritable); per-file failures go in `Summary.Failed`.
  - `const Version = "0.1.0"` in `run.go`.

- [ ] **Step 1: Write failing tests** (fake card in temp dir; `cfg.Source` = that path; `cfg.UseEXIF=false` so dates come from mtimes set with `os.Chtimes`):
  - `TestRunHappyPath`: 2 ARW + 1 JPG pair + 1 MP4/XML → files at expected `DestRel`s, byte-equal, `processed/` exists and empty, manifest has 1 run + 5 file docs, `Copied==5`.
  - `TestRunEmptyCard`: card with empty `DCIM` → `original/`, `processed/` and manifest (header only) exist, `Copied==0`, nil error.
  - `TestRunSkipsSecondTime`: run twice same folder → second `Copied==0`, `Skipped==5`, no new files.
  - `TestRunContinuesCounter`: second run with 1 new card file → gets `_0006`.
  - `TestRunReportsFailure`: one source file `chmod 000` → in `Failed`, others copied, not in manifest.
  - `TestRunCancel`: cancel ctx after first manifest append (hook via small files + cancel from a goroutine watching the manifest, or cancel before start) → no `.partial` anywhere under destination, manifest decodes with `LoadImported`, returns Summary with `Copied < total`.
  - `TestRunProcessedUntouched`: pre-existing `processed/edit.tif` survives.
  - `TestRunTimings`: after happy path, `Summary.Timings` phases equal the ordered list above; last manifest doc is `kind: summary` with `copied: 5`, `workers` == `cfg.Workers`, same phases.

- [ ] **Step 2: Run** `go test ./internal/picb/ -run Run -v` — FAIL.

- [ ] **Step 3: Implement `Run`:** resolve source (`auto` → `FindCardRoot("/Volumes")`); `MkdirAll` `original/`, `processed/`, manifests dir; remove leftover `.*.partial` files in `original/` and its `RAW/`, `JPEG/`, `VIDEO/` (crash leftovers; CopyVerify temps are uniquely named); timed `Scan`; timed `LoadImported`; timed dates phase = `CaptureDate` for every scanned file **not** in `imported` (so skipped files are never opened), stored in a map that the `Dater` passed to `BuildPlan` reads from; `NextCounter` + timed `BuildPlan`; print plan summary (photos, videos, total size, destination); `CreateManifest`; `MkdirAll` needed subfolders; timed copy: feed groups (items sharing `Item.Counter`) to `cfg.Workers` goroutines over a channel; each calls `CopyVerify` for every member, then one `Manifest.Append(entries...)` only if all succeeded (any failure → whole group in `Failed`, nothing appended) and adds its `CopyStats` to shared totals (mutex); stop feeding on `ctx.Done()`; wait; `Manifest.Finish(SummaryDoc{…})` — also on cancel; print summary (copied, skipped, failed with reasons, ignored, mtime fallbacks, manifest path) then the timings table (phase, seconds to 3 decimals, note). Progress: one line per completed file `[n/total] DestRel`.

- [ ] **Step 4: Implement `cmd/picb/main.go`:** `flag.String("config", "", …)`; exactly one positional arg else usage + exit 2; `ValidateFolderName`; `LoadConfig` (on `ErrNoConfig`: if stdin is a terminal, ask `No config at <path>. Create it with defaults? [y/N]`; on `y` write `SampleConfig` to that path (creating `~/.pic-bridge/`, file mode 0644, never overwriting), print the path and tell the user to check `destination` and re-run, exit 0; on anything else, or when stdin is not a terminal, print expected path + `SampleConfig`, exit 1). Add `func WriteSampleConfig(path string) error` to `config.go` with test `TestWriteSampleConfig` (creates parent dir; result loads via `LoadConfig`; second call errors, file unchanged); `signal.NotifyContext(ctx, os.Interrupt)`; `Run`; exit 1 if setup error or `len(Failed)>0`, else 0.

- [ ] **Step 5: Run** `go test ./... -v` — PASS. `go vet ./...` — clean. `go build -o picb ./cmd/picb && ./picb` — prints usage, exit 2.

- [ ] **Step 6: Manual check with the user's card** (ask before running): `./picb test-import` with a scratch `destination`; confirm layout, `processed/`, manifest with closing summary; run again → all skipped.

- [ ] **Step 7: Timed worker check** (ask before running): same card, fresh scratch destination each time, `workers` = 1, 2, 3, 4. Record `copy` seconds, MB/s, `copy.read`, `copy.verify` from each run's timings in a table for the user. Set the `workers` default in `config.go` (and `TestLoadConfigDefaults`, README, spec) to the smallest count within ~5% of the best MB/s.

- [ ] **Step 8: Commit (only on user instruction)** `feat: picb import command`
