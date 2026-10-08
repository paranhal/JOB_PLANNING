package hwpx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlaceholderLooksLikeHangulDocument(t *testing.T) {
	raw := BuildPlaceholderTemplate()
	assertHangulZIP(t, raw)
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	need := map[string]bool{
		"version.xml": false, "settings.xml": false,
		"META-INF/container.rdf": false, "Preview/PrvImage.png": false,
		"Contents/header.xml": false, "Contents/section0.xml": false,
	}
	for _, f := range zr.File {
		if _, ok := need[f.Name]; ok {
			need[f.Name] = true
		}
		if f.Name == "version.xml" && f.Method != zip.Store {
			t.Fatal("version.xml 은 한글처럼 Store 여야 한다")
		}
	}
	for n, ok := range need {
		if !ok {
			t.Fatalf("한글 필수 항목 없음: %s", n)
		}
	}
	ver := string(mustZipFile(t, raw, "version.xml"))
	if !strings.Contains(ver, `tag="HWP Document File"`) || !strings.Contains(ver, `targetApplication="WORDPROCESSOR"`) {
		t.Fatalf("version.xml 이 §70.1.2 형식이 아니다: %s", ver)
	}
	man := string(mustZipFile(t, raw, "META-INF/manifest.xml"))
	for _, p := range []string{"version.xml", "META-INF/container.xml", "Preview/PrvImage.png", "Preview/PrvText.txt"} {
		if !strings.Contains(man, p) {
			t.Fatalf("manifest 에 %s 없음", p)
		}
	}
	prv := mustZipFile(t, raw, "Preview/PrvText.txt")
	if len(prv) < 2 || prv[0] != 0xFF || prv[1] != 0xFE {
		t.Fatal("PrvText.txt 가 UTF-16LE BOM 이 아니다")
	}
	hdr := string(mustZipFile(t, raw, "Contents/header.xml"))
	if !strings.Contains(hdr, "itemCnt=") {
		t.Fatal("header.xml 에 itemCnt 가 없다")
	}
}

func TestPackHWPXMimetypeFirstNoDescriptor(t *testing.T) {
	raw := BuildPlaceholderTemplate()
	assertHangulZIP(t, raw)

	out, err := Replace(raw, emptyValues())
	if err != nil {
		t.Fatal(err)
	}
	assertHangulZIP(t, out)
}

func TestWritePlaceholderKeepsAnyExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "장애처리보고서.hwpx")
	keep := []byte("PK-small-existing")
	if err := os.WriteFile(path, keep, 0644); err != nil {
		t.Fatal(err)
	}
	if err := WritePlaceholderTemplate(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, keep) {
		t.Fatal("있는 양식을 덮어썼다")
	}
}

func TestWritePlaceholderCreatesWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "장애처리보고서.hwpx")
	if err := WritePlaceholderTemplate(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertHangulZIP(t, got)
}

func TestIsUserSuppliedTemplateKeepsPreviewImage(t *testing.T) {
	data, err := packHWPX([]hwpxEntry{
		{Name: "mimetype", Body: []byte("application/hwp+zip")},
		{Name: "Preview/PrvImage.png", Body: bytes.Repeat([]byte{1}, 12_000)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !IsUserSuppliedTemplate(data) {
		t.Fatal("사용자 한글 서식을 자리표시자로 오인했다")
	}
}

func assertHangulZIP(t *testing.T, data []byte) {
	t.Helper()
	if len(data) < 38 || string(data[0:2]) != "PK" {
		t.Fatalf("ZIP 시그니처 없음")
	}
	nameLen := binary.LittleEndian.Uint16(data[26:28])
	extraLen := binary.LittleEndian.Uint16(data[28:30])
	flags := binary.LittleEndian.Uint16(data[6:8])
	method := binary.LittleEndian.Uint16(data[8:10])
	name := string(data[30 : 30+int(nameLen)])
	if name != "mimetype" {
		t.Fatalf("첫 엔트리가 mimetype 이 아님: %q", name)
	}
	if extraLen != 0 {
		t.Fatalf("mimetype extra=%d (한글은 extra 금지)", extraLen)
	}
	if method != 0 {
		t.Fatalf("mimetype 이 압축됨: method=%d", method)
	}
	if flags&0x8 != 0 {
		t.Fatalf("data descriptor(flag bit3) 있음: flags=%d", flags)
	}
	start := 30 + int(nameLen)
	want := "application/hwp+zip"
	if start+len(want) > len(data) || string(data[start:start+len(want)]) != want {
		t.Fatalf("mimetype 내용이 offset %d 에 없음", start)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if zr.File[0].Name != "mimetype" {
		t.Fatalf("zip 순서 첫 파일이 %s", zr.File[0].Name)
	}
	for _, f := range zr.File {
		if f.Modified.Month() < 1 || f.Modified.Day() < 1 {
			t.Fatalf("%s ZIP 날짜가 달력에 없다 %v", f.Name, f.Modified)
		}
		if f.Flags&0x8 != 0 {
			t.Fatalf("%s data descriptor 플래그", f.Name)
		}
		if len(f.Extra) != 0 {
			t.Fatalf("%s extra=%d", f.Name, len(f.Extra))
		}
		if f.Name == "mimetype" && f.Method != zip.Store {
			t.Fatal("mimetype Store 아님")
		}
	}
}
