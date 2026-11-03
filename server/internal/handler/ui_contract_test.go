package handler

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// 95-96 브리지와 같은 AS 조치 접두어 대응. 서버는 s{N}_이름, 화면은 {{$p}}이름.
var asActionPrefixFields = []string{
	"set_id", "assigned_to", "assigned_user_id",
	"work_place", "process_type", "process_type_reason",
	"cause_cat1", "cause_cat1_custom", "cause_cat2", "cause_cat3", "cause_detail",
	"work_date", "start_time", "duration_min", "leave_same", "leave_time",
	"action_taken", "action_na", "action_na_reason", "result_code",
}

var alpineFnAllow = map[string]bool{
	"itemCatalog": true, "salesProjectForm": true, "quoteForm": true,
	"Object": true, "assign": true,
}

var getElementByIdAllow = map[string]bool{
	"board-m-kind": true, "board-m-ref": true, "board-m-title": true, "board-m-assignee": true,
	"board-m-from-col": true, "board-m-dur": true, "board-m-date": true, "board-m-start": true, "board-m-end": true,
	"board-unlock": true, "pw-assignee-code": true,
	"as-set-toast": true, // JS가 없으면 만들어서 붙임
}

var xDataNameRe = regexp.MustCompile(`x-data="([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
var getByIDRe = regexp.MustCompile(`getElementById\(\s*['"]([^'"]+)['"]\s*\)`)
var alpineLeakRe = regexp.MustCompile(`(?i)(x-data=|@click=|x-show=|x-cloak)`)

func TestUIContractBrokenNavFailsFirst(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("internal", "handler", "testdata", "ui_contract_broken_nav.html"))
	if err != nil {
		t.Fatal(err)
	}
	leaks := htmlTextAlpineLeaks(string(raw))
	if len(leaks) == 0 {
		t.Fatal("사이드바 수정 전(잘린 </nav> 앞 >) 코드에서 x-data=/@click= 본문 누출을 잡아야 한다")
	}
}

func TestUIContractRenderedPages(t *testing.T) {
	e, ids := newUISmokeApp(t)
	admin := jwtCookieUserClaims(t, ids.admin)
	pages := []string{"/", "/login", "/as", "/as/" + ids.asID + "/action", "/work", "/customers", "/quotes"}
	for _, p := range pages {
		p := p
		t.Run(p, func(t *testing.T) {
			rec := uiGET(t, e, p, []*http.Cookie{admin}, false)
			if rec.Code >= 500 {
				t.Fatalf("HTTP %d", rec.Code)
			}
			body := rec.Body.String()
			if rec.Code == 200 {
				if _, err := html.Parse(strings.NewReader(body)); err != nil {
					t.Fatalf("html parse: %v", err)
				}
				if leaks := htmlTextAlpineLeaks(body); len(leaks) > 0 {
					t.Fatalf("본문에 Alpine 글자: %v", leaks)
				}
			}
		})
	}
}

func TestUIContractAlpineFunctionsExist(t *testing.T) {
	t.Parallel()
	root := findTemplateRoot(t)
	src := readTree(t, root, ".html") + readTree(t, filepath.Join(root, "..", "static", "js"), ".js")
	seen := map[string]bool{}
	var missing []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, _ := os.ReadFile(path)
		for _, m := range xDataNameRe.FindAllStringSubmatch(string(b), -1) {
			name := m[1]
			if alpineFnAllow[name] || seen[name] {
				continue
			}
			seen[name] = true
			if !alpineFnDefined(src, name) {
				rel, _ := filepath.Rel(root, path)
				missing = append(missing, filepath.ToSlash(rel)+": "+name)
			}
		}
		return nil
	})
	if len(missing) > 0 {
		t.Fatalf("x-data 함수 정의 없음: %s", strings.Join(missing, ", "))
	}
}

func TestUIContractGetElementById(t *testing.T) {
	t.Parallel()
	root := findTemplateRoot(t)
	htmlAll := readTree(t, root, ".html")
	jsDir := filepath.Join(root, "..", "static", "js")
	var missing []string
	_ = filepath.WalkDir(jsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".js") || strings.HasSuffix(path, "_test.js") {
			return err
		}
		b, _ := os.ReadFile(path)
		for _, m := range getByIDRe.FindAllStringSubmatch(string(b), -1) {
			id := m[1]
			if getElementByIdAllow[id] {
				continue
			}
			if !strings.Contains(htmlAll, `id="`+id+`"`) && !strings.Contains(htmlAll, `id='`+id+`'`) {
				missing = append(missing, filepath.Base(path)+": "+id)
			}
		}
		return nil
	})
	if len(missing) > 0 {
		t.Fatalf("getElementById id 없음: %s", strings.Join(missing, ", "))
	}
}

func TestUIContractASActionPrefixFields(t *testing.T) {
	t.Parallel()
	root := findTemplateRoot(t)
	tpl := readFileMust(t, filepath.Join(root, "as", "_action_set.html")) +
		readFileMust(t, filepath.Join(root, "as", "_cause_cat1.html")) +
		readFileMust(t, filepath.Join(root, "as", "_action_work_time.html"))
	var missing []string
	for _, f := range asActionPrefixFields {
		if !strings.Contains(tpl, "{{$p}}"+f) && !strings.Contains(tpl, namePrefixField(f)) {
			missing = append(missing, f)
		}
		if _, ok := actionSetOwnedFields[f]; ok {
			if !strings.Contains(tpl, "{{$p}}"+f) {
				missing = append(missing, "bridge:"+f)
			}
		}
	}
	if len(missing) > 0 {
		t.Fatalf("s{N}_ 대응 필드 없음: %s", strings.Join(missing, ", "))
	}
	if len(actionSetOwnedFields) == 0 {
		t.Fatal("actionSetOwnedFields 비어 있음")
	}
}

func TestUIContractTemplatesDefined(t *testing.T) {
	t.Parallel()
	TestAllTemplatesParse(t)
}

func htmlTextAlpineLeaks(body string) []string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return []string{"parse:" + err.Error()}
	}
	var out []string
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, skip bool) {
		if n.Type == html.ElementNode {
			d := strings.ToLower(n.Data)
			if d == "script" || d == "style" {
				skip = true
			}
		}
		if n.Type == html.TextNode && !skip {
			txt := strings.TrimSpace(n.Data)
			if txt != "" && alpineLeakRe.MatchString(txt) {
				if len(txt) > 80 {
					txt = txt[:80]
				}
				out = append(out, txt)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, skip)
		}
	}
	walk(doc, false)
	return out
}

func alpineFnDefined(src, name string) bool {
	needles := []string{
		"function " + name, "window." + name + " =", "window." + name + "=",
		name + " = function", name + "=function", "const " + name + " =",
	}
	for _, n := range needles {
		if strings.Contains(src, n) {
			return true
		}
	}
	return false
}

func readTree(t *testing.T, root, suffix string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, suffix) {
			return err
		}
		raw, err := os.ReadFile(path)
		if err == nil {
			b.Write(raw)
			b.WriteByte('\n')
		}
		return nil
	})
	return b.String()
}

func readFileMust(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func namePrefixField(f string) string {
	return `name="{{$p}}` + f + `"`
}
