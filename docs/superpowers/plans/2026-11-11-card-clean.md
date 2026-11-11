# Card Clean Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `picb --clean <folder>` imports, re-verifies every card file against the manifests and destination, asks once, then erases the card's contents; plus a `verify_source` config option for normal imports.

**Architecture:** Run stays as is but returns the resolved source and scanned files in `Summary`. A new `clean.go` holds the gate (`CheckClean`) and the erase (`EraseCard`); device-ID lookup is build-tagged (`clean_unix.go` / `clean_other.go`) behind a test hook. `cmd/picb` owns the flag, the refusal rules, the prompt and exit codes in a testable `cleanCard` function.

**Tech Stack:** Go 1.27, standard library only (existing deps: imagemeta, go.yaml.in/yaml/v3).

**Spec:** `docs/superpowers/specs/2026-11-10-card-clean-design.md`

## Global Constraints

- User rule: ask before editing files and before running any command (tests included). Keep each commit small.
- No new dependencies. No external tools (`diskutil`, `mkfs`, `udisksctl`).
- Never erase unless every gate check passes AND a person typed `y`/`Y` at a terminal.
- `--clean` is a flag only; no config key for it.
- `verify_source: false` is the default.
- Unix (macOS, Linux) only for `--clean`; other OSes: error `not supported on this OS`.
- Commit prefixes: `feat:` / `fix:` / `docs:` / `test:` (release-please). End every commit message with `Co-Authored by Claude 🤖`.
- Run tests with `make test` (`go vet ./...` + `go test -race ./...`).

## Review Focus

1. A card file changed since import (same path, new size or mtime) → no manifest entry → blocks. Test in Task 3.
2. A destination file deleted or edited after import → blocks with its path. Test in Task 3.
3. Ctrl-C after every copy finished but before the prompt → no prompt, no erase. Test in Task 5.
4. Destination configured on the card itself, or source `/` → blocks. Test in Task 3.
5. One entry on the card cannot be removed → the rest are still removed, the failure is listed, exit 1. Test in Task 4.

---

### Task 1: `verify_source` config and copy-time card re-read

**Files:**
- Modify: `internal/picb/config.go` (Config struct, SampleConfig)
- Modify: `internal/picb/copy.go` (CopyVerify)
- Modify: `internal/picb/run.go` (copyGroups call site)
- Test: `internal/picb/config_test.go`, `internal/picb/copy_test.go`

**Interfaces:**
- Produces: `Config.VerifySource bool` (`yaml:"verify_source"`); `CopyVerify(ctx context.Context, src, dst string, modTime time.Time, verifySrc bool) (CopyStats, error)`.

- [ ] **Step 1: Write the failing tests**

```go
// config_test.go
func TestLoadConfigVerifySource(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, "destination: /tmp/x\n"))
	if err != nil || cfg.VerifySource {
		t.Fatalf("default: %v %v", cfg.VerifySource, err)
	}
	cfg, err = LoadConfig(writeConfig(t, "destination: /tmp/x\nverify_source: true\n"))
	if err != nil || !cfg.VerifySource {
		t.Fatalf("explicit: %v %v", cfg.VerifySource, err)
	}
}

// copy_test.go
func TestCopyVerifySourceRereadMismatch(t *testing.T) {
	src := srcFile(t, []byte("card bytes"))
	dst := filepath.Join(t.TempDir(), "out.ARW")
	old := rehash
	t.Cleanup(func() { rehash = old })
	rehash = func(p string) (string, error) {
		if p == src {
			return "bad", nil
		}
		return old(p)
	}
	_, err := CopyVerify(context.Background(), src, dst, time.Now(), true)
	if err == nil || !strings.Contains(err.Error(), "card re-read mismatch") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Lstat(dst); err == nil {
		t.Fatal("dst placed despite mismatch")
	}
	noPartials(t, filepath.Dir(dst))
}

func TestCopyVerifySourceRereadOff(t *testing.T) {
	// same hook as above; verifySrc=false → rehash never called with src, copy succeeds
}
```

