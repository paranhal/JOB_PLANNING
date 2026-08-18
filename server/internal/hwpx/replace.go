package hwpx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// ErrLeftoverPlaceholder 치환 뒤 {{ 가 남으면 고객에게 나가지 않게 막는다. §12.10.6
var ErrLeftoverPlaceholder = errors.New("자리표시자가 남아 있습니다")

// hwpxParagraphBreak 줄바꿈 \n 을 새 문단으로 나눈다. 표 행 복제는 하지 않는다. §12.10.2 · §12.10.6
const hwpxParagraphBreak = `</hp:t></hp:run></hp:p><hp:p paraPrIDRef="0" styleIDRef="0"><hp:run charPrIDRef="0"><hp:t>`

// Replace 템플릿 ZIP 사본을 메모리에서 만들어 자리표시자를 치환한다. 원본 바이트·파일은 건드리지 않는다. §12.10.6
func Replace(template []byte, values map[string]string) ([]byte, error) {
	if len(template) == 0 {
		return nil, fmt.Errorf("HWPX 템플릿이 비어 있습니다")
	}
	zr, err := zip.NewReader(bytes.NewReader(template), int64(len(template)))
	if err != nil {
		return nil, fmt.Errorf("HWPX 템플릿을 열 수 없습니다: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("%s 를 읽지 못했습니다: %w", f.Name, err)
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			_ = zw.Close()
			return nil, err
		}

		out := raw
		switch {
		case isXMLName(f.Name):
			out, err = replaceXML(raw, values)
			if err != nil {
				_ = zw.Close()
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
		case isPlainPreview(f.Name):
			out, err = replacePlain(raw, values)
			if err != nil {
				_ = zw.Close()
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
		}

		method := f.Method
		if f.Name == "mimetype" {
			method = zip.Store
		}
		hdr := &zip.FileHeader{Name: f.Name, Method: method}
		if !f.Modified.IsZero() {
			hdr.SetModTime(f.Modified)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := w.Write(out); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isXMLName(name string) bool {
	n := strings.ToLower(filepath.ToSlash(name))
	return strings.HasSuffix(n, ".xml") || strings.HasSuffix(n, ".hpf")
}

func isPlainPreview(name string) bool {
	n := strings.ToLower(filepath.ToSlash(name))
	return strings.HasSuffix(n, ".txt")
}

func replaceXML(data []byte, values map[string]string) ([]byte, error) {
	s := normalizePlaceholders(string(data))
	for k, v := range values {
		s = strings.ReplaceAll(s, "{{"+k+"}}", xmlReplacement(v))
	}
	if i := strings.Index(s, "{{"); i >= 0 {
		return nil, fmt.Errorf("%w: %s", ErrLeftoverPlaceholder, snippet(s, i))
	}
	return []byte(s), nil
}

func replacePlain(data []byte, values map[string]string) ([]byte, error) {
	s := string(data)
	for k, v := range values {
		s = strings.ReplaceAll(s, "{{"+k+"}}", plainReplacement(v))
	}
	if i := strings.Index(s, "{{"); i >= 0 {
		return nil, fmt.Errorf("%w: %s", ErrLeftoverPlaceholder, snippet(s, i))
	}
	return []byte(s), nil
}

func xmlReplacement(v string) string {
	parts := splitLines(sanitizeValue(v))
	escaped := make([]string, len(parts))
	for i, p := range parts {
		escaped[i] = xmlEscape(p)
	}
	return strings.Join(escaped, hwpxParagraphBreak)
}

func plainReplacement(v string) string {
	return strings.Join(splitLines(sanitizeValue(v)), "\n")
}

func sanitizeValue(v string) string {
	v = strings.ReplaceAll(v, "\r\n", "\n")
	v = strings.ReplaceAll(v, "\r", "\n")
	// 값에 {{ 가 있어도 자리표시자 잔여 검사와 섞이지 않게 한다.
	return strings.ReplaceAll(v, "{{", "｛｛")
}

func splitLines(v string) []string {
	if v == "" {
		return []string{""}
	}
	return strings.Split(v, "\n")
}

func xmlEscape(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
	}
	return b.String()
}

// normalizePlaceholders 한글이 런을 쪼개 {{고</hp:t>…<hp:t>객명}} 이 되어도 한 토막으로 모은다.
func normalizePlaceholders(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '{' && s[i+1] == '{' {
			j := i + 2
			var name strings.Builder
			found := false
			for j < len(s) {
				if j+1 < len(s) && s[j] == '}' && s[j+1] == '}' {
					b.WriteString("{{")
					b.WriteString(name.String())
					b.WriteString("}}")
					i = j + 2
					found = true
					break
				}
				if s[j] == '<' {
					k := strings.IndexByte(s[j:], '>')
					if k < 0 {
						b.WriteString(s[i:])
						return b.String()
					}
					j += k + 1
					continue
				}
				name.WriteByte(s[j])
				j++
			}
			if !found {
				b.WriteString(s[i:])
				return b.String()
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func snippet(s string, i int) string {
	end := i + 40
	if end > len(s) {
		end = len(s)
	}
	return strings.ReplaceAll(s[i:end], "\n", " ")
}
