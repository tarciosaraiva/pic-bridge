package picb

import (
	"os"
	"syscall"
)

// noCache sets fcntl F_NOCACHE on f, a hint to keep its data out of the page
// cache, so the re-hash reads the disk. Measured on APFS with fs_usage (#27):
// with it on the writer, the re-hash read every copied byte from disk; without
// it, none (served from cache), at no measurable cost in copy time.
// f.Fd() switches f to blocking mode, harmless for regular files.
func noCache(f *os.File) error {
	if _, _, e := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_NOCACHE, 1); e != 0 {
		return e
	}
	return nil
}
