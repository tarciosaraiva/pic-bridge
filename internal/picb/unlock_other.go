//go:build !darwin

package picb

// unlock is only needed on macOS; see unlock_darwin.go.
func unlock(string) {}
