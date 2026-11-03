package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewTestFilesDoNotCallTimeNow(t *testing.T) {
	root := findServerRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "testdata", "now_direct_allowlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	allow := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		allow[filepath.ToSlash(line)] = true
	}
	var bad []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "tmp" || base == "vendor" || base == "data" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if allow[rel] || rel == "internal/handler/now_direct_test.go" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "time."+"Now(") {
			bad = append(bad, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("신규 테스트에서 time.Now 직접 호출 (appclock 을 쓰세요): %s", strings.Join(bad, ", "))
	}
}

func findServerRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod 를 찾지 못했다")
	return ""
}
