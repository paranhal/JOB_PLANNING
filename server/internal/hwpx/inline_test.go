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

func confirmedSigBox() SignatureBox {
	return SignatureBox{Size: 4600, RightGap: 900, NudgeY: 0}
}

func TestSignatureOffsetUsesCellGeometryNotRunText(t *testing.T) {
	h1, v1 := SignatureOffset(SigCellInspHWPUNIT, 4600, 900, 0)
	h2, v2 := SignatureOffset(SigCellConfHWPUNIT, 4600, 900, 0)
	if h1 != 14190 || v1 != -1093 {
		t.Fatalf("점검자 %d,%d", h1, v1)
	}
	if h2 != 8935 || v2 != -1093 {
		t.Fatalf("확인자 %d,%d", h2, v2)
	}
	if h1 == h2 {
		t.Fatal("두 칸에 같은 horzOffset 을 쓰면 안 된다")
	}
	again, _ := SignatureOffset(SigCellInspHWPUNIT, 4600, 900, 0)
	if again != h1 {
		t.Fatal("칸 기하가 아닌 값에 의존한다")
	}
}

func TestInlineImageAtFallbackLeavesText(t *testing.T) {
	vals := emptyValues()
	tpl := BuildPlaceholderTemplate()
	out, err := InlineImageAt(tpl, "점검자사인", nil, confirmedSigBox(), "(사인)")
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
	out, err := InlineImageAt(tpl, "점검자사인", png, confirmedSigBox(), "(사인)")
	if err != nil {
		t.Fatal(err)
	}
	out, err = InlineImageAt(out, "확인자사인", png, confirmedSigBox(), "(사인)")
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
		t.Fatal("글 앞으로가 아니다")
	}
	if !strings.Contains(sec, `treatAsChar="0"`) {
		t.Fatal("글자처럼 취급되면 칸이 늘어난다")
	}
	if !strings.Contains(sec, `horzOffset="14190"`) || !strings.Contains(sec, `horzOffset="8935"`) {
		t.Fatal("두 칸 가로 자리가 다르다")
	}
	if !strings.Contains(sec, `vertOffset="-1093"`) {
		t.Fatal("세로 자리가 틀리다")
	}
	if strings.Count(sec, "<hp:pic") != 2 {
		t.Fatal("사인 그림이 둘이 아니다")
	}
	if strings.Contains(sec, "</hp:tbl>") && !strings.Contains(sec[strings.LastIndex(sec, "</hp:tbl>"):], "<hp:pic") {
		t.Fatal("표가 있으면 그림은 표 뒤에 떠야 한다")
	}
	hdr := string(mustZipFile(t, out, "Contents/header.xml"))
	if !strings.Contains(hdr, `BinData="sig1.png"`) {
		t.Fatal("header.xml 에 그림 항목이 없다")
	}
	if mustZipFile(t, out, "Contents/BinData/sig1.png") == nil {
		t.Fatal("PNG가 없다")
	}
	if !strings.Contains(sec, `orgSz width="30000"`) {
		t.Fatal("orgSz 는 PNG 원본 크기여야 한다")
	}
	if !strings.Contains(sec, `curSz width="4600"`) || !strings.Contains(sec, `hp:sz width="4600"`) {
		t.Fatal("상자 크기는 hp:sz 4600 이다")
	}
}

func TestInlineImageAtOfficialPicFloatsOutsideLastTable(t *testing.T) {
	png := inkPNG(t)
	tpl, err := FitASReportTables(readOfficialReportTemplate(t))
	if err != nil {
		t.Fatal(err)
	}
	out, err := InlineImageAt(tpl, "점검자사인", png, confirmedSigBox(), "(사인)")
	if err != nil {
		t.Fatal(err)
	}
	out, err = InlineImageAt(out, "확인자사인", png, confirmedSigBox(), "(사인)")
	if err != nil {
		t.Fatal(err)
	}
	out, err = Replace(out, emptyValues())
	if err != nil {
		t.Fatal(err)
	}
	sec := string(mustZipFile(t, out, "Contents/section0.xml"))
	lastTbl := strings.LastIndex(sec, "</hp:tbl>")
	if lastTbl < 0 {
		t.Fatal("표가 없다")
	}
	if strings.Count(sec, "<hp:pic") != 2 {
		t.Fatal("사인 그림이 둘이 아니다")
	}
	if strings.Contains(sec[:lastTbl], "<hp:pic") {
		t.Fatal("그림이 표 칸 안에 남아 있다")
	}
	after := sec[lastTbl:]
	if !strings.Contains(after, `binaryItemIDRef="sig1"`) || !strings.Contains(after, `binaryItemIDRef="sig2"`) {
		t.Fatal("그림이 마지막 표 뒤에 없다")
	}
	if strings.Contains(after, "<hp:p ") {
		t.Fatal("표 뒤에 문단을 더 붙이면 한글이 페이지를 버린다")
	}
	if !strings.Contains(sec, `horzOffset="22580"`) || !strings.Contains(sec, `horzOffset="44259"`) {
		t.Fatal("표 밖 가로 자리가 틀리다")
	}
	if !strings.Contains(sec, `vertOffset="3298"`) {
		t.Fatal("표 밖 세로 자리가 틀리다")
	}
	if !strings.Contains(sec, "(사인)") {
		t.Fatal("칸의 (사인) 글자가 지워졌다")
	}
	if !strings.Contains(sec, `flowWithText="0"`) || !strings.Contains(sec, `holdAnchorAndSO="1"`) {
		t.Fatal("끌면 문단·표에 다시 붙는다")
	}
	tbl := sec[strings.LastIndex(sec, "<hp:tbl"):lastTbl]
	if pos := strings.Index(tbl, "<hp:pos "); pos >= 0 {
		end := strings.Index(tbl[pos:], "/>")
		if end >= 0 {
			tbl = tbl[pos : pos+end]
		}
	}
	if !strings.Contains(tbl, `allowOverlap="1"`) {
		t.Fatal("표가 겹침을 막으면 사인이 한 점으로 밀린다")
	}
	if !strings.Contains(sec, `id="1001"`) || !strings.Contains(sec, `id="1002"`) {
		t.Fatal("그림 id 가 겹치면 한글이 한 자리로 모은다")
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
