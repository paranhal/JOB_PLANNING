package handler

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const uiLintBaselinePath = "internal/handler/testdata/ui_lint_baseline.json"

var (
	uiRawColorRe = regexp.MustCompile(`\b(bg|text|border|ring|from|to|divide)-(gray|slate|zinc|blue|red|green|amber|yellow|violet|purple|indigo|orange|emerald|teal|sky|rose|pink)-\d{2,3}\b`)
	uiHexRe      = regexp.MustCompile(`(?i)#(?:[0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})\b`)
	uiInlineRe   = regexp.MustCompile(`(?i)\bstyle\s*=`)
	// 사이드바 <nav> 시작 태그에 필요한 iOS 스크롤. 색 토큰이 아니다.
	uiWebkitOverflowStyleRe = regexp.MustCompile(`(?i)\s*\bstyle\s*=\s*["']-webkit-overflow-scrolling:\s*touch["']`)
	uiStyleTagRe            = regexp.MustCompile(`(?i)<style\b`)
	// 95-210: <a ...> 시작 태그, class 안의 underline / hover:underline (no-underline 은 제외), 허용 표식
	uiAnchorRe        = regexp.MustCompile(`(?is)<a\b[^>]*>`)
	uiUnderlineClsRe  = regexp.MustCompile(`(?is)\bclass\s*=\s*"(?:[^"]*[\s:])?underline\b[^"]*"`)
	uiLinkTextAllowRe = regexp.MustCompile(`(?i)\bdata-link\s*=\s*"text"`)
	uiCDNNeedles      = []string{
		"cdn.tailwindcss.com",
		"unpkg.com",
		"cdn.jsdelivr.net",
		"cdnjs.cloudflare.com",
		"googleapis.com",
		"fonts.gstatic.com",
		"fonts.google.com",
	}
)

type uiLintCounts struct {
	RawColor    int `json:"raw_color"`
	Hex         int `json:"hex"`
	InlineStyle int `json:"inline_style"`
	StyleTag    int `json:"style_tag"`
	CDN         int `json:"cdn"`
	// 95-210: 밑줄 글자 링크(독립된 행동·이동은 btn-* 버튼으로). 표 안 이름·문장 속 링크처럼 링크로 남기는 것은
	// data-link="text" 를 붙여 허용한다. 기존 화면은 baseline 으로 더 늘지 않게만 막는다.
	LinkUnderline int `json:"link_underline"`
}

func (c uiLintCounts) total() int {
	return c.RawColor + c.Hex + c.InlineStyle + c.StyleTag + c.CDN
}

func (c uiLintCounts) zero() bool {
	return c.total() == 0
}

// countLinkUnderline 밑줄 클래스가 붙은 <a> 수. data-link="text" 는 링크로 남기는 종류라 센다고 하지 않는다.
func countLinkUnderline(body string) int {
	n := 0
	for _, tag := range uiAnchorRe.FindAllString(body, -1) {
		if uiLinkTextAllowRe.MatchString(tag) {
			continue
		}
		if uiUnderlineClsRe.MatchString(tag) {
			n++
		}
	}
	return n
}

func countUILint(body string) uiLintCounts {
	cdn := 0
	for _, n := range uiCDNNeedles {
		cdn += strings.Count(body, n)
	}
	stripped := uiWebkitOverflowStyleRe.ReplaceAllString(body, "")
	return uiLintCounts{
		RawColor:      len(uiRawColorRe.FindAllString(body, -1)),
		Hex:           len(uiHexRe.FindAllString(body, -1)),
		InlineStyle:   len(uiInlineRe.FindAllString(stripped, -1)),
		StyleTag:      len(uiStyleTagRe.FindAllString(body, -1)),
		CDN:           cdn,
		LinkUnderline: countLinkUnderline(body),
	}
}

