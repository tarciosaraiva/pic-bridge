//go:build darwin

package picb

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// diskutilInfo is `diskutil info -plist <path>`; a test hook.
var diskutilInfo = func(path string) ([]byte, error) {
	return exec.Command("diskutil", "info", "-plist", path).Output()
}

// externalVolumes lists the volumes under /Volumes that are external hard
// drives: not internal, not removable media (SD cards), not SD or a disk
// image.
func externalVolumes() ([]string, error) {
	entries, err := os.ReadDir(volumesDir)
	if err != nil {
		return nil, err
	}
	var vols []string
	for _, e := range entries {
		vol := filepath.Join(volumesDir, e.Name())
		out, err := diskutilInfo(vol)
		if err != nil {
			continue // not a disk, e.g. a network share
		}
		if info, err := parseDiskInfo(bytes.NewReader(out)); err == nil && info.external() {
			vols = append(vols, vol)
		}
	}
	return vols, nil
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
