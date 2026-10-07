package hwpx

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"path/filepath"
	"strings"
)

// JPEGPhoto 조치완료보고서에 붙일 JPEG. §12.9.8 · §12.10
type JPEGPhoto struct {
	JPEG    []byte
	Caption string
}

// EmbeddedImage HWPX에 붙일 그림. PNG 사인·JPEG 조치 사진.
type EmbeddedImage struct {
	Data    []byte
	Caption string
	MIME    string
}

type jpegAdd struct {
	id, href, mime string
	blob           []byte
	w, h           int
	caption        string
}

// AppendJPEGs 치환이 끝난 HWPX 끝에 사진을 붙인다. 원본 바이트는 건드리지 않는다.
func AppendJPEGs(doc []byte, photos []JPEGPhoto) ([]byte, error) {
	var imgs []EmbeddedImage
	for _, p := range photos {
		imgs = append(imgs, EmbeddedImage{Data: p.JPEG, Caption: p.Caption, MIME: "image/jpeg"})
	}
	return AppendImages(doc, imgs)
}

// AppendImages JPEG·PNG를 HWPX 끝에 붙인다. 빈 목록이면 원본을 돌려준다.
func AppendImages(doc []byte, photos []EmbeddedImage) ([]byte, error) {
	if len(photos) == 0 {
		return doc, nil
	}
	if len(doc) == 0 {
		return nil, fmt.Errorf("HWPX가 비어 있습니다")
	}
	parts, err := unzipEntries(doc)
	if err != nil {
		return nil, fmt.Errorf("HWPX를 열 수 없습니다: %w", err)
	}

	var added []jpegAdd
	for i, p := range photos {
		if len(p.Data) == 0 {
			continue
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(p.Data))
		if err != nil {
			return nil, fmt.Errorf("보고서 사진 %d를 읽지 못했습니다: %w", i+1, err)
		}
		mime := strings.ToLower(strings.TrimSpace(p.MIME))
		ext := ".jpg"
		if mime == "image/png" || strings.HasPrefix(mime, "image/png") {
			ext = ".png"
			mime = "image/png"
		} else {
			mime = "image/jpeg"
		}
		id := fmt.Sprintf("actionphoto%d", i+1)
		href := "BinData/" + id + ext
		added = append(added, jpegAdd{id: id, href: href, mime: mime, blob: p.Data, w: cfg.Width, h: cfg.Height, caption: p.Caption})
	}
	if len(added) == 0 {
		return doc, nil
	}

	for _, a := range added {
		parts = append(parts, hwpxEntry{Name: "Contents/" + a.href, Body: a.blob})
	}

	lastSec := -1
	for i := range parts {
		n := filepath.ToSlash(parts[i].Name)
		switch {
		case n == "Contents/content.hpf":
			parts[i].Body = injectManifestItems(parts[i].Body, added)
		case n == "META-INF/manifest.xml":
			parts[i].Body = injectODFEntries(parts[i].Body, added)
		case strings.HasPrefix(n, "Contents/section") && strings.HasSuffix(strings.ToLower(n), ".xml"):
			lastSec = i
		}
	}
	if lastSec >= 0 {
		parts[lastSec].Body = appendPhotoParagraphs(parts[lastSec].Body, added)
	}

	packed, err := packHWPX(parts)
	if err != nil {
		return nil, err
	}
	return packed, nil
}

func injectManifestItems(raw []byte, added []jpegAdd) []byte {
	s := string(raw)
	var b strings.Builder
	for _, a := range added {
		b.WriteString(fmt.Sprintf(`    <opf:item id="%s" href="%s" media-type="%s"/>`+"\n", a.id, a.href, a.mime))
	}
	if i := strings.LastIndex(s, "</opf:manifest>"); i >= 0 {
		return []byte(s[:i] + b.String() + s[i:])
	}
	return raw
}

func injectODFEntries(raw []byte, added []jpegAdd) []byte {
	s := string(raw)
	var b strings.Builder
	for _, a := range added {
		b.WriteString(fmt.Sprintf(`  <odf:file-entry odf:full-path="Contents/%s" odf:media-type="%s"/>`+"\n", a.href, a.mime))
	}
	if i := strings.LastIndex(s, "</odf:manifest>"); i >= 0 {
		return []byte(s[:i] + b.String() + s[i:])
	}
	return raw
}

func appendPhotoParagraphs(raw []byte, added []jpegAdd) []byte {
	s := string(raw)
	if !strings.Contains(s, `xmlns:hc=`) {
		s = strings.Replace(s, "<hs:sec", `<hs:sec xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core"`, 1)
	}
	var b strings.Builder
	for _, a := range added {
		wu, hu := hwpSize(a.w, a.h)
		b.WriteString(photoParagraph(a.id, wu, hu, a.caption))
	}
	if i := strings.LastIndex(s, "</hs:sec>"); i >= 0 {
		return []byte(s[:i] + b.String() + s[i:])
	}
	return append(raw, []byte(b.String())...)
}

func hwpSize(w, h int) (int, int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	const maxEdge = 36000 // ~127mm
	wu := w * 75
	hu := h * 75
	if wu >= hu && wu > maxEdge {
		hu = hu * maxEdge / wu
		wu = maxEdge
	} else if hu > wu && hu > maxEdge {
		wu = wu * maxEdge / hu
		hu = maxEdge
	}
	if wu < 1 {
		wu = 1
	}
	if hu < 1 {
		hu = 1
	}
	return wu, hu
}

func photoParagraph(id string, w, h int, caption string) string {
	cap := xmlEscape(sanitizeValue(caption))
	return fmt.Sprintf(`  <hp:p id="0" paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:pic id="0" zOrder="1" numberingType="PICTURE" textWrap="TOP_AND_BOTTOM" textFlow="BOTH_SIDES" lock="0"><hp:offset x="0" y="0"/><hp:orgSz width="%d" height="%d"/><hp:curSz width="%d" height="%d"/><hp:flip horizontal="0" vertical="0"/><hp:rotationInfo angle="0" centerX="0" centerY="0"/><hp:imgRect><hc:pt0 x="0" y="0"/><hc:pt1 x="%d" y="0"/><hc:pt2 x="%d" y="%d"/><hc:pt3 x="0" y="%d"/></hp:imgRect><hp:imgClip left="0" right="0" top="0" bottom="0"/><hp:inMargin left="0" right="0" top="0" bottom="0"/><hp:img binaryItemIDRef="%s" bright="0" contrast="0" effect="REAL_PIC" alpha="0"/></hp:pic></hp:run></hp:p>
  <hp:p id="0" paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>%s</hp:t></hp:run></hp:p>
`, w, h, w, h, w, w, h, h, id, cap)
}
