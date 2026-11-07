package picb

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Class is what kind of camera file something is.
type Class int

const (
	ClassRAW Class = iota + 1
	ClassJPEG
	ClassVideo
	ClassSidecar
)

// SourceFile is one importable file on the card.
type SourceFile struct {
	Rel     string // slash-separated, relative to card root
	Abs     string
	Size    int64
	ModTime time.Time
	Class   Class
}

var extClass = map[string]Class{
	".ARW": ClassRAW, ".CR2": ClassRAW, ".CR3": ClassRAW,
	".JPG": ClassJPEG, ".JPEG": ClassJPEG,
	".HIF": ClassJPEG, ".HEIF": ClassJPEG, ".HEIC": ClassJPEG, // processed photos, filed with JPEG
	".MP4": ClassVideo, ".MOV": ClassVideo,
}

// scanDirs are the card folders holding photos (DCIM) and Sony video clips.
var scanDirs = []string{"DCIM", "PRIVATE/M4ROOT/CLIP"}

// FindCardRoot returns the single volume under volumesDir that has a DCIM folder.
func FindCardRoot(volumesDir string) (string, error) {
	entries, err := os.ReadDir(volumesDir)
	if err != nil {
		return "", err
	}
	var cards []string
	for _, e := range entries {
		dir := filepath.Join(volumesDir, e.Name())
		if fi, err := os.Stat(filepath.Join(dir, "DCIM")); err == nil && fi.IsDir() {
			cards = append(cards, dir)
		}
	}
	switch len(cards) {
	case 1:
		return cards[0], nil
	case 0:
		return "", fmt.Errorf("no camera card found under %s (no volume with a DCIM folder)", volumesDir)
	}
	return "", fmt.Errorf("several camera cards found (%s); set source in the config", strings.Join(cards, ", "))
}

// Scan lists importable files under root, sorted by Rel. Unknown files and
// sidecars without a video are returned in ignored; hidden files are skipped.
func Scan(root string) (files []SourceFile, ignored []string, err error) {
	videos := map[string]bool{} // upper-cased dir/base of every video
	var xmls []SourceFile
	for _, d := range scanDirs {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(d)), func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.HasPrefix(e.Name(), ".") {
				if e.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if e.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			ext := strings.ToUpper(path.Ext(rel))
			cls, ok := extClass[ext]
			if !ok && ext != ".XML" {
				ignored = append(ignored, rel)
				return nil
			}
			fi, err := e.Info()
			if err != nil {
				return err
			}
			f := SourceFile{Rel: rel, Abs: p, Size: fi.Size(), ModTime: fi.ModTime(), Class: cls}
			if ext == ".XML" {
				f.Class = ClassSidecar
				xmls = append(xmls, f)
				return nil
			}
			if cls == ClassVideo {
				videos[strings.ToUpper(strings.TrimSuffix(rel, path.Ext(rel)))] = true
			}
			files = append(files, f)
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, err
		}
	}
	// Sony sidecar: C0001M01.XML belongs to C0001.MP4 in the same folder.
	for _, x := range xmls {
		stem := strings.ToUpper(strings.TrimSuffix(x.Rel, path.Ext(x.Rel)))
		if strings.HasSuffix(stem, "M01") && videos[strings.TrimSuffix(stem, "M01")] {
			files = append(files, x)
		} else {
			ignored = append(ignored, x.Rel)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	sort.Strings(ignored)
	return files, ignored, nil
}
