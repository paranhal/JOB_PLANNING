package hwpx

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
)

// SignatureBox 크기·보정. 숫자는 app_settings·users.signature_box 에서 온다. §71.3.3-2
type SignatureBox struct {
	Size, RightGap, NudgeY int
}

// 양식 마지막 줄 실측. 고정 화면좌표가 아니라 칸 기하로 자리를 셈한다. §71.3.3-3
const (
	sigCellPadHWPUNIT   = 540
	sigRowHeightHWPUNIT = 2414
	SigCellInspHWPUNIT  = 18430
	SigCellConfHWPUNIT  = 13175
	sigLabelInspHWPUNIT = 8390
	sigLabelConfHWPUNIT = 8504
	sigPNGMinSidePx     = 400
)

// InlineImageAt 자리표시자를 「글 앞으로」 그림으로 바꾼다. PNG가 없으면 fallback 글자. §71.3.3
func InlineImageAt(doc []byte, placeholder string, png []byte, box SignatureBox, fallback string) ([]byte, error) {
	if len(doc) == 0 {
		return nil, fmt.Errorf("HWPX가 비어 있습니다")
	}
	placeholder = strings.TrimSpace(placeholder)
	if placeholder == "" {
		return nil, fmt.Errorf("자리표시자가 없습니다")
	}
	if box.Size <= 0 {
		return nil, fmt.Errorf("사인 크기가 없습니다")
	}
	if fallback == "" {
		fallback = "(사인)"
	}

	parts, err := unzipEntries(doc)
	if err != nil {
		return nil, fmt.Errorf("HWPX를 열 수 없습니다: %w", err)
	}

	token := "{{" + placeholder + "}}"
	found := false
	for _, e := range parts {
		if strings.Contains(normalizePlaceholders(string(e.Body)), token) {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("자리표시자 %s 가 없습니다", placeholder)
	}
	if len(png) == 0 {
		replacePlaceholderText(parts, token, fallback)
		return packHWPX(parts)
	}

	padded, err := prepareSignaturePNG(png)
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(padded))
	if err != nil {
		return nil, fmt.Errorf("사인 그림 크기를 읽지 못했습니다: %w", err)
	}
	orgW, orgH := cfg.Width*75, cfg.Height*75
	id := nextBinID(parts, "sig")
	href := "BinData/" + id + ".png"
	added := []jpegAdd{{id: id, href: href, mime: "image/png", blob: padded}}
	parts = append(parts, hwpxEntry{Name: "Contents/" + href, Body: padded})

	inserted := false
	for i := range parts {
		n := filepath.ToSlash(parts[i].Name)
		switch {
		case n == "Contents/content.hpf":
			parts[i].Body = injectManifestItems(parts[i].Body, added)
		case n == "Contents/header.xml":
			parts[i].Body = injectHeaderBinItems(parts[i].Body, added)
		case n == "META-INF/manifest.xml":
			parts[i].Body = injectODFEntriesLoose(parts[i].Body, added)
		case strings.HasPrefix(n, "Contents/section") && strings.HasSuffix(strings.ToLower(n), ".xml"):
			s := normalizePlaceholders(string(parts[i].Body))
			if !strings.Contains(s, `xmlns:hc=`) {
				s = strings.Replace(s, "<hs:sec", `<hs:sec xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core"`, 1)
			}
			if !strings.Contains(s, token) {
				continue
			}
			horz, vert := SignatureOffset(signatureCellWidth(placeholder), box.Size, box.RightGap, box.NudgeY)
			shapeID := nextShapeID(s)
			zOrder := 5 + strings.Count(s, "<hp:pic")
			if strings.Contains(s, "</hp:tbl>") {
				horz, vert = signatureOutsideTableOffset(s, placeholder, box.Size, box.RightGap, box.NudgeY)
				pic := signatureFloatPic(id, shapeID, zOrder, orgW, orgH, box.Size, horz, vert)
				esc := xmlEscape(sanitizeValue(fallback))
				s = strings.Replace(s, token, esc, 1)
				s = allowLastTableOverlap(s)
				parts[i].Body = []byte(insertPicAfterLastTable(s, pic))
			} else {
				pic := signatureFloatPic(id, shapeID, zOrder, orgW, orgH, box.Size, horz, vert)
				parts[i].Body = []byte(insertFloatPicAtToken(s, token, pic))
			}
			inserted = true
		case isPlainPreview(n):
			s := string(parts[i].Body)
			parts[i].Body = []byte(strings.ReplaceAll(s, token, fallback))
		}
	}
	if !inserted {
		return nil, fmt.Errorf("자리표시자 %s 가 본문에 없습니다", placeholder)
	}
	return packHWPX(parts)
}

func insertFloatPicAtToken(s, token, pic string) string {
	i := strings.Index(s, token)
	if i < 0 {
		return s
	}
	s = s[:i] + s[i+len(token):]
	closeT := strings.Index(s[i:], "</hp:t>")
	if closeT < 0 {
		return s[:i] + pic + s[i:]
	}
	at := i + closeT + len("</hp:t>")
	return s[:at] + pic + s[at:]
}

