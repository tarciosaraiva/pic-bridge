//go:build unix

package picb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockDestExcludesSecondRun(t *testing.T) {
	dest := t.TempDir()
	os.MkdirAll(filepath.Join(dest, ".pic-bridge"), 0o755)

	unlock, err := lockDest(dest)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	// flock locks belong to the open file, so a second open in this process conflicts.
	if _, err := lockDest(dest); err == nil || !strings.Contains(err.Error(), "another picb run") {
		t.Fatalf("second lock: got %v, want 'another picb run' error", err)
	}
	unlock()

	unlock, err = lockDest(dest)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	unlock()
}
