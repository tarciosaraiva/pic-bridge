package picb

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// diskutilInfo is `diskutil info -plist <path>`; a test hook.
var diskutilInfo = func(path string) ([]byte, error) {
	return exec.Command("diskutil", "info", "-plist", path).Output()
}

// ArchiveDrives lists the external hard drives under /Volumes that the
// archive could use: not internal, not removable media (SD cards), not SD
// or a disk image, not the destination's drive, and not a drive a target
// already uses. Each comes back as /Volumes/<Name>/pic-bridge.
func (a *Archive) ArchiveDrives() ([]string, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("auto archive target is macOS only")
	}
	entries, err := os.ReadDir(volumesDir)
	if err != nil {
		return nil, err
	}
	destDev, err := deviceID(a.Src)
	if err != nil {
		return nil, err
	}
	var drives []string
	for _, e := range entries {
		vol := filepath.Join(volumesDir, e.Name())
		out, err := diskutilInfo(vol)
		if err != nil {
			continue // not a disk, e.g. a network share
		}
		info, err := parseDiskInfo(bytes.NewReader(out))
		if err != nil || !info.external() {
			continue
		}
		if dev, err := deviceID(vol); err != nil || dev == destDev || a.devs[dev] {
			continue
		}
		drives = append(drives, filepath.Join(vol, "pic-bridge"))
	}
	return drives, nil
}

// diskInfo is the part of diskutil's plist that auto needs. A key missing
// from the plist leaves its pointer nil.
type diskInfo struct {
	Internal, Removable *bool
	Bus                 *string
}

func (d diskInfo) external() bool {
	return d.Internal != nil && !*d.Internal &&
		d.Removable != nil && !*d.Removable &&
		d.Bus != nil && *d.Bus != "Secure Digital" && *d.Bus != "Disk Image"
}

// parseDiskInfo reads the top-level dict of a diskutil plist. Nested
// values are skipped.
func parseDiskInfo(r io.Reader) (diskInfo, error) {
	d := xml.NewDecoder(r)
	for { // find the top-level <dict>
		tok, err := d.Token()
		if err != nil {
			return diskInfo{}, err
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "dict" {
			break
		}
	}
	var info diskInfo
	key := ""
	for {
		tok, err := d.Token()
		if err != nil {
			return diskInfo{}, err
		}
		switch t := tok.(type) {
		case xml.EndElement: // </dict>
			return info, nil
		case xml.StartElement:
			if t.Name.Local == "key" {
				if err := d.DecodeElement(&key, &t); err != nil {
					return diskInfo{}, err
				}
				continue
			}
			b := t.Name.Local == "true"
			switch key {
			case "Internal":
				info.Internal = &b
			case "RemovableMedia":
				info.Removable = &b
			case "BusProtocol":
				var s string
				if err := d.DecodeElement(&s, &t); err != nil {
					return diskInfo{}, err
				}
				info.Bus = &s
				continue
			}
			if err := d.Skip(); err != nil {
				return diskInfo{}, err
			}
		}
	}
}
