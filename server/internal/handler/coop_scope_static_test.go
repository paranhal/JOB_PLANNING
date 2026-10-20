package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoopOrgScopeStaticAudit(t *testing.T) {
	root := filepath.Join("..", "repository")
	if _, err := os.Stat(filepath.Join(root, "coop_repo.go")); err != nil {
		root = "internal/repository"
	}
	b, err := os.ReadFile(filepath.Join(root, "coop_repo.go"))
	if err != nil {
		t.Fatal(err)
	}
	need := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "coop-scope:") {
			need++
		}
	}
	if need < 3 {
		t.Fatalf("협조 조회 사유 주석 %d", need)
	}
}
