package handler

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 템플릿 구문 오류로 인한 500을 조기에 잡는다.
func TestAllTemplatesParse(t *testing.T) {
	root := findTemplateRoot(t)
	layout := filepath.Join(root, "layout", "base.html")

	var pages []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		if strings.HasPrefix(info.Name(), "_") || strings.Contains(path, "layout") {
			return nil
		}
		pages = append(pages, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("검사할 템플릿을 찾지 못했습니다")
	}

	for _, page := range pages {
		files := []string{layout, page}
		if partials, err := filepath.Glob(filepath.Join(filepath.Dir(page), "_*.html")); err == nil {
			files = append(files, partials...)
		}
		if kpartials, err := filepath.Glob(filepath.Join(root, "kanban", "_*.html")); err == nil {
			files = append(files, kpartials...)
		}
		if strings.Contains(filepath.ToSlash(page), "/admin_work/") {
			files = append(files, filepath.Join(root, "workboard", "_recurrence_fields.html"))
		}
		if _, err := template.New("").Funcs(funcMap()).ParseFiles(files...); err != nil {
			t.Errorf("%s: %v", page, err)
		}
	}
	t.Logf("검사한 템플릿 %d개", len(pages))
}

// 영업 상세에서 활동 패널이 x-data 밖으로 나가면 window.open 을 집어 화면이 잠긴다.
func TestSalesActivityPanelStaysInsideXData(t *testing.T) {
	root := findTemplateRoot(t)
	rels := []string{filepath.Join("sales", "show.html")}
	if _, err := os.Stat(filepath.Join(root, "sales", "activities.html")); err == nil {
		rels = append(rels, filepath.Join("sales", "activities.html"))
	}
	for _, rel := range rels {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		script := strings.Index(src, "<script>")
		if script < 0 {
			t.Fatalf("%s: <script> 없음", rel)
		}
		panel := strings.Index(src, `{{template "sales_activity_panel"`)
		if panel < 0 {
			t.Fatalf("%s: sales_activity_panel 없음", rel)
		}
		closeDiv := strings.LastIndex(src[:script], "</div>")
		if panel > closeDiv {
			t.Fatalf("%s: sales_activity_panel 이 x-data 닫는 </div> 밖에 있다", rel)
		}
	}
}

func TestFixedInsetAlpineOverlaysDoNotUseBareOpen(t *testing.T) {
	root := findTemplateRoot(t)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(b), "\n")
		for i := range lines {
			chunk := lines[i]
			if i+1 < len(lines) {
				chunk += " " + lines[i+1]
			}
			if !strings.Contains(chunk, "inset-0") || !strings.Contains(chunk, "fixed") {
				continue
			}
			if strings.Contains(chunk, `x-show="open"`) || strings.Contains(chunk, `x-show='open'`) {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s:%d fixed inset-0 이 x-show=\"open\" 이다. window.open 을 집는다", filepath.ToSlash(rel), i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func findTemplateRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, "web", "templates")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("web/templates 디렉터리를 찾을 수 없습니다")
	return ""
}
