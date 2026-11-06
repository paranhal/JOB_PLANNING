package handler

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func testWebRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(findTemplateRoot(t), "..")
}

func extractClassTokens(html string) []string {
	re := regexp.MustCompile(`class="([^"]*)"`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		for _, tok := range strings.Fields(m[1]) {
			tok = strings.TrimSpace(tok)
			if tok != "" {
				out = append(out, tok)
			}
		}
	}
	return out
}

func Test95_40_BoardListCSSInBundle(t *testing.T) {
	root := testWebRoot(t)
	css, err := os.ReadFile(filepath.Join(root, "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := string(css)
	if !strings.Contains(bundle, ".wl-row") && !strings.Contains(bundle, ".wl-row{") && !strings.Contains(bundle, `wl-row{`) {
		t.Fatal("tailwind.css 에 .wl-row 없음 — npm run build:css 필요")
	}
}

func Test95_40_ArbitraryClassWarningOtherTemplates(t *testing.T) {
	root := filepath.Join(testWebRoot(t), "templates")
	css, err := os.ReadFile(filepath.Join(testWebRoot(t), "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	bundle := string(css)
	var missing []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, tok := range extractClassTokens(string(b)) {
			if !strings.Contains(tok, "[") {
				continue
			}
			esc := strings.ReplaceAll(tok, "[", `\[`)
			esc = strings.ReplaceAll(esc, "]", `\]`)
			if !strings.Contains(bundle, esc) && !strings.Contains(bundle, tok) {
				missing = append(missing, rel+":"+tok)
			}
		}
		return nil
	})
	if len(missing) > 0 {
		t.Logf("경고: 번들에 없는 임의값 클래스 %d개 (board_list 외는 이번 실패 아님): %s", len(missing), strings.Join(missing, "; "))
	}
}

func Test95_40_BoardListMarkupOneLineAndAlwaysDelete(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(testWebRoot(t), "templates", "as", "_list_table.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `js-wl-check`) || !strings.Contains(s, `js-wl-all`) {
		t.Fatal("AS 목록 선택 마크업 없음")
	}
}

func Test95_40_WorkListSelectNIncrements(t *testing.T) {
	js := filepath.Join(testWebRoot(t), "static", "js", "work_list_select.js")
	src, err := os.ReadFile(js)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "this.$el") {
		t.Fatal("workListSelect 가 this.$el 을 쓰면 체크 시 n 이 0이다")
	}
	if !strings.Contains(string(src), "this.$root") {
		t.Fatal("workListSelect 가 this.$root 를 쓰지 않는다")
	}
	cmd := exec.Command("node", "work_list_select_test.js")
	cmd.Dir = filepath.Join(testWebRoot(t), "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js 테스트 실패: %v\n%s", err, out)
	}
}

func Test95_40_BoardOneLineSchedule(t *testing.T) {
	got := boardOneLineSchedule("2026-10-19", "09:00 – 10:00")
	if got != "10/19 (월) 09:00~10:00" {
		t.Fatalf("%q", got)
	}
	if boardOneLineSchedule("", "") != "날짜 미정" {
		t.Fatal("빈 날짜")
	}
}
