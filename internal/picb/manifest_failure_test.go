package picb

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Each injected fault happens once; later I/O would succeed if attempted.
type manifestFaultFile struct {
	*os.File
	writeLimit            int
	writeErr, syncErr     error
	closeErr              error
	writes, syncs, closes int
}

func (f *manifestFaultFile) Write(p []byte) (int, error) {
	f.writes++
	if f.writeLimit > 0 {
		limit := f.writeLimit
		f.writeLimit = 0
		n, err := f.File.Write(p[:min(limit, len(p))])
		if err != nil {
			return n, err
		}
		return n, f.writeErr
	}
	return f.File.Write(p)
}

func (f *manifestFaultFile) Sync() error {
	f.syncs++
	if f.syncErr != nil {
		err := f.syncErr
		f.syncErr = nil
		return err
	}
	return f.File.Sync()
}

func (f *manifestFaultFile) Close() error {
	f.closes++
	if err := f.File.Close(); err != nil {
		return err
	}
	return f.closeErr
}

func newFaultManifest(t *testing.T, dir string) (*Manifest, *manifestFaultFile) {
	t.Helper()
	m := newManifest(t, dir)
	f := &manifestFaultFile{File: m.f.(*os.File)}
	m.f = f
	t.Cleanup(func() { f.File.Close() })
	return m, f
}

type manifestMarshalError struct{ err error }

func (v manifestMarshalError) MarshalYAML() (any, error) { return nil, v.err }

func TestManifestFirstIOErrorIsSticky(t *testing.T) {
	writeErr := errors.New("write failed")
	syncErr := errors.New("sync failed")
	const short = len("---\nkind: file\nsource: \"DCIM/")
	for _, tc := range []struct {
		name             string
		limit            int
		write, sync, err error
	}{
		{"partial write", short, writeErr, nil, writeErr},
		{"short write", short, nil, nil, io.ErrShortWrite},
		{"sync", 0, nil, syncErr, syncErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			m, f := newFaultManifest(t, dir)
			good := entry("DCIM/good.ARW", 10)
			if err := m.Append(good); err != nil {
				t.Fatal(err)
			}
			f.writeLimit, f.writeErr, f.syncErr = tc.limit, tc.write, tc.sync
			f.closeErr = errors.New("close failed")
			if err := m.Append(entry("DCIM/partial\t.ARW", 20)); err != tc.err {
				t.Errorf("first Append = %v, want %v", err, tc.err)
			}
			wantSyncs := 2
			if tc.limit > 0 {
				wantSyncs = 1
			}
			if f.writes != 2 || f.syncs != wantSyncs {
				t.Errorf("I/O calls = %d writes, %d syncs; want 2, %d", f.writes, f.syncs, wantSyncs)
			}
			before, err := os.ReadFile(m.Path())
			if err != nil {
				t.Fatal(err)
			}
			if tc.limit > 0 {
				dec := yaml.NewDecoder(bytes.NewReader(before))
				for i := 0; i < 2; i++ { // The header and earlier entry are intact.
					var doc FileEntry
					if err := dec.Decode(&doc); err != nil {
						t.Fatal(err)
					}
				}
				var tail FileEntry
				if err := dec.Decode(&tail); err == nil || err == io.EOF {
					t.Fatalf("fault did not leave a broken trailing document: %v", err)
				}
			}
			writes, syncs := f.writes, f.syncs
			errs := make(chan error, 4)
			for i := 0; i < cap(errs); i++ {
				go func() { errs <- m.Append(entry("DCIM/later.ARW", 30)) }()
			}
			for i := 0; i < cap(errs); i++ {
				if err := <-errs; err != tc.err {
					t.Errorf("later Append = %v, want the first error %v", err, tc.err)
				}
			}
			if err := m.write(manifestMarshalError{errors.New("marshal failed")}); err != tc.err {
				t.Errorf("write after failure = %v, want the first error %v", err, tc.err)
			}
			if err := m.Finish(SummaryDoc{Failed: 1}); err != tc.err {
				t.Errorf("Finish = %v, want the first error %v", err, tc.err)
			}
			if f.writes != writes || f.syncs != syncs || f.closes != 1 {
				t.Errorf("I/O calls after failure = %d writes, %d syncs, %d closes; want %d, %d, 1",
					f.writes, f.syncs, f.closes, writes, syncs)
			}
			after, err := os.ReadFile(m.Path())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Error("manifest changed after the first I/O error")
			}
			got, err := LoadImported(dir)
			if err != nil || !got[Key{Source: good.Source, Size: good.Size, ModUnix: good.Modified.Unix()}] {
				t.Errorf("earlier entry was lost: %v, %v", got, err)
			}
		})
	}
}

func TestManifestMarshalErrorDoesNotPoison(t *testing.T) {
	dir := t.TempDir()
	m, f := newFaultManifest(t, dir)
	want := errors.New("marshal failed")
	if err := m.write(manifestMarshalError{want}); err != want {
		t.Fatalf("write = %v, want %v", err, want)
	}
	if f.writes != 0 || f.syncs != 0 {
		t.Fatal("marshal failure performed I/O")
	}
	if err := m.Append(entry("DCIM/good.ARW", 10)); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadImported(dir); err != nil || len(got) != 1 {
		t.Errorf("LoadImported = %v, %v; want 1 entry", got, err)
	}
}

func TestManifestCloseReportsCloseError(t *testing.T) {
	m, f := newFaultManifest(t, t.TempDir())
	writeErr := errors.New("sync failed")
	closeErr := errors.New("close failed")
	f.syncErr, f.closeErr = writeErr, closeErr
	if err := m.Append(entry("DCIM/good.ARW", 10)); err != writeErr {
		t.Fatalf("Append = %v, want %v", err, writeErr)
	}
	if err := m.Close(); err != closeErr {
		t.Errorf("Close = %v, want %v", err, closeErr)
	}
	if f.closes != 1 {
		t.Errorf("Close calls = %d, want 1", f.closes)
	}
}
