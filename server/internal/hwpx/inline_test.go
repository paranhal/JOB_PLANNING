package hwpx

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInlineImageAtFallbackLeavesText(t *testing.T) {
	vals := emptyValues()
	tpl := BuildPlaceholderTemplate()
	out, err := InlineImageAt(tpl, "점검자사인", nil, DefaultSignatureSideHWPUNIT, "(사인)")
	if err != nil {
		t.Fatal(err)
	}
	out, err = Replace(out, vals)
	if err != nil {
		t.Fatal(err)
	}
	sec := string(mustZipFile(t, out, "Contents/section0.xml"))
	if !strings.Contains(sec, "(사인)") {
		t.Fatal("없으면 (사인) 글자가 나가야 한다")
	}
	if strings.Contains(sec, "<hp:pic") {
		t.Fatal("없는 사인에 그림이 들어갔다")
	}
	if strings.Contains(sec, "{{") {
		t.Fatal("자리표시자가 남았다")
	}
}

func TestInlineImageAtFloatsInFrontOfText(t *testing.T) {
	png := inkPNG(t)
	tpl := BuildPlaceholderTemplate()
	out, err := InlineImageAt(tpl, "점검자사인", png, DefaultSignatureSideHWPUNIT, "(사인)")
	if err != nil {
		t.Fatal(err)
	}
	out, err = InlineImageAt(out, "확인자사인", png, DefaultSignatureSideHWPUNIT, "(사인)")
	if err != nil {
		t.Fatal(err)
	}
	out, err = Replace(out, emptyValues())
	if err != nil {
		t.Fatal(err)
	}

	sec := string(mustZipFile(t, out, "Contents/section0.xml"))
	if strings.Contains(sec, "{{") {
		t.Fatal("자리표시자가 남았다")
	}
	if !strings.Contains(sec, `textWrap="IN_FRONT_OF_TEXT"`) {
		t.Fatal("글 앞으로가 없다")
	}
	if !strings.Contains(sec, `treatAsChar="0"`) {
		t.Fatal("treatAsChar=0 이 없다")
	}
	if strings.Contains(sec, `treatAsChar="1"`) {
		t.Fatal("treatAsChar=1 이면 배치가 무시된다")
	}
	if !strings.Contains(sec, `allowOverlap="1"`) || !strings.Contains(sec, `flowWithText="1"`) {
		t.Fatal("겹침·문단 따라가기가 없다")
	}
	if !strings.Contains(sec, `vertRelTo="PARA"`) || !strings.Contains(sec, `horzRelTo="PARA"`) {
		t.Fatal("문단 기준이 없다")
	}
	if !strings.Contains(sec, `orgSz width="4600" height="4600"`) {
		t.Fatal("46pt 정사각형이 아니다")
	}
	if strings.Count(sec, "<hp:pic") != 2 {
		t.Fatalf("그림 수=%d", strings.Count(sec, "<hp:pic"))
	}
	end := strings.LastIndex(sec, "</hs:sec>")
	if end < 0 {
		t.Fatal("section 끝 없음")
	}
	tail := sec[end-80 : end]
	if strings.Contains(tail, "<hp:pic") {
		t.Fatal("문서 끝에 사인을 붙였다")
	}

	inspH, inspV := signatureOffsets("점검자사인", 4600)
	confH, confV := signatureOffsets("확인자사인", 4600)
	if inspH == confH {
		t.Fatalf("두 칸 horzOffset 이 같다: %d", inspH)
	}
	if inspH != 13460 || confH != 8205 {
		t.Fatalf("horzOffset 점검자=%d 확인자=%d", inspH, confH)
	}
	if inspV != -1393 || confV != -1393 {
		t.Fatalf("vertOffset 점검자=%d 확인자=%d", inspV, confV)
	}
	if !strings.Contains(sec, `horzOffset="13460"`) || !strings.Contains(sec, `horzOffset="8205"`) {
		t.Fatalf("칸별 offset 이 XML에 없다")
	}

	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	var pngs int
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".png") && strings.Contains(strings.ToLower(f.Name), "bindata") {
			pngs++
		}
	}
	if pngs < 2 {
		t.Fatalf("BinData PNG=%d", pngs)
	}
}

func TestInlineImageAtDoesNotStretchWidePNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 80, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	got, err := padPNGToSquare(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := image.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	b := out.Bounds()
	if b.Dx() != 80 || b.Dy() != 80 {
		t.Fatalf("정사각 패딩 실패 %dx%d", b.Dx(), b.Dy())
	}
}

func TestReplaceOfficialSignatureTemplateNeedsBothKeys(t *testing.T) {
	data := readOfficialReportTemplate(t)
	vals := emptyValues()
	delete(vals, "점검자사인")
	if _, err := Replace(data, vals); err == nil {
		t.Fatal("키 없이 양식만 바꾸면 발급이 막혀야 한다")
	}
	vals = emptyValues()
	vals["점검자사인"] = "(사인)"
	vals["확인자사인"] = "(사인)"
	out, err := Replace(data, vals)
	if err != nil {
		t.Fatal(err)
	}
	sec := string(mustZipFile(t, out, "Contents/section0.xml"))
	if strings.Contains(sec, "{{") {
		t.Fatal("14키를 넣었는데 {{ 가 남았다")
	}
	if !strings.Contains(sec, "(사인)") {
		t.Fatal("사인 없을 때 (사인) 글자가 없다")
	}
}

func readOfficialReportTemplate(t *testing.T) []byte {
	t.Helper()
	paths := []string{
		filepath.Join("..", "..", "templates", "report", "장애처리보고서.hwpx"),
		filepath.Join("templates", "report", "장애처리보고서.hwpx"),
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			continue
		}
		ok := false
		for _, f := range zr.File {
			if f.Name != "Contents/section0.xml" {
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
			ok = strings.Contains(string(raw), "{{점검자사인}}")
		}
		if !ok {
			t.Fatalf("사인 양식이 아니다: %s", p)
		}
		return b
	}
	t.Fatal("운영 양식을 읽지 못했다")
	return nil
}

func inkPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 10; y < 30; y++ {
		for x := 10; x < 30; x++ {
			img.Set(x, y, color.RGBA{A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