func scanTemplateUILint(t *testing.T) map[string]uiLintCounts {
	t.Helper()
	root := findTemplateRoot(t)
	out := map[string]uiLintCounts{}
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
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = countUILint(string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mergeUILintBaselineDecreaseOnly(baseline, actual map[string]uiLintCounts) (map[string]uiLintCounts, []string) {
	out := map[string]uiLintCounts{}
	var increased []string
	for rel, got := range actual {
		want, ok := baseline[rel]
		if !ok {
			if got.zero() && got.LinkUnderline == 0 {
				out[rel] = got
			} else {
				increased = append(increased, rel+": 신규 템플릿 위반")
			}
			continue
		}
		if got.total() > want.total() || got.LinkUnderline > want.LinkUnderline {
			increased = append(increased, rel)
			out[rel] = want
			continue
		}
		out[rel] = got
	}
	return out, increased
}

func writeUILintBaseline(t *testing.T, data map[string]uiLintCounts) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(uiLintBaselinePath), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uiLintBaselinePath, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUILintRatchet(t *testing.T) {
	actual := scanTemplateUILint(t)
	if os.Getenv("UPDATE_UI_LINT_BASELINE") == "1" {
		raw, err := os.ReadFile(uiLintBaselinePath)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		baseline := map[string]uiLintCounts{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &baseline); err != nil {
				t.Fatal(err)
			}
		}
		merged, increased := mergeUILintBaselineDecreaseOnly(baseline, actual)
		if len(increased) > 0 {
			t.Fatalf("UI 린트 증가가 있어 baseline 을 갱신하지 않습니다: %s", strings.Join(increased, ", "))
		}
		writeUILintBaseline(t, merged)
		t.Logf("baseline 갱신(감소만): %s (%d files)", uiLintBaselinePath, len(merged))
		return
	}

	raw, err := os.ReadFile(uiLintBaselinePath)
	if err != nil {
		t.Fatalf("baseline 없음 (%s). UPDATE_UI_LINT_BASELINE=1 로 생성하세요: %v", uiLintBaselinePath, err)
	}
	baseline := map[string]uiLintCounts{}
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatal(err)
	}

	for rel, got := range actual {
		want, ok := baseline[rel]
		if !ok {
			if !got.zero() {
				t.Errorf("%s: 신규 템플릿 위반이 0이어야 함 %+v", rel, got)
			}
			if got.LinkUnderline > 0 {
				t.Errorf("%s: 신규 템플릿에 밑줄 링크 %d개 — 독립된 행동·이동은 btn-* 버튼으로(link_button 파셜), 링크로 남기는 종류는 data-link=\"text\"", rel, got.LinkUnderline)
			}
			continue
		}
		// 95-210: 밑줄 링크는 total() 과 따로 늘지 않게만 막는다(줄면 baseline 갱신으로 고정).
		if got.LinkUnderline > want.LinkUnderline {
			t.Errorf("%s: 밑줄 링크 증가 baseline=%d actual=%d — btn-* 버튼으로 쓰거나 data-link=\"text\"", rel, want.LinkUnderline, got.LinkUnderline)
		}
		gotCmp, wantCmp := got, want
		gotCmp.LinkUnderline, wantCmp.LinkUnderline = 0, 0
		if gotCmp != wantCmp {
			switch {
			case got.total() > want.total():
				t.Errorf("%s: 위반 증가 baseline=%+v actual=%+v", rel, want, got)
			case got.total() < want.total():
				t.Logf("%s: 위반 감소(baseline 유지) baseline=%+v actual=%+v", rel, want, got)
			default:
				t.Errorf("%s: 항목별 수가 바뀜 baseline=%+v actual=%+v", rel, want, got)
			}
		}
	}
	for rel := range baseline {
		if _, ok := actual[rel]; !ok {
			t.Errorf("%s: baseline에 있으나 템플릿이 없다. baseline을 갱신하세요", rel)
		}
	}

	files, total := 0, 0
	for _, c := range actual {
		if !c.zero() {
			files++
			total += c.total()
		}
	}
	t.Logf("UI lint: 위반 파일 %d, 합계 %d (전체 템플릿 %d)", files, total, len(actual))
}

func TestUILintBaselineUpdateRejectsIncrease(t *testing.T) {
	base := map[string]uiLintCounts{"a.html": {RawColor: 2}}
	got := map[string]uiLintCounts{"a.html": {RawColor: 3}}
	_, inc := mergeUILintBaselineDecreaseOnly(base, got)
	if len(inc) == 0 {
		t.Fatal("증가를 거부해야 한다")
	}
	dec := map[string]uiLintCounts{"a.html": {RawColor: 1}}
	merged, inc := mergeUILintBaselineDecreaseOnly(base, dec)
	if len(inc) != 0 || merged["a.html"].RawColor != 1 {
		t.Fatalf("감소 반영 실패 %+v inc=%v", merged, inc)
	}
}

// 95-22: 고객 선택 필드를 <select> 로 직접 두면 실패. 허용 목록은 TestCustomerComboSelectLint.
func TestUILintCustomerSelect(t *testing.T) {
	TestCustomerComboSelectLint(t)
}
