package handler

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 모든 화면 HTML 을 ParseFiles 로 검사한다. show.html 의 {{if}}/{{end}} 불일치처럼
// 배포 전 템플릿 파싱 500 을 막기 위한 전수 테스트다.
func TestAllHTMLTemplatesParseFiles(t *testing.T) {
	root := findTemplateRoot(t)
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("web/templates 에 html 이 없다")
	}
	var showOK bool
	for _, path := range files {
		name := filepath.Base(path)
		if _, err := template.New(name).Funcs(funcMap()).ParseFiles(path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(root, "project", "show.html")) {
			showOK = true
		}
	}
	if !showOK {
		t.Fatal("project/show.html 을 검사하지 못했다")
	}
	t.Logf("ParseFiles 전수 %d개", len(files))
}
