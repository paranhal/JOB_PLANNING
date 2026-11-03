package handler

import (
	"html"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

var alpineAttrNameRe = regexp.MustCompile(`(?i)^(x-data|x-show|x-cloak|x-if|x-for|@click|:aria-expanded|:class|:aria-hidden)\s*=`)

func firstOpenTag(htmlSrc, name string) (tag string, ok bool) {
	needle := "<" + name
	i := strings.Index(strings.ToLower(htmlSrc), needle)
	if i < 0 {
		return "", false
	}
	end, ok := endOfTag(htmlSrc, i)
	if !ok {
		return "", false
	}
	return htmlSrc[i : end+1], true
}

func endOfTag(s string, start int) (int, bool) {
	quote := byte(0)
	for i := start; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '>' {
			return i, true
		}
	}
	return 0, false
}

func leakedAlpineAttrHits(src string) []string {
	src = stripHTMLComments(src)
	src = stripGoTemplateComments(src)
	var hits []string
	quote := byte(0)
	inTag := false
	i := 0
	for i < len(src) {
		c := src[i]
		if inTag {
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				i++
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				i++
				continue
			}
			if c == '>' {
				inTag = false
			}
			i++
			continue
		}
		if c == '<' {
			inTag = true
			i++
			continue
		}
		if alpineAttrNameRe.MatchString(src[i:]) {
			end := i
			for end < len(src) && src[end] != '\n' && end-i < 80 {
				end++
			}
			hits = append(hits, strings.TrimSpace(src[i:end]))
			i = end
			continue
		}
		i++
	}
	return hits
}

func stripHTMLComments(s string) string {
	for {
		a := strings.Index(s, "<!--")
		if a < 0 {
			return s
		}
		b := strings.Index(s[a+4:], "-->")
		if b < 0 {
			return s[:a]
		}
		s = s[:a] + s[a+4+b+3:]
	}
}

func stripGoTemplateComments(s string) string {
	for {
		a := strings.Index(s, "{{/*")
		if a < 0 {
			return s
		}
		b := strings.Index(s[a+4:], "*/}}")
		if b < 0 {
			return s[:a]
		}
		s = s[:a] + s[a+4+b+4:]
	}
}

func htmlTextContent(s string) string {
	s = stripHTMLComments(s)
	var b strings.Builder
	quote := byte(0)
	inTag := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inTag {
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				continue
			}
			if c == '>' {
				inTag = false
			}
			continue
		}
		if c == '<' {
			inTag = true
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
	}
	return html.UnescapeString(b.String())
}

func TestNavTreeOpeningTagKeepsAlpine(t *testing.T) {
	root := findTemplateRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "layout", "_nav_tree.html"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	tag, ok := firstOpenTag(src, "nav")
	if !ok {
		t.Fatal("<nav 시작 태그가 없다")
	}
	if !strings.Contains(tag, `x-data="navMenu(`) {
		t.Fatalf("nav 시작 태그에 x-data=navMenu 가 없다:\n%s", tag)
	}
	if !strings.Contains(tag, `@click=`) {
		t.Fatalf("nav 시작 태그에 @click 이 없다:\n%s", tag)
	}
	if !strings.Contains(tag, `style="-webkit-overflow-scrolling:touch"`) {
		t.Fatal("nav 시작 태그에 webkit overflow style 이 없다")
	}
	if strings.HasSuffix(strings.TrimSpace(strings.Split(src, "\n")[1]), `text-sm">`) {
		t.Fatal("2번째 줄에서 nav 태그가 닫혀 Alpine 속성이 본문으로 샌다")
	}
}

func TestNavTreeRenderedKeepsAlpineOnNavTag(t *testing.T) {
	e, ids := newUISmokeApp(t)
	rec := uiGET(t, e, "/", []*http.Cookie{jwtCookieUserClaims(t, ids.admin)}, false)
	if rec.Code != 200 {
		t.Fatalf("GET / status=%d body=%s", rec.Code, truncate(rec.Body.String(), 400))
	}
	body := rec.Body.String()
	found := 0
	rest := body
	for {
		tag, ok := firstOpenTag(rest, "nav")
		if !ok {
			break
		}
		if strings.Contains(tag, `x-data="navMenu(`) {
			found++
			if !strings.Contains(tag, `@click=`) {
				t.Fatalf("렌더 nav 태그에 @click 없음:\n%s", tag)
			}
		}
		i := strings.Index(strings.ToLower(rest), "<nav")
		rest = rest[i+4:]
	}
	if found < 1 {
		t.Fatal("렌더 HTML 의 nav 시작 태그에 x-data=\"navMenu( 가 없다")
	}
	text := htmlTextContent(body)
	if strings.Contains(text, "x-data=") || strings.Contains(text, "@click=") {
		t.Fatalf("본문에 Alpine 속성이 글자로 보인다: %q", previewLeak(text))
	}
}

func TestTemplateAlpineAttrsDoNotLeakAsText(t *testing.T) {
	bad := `<nav class="text-sm">
     x-data="navMenu('a')"
     @click="x">`
	if hits := leakedAlpineAttrHits(bad); len(hits) == 0 {
		t.Fatal("잘린 nav 태그를 감지해야 한다")
	}
	good := `<nav class="text-sm" style="-webkit-overflow-scrolling:touch"
     x-data="navMenu('a')"
     @click="x">`
	if hits := leakedAlpineAttrHits(good); len(hits) != 0 {
		t.Fatalf("정상 nav 를 오탐: %v", hits)
	}

	root := findTemplateRoot(t)
	var leaked []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hits := leakedAlpineAttrHits(string(b))
		if len(hits) == 0 {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		leaked = append(leaked, filepath.ToSlash(rel)+": "+strings.Join(hits, " | "))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(leaked) > 0 {
		t.Fatalf("속성이 본문 글자로 샌다:\n%s", strings.Join(leaked, "\n"))
	}
}

func previewLeak(text string) string {
	for _, needle := range []string{"x-data=", "@click="} {
		i := strings.Index(text, needle)
		if i < 0 {
			continue
		}
		a := i
		for a > 0 && !unicode.IsSpace(rune(text[a-1])) {
			a--
		}
		b := i + len(needle)
		if b > len(text) {
			b = len(text)
		}
		if b-a > 60 {
			b = a + 60
		}
		return strings.TrimSpace(text[a:b])
	}
	return ""
}