Also check `SampleConfig` still loads (existing `TestWriteSampleConfig` covers it once the line is added).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/picb -run 'VerifySource|SourceReread'`
Expected: FAIL (unknown field `verify_source`; too many arguments to `CopyVerify`)

- [ ] **Step 3: Implement**

- Add `VerifySource bool \`yaml:"verify_source"\`` to `Config`; no default needed (false).
- Add to `SampleConfig` after `workers`: `verify_source: false         # default false; true reads each card file twice (slower, catches bad card reads)`
- In `CopyVerify`, after the temp re-hash matches and before `Chtimes`: if `verifySrc`, `rehash(src)`; mismatch → `fmt.Errorf("%s: card re-read mismatch", src)`. Time it inside `st.Verify`.
- `copyGroups` passes `cfg.VerifySource`. Update every existing `CopyVerify` call in tests to pass `false`.

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/picb/config.go internal/picb/copy.go internal/picb/run.go internal/picb/*_test.go
git commit -m "feat: verify_source re-reads each card file before placing it"
```

---

### Task 2: Load full manifest entries

**Files:**
- Modify: `internal/picb/manifest.go` (LoadImported)
- Test: `internal/picb/manifest_test.go`

**Interfaces:**
- Produces: `LoadEntries(dir string) (map[Key][]FileEntry, error)` — every `kind: file` entry across all manifests, grouped by Key, broken trailing document dropped (same rule as today). `LoadImported` becomes a thin wrapper over it (same signature, same behaviour).

- [ ] **Step 1: Write the failing test**

```go
func TestLoadEntriesKeepsShaAndDuplicates(t *testing.T) {
	dir := t.TempDir()
	for _, sha := range []string{"aaa", "bbb"} {
		m := newManifest(t, dir)
		e := entry("DCIM/A.ARW", 10)
		e.SHA256, e.Destination = sha, "w/original/"+sha+".ARW"
		if err := m.Append(e); err != nil {
			t.Fatal(err)
		}
		m.Close()
	}
	got, err := LoadEntries(dir)
	es := got[Key{Source: "DCIM/A.ARW", Size: 10, ModUnix: entry("DCIM/A.ARW", 10).Modified.Unix()}]
	if err != nil || len(es) != 2 || es[0].SHA256 == es[1].SHA256 || es[0].Destination == "" {
		t.Fatalf("got %+v, %v", es, err)
	}
}
```

(`newManifest` must produce distinct file names for two calls in one second; `CreateManifest` already suffixes `_2`.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/picb -run LoadEntries`
Expected: FAIL (`undefined: LoadEntries`)

- [ ] **Step 3: Implement `LoadEntries`; rewrite `LoadImported` as its key set**

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: PASS (existing `LoadImported` tests unchanged)

- [ ] **Step 5: Commit**

```bash
git add internal/picb/manifest.go internal/picb/manifest_test.go
git commit -m "feat: load full manifest entries for the clean gate"
```

---

### Task 3: The gate

**Files:**
- Create: `internal/picb/clean.go`
- Create: `internal/picb/clean_unix.go` (`//go:build unix`)
- Create: `internal/picb/clean_other.go` (`//go:build !unix`)
- Modify: `internal/picb/run.go` (Summary fields)
- Test: `internal/picb/clean_test.go`

**Interfaces:**
- Consumes: `LoadEntries` (Task 2), `hashFile` (copy.go), `Scan`'s `[]SourceFile` / ignored list, `KeyOf`.
- Produces:
  - `Summary.Source string` (resolved card root) and `Summary.Files []SourceFile` (scan result), set by `Run`.
  - `type Blocker struct{ Path, Reason string }`
  - `CheckClean(ctx context.Context, cfg Config, src string, files []SourceFile, ignored []string, out io.Writer) ([]Blocker, error)` — blockers = content problems; error = setup problem (unsupported OS, manifest read error, ctx cancelled).
  - `var deviceID = deviceOf` with `deviceOf(path string) (uint64, error)`: unix reads `syscall.Stat_t.Dev` (convert with `uint64(...)`; type differs darwin/linux); other OSes return `errors.New("not supported on this OS")`.

Gate order and exact reasons (Path is card-relative for file checks, `src` for the volume checks):

| # | Check | Reason |
|---|-------|--------|
| 1 | `filepath.Clean(src) == "/"` | `is the filesystem root` |
| 1 | `deviceID(src) == deviceID(filepath.Dir(src))` | `not a mounted volume` |
| 1 | no `src/DCIM` folder | `no DCIM folder` |
| 2 | `deviceID(cfg.Destination) == deviceID(src)` | `destination is on the card` |
| 3 | each ignored path | `not imported (unknown file)` |
| 4 | no entry for `KeyOf(f)` | `no manifest entry` |
| 5 | `hashFile(f.Abs)` ≠ every entry's `SHA256` | `card re-read does not match the manifest` |
| 6 | for the matching-sha entries, `<cfg.Destination>/<Destination>` missing, or its hash ≠ | `destination <entry.Destination> missing` / `destination <entry.Destination> does not match the manifest` |

Checks 1–2 failing return immediately (no hashing). Checks 3–6 collect every blocker. Hash checks 5–6 run on `cfg.Workers` goroutines; print `Verifying [i/N] <rel>` per file; stop and return `ctx.Err()` when cancelled. Sort blockers by Path.

- [ ] **Step 1: Write the failing tests**

Helper in `clean_test.go`: `fakeDevices(t, card, dest string)` swaps `deviceID` so `card`→1, `filepath.Dir(card)`→2, anything under `dest`→3. Use `fakeCard(t, happyCard)` + `mustRun` to get a real imported destination and manifests.

```go
func TestCheckCleanPasses(t *testing.T)            // happy import → blockers empty, err nil
func TestCheckCleanNotMounted(t *testing.T)        // card and parent same device → [{card, "not a mounted volume"}]
func TestCheckCleanRoot(t *testing.T)              // src "/" → "is the filesystem root"
func TestCheckCleanDestinationOnCard(t *testing.T) // dest device == card device → "destination is on the card"
func TestCheckCleanIgnoredBlocks(t *testing.T)     // add DCIM/100MSDCF/X.DNG → blocker reason "not imported (unknown file)"
func TestCheckCleanCardFileChanged(t *testing.T)   // after import, rewrite DSC00001.ARW with new bytes+mtime → "no manifest entry"
func TestCheckCleanCardBadRead(t *testing.T)       // overwrite a card file with same-length bytes, restore its mtime → "card re-read does not match the manifest"
func TestCheckCleanAfterReRun(t *testing.T)        // mustRun twice on one card; gate on the 2nd Summary (0 copied, all skipped) passes via the 1st manifest
func TestCheckCleanDestMissing(t *testing.T)       // delete wedding/original/20261107_0001.ARW → "destination wedding/original/20261107_0001.ARW missing"
func TestCheckCleanDestEdited(t *testing.T)        // overwrite it → "... does not match the manifest"
func TestCheckCleanCancelled(t *testing.T)         // cancelled ctx → err == context.Canceled
func TestRunSummarySourceAndFiles(t *testing.T)    // mustRun → s.Source == card, len(s.Files) == 5
```

Each test asserts the exact `[]Blocker` (Path and Reason).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/picb -run 'CheckClean|SummarySource'`
Expected: FAIL (`undefined: CheckClean`)

- [ ] **Step 3: Implement** the Summary fields in `Run` (set right after scan), `deviceOf` in both build-tagged files, `Blocker` and `CheckClean` in `clean.go`.

- [ ] **Step 4: Run tests**

Run: `make test` and `GOOS=windows go vet ./...`
Expected: PASS; vet clean (stub compiles)

- [ ] **Step 5: Commit**

```bash
git add internal/picb/clean*.go internal/picb/run.go
git commit -m "feat: clean gate re-verifies every card file"
```

---

### Task 4: Erase the card

**Files:**
- Modify: `internal/picb/clean.go`
- Test: `internal/picb/clean_test.go`

**Interfaces:**
- Produces: `EraseCard(root string) []string` — removes every entry in `root` with `os.RemoveAll`, skipping names starting with `.` at the root only; returns `"<path>: <error>"` for each entry it could not remove (nil when all went). Keeps going after an error.

- [ ] **Step 1: Write the failing tests**

```go
func TestEraseCard(t *testing.T) {
	root := fakeCard(t, happyCard)
	os.WriteFile(filepath.Join(root, "DCIM", ".hidden"), nil, 0o644)
	os.Mkdir(filepath.Join(root, ".Trashes"), 0o755)
	if fails := EraseCard(root); fails != nil {
		t.Fatal(fails)
	}
	ents, _ := os.ReadDir(root)
	if len(ents) != 1 || ents[0].Name() != ".Trashes" {
		t.Fatalf("left %v", ents)
	}
}

func TestEraseCardKeepsGoing(t *testing.T) {
	// root has A/locked.txt with A chmod 0o555, plus DCIM/x and B/y.
	// EraseCard → DCIM and B gone, one failure naming A; t.Cleanup restores 0o755.
	// Skip when os.Getuid() == 0 (root ignores permissions).
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/picb -run EraseCard`
Expected: FAIL (`undefined: EraseCard`)

- [ ] **Step 3: Implement `EraseCard`**

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/picb/clean.go internal/picb/clean_test.go
git commit -m "feat: erase card contents after a passed gate"
```

---

### Task 5: `--clean` flag, refusals, prompt

**Files:**
- Modify: `cmd/picb/main.go`
- Create: `cmd/picb/main_test.go`

**Interfaces:**
- Consumes: `picb.Run`, `Summary.Source/Files/Ignored/Failed/NotAttempted`, `picb.CheckClean`, `picb.EraseCard`, `picb.Blocker`.
- Produces: `cleanCard(ctx context.Context, cfg picb.Config, s picb.Summary, in io.Reader, tty bool, out io.Writer) int` (exit code); `isTerminal() bool` (extracted from `offerConfig`, which then uses it).

Flow in `run()`: parse `--clean` (works before or after the folder, like `--config`); usage line becomes `usage: picb [--config path] [--clean] <folder>`. If clean, set `cfg.VerifySource = false` before `Run` (the gate does the second read). After `Run` succeeds: if clean, `return cleanCard(ctx, cfg, s, os.Stdin, isTerminal(), os.Stdout)`; else today's exit logic.

`cleanCard` exact behaviour and copy:

| Condition | Output | Exit |
|-----------|--------|------|
| `len(s.Failed) > 0 \|\| s.NotAttempted > 0` | `Not cleaning: the import did not finish cleanly.` | 1 |
| `ctx.Err() != nil` | `Not cleaning: stopped.` | 1 |
| `CheckClean` error | `Not cleaning: <err>` | 1 |
| blockers | `Not cleaning the card:` then `  <Path>: <Reason>` per blocker | 1 |
| `!tty` | `Not cleaning: no terminal` | 1 |
| prompt | `All <len(s.Files)> files verified against <cfg.Destination>. Erase everything on card <s.Source>? [y/N] ` | — |
| answer not `y`/`Y` (EOF counts as no) | `Card left as is.` | 0 |
| `ctx.Err()` after answer | `Not cleaning: stopped.` | 1 |
| `EraseCard` failures | `Could not remove:` then `  <line>` each | 1 |
| success | `Card <s.Source> erased. Eject it before removing it.` | 0 |

Refusal lines go to `out` (the caller passes stdout); keep it one writer so tests read one buffer.

- [ ] **Step 1: Write the failing tests** (`package main`; build a real `Summary` via `picb.Run` on a temp card, fake devices by setting the exported test seam below)

Tests need to fake `deviceID` from another package. Add to `internal/picb/clean.go`: `func SetDeviceIDForTest(f func(string) (uint64, error)) (restore func())`. ponytail: exported test seam; move gate tests to an internal package if more seams pile up.

```go
func TestCleanCardRefusesAfterFailure(t *testing.T)  // s.Failed non-empty → exit 1, "Not cleaning: the import did not finish cleanly."
func TestCleanCardRefusesWhenStopped(t *testing.T)   // cancelled ctx, clean Summary → exit 1, "Not cleaning: stopped.", card intact
func TestCleanCardNoTerminal(t *testing.T)           // tty=false → exit 1, card intact
func TestCleanCardAnswerNo(t *testing.T)             // in="n\n" and in="" → exit 0, "Card left as is.", card intact
func TestCleanCardErases(t *testing.T)               // in="y\n" → exit 0, card root has no non-dot entries, prompt text exact
func TestCleanCardBlockersListed(t *testing.T)       // delete one destination file → exit 1, output lists it, card intact
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./cmd/picb`
Expected: FAIL (`undefined: cleanCard`)

- [ ] **Step 3: Implement** `SetDeviceIDForTest`, `isTerminal`, the flag, and `cleanCard`.

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/picb internal/picb/clean.go
git commit -m "feat: --clean verifies and erases the card after import"
```

---

### Task 6: Docs and issues

**Files:**
- Modify: `README.md` (usage: `--clean`; config: `verify_source`)
- Modify: `docs/superpowers/specs/2026-11-07-pic-bridge-design.md` (non-goals lines for #1 and #4 → "Done: see 2026-11-10-card-clean-design.md")
- Modify: `internal/picb/manifest.go` Append comment ("the future card wipe" → "the clean gate")

- [ ] **Step 1: Edit the three files**
- [ ] **Step 2: Manual check on a real card** (user runs it): `picb --clean <folder>` on a scratch import; answer `n` once, then `y`; card mounts empty in the camera.
- [ ] **Step 3: Commit**

```bash
git add README.md docs internal/picb/manifest.go
git commit -m "docs: document --clean and verify_source"
```

- [ ] **Step 4: Issues (ask the user first; outward-facing)**
  - #1: rewrite body to "flag + prompt, all manifests, erase contents"; link the PR.
  - #4: comment that `--clean` covers it during cleaning and `verify_source` covers normal imports; close when merged.
  - New issue labelled `later`, `priority-two`: "Linux support: auto-detect card under /media/$USER and /run/media/$USER; Linux release builds".
