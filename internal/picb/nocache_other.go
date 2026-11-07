//go:build !darwin

package picb

import "os"

// ponytail: no-op off macOS; add posix_fadvise(DONTNEED) on Linux if picb runs there.
func noCache(*os.File) error { return nil }
