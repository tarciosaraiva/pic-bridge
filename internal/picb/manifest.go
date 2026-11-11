package picb

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

// RunHeader is the first document of every manifest.
type RunHeader struct {
	Kind        string    `yaml:"kind"`
	StartedAt   time.Time `yaml:"started_at"`
	Source      string    `yaml:"source"`
	Folder      string    `yaml:"folder"`
	PicbVersion string    `yaml:"picb_version"`
	Config      Config    `yaml:"config"`
}

// FileEntry records one verified copy.
type FileEntry struct {
	Kind        string     `yaml:"kind"`
	Source      string     `yaml:"source"`
	Size        int64      `yaml:"size"`
	Modified    time.Time  `yaml:"modified"`
	Destination string     `yaml:"destination"`
	SHA256      string     `yaml:"sha256"`
	DateSource  DateSource `yaml:"date_source"`
	CopiedAt    time.Time  `yaml:"copied_at"`
}

// Timing is one phase of a run.
type Timing struct {
	Phase   string  `yaml:"phase"`
	Seconds float64 `yaml:"seconds"`
	Note    string  `yaml:"note,omitempty"`
}

// SummaryDoc closes a manifest.
type SummaryDoc struct {
	Kind         string    `yaml:"kind"`
	FinishedAt   time.Time `yaml:"finished_at"`
	Copied       int       `yaml:"copied"`
	Skipped      int       `yaml:"skipped"`
	Failed       int       `yaml:"failed"`
	NotAttempted int       `yaml:"not_attempted,omitempty"`
	Bytes        int64     `yaml:"bytes"`
	Workers      int       `yaml:"workers"`
	Timings      []Timing  `yaml:"timings"`
}

// Key identifies a card file from its folder listing alone.
type Key struct {
	Source  string
	Size    int64
	ModUnix int64
}

// KeyOf is f's Key (modified time at second precision).
func KeyOf(f SourceFile) Key { return Key{Source: f.Rel, Size: f.Size, ModUnix: f.ModTime.Unix()} }

// Manifest is an append-only YAML document stream, synced after every document.
type Manifest struct {
	mu sync.Mutex
	f  *os.File
}

// CreateManifest makes <dir>/<YYYYMMDD_HHmmss>.yaml (never reusing a name) and writes h.
func CreateManifest(dir string, h RunHeader) (*Manifest, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	base := h.StartedAt.Local().Format("20060102_150405")
	for i := 1; ; i++ {
		name := base + ".yaml"
		if i > 1 {
			name = fmt.Sprintf("%s_%d.yaml", base, i)
		}
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		m := &Manifest{f: f}
		h.Kind = "run"
		if err := m.write(h); err != nil {
			f.Close()
			return nil, err
		}
		return m, nil
	}
}

// Path is the manifest file's path.
func (m *Manifest) Path() string { return m.f.Name() }

// Append records a group of files in one write + fsync. Call it only after
// every copy is verified and in place at its final name: LoadImported treats
// every entry as imported, and re-runs (and the future card wipe) skip those
// files. Safe for concurrent use.
func (m *Manifest) Append(es ...FileEntry) error {
	docs := make([]any, len(es))
	for i, e := range es {
		e.Kind = "file"
		docs[i] = e
	}
	return m.write(docs...)
}

// Finish appends the summary document and closes the manifest.
func (m *Manifest) Finish(s SummaryDoc) error {
	s.Kind = "summary"
	if err := m.write(s); err != nil {
		m.Close()
		return err
	}
	return m.Close()
}

// Close closes the manifest without a summary.
func (m *Manifest) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.f.Close()
}

func (m *Manifest) write(vs ...any) error {
	var buf []byte
	for _, v := range vs {
		b, err := yaml.Marshal(v)
		if err != nil {
			return err
		}
		buf = append(append(buf, "---\n"...), b...)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.f.Write(buf); err != nil {
		return err
	}
	return m.f.Sync()
}

// LoadImported returns the Key of every file entry in every manifest in dir.
func LoadImported(dir string) (map[Key]bool, error) {
	entries, err := LoadEntries(dir)
	if err != nil {
		return nil, err
	}
	keys := make(map[Key]bool, len(entries))
	for k := range entries {
		keys[k] = true
	}
	return keys, nil
}

// LoadEntries returns every file entry in every manifest in dir, grouped by
// Key (one card file can have several after a re-import).
// A broken trailing document (crash mid-write) is dropped; earlier ones count.
func LoadEntries(dir string) (map[Key][]FileEntry, error) {
	entries := map[Key][]FileEntry{}
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		dec := yaml.NewDecoder(f)
		for {
			var e FileEntry
			if err := dec.Decode(&e); err != nil {
				break // io.EOF, or the broken tail of a crashed run
			}
			if e.Kind == "file" {
				k := Key{Source: e.Source, Size: e.Size, ModUnix: e.Modified.Unix()}
				entries[k] = append(entries[k], e)
			}
		}
		f.Close()
	}
	return entries, nil
}
