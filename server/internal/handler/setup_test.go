package handler

import (
	"html/template"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// 모듈 테스트 시 html/template 이 web/templates 를 찾을 수 있도록 작업 디렉터리를 server 루트로 맞춘다.
func TestMain(m *testing.M) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	if err := os.Chdir(root); err != nil {
		panic(err)
	}
	testSharedRenderer = &TemplateRenderer{cache: map[string]*template.Template{}}
	os.Exit(m.Run())
}
