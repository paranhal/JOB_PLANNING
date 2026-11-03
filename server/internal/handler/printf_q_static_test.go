package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintfQNotUsedInTemplates(t *testing.T) {
	root := findTemplateRoot(t)
	var hits []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
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
		if strings.Contains(string(b), `printf "%q"`) {
			rel, _ := filepath.Rel(root, path)
			hits = append(hits, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("JS 컨텍스트 printf %%q 재발:\n%s", strings.Join(hits, "\n"))
	}
}
