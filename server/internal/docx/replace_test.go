package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
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
	sec := mustZipFile(t, out, "word/document.xml")
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

func TestReplaceMergesSplitRuns(t *testing.T) {
	vals := emptyValues()
	vals["고객명"] = "채움씨앤아이"
	out, err := Replace(BuildSplitPlaceholderTemplate(true), vals)
	if err != nil {
		t.Fatal(err)
	}
	sec := string(mustZipFile(t, out, "word/document.xml"))
	assertXMLWellFormed(t, []byte(sec))
	if strings.Contains(sec, "{{") {
		t.Fatal("쪼갠 자리표시자가 남았다")
	}
	if !strings.Contains(sec, "채움씨앤아이") {
		t.Fatalf("치환 실패: %s", sec)
	}
}

func TestReplaceMultilineBecomesBreaks(t *testing.T) {
	vals := emptyValues()
	vals["조치내용"] = "2026-08-01 1차\n2026-08-05 2차\n2026-08-10 3차"
	out, err := Replace(BuildPlaceholderTemplate(), vals)
	if err != nil {
		t.Fatal(err)
	}
	sec := string(mustZipFile(t, out, "word/document.xml"))
	assertXMLWellFormed(t, []byte(sec))
	for _, want := range []string{"1차", "2차", "3차"} {
		if !strings.Contains(sec, want) {
			t.Fatalf("%s 없음", want)
		}
	}
	if strings.Count(sec, "<w:br") < 2 {
		t.Fatalf("줄바꿈이 안 나뉨: br=%d", strings.Count(sec, "<w:br"))
	}
}

func TestReplaceLeftoverPlaceholderErrors(t *testing.T) {
	vals := emptyValues()
	delete(vals, "결론")
	_, err := Replace(BuildPlaceholderTemplate(), vals)
	if !errors.Is(err, ErrLeftoverPlaceholder) {
		t.Fatalf("남은 {{ 를 에러로 안 막음: %v", err)
	}
}

func TestReplaceDoesNotMutateInput(t *testing.T) {
	orig := BuildPlaceholderTemplate()
	in := append([]byte(nil), orig...)
	if _, err := Replace(in, emptyValues()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(in, orig) {
		t.Fatal("입력 슬라이스를 바꿨다")
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
