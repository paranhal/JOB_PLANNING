package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoDeleteFromUsersLiteral(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "DELETE FROM users") {
			t.Errorf("하드 삭제 남음: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
