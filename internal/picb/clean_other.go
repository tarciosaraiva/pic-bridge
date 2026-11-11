//go:build !unix

package picb

import "errors"

func deviceOf(string) (uint64, error) { return 0, errors.New("not supported on this OS") }
