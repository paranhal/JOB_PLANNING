package hwpx

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
)

// DefaultTemplatePath 운영 템플릿. 원본은 읽기만 한다. §12.10.2
func DefaultTemplatePath() string {
	return filepath.Join("templates", "report", "장애처리보고서.hwpx")
}

// PlaceholderKeys 템플릿에 있어야 하는 자리표시자. §12.10.3
var PlaceholderKeys = []string{
	"고객명", "부서", "담당자", "연락처", "서비스",
	"장애사항", "장애원인", "결론",
	"보고일자", "점검자", "확인자",
	"작업일자", "조치내용",
}

// WritePlaceholderTemplate 최소 유효 템플릿을 path 에 쓴다. 이미 있으면 덮어쓰지 않는다.
func WritePlaceholderTemplate(path string) error {
	if path == "" {
		path = DefaultTemplatePath()
	}
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, BuildPlaceholderTemplate(), 0644)
}


// BuildPlaceholderTemplate 자리표시자가 한 런에 들어 있는 최소 유효 HWPX. 한글 서식 템플릿을 대체하지 않는다.
func BuildPlaceholderTemplate() []byte {
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

	store("mimetype", []byte("application/hwp+zip"))
	deflate("version.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<ha:HWPApplicationSetting xmlns:ha="http://www.hancom.co.kr/hwpml/2011/app">
  <ha:Version>5.0.0.0</ha:Version>
</ha:HWPApplicationSetting>`)
	deflate("META-INF/container.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<ocf:container xmlns:ocf="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">
  <ocf:rootfiles>
    <ocf:rootfile full-path="Contents/content.hpf" media-type="application/hwpml-package+xml"/>
  </ocf:rootfiles>
</ocf:container>`)
	deflate("META-INF/manifest.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<odf:manifest xmlns:odf="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0">
  <odf:file-entry odf:full-path="/" odf:media-type="application/hwp+zip"/>
  <odf:file-entry odf:full-path="Contents/content.hpf" odf:media-type="application/hwpml-package+xml"/>
  <odf:file-entry odf:full-path="Contents/header.xml" odf:media-type="application/xml"/>
  <odf:file-entry odf:full-path="Contents/section0.xml" odf:media-type="application/xml"/>
</odf:manifest>`)
	deflate("Contents/content.hpf", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<opf:package xmlns:opf="http://www.hancom.co.kr/hwpml/2011/package">
  <opf:metadata/>
  <opf:manifest>
    <opf:item id="header" href="header.xml" media-type="application/xml"/>
    <opf:item id="section0" href="section0.xml" media-type="application/xml"/>
  </opf:manifest>
  <opf:spine>
    <opf:itemref idref="header"/>
    <opf:itemref idref="section0"/>
  </opf:spine>
</opf:package>`)
	deflate("Contents/header.xml", minimalHeaderXML)
	deflate("Contents/section0.xml", buildSection0())
	deflate("Preview/PrvText.txt", previewText())
	_ = w.Close()
	return buf.Bytes()
}

func buildSection0() string {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	b.WriteString(`<hs:sec xmlns:hs="http://www.hancom.co.kr/hwpml/2011/section" xmlns:hp="http://www.hancom.co.kr/hwpml/2011/paragraph">` + "\n")
	for _, key := range PlaceholderKeys {
		b.WriteString(`  <hp:p id="0" paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>{{`)
		b.WriteString(key)
		b.WriteString(`}}</hp:t></hp:run></hp:p>` + "\n")
	}
	b.WriteString(`</hs:sec>`)
	return b.String()
}

func previewText() string {
	var b bytes.Buffer
	for _, key := range PlaceholderKeys {
		b.WriteString("{{")
		b.WriteString(key)
		b.WriteString("}}\n")
	}
	return b.String()
}

const minimalHeaderXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<hh:head xmlns:hh="http://www.hancom.co.kr/hwpml/2011/head" xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core" version="1.1" secCnt="1">
  <hh:beginNum page="1" footnote="1" endnote="1" pic="1" tbl="1" equation="1"/>
  <hh:refList>
    <hh:fontfaces>
      <hh:fontface lang="HANGUL"><hh:font id="0" face="함초롬바탕" type="ttf" isEmbedded="0"/></hh:fontface>
      <hh:fontface lang="LATIN"><hh:font id="0" face="함초롬바탕" type="ttf" isEmbedded="0"/></hh:fontface>
      <hh:fontface lang="HANJA"><hh:font id="0" face="함초롬바탕" type="ttf" isEmbedded="0"/></hh:fontface>
      <hh:fontface lang="JAPANESE"><hh:font id="0" face="함초롬바탕" type="ttf" isEmbedded="0"/></hh:fontface>
      <hh:fontface lang="OTHER"><hh:font id="0" face="함초롬바탕" type="ttf" isEmbedded="0"/></hh:fontface>
      <hh:fontface lang="SYMBOL"><hh:font id="0" face="함초롬바탕" type="ttf" isEmbedded="0"/></hh:fontface>
      <hh:fontface lang="USER"><hh:font id="0" face="함초롬바탕" type="ttf" isEmbedded="0"/></hh:fontface>
    </hh:fontfaces>
    <hh:charProperties>
      <hh:charPr id="0" height="1000" textColor="#000000" shadeColor="none" useFontSpace="0" useKerning="0" symMark="NONE" borderFillIDRef="0">
        <hh:fontRef hangul="0" latin="0" hanja="0" japanese="0" other="0" symbol="0" user="0"/>
        <hh:ratio hangul="100" latin="100" hanja="100" japanese="100" other="100" symbol="100" user="100"/>
        <hh:spacing hangul="0" latin="0" hanja="0" japanese="0" other="0" symbol="0" user="0"/>
        <hh:relSz hangul="100" latin="100" hanja="100" japanese="100" other="100" symbol="100" user="100"/>
        <hh:offset hangul="0" latin="0" hanja="0" japanese="0" other="0" symbol="0" user="0"/>
      </hh:charPr>
    </hh:charProperties>
    <hh:paraProperties>
      <hh:paraPr id="0" tabPrIDRef="0" condense="0" fontLineHeight="0" snapToGrid="0" suppressLineNumbers="0" checked="0">
        <hh:align horizontal="JUSTIFY" vertical="CENTER"/>
        <hh:heading type="NONE" idRef="0" level="0"/>
        <hh:breakSetting breakLatinWord="KEEP_WORD" breakNonLatinWord="KEEP_WORD" widowOrphan="0" keepWithNext="0" keepLines="0" pageBreakBefore="0" lineWrap="BREAK"/>
        <hh:lineSpacing type="PERCENT" value="160" unit="HWPUNIT"/>
      </hh:paraPr>
    </hh:paraProperties>
    <hh:styles>
      <hh:style id="0" type="PARA" name="바탕글" engName="Normal" paraPrIDRef="0" charPrIDRef="0" nextStyleIDRef="0" langID="1042" lockForm="0"/>
    </hh:styles>
  </hh:refList>
</hh:head>`
