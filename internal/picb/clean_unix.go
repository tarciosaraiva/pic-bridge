//go:build unix

package picb

import (
	"errors"
	"os"
	"syscall"
)

// deviceOf is the ID of the device holding path (symlinks followed).
func deviceOf(path string) (uint64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("not supported on this OS")
	}
	return uint64(st.Dev), nil // Dev is int32 on darwin, uint64 on linux
}
