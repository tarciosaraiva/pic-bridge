//go:build unix

package picb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockDest takes an exclusive, non-blocking lock on <dest>/.pic-bridge/lock so
// two runs cannot share a destination. The kernel releases it if picb dies.
// The file is never deleted: unlinking a lock file lets two runs lock
// different inodes at the same path.
func lockDest(dest string) (unlock func(), err error) {
	p := filepath.Join(dest, ".pic-bridge", "lock")
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("another picb run is importing into %s; wait for it to finish", dest)
		}
		return nil, fmt.Errorf("lock %s: %w", p, err)
	}
	return func() { f.Close() }, nil
}
