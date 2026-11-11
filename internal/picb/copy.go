package picb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// ErrExists means the destination name is taken; nothing was overwritten.
var ErrExists = errors.New("destination exists")

// Test hooks for failure paths a real disk won't produce on demand.
var (
	link    = os.Link
	rehash  = hashFile
	syncDir = fsyncDir
)

// CopyStats describes one verified copy. Read is the card→temp copy,
// Verify the sync + re-hash of the temp file.
type CopyStats struct {
	SHA256       string
	Bytes        int64
	Read, Verify time.Duration
}

// CopyVerify copies src to a hidden .partial next to dst while hashing it,
// re-hashes the temp (with a no-cache hint; see noCache), sets its mtime, then links it to dst
// (never overwriting) and fsyncs the folder. The temp is removed on every path.
// With verifySrc the card file is read a second time and must hash the same.
func CopyVerify(ctx context.Context, src, dst string, modTime time.Time, verifySrc bool) (st CopyStats, err error) {
	if _, err := os.Lstat(dst); err == nil {
		return st, fmt.Errorf("%s: %w", dst, ErrExists) // fail before reading the card
	}
	in, err := os.Open(src)
	if err != nil {
		return st, err
	}
	defer in.Close()
	// Keep the card's bytes out of the page cache: a later re-read of src
	// (verifySrc, the clean gate) must come from the card, not from memory.
	if err := noCache(in); err != nil {
		return st, err
	}

	// Unique temp per call: two copies to one dst never share a temp.
	out, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".*.partial")
	if err != nil {
		return st, err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	defer out.Close()
	if err := noCache(out); err != nil {
		return st, err
	}

	t0 := time.Now()
	h := sha256.New()
	if st.Bytes, err = io.Copy(io.MultiWriter(out, h), ctxReader{ctx, in}); err != nil {
		return st, err
	}
	st.Read = time.Since(t0)

	t0 = time.Now()
	if err := out.Sync(); err != nil {
		return st, err
	}
	if err := out.Close(); err != nil {
		return st, err
	}
	st.SHA256 = hex.EncodeToString(h.Sum(nil))
	again, err := rehash(tmp)
	if err != nil {
		return st, err
	}
	if again != st.SHA256 {
		return st, fmt.Errorf("%s: checksum mismatch after copy", dst)
	}
	if verifySrc {
		card, err := rehash(src)
		if err != nil {
			return st, err
		}
		if card != st.SHA256 {
			return st, fmt.Errorf("%s: card re-read mismatch", src)
		}
	}
	st.Verify = time.Since(t0)

	if err := os.Chtimes(tmp, modTime, modTime); err != nil {
		return st, err
	}
	if err := link(tmp, dst); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return st, fmt.Errorf("%s: %w", dst, ErrExists)
		}
		return st, fmt.Errorf("%s: %w", dst, err)
	}
	// Persist the new name: a power cut must not drop a file the manifest calls imported.
	if err := syncDir(filepath.Dir(dst)); err != nil {
		return st, fmt.Errorf("%s: sync folder: %w", dst, err)
	}
	return st, nil
}

// fsyncDir flushes a folder's entries (F_FULLFSYNC on macOS via File.Sync).
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := noCache(f); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ctxReader stops a copy between reads once ctx is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
