package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMobileTabBarAndMoreSheetShareNavTree(t *testing.T) {
	p := "web/templates/layout/base.html"
	b, err := os.ReadFile(p)
	if err != nil {
		b, err = os.ReadFile(filepath.Join("..", "..", p))
	}
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `id="mobile-tab-bar"`) {
		t.Fatal("하단 탭 바 없음")
	}
	for _, need := range []string{`href="/"`, `href="/work"`, `href="/approvals/inbox"`, `href="/leaves"`, "더보기", "휴가 신청"} {
		if !strings.Contains(s, need) {
			t.Fatalf("탭 바에 %s 없음", need)
		}
	}
	if !strings.Contains(s, `id="mobile-more-sheet"`) || strings.Count(s, `{{template "nav_tree" .}}`) < 2 {
		t.Fatal("더보기 시트가 nav_tree 를 공유하지 않는다")
	}
	if strings.Contains(s, "영업 전용") {
		t.Fatal("영업 전용 탭이 있으면 안 된다")
	}
}
