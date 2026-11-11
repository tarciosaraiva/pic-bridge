package picb

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var shotAt = time.Date(2026, 11, 7, 9, 15, 42, 0, time.Local)

func noPartials(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".partial") {
			t.Errorf("leftover %s", e.Name())
		}
	}
}

func srcFile(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "DSC00001.ARW")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCopyVerifyOK(t *testing.T) {
	data := make([]byte, 1<<20)
	rand.Read(data)
	src, dir := srcFile(t, data), t.TempDir()
	dst := filepath.Join(dir, "20261107_0001.ARW")
	st, err := CopyVerify(context.Background(), src, dst, shotAt, false)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(data) {
		t.Error("dst bytes differ")
	}
	sum := sha256.Sum256(data)
	if st.SHA256 != hex.EncodeToString(sum[:]) || st.Bytes != 1<<20 || st.Read <= 0 || st.Verify <= 0 {
		t.Errorf("stats = %+v", st)
	}
	if fi, _ := os.Stat(dst); fi.ModTime().Unix() != shotAt.Unix() {
		t.Errorf("mtime = %v, want %v", fi.ModTime(), shotAt)
	}
	noPartials(t, dir)
}

func TestCopyVerifyRefusesExisting(t *testing.T) {
	src, dir := srcFile(t, []byte("new")), t.TempDir()
	dst := filepath.Join(dir, "20261107_0001.ARW")
	os.WriteFile(dst, []byte("keep"), 0o644)
	if _, err := CopyVerify(context.Background(), src, dst, shotAt, false); !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "keep" {
		t.Errorf("dst overwritten: %q", got)
	}
	noPartials(t, dir)
}

