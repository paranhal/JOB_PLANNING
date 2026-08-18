package hwpx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceEscapesXMLAndKeepsQuotes(t *testing.T) {
	vals := map[string]string{
		"고객명": "가나", "부서": "", "담당자": "", "연락처": "", "서비스": "",
		"장애사항": `게이트 <고장> & "소음"`,
		"장애원인": "전원", "결론": "정상",
		"보고일자": "2026-08-18", "점검자": "", "확인자": "",
		"작업일자": "", "조치내용": "",
	}
	out, err := Replace(BuildPlaceholderTemplate(), vals)
	if err != nil {
		t.Fatal(err)
	}
	sec := mustZipFile(t, out, "Contents/section0.xml")
	assertXMLWellFormed(t, sec)
	if strings.Contains(string(sec), "{{") {
		t.Fatal("자리표시자가 남았다")
	}
	if !strings.Contains(string(sec), "&lt;") || !strings.Contains(string(sec), "&amp;") {
		t.Fatalf("& < 이스케이프 없음: %s", sec)
	}
	if strings.Contains(string(sec), "<고장>") {
		t.Fatal("원문 < 가 XML에 그대로 들어갔다")
	}
}

func TestReplaceMultilineBecomesParagraphs(t *testing.T) {
	vals := emptyValues()
	vals["조치내용"] = "2026-08-01 1차\n2026-08-05 2차\n2026-08-10 3차"
	out, err := Replace(BuildPlaceholderTemplate(), vals)
	if err != nil {
		t.Fatal(err)
	}
	sec := string(mustZipFile(t, out, "Contents/section0.xml"))
	assertXMLWellFormed(t, []byte(sec))
	for _, want := range []string{"1차", "2차", "3차"} {
		if !strings.Contains(sec, want) {
			t.Fatalf("%s 없음", want)
		}
	}
	if strings.Count(sec, "<hp:p") < 3 {
		t.Fatalf("줄바꿈이 문단으로 안 나뉨: p=%d", strings.Count(sec, "<hp:p"))
	}
	if strings.Contains(sec, "1차\n2026") {
		t.Fatal("\\n 이 그대로 남아 한 줄로 붙을 수 있다")
	}
}

func TestReplaceLeftoverPlaceholderErrors(t *testing.T) {
	tpl := BuildPlaceholderTemplate()
	vals := emptyValues()
	delete(vals, "결론")
	_, err := Replace(tpl, vals)
	if !errors.Is(err, ErrLeftoverPlaceholder) {
		t.Fatalf("남은 {{ 를 에러로 안 막음: %v", err)
	}
}

func TestReplaceDoesNotMutateInputOrDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "장애처리보고서.hwpx")
	orig := BuildPlaceholderTemplate()
	if err := os.WriteFile(path, orig, 0644); err != nil {
		t.Fatal(err)
	}
	in := append([]byte(nil), orig...)
	if _, err := Replace(in, emptyValues()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(in, orig) {
		t.Fatal("입력 슬라이스를 바꿨다")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, orig) {
		t.Fatal("디스크 템플릿을 덮어썼다")
	}
}

func TestNormalizeSplitPlaceholder(t *testing.T) {
	raw := `<hp:t>{{고</hp:t></hp:run><hp:run><hp:t>객명}}</hp:t>`
	got := normalizePlaceholders(raw)
	if !strings.Contains(got, "{{고객명}}") {
		t.Fatalf("쪼갠 자리표시자를 못 모음: %s", got)
	}
}

func TestMaterializeDefaultTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "장애처리보고서.hwpx")
	if err := WritePlaceholderTemplate(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Replace(data, emptyValues())
	if err != nil {
		t.Fatal(err)
	}
	assertXMLWellFormed(t, mustZipFile(t, out, "Contents/section0.xml"))
}

func TestUnzippedXMLWellFormed(t *testing.T) {
	out, err := Replace(BuildPlaceholderTemplate(), emptyValues())
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range zr.File {
		if !isXMLName(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		assertXMLWellFormed(t, data)
		n++
	}
	if n < 3 {
		t.Fatalf("XML 파일이 너무 적다: %d", n)
	}
}

func emptyValues() map[string]string {
	m := map[string]string{}
	for _, k := range PlaceholderKeys {
		m[k] = ""
	}
	return m
}

func mustZipFile(t *testing.T, zipped []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatalf("%s 없음", name)
	return nil
}

func assertXMLWellFormed(t *testing.T, data []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("XML 유효하지 않음: %v\n%s", err, data)
		}
	}
}
