package handler

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// uiComponentClasses §3.1 @layer components 클래스. 템플릿 사용 여부와 무관하게 빌드 CSS에 있어야 한다.
var uiComponentClasses = []string{
	"btn", "btn-primary", "btn-secondary", "btn-danger", "btn-ghost", "btn-sm",
	"input", "select", "textarea", "is-invalid", "label", "field-hint", "field-error",
	"card", "card-header", "card-body",
	"badge", "badge-success", "badge-warning", "badge-danger", "badge-info", "badge-neutral",
	"badge-target-a", "badge-target-b", "badge-target-c", "badge-overdue",
	"table-base", "list-row", "list-row-selected",
	"flash", "flash-success", "flash-warning", "flash-danger",
	"tab", "tab-active",
	"kbd",
	"avatar", "avatar-sm",
	"menu", "menu-item",
	"modal-backdrop", "modal-panel", "peek-panel",
	"stepper", "stepper-step", "stepper-step-done", "stepper-step-current", "stepper-step-pending", "stepper-step-rejected",
	"toast",
	"sidebar-section", "sidebar-item", "sidebar-item-active", "sidebar-count",
}

var uiTokenClassRe = regexp.MustCompile(`\b(?:bg|text|border|ring|from|to|divide|placeholder:text|hover:bg|hover:text|focus:border|focus:ring)-(?:canvas|surface|hover|selected|sidebar|sidebar-text|line|line-strong|ink|ink-muted|ink-subtle|primary|primary-hover|primary-soft|primary-fg|success|success-soft|warning|warning-soft|danger|danger-soft|info|info-soft|target-a|target-b|target-c|overdue)\b`)

func readBuiltTailwindCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("web", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func cssHasClass(css, name string) bool {
	esc := regexp.QuoteMeta(name)
	re := regexp.MustCompile(`\.` + esc + `(?:\\[0-9a-zA-Z]+)?(?:\{|,|:|\s)`)
	return re.MatchString(css)
}

func TestComponentClassesExistInBuiltCSS(t *testing.T) {
	css := readBuiltTailwindCSS(t)
	for _, name := range uiComponentClasses {
		if !cssHasClass(css, name) && !strings.Contains(css, "."+name+"{") && !strings.Contains(css, "."+name+",") {
			t.Errorf("빌드된 tailwind.css에 .%s 없음", name)
		}
	}

	root := findTemplateRoot(t)
	classSet := map[string]struct{}{}
	for _, n := range uiComponentClasses {
		classSet[n] = struct{}{}
	}
	wordRe := regexp.MustCompile(`\b[a-z][a-z0-9:-]*\b`)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		body := string(b)
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for _, tok := range uiTokenClassRe.FindAllString(body, -1) {
			if !strings.Contains(css, strings.TrimPrefix(strings.TrimPrefix(tok, "hover:"), "focus:")) &&
				!strings.Contains(css, tok) {
				// 유틸은 접두가 이스케이프될 수 있어 토큰 조각만 확인
				base := tok
				if i := strings.LastIndex(base, ":"); i >= 0 {
					base = base[i+1:]
				}
				if !strings.Contains(css, base) {
					t.Errorf("%s: 토큰 클래스 %q 가 tailwind.css에 없음", rel, tok)
				}
			}
		}
		for _, w := range wordRe.FindAllString(body, -1) {
			if _, ok := classSet[w]; !ok {
				continue
			}
			if !cssHasClass(css, w) && !strings.Contains(css, "."+w+"{") && !strings.Contains(css, "."+w+",") {
				t.Errorf("%s: 컴포넌트 클래스 %q 가 tailwind.css에 없음", rel, w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPretendardSelfHosted(t *testing.T) {
	woff := filepath.Join("web", "static", "fonts", "PretendardVariable.woff2")
	st, err := os.Stat(woff)
	if err != nil {
		t.Fatalf("Pretendard woff2 없음: %v", err)
	}
	if st.Size() < 100_000 {
		t.Fatalf("Pretendard woff2 가 너무 작음: %d", st.Size())
	}
	b, err := os.ReadFile(woff)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:4]) != "wOF2" {
		t.Fatal("Pretendard 파일이 woff2 가 아님")
	}
	if _, err := os.Stat(filepath.Join("web", "static", "fonts", "OFL.txt")); err != nil {
		t.Fatalf("OFL 라이선스 파일 없음: %v", err)
	}
	css, err := os.ReadFile(filepath.Join("web", "css", "input.css"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	if !strings.Contains(s, "@font-face") || !strings.Contains(s, "Pretendard Variable") {
		t.Fatal("input.css 에 Pretendard @font-face 없음")
	}
	if strings.Contains(s, "fonts.googleapis.com") || strings.Contains(s, "fonts.gstatic.com") {
		t.Fatal("input.css 가 웹폰트 CDN을 참조함")
	}
	built := readBuiltTailwindCSS(t)
	if !strings.Contains(built, "Pretendard Variable") {
		t.Fatal("빌드된 CSS에 Pretendard Variable 없음")
	}
}
