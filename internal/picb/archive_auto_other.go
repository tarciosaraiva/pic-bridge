//go:build !darwin

package picb

import "errors"

func externalVolumes() ([]string, error) {
	return nil, errors.New("auto archive target is macOS only")
}
