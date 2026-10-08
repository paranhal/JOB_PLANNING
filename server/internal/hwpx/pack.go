package hwpx

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"hash/crc32"
	"io"
	"strings"
	"time"
)

var zipDOSEpoch = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

type hwpxEntry struct {
	Name string
	Body []byte
}

// packHWPX 한글이 요구하는 ZIP 규칙을 지킨다.
// mimetype 이 맨 앞, 무압축, extra 없음, data descriptor(flag bit 3) 없음.
// Go zip.Writer 기본값은 extra·descriptor 를 붙여 한글에서 「손상된 파일」이 된다.
func packHWPX(entries []hwpxEntry) ([]byte, error) {
	ordered := make([]hwpxEntry, 0, len(entries))
	var mime *hwpxEntry
	for i := range entries {
		if entries[i].Name == "mimetype" {
			e := entries[i]
			mime = &e
			continue
		}
		ordered = append(ordered, entries[i])
	}
	if mime != nil {
		ordered = append([]hwpxEntry{*mime}, ordered...)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range ordered {
		store := e.Name == "mimetype" || e.Name == "version.xml"
		body := e.Body
		comp := body
		method := uint16(zip.Store)
		flags := uint16(0)
		if !store {
			var cbuf bytes.Buffer
			fw, err := flate.NewWriter(&cbuf, flate.DefaultCompression)
			if err != nil {
				_ = zw.Close()
				return nil, err
			}
			if _, err := fw.Write(body); err != nil {
				_ = zw.Close()
				return nil, err
			}
			if err := fw.Close(); err != nil {
				_ = zw.Close()
				return nil, err
			}
			comp = cbuf.Bytes()
			method = zip.Deflate
			flags = 0x4 // 한글 원본과 같이 Deflate Fast
		}
		hdr := &zip.FileHeader{
			Name:               e.Name,
			Method:             method,
			CRC32:              crc32.ChecksumIEEE(body),
			CompressedSize64:   uint64(len(comp)),
			UncompressedSize64: uint64(len(body)),
			CreatorVersion:     20,
			ReaderVersion:      20,
			Flags:              flags,
			Modified:           zipDOSEpoch,
		}
		hdr.SetModTime(zipDOSEpoch)
		w, err := zw.CreateRaw(hdr)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := w.Write(comp); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unzipEntries(doc []byte) ([]hwpxEntry, error) {
	zr, err := zip.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		return nil, err
	}
	out := make([]hwpxEntry, 0, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, hwpxEntry{Name: f.Name, Body: raw})
	}
	return out, nil
}

// IsUserSuppliedTemplate 한글이 만든 원본 서식인지 본다. 자리표시자 ZIP은 다시 만든다.
func IsUserSuppliedTemplate(data []byte) bool {
	if len(data) >= 50_000 {
		return true
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		n := strings.ToLower(strings.ReplaceAll(f.Name, "\\", "/"))
		if strings.Contains(n, "prvimage") && f.UncompressedSize64 >= 10_000 {
			return true
		}
		if strings.HasPrefix(n, "bindata/") || strings.HasPrefix(n, "contents/bindata/") {
			return true
		}
	}
	return false
}
