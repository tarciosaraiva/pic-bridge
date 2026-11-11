package picb

import (
	"io/fs"
	"path/filepath"
	"syscall"
)

const ufImmutable = 0x2 // UF_IMMUTABLE, shown as uchg by ls -lO

// unlock clears the uchg flag of everything under p. macOS shows the FAT
// read-only attribute (Sony sets it on PRIVATE/SONY/SONYCARD.IND) as uchg,
// and a flagged file cannot be removed. Only that bit is cleared: the same
// file also carries arch, a flag only root may change, so setting the flags
// to 0 fails as a whole. Errors are left to the caller's retry to report.
func unlock(p string) {
	filepath.WalkDir(p, func(q string, _ fs.DirEntry, err error) error {
		var st syscall.Stat_t
		if err == nil && syscall.Lstat(q, &st) == nil && st.Flags&ufImmutable != 0 {
			syscall.Chflags(q, int(st.Flags&^ufImmutable))
		}
		return nil
	})
}
