package picb

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Sony marks PRIVATE/SONY/SONYCARD.IND read-only on the card; macOS shows
// that as the uchg flag, and unlink then fails with "operation not permitted".
func TestEraseCardClearsImmutableFlag(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "PRIVATE/SONY/SONYCARD.IND", "DCIM/100MSDCF/DSC00001.ARW")
	locked := filepath.Join(root, "PRIVATE", "SONY", "SONYCARD.IND")
	if err := syscall.Chflags(locked, ufImmutable); err != nil {
		t.Skip("file flags not supported here:", err)
	}
	t.Cleanup(func() { syscall.Chflags(locked, 0) }) // let TempDir clean up if the test fails
	if err := os.Remove(locked); err == nil {
		t.Fatal("flag did not lock the file; test proves nothing")
	}
	if fails := EraseCard(root); fails != nil {
		t.Fatal(fails)
	}
	if ents, _ := os.ReadDir(root); len(ents) != 0 {
		t.Fatalf("left %v", ents)
	}
}
