package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func Test95_86_HeadScriptsDeferOrReady(t *testing.T) {
	root := findTemplateRoot(t)
	base := string(mustRead(t, filepath.Join(root, "layout", "base.html")))
	headEnd := strings.Index(base, "</head>")
	if headEnd < 0 {
		t.Fatal("base.html head 없음")
	}
	head := base[:headEnd]
	re := regexp.MustCompile(`<script([^>]*)\ssrc="(/static/js/[^"?]+)`)
	hits := re.FindAllStringSubmatch(head, -1)
	if len(hits) == 0 {
		t.Fatal("head 앱 스크립트 없음")
	}
	jsRoot := filepath.Join(root, "..", "static", "js")
	earlyReturn := regexp.MustCompile(`getElementById\(|querySelector\(`)
	readyWrap := regexp.MustCompile(`readyState\s*===\s*['"]loading['"]|DOMContentLoaded`)
	for _, h := range hits {
		attrs, src := h[1], h[2]
		baseName := filepath.Base(src)
		body, err := os.ReadFile(filepath.Join(jsRoot, baseName))
		if err != nil {
			t.Fatalf("%s 읽기 %v", src, err)
		}
		js := string(body)
		head := js
		if len(head) > 1200 {
			head = head[:1200]
		}
		needs := earlyReturn.MatchString(head) && strings.Contains(head, "return")
		hasDefer := strings.Contains(attrs, "defer")
		hasReady := readyWrap.MatchString(js)
		if needs && !hasDefer && !hasReady {
			t.Fatalf("head 스크립트 %s 가 DOM을 최상위에서 찾고 defer/DOMContentLoaded 없음", src)
		}
		if baseName == "board_schedule.js" || baseName == "board_kanban.js" {
			if !hasDefer {
				t.Fatalf("%s head 에 defer 없음", src)
			}
			if !hasReady {
				t.Fatalf("%s DOMContentLoaded 래핑 없음", src)
			}
		}
	}
	sched := string(mustRead(t, filepath.Join(jsRoot, "board_schedule.js")))
	if strings.Contains(sched, "data-nudge") {
		t.Fatal("board_schedule.js 에 data-nudge 남음")
	}
	bar := string(mustRead(t, filepath.Join(root, "workboard", "_board_bar.html")))
	if strings.Contains(bar, "data-nudge") {
		t.Fatal("바에 ±15 남음")
	}
}
