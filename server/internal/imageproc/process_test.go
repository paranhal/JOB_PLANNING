package imageproc

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestLooksLikeImage(t *testing.T) {
	if !LooksLikeImage("a.HEIC", "") {
		t.Fatal("heic")
	}
	if !LooksLikeImage("a.jpg", "") {
		t.Fatal("jpg")
	}
	if LooksLikeImage("보고서.pdf", "application/pdf") {
		t.Fatal("pdf는 이미지가 아니다")
	}
	if !LooksLikeImage("x.bin", "image/jpeg") {
		t.Fatal("mime")
	}
}

func TestProcessResizesLongEdge(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 2000; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, err := Process(&buf, "wide.png", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if out.MIMEType != "image/jpeg" || out.StoredExt != ".jpg" {
		t.Fatalf("출력 형식: %+v", out)
	}
	full, err := jpeg.Decode(bytes.NewReader(out.Full))
	if err != nil {
		t.Fatal(err)
	}
	fb := full.Bounds()
	if fb.Dx() != FullMaxEdge || fb.Dy() != FullMaxEdge/2 {
		t.Fatalf("축소 크기 %dx%d want %dx%d", fb.Dx(), fb.Dy(), FullMaxEdge, FullMaxEdge/2)
	}
	thumb, err := jpeg.Decode(bytes.NewReader(out.Thumb))
	if err != nil {
		t.Fatal(err)
	}
	tb := thumb.Bounds()
	if tb.Dx() != ThumbMaxEdge || tb.Dy() != ThumbMaxEdge/2 {
		t.Fatalf("썸네일 %dx%d", tb.Dx(), tb.Dy())
	}
}

func TestProcessKeepsSmallImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, err := Process(&buf, "small.png", "image/png")
	if err != nil {
		t.Fatal(err)
	}
	full, _ := jpeg.Decode(bytes.NewReader(out.Full))
	if full.Bounds().Dx() != 80 || full.Bounds().Dy() != 40 {
		t.Fatalf("작은 이미지는 키우지 않는다: %v", full.Bounds())
	}
}

func TestThumbPath(t *testing.T) {
	got := ThumbPath(`data/uploads/as/R1/receipt/1_0.jpg`)
	if got != `data/uploads/as/R1/receipt/1_0_thumb.jpg` &&
		got != `data\uploads\as\R1\receipt\1_0_thumb.jpg` {
		t.Fatalf("thumb path: %q", got)
	}
}

func TestHasHEICBrand(t *testing.T) {
	buf := make([]byte, 16)
	copy(buf[4:8], "ftyp")
	copy(buf[8:12], "heic")
	if !hasHEICBrand(buf) {
		t.Fatal("heic brand")
	}
	copy(buf[8:12], "isom")
	if hasHEICBrand(buf) {
		t.Fatal("isom은 HEIC가 아니다")
	}
}