func insertPicAfterLastTable(s, pic string) string {
	i := strings.LastIndex(s, "</hp:tbl>")
	if i < 0 {
		return s + pic
	}
	at := i + len("</hp:tbl>")
	return s[:at] + pic + s[at:]
}

func lastTableVertOffset(s string) int {
	i := strings.LastIndex(s, "<hp:tbl")
	if i < 0 {
		return 0
	}
	j := strings.Index(s[i:], "</hp:tbl>")
	if j < 0 {
		j = len(s) - i
	}
	chunk := s[i : i+j]
	k := strings.Index(chunk, `vertOffset="`)
	if k < 0 {
		return 0
	}
	k += len(`vertOffset="`)
	end := strings.Index(chunk[k:], `"`)
	if end < 0 {
		return 0
	}
	n := 0
	neg := false
	for _, c := range chunk[k : k+end] {
		if c == '-' {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		return -n
	}
	return n
}

// signatureOutsideTableOffset 마지막 표와 같은 문단, 표 뒤에 띄운다. 칸 안이면 한글이 표에 가둔다.
func signatureOutsideTableOffset(s, placeholder string, size, rightGap, nudgeY int) (horz, vert int) {
	cellH, cellV := SignatureOffset(signatureCellWidth(placeholder), size, rightGap, nudgeY)
	vert = lastTableVertOffset(s) + sigRowHeightHWPUNIT + cellV
	if strings.Contains(placeholder, "확인") {
		return sigLabelInspHWPUNIT + SigCellInspHWPUNIT + sigLabelConfHWPUNIT + cellH, vert
	}
	return sigLabelInspHWPUNIT + cellH, vert
}

func nextBinID(parts []hwpxEntry, prefix string) string {
	used := map[string]bool{}
	for _, e := range parts {
		base := strings.TrimSuffix(filepath.Base(filepath.ToSlash(e.Name)), filepath.Ext(e.Name))
		used[base] = true
		s := string(e.Body)
		for i := 1; i < 1000; i++ {
			id := fmt.Sprintf("%s%d", prefix, i)
			if strings.Contains(s, `id="`+id+`"`) || strings.Contains(s, id+".png") || strings.Contains(s, id+".jpg") {
				used[id] = true
			}
		}
	}
	for i := 1; i < 1000; i++ {
		id := fmt.Sprintf("%s%d", prefix, i)
		if !used[id] {
			return id
		}
	}
	return prefix + "999"
}

func replacePlaceholderText(parts []hwpxEntry, token, fallback string) {
	esc := xmlEscape(sanitizeValue(fallback))
	plain := plainReplacement(fallback)
	for i := range parts {
		n := filepath.ToSlash(parts[i].Name)
		s := string(parts[i].Body)
		if isXMLName(n) {
			s = normalizePlaceholders(s)
			parts[i].Body = []byte(strings.ReplaceAll(s, token, esc))
			continue
		}
		if isPlainPreview(n) {
			parts[i].Body = []byte(strings.ReplaceAll(s, token, plain))
		}
	}
}

func signatureCaption(placeholder string) string {
	if strings.Contains(placeholder, "확인") {
		return "확인자 사인"
	}
	return "점검자 사인"
}

func signatureCellWidth(placeholder string) int {
	if strings.Contains(placeholder, "확인") {
		return SigCellConfHWPUNIT
	}
	return SigCellInspHWPUNIT
}

// SignatureOffset 칸 왼쪽 위 기준. 글자 폭을 재지 않는다. §71.3.3-3 · 62-F-1
func SignatureOffset(cellWidth, size, rightGap, nudgeY int) (horz, vert int) {
	return cellWidth - sigCellPadHWPUNIT - size + rightGap, sigRowHeightHWPUNIT/2 - size/2 + nudgeY
}


func nextShapeID(s string) int {
	return 1001 + strings.Count(s, "<hp:pic")
}

func allowLastTableOverlap(s string) string {
	i := strings.LastIndex(s, "<hp:tbl")
	if i < 0 {
		return s
	}
	j := strings.Index(s[i:], "</hp:tbl>")
	if j < 0 {
		return s
	}
	chunk := s[i : i+j]
	n := strings.Replace(chunk, `allowOverlap="0"`, `allowOverlap="1"`, 1)
	return s[:i] + n + s[i+j:]
}

func signatureFloatParagraph(id string, orgW, orgH, box, horz, vert int) string {
	return `<hp:p id="0" paraPrIDRef="0" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="0">` +
		signatureFloatPic(id, 1001, 5, orgW, orgH, box, horz, vert) +
		`</hp:run></hp:p>`
}

// signatureFloatPic 한글 「글 앞으로」 본. orgSz는 PNG 원본, 상자 크기는 hp:sz. 62-B-2
func signatureFloatPic(binID string, shapeID, zOrder, orgW, orgH, box, horz, vert int) string {
	if orgW < 1 {
		orgW = box
	}
	if orgH < 1 {
		orgH = box
	}
	if shapeID < 1 {
		shapeID = 1001
	}
	if zOrder < 1 {
		zOrder = 5
	}
	return fmt.Sprintf(
		`<hp:pic id="%d" zOrder="%d" numberingType="PICTURE" textWrap="IN_FRONT_OF_TEXT" textFlow="BOTH_SIDES" lock="0" dropcapstyle="None" href="" groupLevel="0" instid="%d">`+
			`<hp:offset x="0" y="0"/>`+
			`<hp:orgSz width="%d" height="%d"/>`+
			`<hp:curSz width="%d" height="%d"/>`+
			`<hp:flip horizontal="0" vertical="0"/>`+
			`<hp:rotationInfo angle="0" centerX="0" centerY="0"/>`+
			`<hp:renderingInfo>`+
			`<hc:transMatrix e1="1" e2="0" e3="0" e4="0" e5="1" e6="0"/>`+
			`<hc:scaMatrix e1="1" e2="0" e3="0" e4="0" e5="1" e6="0"/>`+
			`<hc:rotMatrix e1="1" e2="0" e3="0" e4="0" e5="1" e6="0"/>`+
			`</hp:renderingInfo>`+
			`<hp:imgRect><hc:pt0 x="0" y="0"/><hc:pt1 x="%d" y="0"/><hc:pt2 x="%d" y="%d"/><hc:pt3 x="0" y="%d"/></hp:imgRect>`+
			`<hp:imgClip left="0" right="0" top="0" bottom="0"/>`+
			`<hp:inMargin left="0" right="0" top="0" bottom="0"/>`+
			`<hp:img binaryItemIDRef="%s" bright="0" contrast="0" effect="REAL_PIC" alpha="0"/>`+
			`<hp:effects/>`+
			`<hp:sz width="%d" widthRelTo="ABSOLUTE" height="%d" heightRelTo="ABSOLUTE" protect="0"/>`+
			`<hp:pos treatAsChar="0" affectLSpacing="0" flowWithText="0" allowOverlap="1" holdAnchorAndSO="1" vertRelTo="PARA" horzRelTo="PARA" vertAlign="TOP" horzAlign="LEFT" vertOffset="%d" horzOffset="%d"/>`+
			`<hp:outMargin left="0" right="0" top="0" bottom="0"/>`+
			`</hp:pic>`,
		shapeID, zOrder, shapeID, orgW, orgH, box, box, orgW, orgW, orgH, orgH, binID, box, box, vert, horz)
}

func nextBinDataID(parts []hwpxEntry) string {
	used := map[string]bool{}
	for _, e := range parts {
		base := strings.TrimSuffix(filepath.Base(filepath.ToSlash(e.Name)), filepath.Ext(e.Name))
		used[base] = true
		s := string(e.Body)
		for i := 1; i < 1000; i++ {
			id := fmt.Sprintf("BIN%04d", i)
			if strings.Contains(s, `id="`+id+`"`) || strings.Contains(s, id+".png") || strings.Contains(s, id+".jpg") {
				used[id] = true
			}
		}
	}
	for i := 1; i < 1000; i++ {
		id := fmt.Sprintf("BIN%04d", i)
		if !used[id] {
			return id
		}
	}
	return "BIN0999"
}

func injectODFEntriesLoose(raw []byte, added []jpegAdd) []byte {
	s := strings.TrimSpace(string(raw))
	if strings.Contains(s, "</odf:manifest>") {
		return injectODFEntries(raw, added)
	}
	if strings.HasSuffix(s, "/>") {
		open := strings.TrimSuffix(s, "/>") + ">"
		return injectODFEntries([]byte(open+"</odf:manifest>"), added)
	}
	return injectODFEntries(raw, added)
}

func padPNGToSquare(raw []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("사인 그림을 읽지 못했습니다: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return nil, fmt.Errorf("사인 그림이 비었습니다")
	}
	side := w
	if h > side {
		side = h
	}
	out := image.NewRGBA(image.Rect(0, 0, side, side))
	ox := (side - w) / 2
	oy := (side - h) / 2
	draw.Draw(out, image.Rect(ox, oy, ox+w, oy+h), img, b.Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func prepareSignaturePNG(raw []byte) ([]byte, error) {
	padded, err := padPNGToSquare(raw)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(padded))
	if err != nil {
		return nil, fmt.Errorf("사인 그림을 읽지 못했습니다: %w", err)
	}
	b := img.Bounds()
	if b.Dx() >= sigPNGMinSidePx && b.Dy() >= sigPNGMinSidePx {
		return padded, nil
	}
	up := imaging.Resize(img, sigPNGMinSidePx, sigPNGMinSidePx, imaging.Lanczos)
	var buf bytes.Buffer
	if err := png.Encode(&buf, up); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func jpegFromSignaturePNG(raw []byte) ([]byte, error) {
	padded, err := prepareSignaturePNG(raw)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(padded))
	if err != nil {
		return nil, fmt.Errorf("사인 그림을 읽지 못했습니다: %w", err)
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, b, img, b.Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
