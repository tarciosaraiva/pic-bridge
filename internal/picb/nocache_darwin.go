package picb

import (
	"os"
	"syscall"
)

// noCache sets fcntl F_NOCACHE on f, a hint to keep its data out of the page
// cache, so the re-hash is more likely to read the disk. ponytail: unverified
// that it forces a device read on APFS; measure with fs_usage if it matters.
// f.Fd() switches f to blocking mode, harmless for regular files.
func noCache(f *os.File) error {
	if _, _, e := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_NOCACHE, 1); e != 0 {
		return e
	}
	return nil
}