func TestCopyVerifyIgnoresStalePartial(t *testing.T) {
	src, dir := srcFile(t, []byte("fresh")), t.TempDir()
	dst := filepath.Join(dir, "20261107_0001.ARW")
	stale := filepath.Join(dir, ".20261107_0001.ARW.partial")
	os.WriteFile(stale, []byte("stale from a crash"), 0o644)
	if _, err := CopyVerify(context.Background(), src, dst, shotAt, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "fresh" {
		t.Errorf("dst = %q", got)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*.partial")); len(m) != 1 || m[0] != stale {
		t.Errorf("partials = %v, want only the stale one (run cleans those)", m)
	}
}

func TestCopyVerifyConcurrentSameDst(t *testing.T) {
	a, b := make([]byte, 256<<10), make([]byte, 256<<10)
	rand.Read(a)
	rand.Read(b)
	srcA, srcB := srcFile(t, a), srcFile(t, b)
	for i := range 30 {
		dir := t.TempDir()
		dst := filepath.Join(dir, "20261107_0001.ARW")
		errs := make(chan error, 2)
		for _, s := range []string{srcA, srcB} {
			go func() { _, err := CopyVerify(context.Background(), s, dst, shotAt, false); errs <- err }()
		}
		e1, e2 := <-errs, <-errs
		if (e1 == nil) == (e2 == nil) || !(errors.Is(e1, ErrExists) || errors.Is(e2, ErrExists)) {
			t.Fatalf("iter %d: errs %v, %v; want one success and one ErrExists", i, e1, e2)
		}
		if got, _ := os.ReadFile(dst); string(got) != string(a) && string(got) != string(b) {
			t.Fatalf("iter %d: dst is neither source", i)
		}
		noPartials(t, dir)
	}
}

// withLink swaps the link step for one test.
func withLink(t *testing.T, f func(oldname, newname string) error) {
	t.Helper()
	orig := link
	link = f
	t.Cleanup(func() { link = orig })
}

func TestCopyVerifyLinkError(t *testing.T) {
	src, dir := srcFile(t, []byte("x")), t.TempDir()
	dst := filepath.Join(dir, "20261107_0001.ARW")
	withLink(t, func(string, string) error { return errors.New("operation not supported") })
	_, err := CopyVerify(context.Background(), src, dst, shotAt, false)
	if err == nil || errors.Is(err, ErrExists) || !strings.Contains(err.Error(), dst) {
		t.Fatalf("err = %v, want non-ErrExists error naming dst", err)
	}
	noPartials(t, dir)
}

func TestCopyVerifyDstAppearsBeforeLink(t *testing.T) {
	src, dir := srcFile(t, []byte("new")), t.TempDir()
	dst := filepath.Join(dir, "20261107_0001.ARW")
	withLink(t, func(o, n string) error {
		os.WriteFile(n, []byte("keep"), 0o644) // another writer wins the race
		return os.Link(o, n)
	})
	_, err := CopyVerify(context.Background(), src, dst, shotAt, false)
	if !errors.Is(err, ErrExists) || !strings.Contains(err.Error(), dst) {
		t.Fatalf("err = %v, want ErrExists naming dst", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "keep" {
		t.Errorf("dst overwritten: %q", got)
	}
	noPartials(t, dir)
}

func TestCopyVerifyChecksumMismatch(t *testing.T) {
	src, dir := srcFile(t, []byte("x")), t.TempDir()
	dst := filepath.Join(dir, "20261107_0001.ARW")
	orig := rehash
	rehash = func(string) (string, error) { return "deadbeef", nil } // disk returned other bytes
	t.Cleanup(func() { rehash = orig })
	_, err := CopyVerify(context.Background(), src, dst, shotAt, false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") || !strings.Contains(err.Error(), dst) {
		t.Fatalf("err = %v, want checksum mismatch naming dst", err)
	}
	if _, err := os.Stat(dst); err == nil {
		t.Error("dst created despite mismatch")
	}
	noPartials(t, dir)
}

func TestCopyVerifyDirSyncError(t *testing.T) {
	src, dir := srcFile(t, []byte("x")), t.TempDir()
	dst := filepath.Join(dir, "20261107_0001.ARW")
	orig := syncDir
	syncDir = func(string) error { return errors.New("sync failed") }
	t.Cleanup(func() { syncDir = orig })
	if _, err := CopyVerify(context.Background(), src, dst, shotAt, false); err == nil || !strings.Contains(err.Error(), dst) {
		t.Fatalf("err = %v, want error naming dst", err)
	}
	noPartials(t, dir)
}

// Only checks fcntl accepts the flag; not that caching is off.
func TestNoCacheFcntlAccepted(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := noCache(f); err != nil {
		t.Fatalf("noCache: %v", err)
	}
}

func TestCopyVerifySourceError(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "20261107_0001.ARW")
	if _, err := CopyVerify(context.Background(), filepath.Join(dir, "missing"), dst, shotAt, false); err == nil {
		t.Fatal("want error for missing source")
	}
	if _, err := os.Stat(dst); err == nil {
		t.Error("dst created")
	}
	noPartials(t, dir)
}

func TestCopyVerifyCancelled(t *testing.T) {
	src, dir := srcFile(t, make([]byte, 1<<20)), t.TempDir()
	dst := filepath.Join(dir, "20261107_0001.ARW")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CopyVerify(ctx, src, dst, shotAt, false); err == nil {
		t.Fatal("want error on cancelled context")
	}
	if _, err := os.Stat(dst); err == nil {
		t.Error("dst created")
	}
	noPartials(t, dir)
}

// badCardReread makes every re-read of src return a wrong hash and counts them.
func badCardReread(t *testing.T, src string) *int {
	t.Helper()
	n := 0
	old := rehash
	t.Cleanup(func() { rehash = old })
	rehash = func(p string) (string, error) {
		if p == src {
			n++
			return "bad", nil
		}
		return old(p)
	}
	return &n
}

func TestCopyVerifySourceRereadMismatch(t *testing.T) {
	src := srcFile(t, []byte("card bytes"))
	dst := filepath.Join(t.TempDir(), "out.ARW")
	badCardReread(t, src)
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
	src := srcFile(t, []byte("card bytes"))
	dst := filepath.Join(t.TempDir(), "out.ARW")
	n := badCardReread(t, src)
	if _, err := CopyVerify(context.Background(), src, dst, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if *n != 0 {
		t.Fatalf("card re-read %d times with verifySrc off", *n)
	}
}
