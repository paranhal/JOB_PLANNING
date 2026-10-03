package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// AppendPNG 문서 끝에 사인 PNG를 붙인다. 실패해도 호출 쪽에서 원본을 유지한다. §54.5
func AppendPNG(doc, png []byte) ([]byte, error) {
	if len(png) == 0 {
		return doc, nil
	}
	if len(doc) == 0 {
		return nil, fmt.Errorf("DOCX가 비어 있습니다")
	}
	zr, err := zip.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		return nil, err
	}
	type part struct {
		name  string
		body  []byte
		store bool
	}
	var parts []part
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		parts = append(parts, part{name: f.Name, body: raw, store: f.Method == zip.Store || f.Name == "mimetype"})
	}
	parts = append(parts, part{name: "word/media/signature.png", body: png})
	for i := range parts {
		switch parts[i].name {
		case "[Content_Types].xml":
			parts[i].body = ensurePNGContentType(parts[i].body)
		case "word/_rels/document.xml.rels":
			parts[i].body = ensureSigRel(parts[i].body)
		case "word/document.xml":
			parts[i].body = appendSigDrawing(parts[i].body)
		}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		method := zip.Deflate
		if p.store {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: p.name, Method: method})
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := w.Write(p.body); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ensurePNGContentType(raw []byte) []byte {
	s := string(raw)
	if strings.Contains(s, `Extension="png"`) {
		return raw
	}
	ins := `  <Default Extension="png" ContentType="image/png"/>` + "\n"
	if i := strings.Index(s, "<Override"); i >= 0 {
		return []byte(s[:i] + ins + s[i:])
	}
	if i := strings.Index(s, "</Types>"); i >= 0 {
		return []byte(s[:i] + ins + s[i:])
	}
	return raw
}

func ensureSigRel(raw []byte) []byte {
	s := string(raw)
	if strings.Contains(s, "word/media/signature.png") {
		return raw
	}
	rel := `  <Relationship Id="rIdSig1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/signature.png"/>` + "\n"
	if i := strings.LastIndex(s, "</Relationships>"); i >= 0 {
		return []byte(s[:i] + rel + s[i:])
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
` + rel + `</Relationships>`)
}

func appendSigDrawing(raw []byte) []byte {
	s := string(raw)
	if !strings.Contains(s, `xmlns:r=`) {
		s = strings.Replace(s, `<w:document `, `<w:document xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture" `, 1)
	}
	draw := `<w:p><w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="1714500" cy="628650"/><wp:docPr id="91" name="signature"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="signature.png"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="rIdSig1"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="1714500" cy="628650"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`
	if i := strings.LastIndex(s, "<w:sectPr"); i >= 0 {
		return []byte(s[:i] + draw + s[i:])
	}
	if i := strings.LastIndex(s, "</w:body>"); i >= 0 {
		return []byte(s[:i] + draw + s[i:])
	}
	return raw
}
