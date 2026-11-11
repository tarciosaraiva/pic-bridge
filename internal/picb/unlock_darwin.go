package picb

import (
	"io/fs"
	"path/filepath"
	"syscall"
)

// unlock clears the file flags of everything under p. macOS shows the FAT
// read-only attribute (Sony sets it on PRIVATE/SONY/SONYCARD.IND) as the uchg
// flag, and a flagged file cannot be removed. Errors are left to the caller's
// retry to report.
func unlock(p string) {
	filepath.WalkDir(p, func(q string, _ fs.DirEntry, err error) error {
		if err == nil {
			syscall.Chflags(q, 0)
		}
		return nil
	})
}
