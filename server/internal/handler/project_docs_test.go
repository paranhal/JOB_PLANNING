package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestProjectDocsFigmaLayout(t *testing.T) {
	e, _, _ := newProjectServer(t)
	page := doGet(t, e, "/projects/docs")
	if page.Code != http.StatusOK {
		t.Fatalf("status=%d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{
		"서류 작성·관리",
		"이 화면은 준비 중입니다",
		"사업 등록 관리",
		"/projects/registry",
		"data-project-pending",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
	for _, leak := range []string{"A동 골조공사", "김현우", "견적서", "data-node-id"} {
		if strings.Contains(body, leak) {
			t.Fatalf("샘플/원본 흔적 %q", leak)
		}
	}
}
