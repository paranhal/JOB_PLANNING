// Package imageproc는 기획서 §12.9.4 접수 사진 자동 축소·썸네일·EXIF 회전을 담당한다.
package imageproc

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"golang.org/x/image/webp"
)

const (
	FullMaxEdge  = 1600
	ThumbMaxEdge = 320
	JPEGQuality  = 85
)

// Result 축소본(원본은 보관하지 않음)과 썸네일.
type Result struct {
	Full      []byte
	Thumb     []byte
	MIMEType  string
	StoredExt string // ".jpg"
}

// LooksLikeImage 확장자·MIME으로 이미지 여부를 본다. 일반 파일(pdf 등)은 false.
func LooksLikeImage(name, mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if strings.HasPrefix(mime, "image/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".heic", ".heif", ".bmp", ".tif", ".tiff":
		return true
	}
	return false
}

// Process 이미지를 긴 변 1600px로 줄이고 320px 썸네일을 만든다. EXIF 회전을 반영한다.
// HEIC는 JPEG로 변환해 브라우저에서 보이게 한다. 원본 바이트는 반환하지 않는다.
func Process(r io.Reader, origName, mime string) (*Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("빈 파일입니다")
	}
	img, err := decode(data, origName, mime)
	if err != nil {
		return nil, err
	}
	full := resizeLongEdge(img, FullMaxEdge)
	thumb := resizeLongEdge(img, ThumbMaxEdge)

	fullBuf, err := encodeJPEG(full)
	if err != nil {
		return nil, err
	}
	thumbBuf, err := encodeJPEG(thumb)
	if err != nil {
		return nil, err
	}
	return &Result{
		Full:      fullBuf,
		Thumb:     thumbBuf,
		MIMEType:  "image/jpeg",
		StoredExt: ".jpg",
	}, nil
}

func decode(data []byte, origName, mime string) (image.Image, error) {
	if isHEIC(origName, mime, data) {
		img, err := decodeHEIC(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("HEIC 사진을 읽지 못했습니다: %w", err)
		}
		return img, nil
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err == nil {
		return img, nil
	}
	if w, werr := webp.Decode(bytes.NewReader(data)); werr == nil {
		return w, nil
	}
	return nil, fmt.Errorf("이미지를 읽지 못했습니다: %w", err)
}

func isHEIC(name, mime string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".heic" || ext == ".heif" {
		return true
	}
	m := strings.ToLower(mime)
	if strings.Contains(m, "heic") || strings.Contains(m, "heif") {
		return true
	}
	return hasHEICBrand(data)
}

// hasHEICBrand ISO BMFF ftyp 박스의 brand가 HEIC/HEIF 계열인지 본다.
func hasHEICBrand(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	if string(data[4:8]) != "ftyp" {
		return false
	}
	brand := string(data[8:12])
	switch brand {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs", "mif1", "msf1", "heif":
		return true
	}
	return false
}

func resizeLongEdge(img image.Image, max int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return img
	}
	if w <= max && h <= max {
		return img
	}
	if w >= h {
		return imaging.Resize(img, max, 0, imaging.Lanczos)
	}
	return imaging.Resize(img, 0, max, imaging.Lanczos)
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ThumbPath 축소본 경로에서 썸네일 경로를 만든다. 확장자는 항상 .jpg.
func ThumbPath(filePath string) string {
	ext := filepath.Ext(filePath)
	return strings.TrimSuffix(filePath, ext) + "_thumb.jpg"
}
