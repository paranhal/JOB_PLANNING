package hwpx

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"path/filepath"
	"strings"
)

// DefaultSignatureSideHWPUNIT 사인 칸 한 변. 호출 쪽이 설정을 넘기지 않을 때의 기본값. §71.3.3-1
const DefaultSignatureSideHWPUNIT = 4600

// 양식 마지막 줄 실측. 고정 화면좌표가 아니라 칸 기하로 자리를 셈한다. §71.3.3-3
const (
	sigCellPadHWPUNIT   = 540
	sigGlyphWidthApprox = 3060
	sigRowHeightHWPUNIT = 2414
	sigCellInspHWPUNIT  = 18430
	sigCellConfHWPUNIT  = 13175
	sigNudgeXHWPUNIT    = -600
	sigNudgeYHWPUNIT    = -300
)

// InlineImageAt 자리표시자를 「글 앞으로」 그림으로 바꾼다. PNG가 없으면 fallback 글자. §71.3.3
func InlineImageAt(doc []byte, placeholder string, png []byte, sideHWPUNIT int, fallback string) ([]byte, error) {
	if len(doc) == 0 {
		return nil, fmt.Errorf("HWPX가 비어 있습니다")
	}
	placeholder = strings.TrimSpace(placeholder)
	if placeholder == "" {
		return nil, fmt.Errorf("자리표시자가 없습니다")
	}
	if sideHWPUNIT <= 0 {
		sideHWPUNIT = DefaultSignatureSideHWPUNIT
	}
	if fallback == "" {
		fallback = "(사인)"
	}

	parts, err := unzipEntries(doc)
	if err != nil {
		return nil, fmt.Errorf("HWPX를 열 수 없습니다: %w", err)
	}

	token := "{{" + placeholder + "}}"
	if len(png) == 0 {
		replacePlaceholderText(parts, token, fallback)
		return packHWPX(parts)
	}

	padded, err := padPNGToSquare(png)
	if err != nil {
		return nil, err
	}
	id := nextBinID(parts, "sig")
	href := "BinData/" + id + ".png"
	added := []jpegAdd{{id: id, href: href, mime: "image/png", blob: padded, w: sideHWPUNIT, h: sideHWPUNIT}}
	parts = append(parts, hwpxEntry{Name: "Contents/" + href, Body: padded})

	horz, vert := signatureOffsets(placeholder, sideHWPUNIT)
	pic := floatingPicXML(id, sideHWPUNIT, horz, vert)
	insert := "</hp:t></hp:run><hp:run charPrIDRef=\"0\">" + pic + "</hp:run><hp:run charPrIDRef=\"0\"><hp:t>"

	for i := range parts {
		n := filepath.ToSlash(parts[i].Name)
		switch {
		case n == "Contents/content.hpf":
			parts[i].Body = injectManifestItems(parts[i].Body, added)
		case n == "META-INF/manifest.xml":
			parts[i].Body = injectODFEntriesLoose(parts[i].Body, added)
		case strings.HasPrefix(n, "Contents/section") && strings.HasSuffix(strings.ToLower(n), ".xml"):
			s := normalizePlaceholders(string(parts[i].Body))
			if !strings.Contains(s, `xmlns:hc=`) {
				s = strings.Replace(s, "<hs:sec", `<hs:sec xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core"`, 1)
			}
			if !strings.Contains(s, token) {
				return nil, fmt.Errorf("자리표시자 %s 가 없습니다", placeholder)
			}
			parts[i].Body = []byte(strings.Replace(s, token, insert, 1))
		case isPlainPreview(n):
			s := string(parts[i].Body)
			parts[i].Body = []byte(strings.ReplaceAll(s, token, fallback))
		}
	}
	return packHWPX(parts)
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

func signatureOffsets(placeholder string, size int) (horz, vert int) {
	cellW := sigCellInspHWPUNIT
	if strings.Contains(placeholder, "확인") {
		cellW = sigCellConfHWPUNIT
	}
	r := cellW - sigCellPadHWPUNIT
	cx := r - sigGlyphWidthApprox/2
	cy := sigRowHeightHWPUNIT / 2
	return cx - size/2 + sigNudgeXHWPUNIT, cy - size/2 + sigNudgeYHWPUNIT
}

func floatingPicXML(id string, side, horz, vert int) string {
	return fmt.Sprintf(
		`<hp:pic id="0" zOrder="5" numberingType="PICTURE" textWrap="IN_FRONT_OF_TEXT" textFlow="BOTH_SIDES" lock="0">`+
			`<hp:offset x="0" y="0"/>`+
			`<hp:orgSz width="%d" height="%d"/>`+
			`<hp:curSz width="%d" height="%d"/>`+
			`<hp:flip horizontal="0" vertical="0"/>`+
			`<hp:rotationInfo angle="0" centerX="0" centerY="0"/>`+
			`<hp:imgRect><hc:pt0 x="0" y="0"/><hc:pt1 x="%d" y="0"/><hc:pt2 x="%d" y="%d"/><hc:pt3 x="0" y="%d"/></hp:imgRect>`+
			`<hp:imgClip left="0" right="0" top="0" bottom="0"/>`+
			`<hp:inMargin left="0" right="0" top="0" bottom="0"/>`+
			`<hp:img binaryItemIDRef="%s" bright="0" contrast="0" effect="REAL_PIC" alpha="0"/>`+
			`<hp:pos treatAsChar="0" allowOverlap="1" flowWithText="1" vertRelTo="PARA" horzRelTo="PARA" vertAlign="TOP" horzAlign="LEFT" vertOffset="%d" horzOffset="%d"/>`+
			`</hp:pic>`,
		side, side, side, side, side, side, side, side, id, vert, horz)
}

func nextBinID(parts []hwpxEntry, prefix string) string {
	used := map[string]bool{}
	for _, e := range parts {
		base := filepath.Base(filepath.ToSlash(e.Name))
		used[strings.TrimSuffix(base, filepath.Ext(base))] = true
	}
	for i := 1; i < 1000; i++ {
		id := fmt.Sprintf("%s%d", prefix, i)
		if !used[id] {
			return id
		}
	}
	return prefix + "x"
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
