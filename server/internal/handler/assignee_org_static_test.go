package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestAssignableOrgStaticAudit(t *testing.T) {
	root := "."
	if _, err := os.Stat("assignee_org.go"); err != nil {
		root = "internal/handler"
	}
	call0 := regexp.MustCompile(`\.(ListAssignable|FindAssignable)\(\s*\)`)
	emptyArg := regexp.MustCompile(`\.(ListAssignable|FindAssignable)\(\s*""`)
	findCall := regexp.MustCompile(`\.FindAssignable\(([^)]*)\)`)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		base := filepath.Base(path)
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, "func (r *UserRepo) ListAssignable") || strings.Contains(line, "func (r *UserRepo) FindAssignable") {
				continue
			}
			if call0.MatchString(line) {
				t.Errorf("%s:%d 인자 없는 호출: %s", base, i+1, strings.TrimSpace(line))
			}
			if emptyArg.MatchString(line) {
				t.Errorf("%s:%d 빈 조직 인자: %s", base, i+1, strings.TrimSpace(line))
			}
			if m := findCall.FindStringSubmatch(line); len(m) > 1 && !strings.Contains(m[1], ",") && !strings.Contains(m[1], "assignableOrgID") {
				t.Errorf("%s:%d FindAssignable 조직 인자 없음: %s", base, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Join("..", "repository")
	if _, err := os.Stat(filepath.Join(repoRoot, "user_repo.go")); err == nil {
		b, _ := os.ReadFile(filepath.Join(repoRoot, "user_repo.go"))
		if strings.Contains(string(b), "func (r *UserRepo) ListAssignable()") {
			t.Error("ListAssignable 0-arg 정의가 남아 있음")
		}
	}
	_ = filepath.WalkDir(filepath.Join(".."), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, "func (r *UserRepo)") {
				continue
			}
			if call0.MatchString(line) {
				t.Errorf("%s:%d 인자 없는 호출: %s", path, i+1, strings.TrimSpace(line))
			}
			if emptyArg.MatchString(line) {
				t.Errorf("%s:%d 빈 조직 인자: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
}
