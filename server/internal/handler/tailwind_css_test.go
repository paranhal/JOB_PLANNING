package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailwindHasProjectArbitraryGrid(t *testing.T) {
	root := filepath.Dir(findTemplateRoot(t))
	cssPath := filepath.Join(root, "static", "vendor", "tailwind.css")
	b, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("tailwind.css 읽기: %v", err)
	}
	css := string(b)
	for _, want := range []string{
		`lg\:grid-cols-\[1fr_16rem\]`,
		`lg\:grid-cols-\[1fr_18rem\]`,
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("tailwind.css 에 %s 없음. npm run build:css 를 다시 돌리세요", want)
		}
	}
}

func TestProjectSampleHelpersRemoved(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Contains(s, "func sampleProject") || strings.Contains(s, "func sampleWorkMgmt") {
			t.Fatalf("%s 에 샘플 함수가 남아 있다", e.Name())
		}
	}
}
