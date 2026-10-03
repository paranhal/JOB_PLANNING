package imageproc

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"

	"github.com/disintegration/imaging"
)

const (
	SignatureMaxWidth = 600
	SignatureMaxBytes = 1 << 20
)

// ProcessSignature PNG/JPG 사인을 흰 배경 투명 PNG로 만든다. 가로 600px·1MB 제한. §54.5
func ProcessSignature(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("사인이 없습니다")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("사인이 없습니다")
	}
	if len(data) > SignatureMaxBytes {
		return nil, fmt.Errorf("사인은 1MB 이하여야 합니다")
	}
	img, err := decode(data, "signature.png", "image/png")
	if err != nil {
		img, err = decode(data, "signature.jpg", "image/jpeg")
		if err != nil {
			return nil, fmt.Errorf("사인을 읽지 못했습니다")
		}
	}
	img = resizeWidth(img, SignatureMaxWidth)
	out := whitesToTransparent(img)
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("사인을 저장하지 못했습니다")
	}
	return buf.Bytes(), nil
}

func resizeWidth(img image.Image, maxW int) image.Image {
	if img == nil || maxW < 1 {
		return img
	}
	b := img.Bounds()
	if b.Dx() <= maxW {
		return img
	}
	return imaging.Resize(img, maxW, 0, imaging.Lanczos)
}

func whitesToTransparent(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := dst.NRGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			if c.R >= 245 && c.G >= 245 && c.B >= 245 {
				dst.SetNRGBA(x, y, color.NRGBA{A: 0})
			}
		}
	}
	return dst
}
