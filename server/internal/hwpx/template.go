package hwpx

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"unicode/utf16"
)

//go:embed hangul_header.xml
var hangulHeaderXML []byte

// DefaultTemplatePath 운영 템플릿. 원본은 읽기만 한다. §12.10.2
func DefaultTemplatePath() string {
	return filepath.Join("templates", "report", "장애처리보고서.hwpx")
}

// PlaceholderKeys 템플릿에 있어야 하는 자리표시자. §12.10.3
var PlaceholderKeys = []string{
	"고객명", "부서", "담당자", "연락처", "서비스",
	"장애사항", "장애원인",
	"보고일자", "점검자", "점검자사인", "확인자", "확인자사인",
	"작업일자", "조치내용",
}

// WritePlaceholderTemplate 파일이 없으면 자리표시자만 만든다. 있으면 크기와 무관하게 그대로 둔다. §70.5
func WritePlaceholderTemplate(path string) error {
	if path == "" {
		path = DefaultTemplatePath()
	}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		log.Printf("조치완료보고서 양식 있음, 그대로 쓴다 (%d바이트)", len(data))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, BuildPlaceholderTemplate(), 0644)
}

// BuildPlaceholderTemplate 한글 12가 여는 HWPX. 서식 원본을 대체하지 않는다.
func BuildPlaceholderTemplate() []byte {
	data, err := packHWPX([]hwpxEntry{
		{Name: "mimetype", Body: []byte("application/hwp+zip")},
		{Name: "version.xml", Body: []byte(versionXML)},
		{Name: "settings.xml", Body: []byte(settingsXML)},
		{Name: "META-INF/container.xml", Body: []byte(containerXML)},
		{Name: "META-INF/container.rdf", Body: []byte(containerRDF)},
		{Name: "META-INF/manifest.xml", Body: []byte(manifestXML)},
		{Name: "Contents/content.hpf", Body: []byte(contentHPF)},
		{Name: "Contents/header.xml", Body: hangulHeaderXML},
		{Name: "Contents/section0.xml", Body: []byte(buildSection0())},
		{Name: "Preview/PrvText.txt", Body: encodeUTF16LE(previewText())},
		{Name: "Preview/PrvImage.png", Body: previewPNG()},
	})
	if err != nil {
		return nil
	}
	return data
}

func buildSection0() string {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>`)
	b.WriteString(`<hs:sec xmlns:ha="http://www.hancom.co.kr/hwpml/2011/app" xmlns:hp="http://www.hancom.co.kr/hwpml/2011/paragraph" xmlns:hp10="http://www.hancom.co.kr/hwpml/2016/paragraph" xmlns:hs="http://www.hancom.co.kr/hwpml/2011/section" xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core" xmlns:hh="http://www.hancom.co.kr/hwpml/2011/head" xmlns:hhs="http://www.hancom.co.kr/hwpml/2011/history" xmlns:hm="http://www.hancom.co.kr/hwpml/2011/master-page" xmlns:hpf="http://www.hancom.co.kr/schema/2011/hpf" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf/" xmlns:ooxmlchart="http://www.hancom.co.kr/hwpml/2016/ooxmlchart" xmlns:hwpunitchar="http://www.hancom.co.kr/hwpml/2016/HwpUnitChar" xmlns:epub="http://www.idpf.org/2007/ops" xmlns:config="urn:oasis:names:tc:opendocument:xmlns:config:1.0">`)
	b.WriteString(`<hp:p id="0" paraPrIDRef="0" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="0">`)
	b.WriteString(secPrXML)
	b.WriteString(`<hp:ctrl><hp:colPr id="" type="NEWSPAPER" layout="LEFT" colCount="1" sameSz="1" sameGap="0"/></hp:ctrl></hp:run></hp:p>`)
	for _, key := range PlaceholderKeys {
		b.WriteString(`<hp:p id="0" paraPrIDRef="0" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="0"><hp:t>{{`)
		b.WriteString(key)
		b.WriteString(`}}</hp:t></hp:run></hp:p>`)
	}
	b.WriteString(`</hs:sec>`)
	return b.String()
}

