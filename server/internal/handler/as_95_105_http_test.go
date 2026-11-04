package handler

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func Test95_105_HiddenActionTakenNotRequired(t *testing.T) {
	root := findTemplateRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "as", "action.html"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	open := regexp.MustCompile(`<textarea[^>]*id="as-action-taken"[^>]*>`).FindString(src)
	if open == "" {
		t.Fatal("as-action-taken 없음")
	}
	if strings.Contains(open, "required") {
		t.Fatalf("숨은 조치내용에 required: %s", open)
	}
	if !strings.Contains(open, "class=\"hidden\"") && !strings.Contains(open, "class='hidden'") && !strings.Contains(open, "hidden") {
		t.Fatalf("as-action-taken 이 hidden 이 아님: %s", open)
	}
	if strings.Contains(src, "ta.required = !on") || strings.Contains(src, "ta.required=!on") {
		t.Fatal("숨은 textarea 에 required 를 다시 붙이는 코드가 있다")
	}
	e, _, _, _, asID := newASActionFixture(t)
	page := getASActionPage(t, e, asID)
	m := regexp.MustCompile(`<textarea[^>]*id="as-action-taken"[^>]*>`).FindString(page)
	if m == "" {
		t.Fatal("렌더에 as-action-taken 없음")
	}
	if strings.Contains(m, "required") {
		t.Fatalf("렌더 숨은 칸 required: %s", m)
	}
}

func Test95_105_JSUnit(t *testing.T) {
	cmd := exec.Command("node", "as_action_sets_test.js")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
}

func Test95_105_BrowserSaveClick(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	cmd := exec.Command("node", "95_105_save_ok.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(out)
		if strings.Contains(msg, "Executable doesn't exist") || (strings.Contains(msg, "playwright") && strings.Contains(msg, "install")) {
			t.Skip(msg)
		}
		t.Fatalf("browser: %v\n%s", err, out)
	}
}

func Test95_105_KeepPostedOnFailStillPresent(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(filepath.Dir(findTemplateRoot(t)), "..", "internal", "handler", "as.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "keepPostedAsDraft") {
		t.Fatal("서버 검증 실패 시 keepPostedAsDraft 가 없다")
	}
}
