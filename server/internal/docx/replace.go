package docx

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

// docxLineBreak 줄바꿈 \n 을 Word 줄바꿈으로 나눈다. 표 행 복제는 하지 않는다. §12.10.2 · §12.10.6
const docxLineBreak = `</w:t><w:br/><w:t>`

// Replace 템플릿 ZIP 사본을 메모리에서 만들어 자리표시자를 치환한다. 원본 바이트·파일은 건드리지 않는다. §12.10.6
func Replace(template []byte, values map[string]string) ([]byte, error) {
	if len(template) == 0 {
		return nil, fmt.Errorf("DOCX 템플릿이 비어 있습니다")
	}
	zr, err := zip.NewReader(bytes.NewReader(template), int64(len(template)))
	if err != nil {
		return nil, fmt.Errorf("DOCX 템플릿을 열 수 없습니다: %w", err)
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
		if isXMLName(f.Name) {
			out, err = replaceXML(raw, values)
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
	return strings.HasSuffix(n, ".xml") || strings.HasSuffix(n, ".rels")
}

func replaceXML(data []byte, values map[string]string) ([]byte, error) {
	s := mergeParagraphRuns(string(data))
	s = normalizePlaceholders(s)
	for k, v := range values {
		s = strings.ReplaceAll(s, "{{"+k+"}}", xmlReplacement(v))
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
	return strings.Join(escaped, docxLineBreak)
}

func sanitizeValue(v string) string {
	v = strings.ReplaceAll(v, "\r\n", "\n")
	v = strings.ReplaceAll(v, "\r", "\n")
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

// mergeParagraphRuns 한 문단의 w:t 를 합친다. Word가 {{고객명}} 을 여러 런으로 쪼개도 치환되게 한다. §12.10.6
func mergeParagraphRuns(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		start := indexOpenTag(s, i, "w:p")
		if start < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i:start])
		end := indexCloseTag(s, start, "w:p")
		if end < 0 {
			b.WriteString(s[start:])
			break
		}
		b.WriteString(mergeRunsInPara(s[start:end]))
		i = end
	}
	return b.String()
}

func mergeRunsInPara(para string) string {
	texts := collectWT(para)
	if len(texts) == 0 {
		return para
	}
	var concat strings.Builder
	for _, t := range texts {
		concat.WriteString(t)
	}
	joined := concat.String()
	if !strings.Contains(joined, "{{") {
		return para
	}
	first := true
	return replaceWT(para, func(inner string) string {
		if first {
			first = false
			return joined
		}
		return ""
	})
}

func collectWT(para string) []string {
	var out []string
	i := 0
	for i < len(para) {
		start := indexOpenTag(para, i, "w:t")
		if start < 0 {
			break
		}
		gt := strings.IndexByte(para[start:], '>')
		if gt < 0 {
			break
		}
		innerStart := start + gt + 1
		close := strings.Index(para[innerStart:], "</w:t>")
		if close < 0 {
			break
		}
		out = append(out, para[innerStart:innerStart+close])
		i = innerStart + close + len("</w:t>")
	}
	return out
}

func replaceWT(para string, repl func(string) string) string {
	var b strings.Builder
	i := 0
	for i < len(para) {
		start := indexOpenTag(para, i, "w:t")
		if start < 0 {
			b.WriteString(para[i:])
			break
		}
		gt := strings.IndexByte(para[start:], '>')
		if gt < 0 {
			b.WriteString(para[i:])
			break
		}
		innerStart := start + gt + 1
		close := strings.Index(para[innerStart:], "</w:t>")
		if close < 0 {
			b.WriteString(para[i:])
			break
		}
		b.WriteString(para[i:innerStart])
		b.WriteString(repl(para[innerStart : innerStart+close]))
		b.WriteString("</w:t>")
		i = innerStart + close + len("</w:t>")
	}
	return b.String()
}

func indexOpenTag(s string, from int, local string) int {
	needle := "<" + local
	i := from
	for {
		p := strings.Index(s[i:], needle)
		if p < 0 {
			return -1
		}
		p += i
		n := p + len(needle)
		if n >= len(s) {
			return -1
		}
		c := s[n]
		if c == '>' || c == ' ' || c == '/' || c == '\t' || c == '\n' || c == '\r' {
			return p
		}
		i = p + 1
	}
}

func indexCloseTag(s string, from int, local string) int {
	needle := "</" + local + ">"
	p := strings.Index(s[from:], needle)
	if p < 0 {
		return -1
	}
	return from + p + len(needle)
}

// normalizePlaceholders 한글이 런을 쪼개 {{고</w:t>…<w:t>객명}} 이 되어도 한 토막으로 모은다.
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
