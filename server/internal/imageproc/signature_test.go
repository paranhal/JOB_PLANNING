package imageproc

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestProcessSignatureMakesWhiteTransparentAndResizes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 800; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	for x := 40; x < 120; x++ {
		for y := 40; y < 80; y++ {
			img.Set(x, y, color.RGBA{R: 20, G: 20, B: 20, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, err := ProcessSignature(&buf)
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != SignatureMaxWidth {
		t.Fatalf("가로 %d want %d", got.Bounds().Dx(), SignatureMaxWidth)
	}
	c := color.NRGBAModel.Convert(got.At(got.Bounds().Min.X, got.Bounds().Min.Y)).(color.NRGBA)
	if c.A != 0 {
		t.Fatalf("흰 배경이 투명이어야 한다 %+v", c)
	}
}

func TestProcessSignatureRejectsTooLarge(t *testing.T) {
	big := bytes.Repeat([]byte("x"), SignatureMaxBytes+1)
	_, err := ProcessSignature(bytes.NewReader(big))
	if err == nil || !strings.Contains(err.Error(), "1MB") {
		t.Fatalf("크기 제한: %v", err)
	}
}
