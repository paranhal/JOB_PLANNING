package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 95-187 A: 사업 상세 헤더의 모든 버튼은 btn btn-sm 이다(옛 px-4 py-2 border 버튼이 없다).
func TestHTTP95_187_HeaderButtonsUnified(t *testing.T) {
	v := setup183(t)
	body := v.get(t, "/projects/"+v.id, v.admin).Body.String()
	n := strings.Count(body, "data-project-header-btn")
	if n < 6 {
		t.Fatalf("헤더 버튼 표식 %d개(수정·합치기·완료·보관·삭제 등 포함 6개 이상 기대)", n)
	}
	for _, m := range regexp.MustCompile(`<(?:a|button)[^>]*data-project-header-btn[^>]*>`).FindAllString(body, -1) {
		if !strings.Contains(m, `btn `) || !strings.Contains(m, "btn-sm") || strings.Contains(m, "px-4 py-2") {
			t.Fatalf("btn btn-sm 이 아닌 헤더 버튼: %s", m)
		}
	}
	// 템플릿 원본(상태별 분기 포함) 헤더 블록에도 옛 버튼 클래스가 없다
	raw, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "project", "show.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	start := strings.Index(s, "data-project-years-link")
	if start < 0 {
		t.Fatal("헤더 블록을 찾지 못했다")
	}
	rest := s[start:]
	n2 := strings.Index(rest, "</form>\n      {{end}}\n      {{end}}\n    </div>")
	if n2 < 0 {
		t.Fatal("헤더 블록 끝을 찾지 못했다")
	}
	if strings.Contains(rest[:n2], "px-4 py-2 text-sm border") {
		t.Fatal("헤더에 옛 버튼 클래스가 남았다")
	}
}
