package handler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 95-55A 수정 전 openEdit: $nextTick 콜백을 })); 로 닫아 괄호가 하나 더 있음.
const alpineOpenEditBeforeFix = `{
  openEdit(el) {
    const d = el && el.dataset ? el.dataset : {};
    this.$nextTick(() => {
      this.$dispatch('mnt-picker-hydrate', {
      target: 'edit',
      ids: [],
      owner: d.owner || '',
      groups: d.groups || d.group || '',
      group: d.group || ''
    }));
  }
}`

var alpineAttrRe = regexp.MustCompile(`(?is)\s(?:x-data|x-init|x-show|x-text|x-html|x-effect|x-model(?:\.[a-z]+)*|x-bind:[a-zA-Z0-9_.:-]+|x-on:[a-zA-Z0-9_.:-]+|@[a-zA-Z]+(?:\.[a-zA-Z0-9-]+)*|:[a-zA-Z][\w.:-]*)\s*=\s*"((?:\\.|[^"\\])*)"`)

var alpineGoTplRe = regexp.MustCompile(`\{\{[\s\S]*?\}\}`)

func Test95_55AlpineOpenEditBeforeFixFails(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node 없음")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "openedit-before.js")
	if err := os.WriteFile(path, []byte("void ("+alpineOpenEditBeforeFix+");\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--check", path)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("수정 전 코드가 통과하면 안 됩니다: %s", out)
	}
	logPath := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "95-55-alpine-openedit-before-fail.log")
	body := "node --check openedit-before.js\n" + string(out) + "\nerr=" + err.Error() + "\n"
	if werr := os.WriteFile(logPath, []byte(body), 0o644); werr != nil {
		t.Logf("로그 기록 실패: %v", werr)
	}
	t.Logf("수정 전 실패 로그:\n%s", body)
}

func Test95_55AlpineSyntaxAllTemplates(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node 없음")
	}
	root := findTemplateRoot(t)
	var files []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".html") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("템플릿 없음")
	}
	var b strings.Builder
	n := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		attrHeads := alpineAttrRe.FindAllString(src, -1)
		matches := alpineAttrRe.FindAllStringSubmatch(src, -1)
		for i, m := range matches {
			if len(m) < 2 {
				continue
			}
			head := ""
			if i < len(attrHeads) {
				head = strings.ToLower(attrHeads[i])
			}
			if strings.Contains(head, "x-for=") {
				continue
			}
			rawCode := m[1]
			if strings.Contains(rawCode, "{{if") || strings.Contains(rawCode, "{{range") || strings.Contains(rawCode, "{{end") {
				continue
			}
			code := alpineGoTplRe.ReplaceAllString(rawCode, "tpl")
			code = strings.ReplaceAll(code, "&quot;", `"`)
			code = strings.ReplaceAll(code, "&#34;", `"`)
			code = strings.ReplaceAll(code, "&amp;", "&")
			code = strings.TrimSpace(code)
			if code == "" || strings.Contains(code, "{{") {
				continue
			}
			n++
			rel, _ := filepath.Rel(root, f)
			fmt.Fprintf(&b, "\n/* %s #%d */\n", rel, n)
			if strings.Contains(head, "x-data=") || strings.HasPrefix(code, "{") {
				fmt.Fprintf(&b, "void (%s);\n", code)
			} else {
				fmt.Fprintf(&b, "void (() => { %s\n});\n", code)
			}
		}
	}
	tmp := filepath.Join(t.TempDir(), "alpine-all.js")
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "--check", tmp).CombinedOutput()
	if err != nil {
		t.Fatalf("Alpine 문법 오류 (%d식)\n%s", n, out)
	}
}

func Test95_55PlanShowOpenEditParensFixed(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "maintenance", "plan_show.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "openEdit(el)") {
		t.Fatal("openEdit 없음")
	}
	idx := strings.Index(s, "this.$dispatch('mnt-picker-hydrate'")
	if idx < 0 {
		t.Fatal("hydrate dispatch 없음")
	}
	chunk := s[idx:]
	if len(chunk) > 500 {
		chunk = chunk[:500]
	}
	if strings.Contains(chunk, "}));") {
		t.Fatalf("openEdit 괄호 오류가 남아 있습니다: %s", chunk)
	}
}
