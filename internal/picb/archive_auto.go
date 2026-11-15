package picb

import "path/filepath"

// ArchiveDrives lists the external hard drives the archive could use: those
// externalVolumes finds, minus the destination's drive and any drive a
// target already uses. Each comes back as <mount>/pic-bridge.
//
// externalVolumes is per OS (archive_auto_<os>.go); to support a new OS,
// add a file for it and narrow the build tag in archive_auto_other.go.
func (a *Archive) ArchiveDrives() ([]string, error) {
	vols, err := externalVolumes()
	if err != nil {
		return nil, err
	}
	destDev, err := deviceID(a.Src)
	if err != nil {
		return nil, err
	}
	var drives []string
	for _, vol := range vols {
		if dev, err := deviceID(vol); err != nil || dev == destDev || a.devs[dev] {
			continue
		}
		drives = append(drives, filepath.Join(vol, "pic-bridge"))
	}
	return drives, nil
}
