package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readServerMain(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "cmd", "server", "main.go"),
		filepath.Join("cmd", "server", "main.go"),
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b)
		}
	}
	t.Fatal("cmd/server/main.go 를 읽지 못했다")
	return ""
}

func navLeafPaths() []string {
	seen := map[string]bool{}
	var out []string
	var walk func([]navDefItem)
	walk = func(items []navDefItem) {
		for _, it := range items {
			if len(it.Children) > 0 {
				walk(it.Children)
				continue
			}
			h := strings.TrimSpace(it.Href)
			if h == "" {
				continue
			}
			if i := strings.Index(h, "?"); i >= 0 {
				h = h[:i]
			}
			if seen[h] {
				continue
			}
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, g := range navDefinition() {
		walk(g.Items)
	}
	for _, g := range execNavDefinition(sampleExecOrgs()) {
		walk(g.Items)
	}
	return out
}

var (
	reGroupAssign = regexp.MustCompile(`^(\w+)\s*:?=\s*(\w+)\.Group\("([^"]*)"`)
	reGETCall     = regexp.MustCompile(`^(\w+)\.GET\("([^"]*)"`)
)

func mainGETPaths(src string) map[string]bool {
	out := map[string]bool{}
	prefix := map[string]string{"e": "", "g": ""}
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if m := reGroupAssign.FindStringSubmatch(line); m != nil {
			prefix[m[1]] = prefix[m[2]] + m[3]
			continue
		}
		if m := reGETCall.FindStringSubmatch(line); m != nil {
			out[prefix[m[1]]+m[2]] = true
		}
	}
	return out
}

func TestNavHrefsHaveGETRoutesInMain(t *testing.T) {
	src := readServerMain(t)
	got := mainGETPaths(src)
	for _, path := range navLeafPaths() {
		if !mainHasGET(got, path) {
			t.Errorf("메뉴 %s 에 대응하는 GET 라우트가 main.go 에 없다", path)
		}
	}
}

func mainHasGET(got map[string]bool, path string) bool {
	if got[path] {
		return true
	}
	if strings.HasPrefix(path, "/exec/org/") {
		return got["/exec/org/:id"]
	}
	return false
}
