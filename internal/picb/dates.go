package picb

import (
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/evanoberholster/imagemeta"
)

// DateSource records where a capture date came from.
type DateSource string

const (
	DateEXIF  DateSource = "exif"
	DateXML   DateSource = "xml"
	DateMvhd  DateSource = "mvhd"
	DateMtime DateSource = "mtime"
)

// CaptureDate returns when f was shot, in local time. With useMeta it tries
// EXIF (photos) or the Sony sidecar then the mvhd box (videos); otherwise, or
// when metadata is missing, it falls back to the file's modified time.
func CaptureDate(f SourceFile, sidecar *SourceFile, useMeta bool) (time.Time, DateSource) {
	if useMeta {
		switch f.Class {
		case ClassRAW, ClassJPEG:
			if t, err := exifDate(f.Abs); err == nil {
				return t.In(time.Local), DateEXIF
			}
		case ClassVideo:
			if sidecar != nil {
				if t, err := withFile(sidecar.Abs, func(r *os.File) (time.Time, error) { return parseSonyXML(r) }); err == nil {
					return t.In(time.Local), DateXML
				}
			}
			if t, err := withFile(f.Abs, func(r *os.File) (time.Time, error) { return parseMvhd(r) }); err == nil {
				return t.In(time.Local), DateMvhd
			}
		}
	}
	return f.ModTime.In(time.Local), DateMtime
}

// withFile opens path for fn. A panic inside fn (e.g. a metadata decoder
// choking on a corrupt file) becomes an error so the caller can fall back.
func withFile(path string, fn func(*os.File) (time.Time, error)) (t time.Time, err error) {
	r, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer r.Close()
	defer func() {
		if p := recover(); p != nil {
			t, err = time.Time{}, fmt.Errorf("reading %s: %v", path, p)
		}
	}()
	return fn(r)
}

func exifDate(path string) (time.Time, error) {
	return withFile(path, func(r *os.File) (time.Time, error) {
		e, err := decodeExif(r)
		// Some Sony ARWs hit EOF in a later IFD after DateTimeOriginal was
		// parsed; imagemeta returns the partial Exif, so keep its date.
		if err != nil && !(errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
			return time.Time{}, err
		}
		t := e.OriginalDate()
		if t.IsZero() {
			return t, errors.New("no DateTimeOriginal")
		}
		if e.ExifIFD.OffsetTimeOriginal == nil {
			// No offset tag: imagemeta labels the camera's wall clock as UTC.
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.Local)
		}
		return t, nil
	})
}

// decodeExif is imagemeta.Decode; tests swap it.
var decodeExif = imagemeta.Decode

var mp4Epoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)

// parseMvhd reads the creation time (UTC) from moov/mvhd of an MP4/MOV.
func parseMvhd(r io.ReadSeeker) (time.Time, error) {
	if _, err := findBox(r, "moov"); err != nil {
		return time.Time{}, err
	}
	if _, err := findBox(r, "mvhd"); err != nil { // first child of moov onwards
		return time.Time{}, err
	}
	var hdr [4]byte // version + flags
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return time.Time{}, err
	}
	var secs uint64
	if hdr[0] == 1 {
		var v uint64
		if err := binary.Read(r, binary.BigEndian, &v); err != nil {
			return time.Time{}, err
		}
		secs = v
	} else {
		var v uint32
		if err := binary.Read(r, binary.BigEndian, &v); err != nil {
			return time.Time{}, err
		}
		secs = uint64(v)
	}
	// 0 = unset; beyond ~2^33 s (year 2176) is corrupt and would overflow time.Duration.
	if secs == 0 || secs > 1<<33 {
		return time.Time{}, errors.New("mvhd creation time missing or out of range")
	}
	return mp4Epoch.Add(time.Duration(secs) * time.Second), nil
}

// findBox scans boxes from the current offset until it finds typ and leaves r
// at that box's payload, returning the payload size.
func findBox(r io.ReadSeeker, typ string) (int64, error) {
	for {
		var h [8]byte
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return 0, errors.New("box " + typ + " not found")
		}
		size := int64(binary.BigEndian.Uint32(h[:4]))
		hdr := int64(8)
		if size == 1 {
			var ext uint64
			if err := binary.Read(r, binary.BigEndian, &ext); err != nil {
				return 0, err
			}
			size, hdr = int64(ext), 16
		}
		if string(h[4:]) == typ {
			return size - hdr, nil
		}
		if size == 0 { // box runs to EOF
			return 0, errors.New("box " + typ + " not found")
		}
		if size < hdr {
			return 0, errors.New("malformed box")
		}
		if _, err := r.Seek(size-hdr, io.SeekCurrent); err != nil {
			return 0, err
		}
	}
}

// parseSonyXML reads <CreationDate value="..."/> from a Sony clip sidecar.
func parseSonyXML(r io.Reader) (time.Time, error) {
	var m struct {
		CreationDate struct {
			Value string `xml:"value,attr"`
		} `xml:"CreationDate"`
	}
	if err := xml.NewDecoder(r).Decode(&m); err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, m.CreationDate.Value)
}
