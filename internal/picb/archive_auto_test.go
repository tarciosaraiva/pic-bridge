package picb

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestParseDiskInfo(t *testing.T) {
	for file, want := range map[string]bool{
		"usb-hard-drive.plist":       true,
		"sd-card-builtin-slot.plist": false,
		"macintosh-hd.plist":         false, // real output, nested dicts
	} {
		f, err := os.Open(filepath.Join("testdata", "diskutil", file))
		if err != nil {
			t.Fatal(err)
		}
		info, err := parseDiskInfo(f)
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if info.Internal == nil || info.Removable == nil || info.Bus == nil {
			t.Errorf("%s: missing keys: %+v", file, info)
		}
		if got := info.external(); got != want {
			t.Errorf("%s: external = %v, want %v", file, got, want)
		}
	}
	// A plist without the keys must never count as external.
	if info, err := parseDiskInfo(strings.NewReader(`<plist><dict><key>X</key><array><string>a</string></array></dict></plist>`)); err != nil || info.external() {
		t.Errorf("empty plist: %+v %v", info, err)
	}
}

func TestArchiveDrives(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	vols := t.TempDir()
	for _, v := range []string{"Backup", "Second", "Card", "Macintosh HD", "Share", "SameAsDest"} {
		os.Mkdir(filepath.Join(vols, v), 0o755)
	}
	fixture := map[string]string{
		"Backup": "usb-hard-drive", "Second": "usb-hard-drive", "SameAsDest": "usb-hard-drive",
		"Card": "sd-card-builtin-slot", "Macintosh HD": "macintosh-hd",
	}
	devs := map[string]uint64{"Backup": 10, "Second": 11, "SameAsDest": 1}
	oldVols, oldInfo, oldDev := volumesDir, diskutilInfo, deviceID
	t.Cleanup(func() { volumesDir, diskutilInfo, deviceID = oldVols, oldInfo, oldDev })
	volumesDir = vols
	diskutilInfo = func(p string) ([]byte, error) {
		name, ok := fixture[filepath.Base(p)]
		if !ok {
			return nil, errors.New("could not find disk") // network share
		}
		return os.ReadFile(filepath.Join("testdata", "diskutil", name+".plist"))
	}
	deviceID = func(p string) (uint64, error) {
		if d, ok := devs[filepath.Base(p)]; ok {
			return d, nil
		}
		return 1, nil // the destination's drive
	}
	a := &Archive{Src: t.TempDir(), devs: map[uint64]bool{11: true}} // a target already on Second
	got, err := a.ArchiveDrives()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Join(vols, "Backup", "pic-bridge")}; !slices.Equal(got, want) {
		t.Errorf("drives = %q, want %q", got, want)
	}
}
