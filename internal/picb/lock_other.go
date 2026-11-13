//go:build !unix

package picb

// lockDest is a no-op where flock is unavailable.
func lockDest(string) (func(), error) { return func() {}, nil }
