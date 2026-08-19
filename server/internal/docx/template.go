package docx

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
)

// DefaultTemplatePath 운영 템플릿. 없으면 DOCX 버튼만 숨긴다. §12.10.6
func DefaultTemplatePath() string {
	return filepath.Join("templates", "report", "장애처리보고서.docx")
}

// PlaceholderKeys 템플릿에 있어야 하는 자리표시자. §12.10.3
var PlaceholderKeys = []string{
	"고객명", "부서", "담당자", "연락처", "서비스",
	"장애사항", "장애원인", "결론",
	"보고일자", "점검자", "확인자",
	"작업일자", "조치내용",
}

// TemplateExists 디스크에 쓸 수 있는 DOCX 템플릿이 있는지. 없으면 HWPX만 낸다.
func TemplateExists(path string) bool {
	if path == "" {
		path = DefaultTemplatePath()
	}
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

// BuildPlaceholderTemplate 자리표시자가 한 런에 들어 있는 최소 유효 DOCX. 원본 서식 템플릿을 대체하지 않는다.
func BuildPlaceholderTemplate() []byte {
	return BuildSplitPlaceholderTemplate(false)
}

// BuildSplitPlaceholderTemplate Word가 자리표시자를 여러 w:t 로 쪼갠 경우를 재현한다.
func BuildSplitPlaceholderTemplate(split bool) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	store := func(name string, body []byte) {
		hdr := &zip.FileHeader{Name: name, Method: zip.Store}
		fw, err := w.CreateHeader(hdr)
		if err != nil {
			return
		}
		_, _ = fw.Write(body)
	}
	deflate := func(name, body string) {
		fw, err := w.Create(name)
		if err != nil {
			return
		}
		_, _ = fw.Write([]byte(body))
	}

	store("[Content_Types].xml", []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`))
	deflate("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)
	deflate("word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`)
	deflate("word/document.xml", buildDocumentXML(split))
	_ = w.Close()
	return buf.Bytes()
}

func buildDocumentXML(split bool) string {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	b.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + "\n")
	for i, key := range PlaceholderKeys {
		if split && i == 0 {
			runes := []rune(key)
			mid := len(runes) / 2
			if mid < 1 {
				mid = 1
			}
			b.WriteString(`<w:p><w:r><w:t>{{`)
			b.WriteString(string(runes[:mid]))
			b.WriteString(`</w:t></w:r><w:r><w:t>`)
			b.WriteString(string(runes[mid:]))
			b.WriteString(`}}</w:t></w:r></w:p>` + "\n")
			continue
		}
		b.WriteString(`<w:p><w:r><w:t>{{`)
		b.WriteString(key)
		b.WriteString(`}}</w:t></w:r></w:p>` + "\n")
	}
	b.WriteString(`<w:sectPr/></w:body></w:document>`)
	return b.String()
}