func previewText() string {
	var b bytes.Buffer
	b.WriteString("조치완료보고서\n")
	for _, key := range PlaceholderKeys {
		b.WriteString("{{")
		b.WriteString(key)
		b.WriteString("}}\n")
	}
	return b.String()
}

func encodeUTF16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 2+len(u)*2)
	out[0], out[1] = 0xFF, 0xFE
	for i, r := range u {
		out[2+i*2] = byte(r)
		out[2+i*2+1] = byte(r >> 8)
	}
	return out
}

func previewPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 160, 226))
	for y := 0; y < 226; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{R: 245, G: 245, B: 245, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

const secPrXML = `<hp:secPr id="" textDirection="HORIZONTAL" spaceColumns="1134" tabStop="8000" tabStopVal="4000" tabStopUnit="HWPUNIT" outlineShapeIDRef="1" memoShapeIDRef="0" textVerticalWidthHead="0" masterPageCnt="0"><hp:grid lineGrid="0" charGrid="0" wonggojiFormat="0"/><hp:startNum pageStartsOn="BOTH" page="0" pic="0" tbl="0" equation="0"/><hp:visibility hideFirstHeader="0" hideFirstFooter="0" hideFirstMasterPage="0" border="SHOW_ALL" fill="SHOW_ALL" hideFirstPageNum="0" hideFirstEmptyLine="0" showLineNumber="0"/><hp:lineNumberShape restartType="0" countBy="0" distance="0" startNumber="0"/><hp:pagePr landscape="WIDELY" width="59528" height="84186" gutterType="LEFT_ONLY"><hp:margin header="4252" footer="4252" gutter="0" left="8504" right="8504" top="5668" bottom="4252"/></hp:pagePr><hp:footNotePr><hp:autoNumFormat type="DIGIT" userChar="" prefixChar="" suffixChar=")" supscript="0"/><hp:noteLine length="-1" type="SOLID" width="0.12 mm" color="#000000"/><hp:noteSpacing betweenNotes="283" belowLine="567" aboveLine="850"/><hp:numbering type="CONTINUOUS" newNum="1"/><hp:placement place="EACH_COLUMN" beneathText="0"/></hp:footNotePr><hp:endNotePr><hp:autoNumFormat type="DIGIT" userChar="" prefixChar="" suffixChar=")" supscript="0"/><hp:noteLine length="14692344" type="SOLID" width="0.12 mm" color="#000000"/><hp:noteSpacing betweenNotes="0" belowLine="567" aboveLine="850"/><hp:numbering type="CONTINUOUS" newNum="1"/><hp:placement place="END_OF_DOCUMENT" beneathText="0"/></hp:endNotePr><hp:pageBorderFill type="BOTH" borderFillIDRef="1" textBorder="PAPER" headerInside="0" footerInside="0" fillArea="PAPER"><hp:offset left="1417" right="1417" top="1417" bottom="1417"/></hp:pageBorderFill><hp:pageBorderFill type="EVEN" borderFillIDRef="1" textBorder="PAPER" headerInside="0" footerInside="0" fillArea="PAPER"><hp:offset left="1417" right="1417" top="1417" bottom="1417"/></hp:pageBorderFill><hp:pageBorderFill type="ODD" borderFillIDRef="1" textBorder="PAPER" headerInside="0" footerInside="0" fillArea="PAPER"><hp:offset left="1417" right="1417" top="1417" bottom="1417"/></hp:pageBorderFill></hp:secPr>`

// version.xml §70.1.2
const versionXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><hv:HCFVersion xmlns:hv="http://www.hancom.co.kr/hwpml/2011/version" tag="HWP Document File" targetApplication="WORDPROCESSOR" major="5" minor="1" micro="1" buildNumber="0"/>`

const settingsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?><ha:HWPApplicationSetting xmlns:ha="http://www.hancom.co.kr/hwpml/2011/app" xmlns:config="urn:oasis:names:tc:opendocument:xmlns:config:1.0"><ha:CaretPosition listIDRef="0" paraIDRef="0" pos="0"/></ha:HWPApplicationSetting>`

const containerXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?><ocf:container xmlns:ocf="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:hpf="http://www.hancom.co.kr/schema/2011/hpf"><ocf:rootfiles><ocf:rootfile full-path="Contents/content.hpf" media-type="application/hwpml-package+xml"/><ocf:rootfile full-path="Preview/PrvText.txt" media-type="text/plain"/><ocf:rootfile full-path="META-INF/container.rdf" media-type="application/rdf+xml"/></ocf:rootfiles></ocf:container>`

const containerRDF = `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about=""><ns0:hasPart xmlns:ns0="http://www.hancom.co.kr/hwpml/2016/meta/pkg#" rdf:resource="Contents/header.xml"/></rdf:Description><rdf:Description rdf:about="Contents/header.xml"><rdf:type rdf:resource="http://www.hancom.co.kr/hwpml/2016/meta/pkg#HeaderFile"/></rdf:Description><rdf:Description rdf:about=""><ns0:hasPart xmlns:ns0="http://www.hancom.co.kr/hwpml/2016/meta/pkg#" rdf:resource="Contents/section0.xml"/></rdf:Description><rdf:Description rdf:about="Contents/section0.xml"><rdf:type rdf:resource="http://www.hancom.co.kr/hwpml/2016/meta/pkg#SectionFile"/></rdf:Description><rdf:Description rdf:about=""><rdf:type rdf:resource="http://www.hancom.co.kr/hwpml/2016/meta/pkg#Document"/></rdf:Description></rdf:RDF>`

const manifestXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><odf:manifest xmlns:odf="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"><odf:file-entry odf:full-path="/" odf:media-type="application/hwp+zip"/><odf:file-entry odf:full-path="mimetype" odf:media-type="text/plain"/><odf:file-entry odf:full-path="version.xml" odf:media-type="application/xml"/><odf:file-entry odf:full-path="settings.xml" odf:media-type="application/xml"/><odf:file-entry odf:full-path="META-INF/container.xml" odf:media-type="application/xml"/><odf:file-entry odf:full-path="META-INF/container.rdf" odf:media-type="application/rdf+xml"/><odf:file-entry odf:full-path="META-INF/manifest.xml" odf:media-type="application/xml"/><odf:file-entry odf:full-path="Contents/content.hpf" odf:media-type="application/hwpml-package+xml"/><odf:file-entry odf:full-path="Contents/header.xml" odf:media-type="application/xml"/><odf:file-entry odf:full-path="Contents/section0.xml" odf:media-type="application/xml"/><odf:file-entry odf:full-path="Preview/PrvText.txt" odf:media-type="text/plain"/><odf:file-entry odf:full-path="Preview/PrvImage.png" odf:media-type="image/png"/></odf:manifest>`

const contentHPF = `<?xml version="1.0" encoding="UTF-8" standalone="yes" ?><opf:package xmlns:ha="http://www.hancom.co.kr/hwpml/2011/app" xmlns:hp="http://www.hancom.co.kr/hwpml/2011/paragraph" xmlns:hp10="http://www.hancom.co.kr/hwpml/2016/paragraph" xmlns:hs="http://www.hancom.co.kr/hwpml/2011/section" xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core" xmlns:hh="http://www.hancom.co.kr/hwpml/2011/head" xmlns:hhs="http://www.hancom.co.kr/hwpml/2011/history" xmlns:hm="http://www.hancom.co.kr/hwpml/2011/master-page" xmlns:hpf="http://www.hancom.co.kr/schema/2011/hpf" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf/" xmlns:ooxmlchart="http://www.hancom.co.kr/hwpml/2016/ooxmlchart" xmlns:hwpunitchar="http://www.hancom.co.kr/hwpml/2016/HwpUnitChar" xmlns:epub="http://www.idpf.org/2007/ops" xmlns:config="urn:oasis:names:tc:opendocument:xmlns:config:1.0" version="" unique-identifier="" id=""><opf:metadata><opf:title>조치완료보고서</opf:title><opf:language>ko</opf:language></opf:metadata><opf:manifest><opf:item id="header" href="Contents/header.xml" media-type="application/xml"/><opf:item id="section0" href="Contents/section0.xml" media-type="application/xml"/><opf:item id="settings" href="settings.xml" media-type="application/xml"/></opf:manifest><opf:spine><opf:itemref idref="header" linear="yes"/><opf:itemref idref="section0" linear="yes"/></opf:spine></opf:package>`
