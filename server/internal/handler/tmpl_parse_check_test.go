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
		if _, err := template.New("").Funcs(funcMap()).ParseFiles(files...); err != nil {
			t.Errorf("%s: %v", page, err)
		}
	}
	t.Logf("검사한 템플릿 %d개", len(pages))
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
