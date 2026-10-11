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
	uiStyleTagRe = regexp.MustCompile(`(?i)<style\b`)
	uiCDNNeedles = []string{
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
}

func (c uiLintCounts) total() int {
	return c.RawColor + c.Hex + c.InlineStyle + c.StyleTag + c.CDN
}

func (c uiLintCounts) zero() bool {
	return c.total() == 0
}

func countUILint(body string) uiLintCounts {
	cdn := 0
	for _, n := range uiCDNNeedles {
		cdn += strings.Count(body, n)
	}
	return uiLintCounts{
		RawColor:    len(uiRawColorRe.FindAllString(body, -1)),
		Hex:         len(uiHexRe.FindAllString(body, -1)),
		InlineStyle: len(uiInlineRe.FindAllString(body, -1)),
		StyleTag:    len(uiStyleTagRe.FindAllString(body, -1)),
		CDN:         cdn,
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

func TestUILintRatchet(t *testing.T) {
	actual := scanTemplateUILint(t)
	if os.Getenv("UPDATE_UI_LINT_BASELINE") == "1" {
		if err := os.MkdirAll(filepath.Dir(uiLintBaselinePath), 0o755); err != nil {
			t.Fatal(err)
		}
		b, err := json.MarshalIndent(actual, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(uiLintBaselinePath, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("baseline 갱신: %s (%d files)", uiLintBaselinePath, len(actual))
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
			continue
		}
		if got != want {
			switch {
			case got.total() > want.total():
				t.Errorf("%s: 위반 증가 baseline=%+v actual=%+v", rel, want, got)
			case got.total() < want.total():
				t.Errorf("%s: 위반이 줄었는데 baseline 미갱신. UPDATE_UI_LINT_BASELINE=1 로 갱신하세요. baseline=%+v actual=%+v", rel, want, got)
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
