package hwpx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"strings"
	"testing"
)

func TestAppendJPEGsAddsBinDataAndPicture(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 200, A: 255})
		}
	}
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}

	vals := map[string]string{}
	for _, k := range PlaceholderKeys {
		vals[k] = "값"
	}
	replaced, err := Replace(BuildPlaceholderTemplate(), vals)
	if err != nil {
		t.Fatal(err)
	}
	out, err := AppendJPEGs(replaced, []JPEGPhoto{{JPEG: jpegBuf.Bytes(), Caption: "교체 후"}})
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		if !strings.HasSuffix(strings.ToLower(f.Name), ".xml") && !strings.HasSuffix(strings.ToLower(f.Name), ".hpf") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(bytes.NewReader(raw))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s XML 무효: %v", f.Name, err)
			}
		}
	}
	joined := strings.Join(names, "\n")
	if !strings.Contains(joined, "Contents/BinData/actionphoto1.jpg") {
		t.Fatalf("BinData 없음: %s", joined)
	}
	sec := ""
	for _, f := range zr.File {
		if f.Name == "Contents/section0.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			sec = string(b)
		}
	}
	if !strings.Contains(sec, `binaryItemIDRef="actionphoto1"`) {
		t.Fatalf("그림 참조 없음: %s", sec)
	}
	if !strings.Contains(sec, "교체 후") {
		t.Fatal("캡션이 없다")
	}
}

func TestAppendJPEGsEmptyIsNoop(t *testing.T) {
	in := BuildPlaceholderTemplate()
	out, err := AppendJPEGs(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(in, out) {
		t.Fatal("빈 사진은 원본을 그대로 돌려야 한다")
	}
}
