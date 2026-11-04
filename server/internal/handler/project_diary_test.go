package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestProjectWorkMenusAndDiaryStats(t *testing.T) {
	e, _, _ := newProjectServer(t)

	dash := doGet(t, e, "/projects/dashboard")
	if dash.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d", dash.Code)
	}
	if !strings.Contains(dash.Body.String(), "사업 대시보드") {
		t.Fatal("대시보드 제목 없음")
	}

	docs := doGet(t, e, "/projects/docs")
	if docs.Code != http.StatusOK {
		t.Fatalf("docs status=%d", docs.Code)
	}
	if !strings.Contains(docs.Body.String(), "서류 작성·관리") {
		t.Fatal("서류 화면 제목 없음")
	}

	page := doGet(t, e, "/projects/diary")
	if page.Code != http.StatusOK {
		t.Fatalf("diary status=%d body=%s", page.Code, clipBody(page.Body.String()))
	}
	if !strings.Contains(page.Body.String(), "업무일지·투입관리") {
		t.Fatal("일지 제목 없음")
	}

	list := doGet(t, e, "/projects")
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d", list.Code)
	}
	if !strings.Contains(list.Body.String(), "사업·업무관리") || !strings.Contains(list.Body.String(), "이 화면은 준비 중입니다") {
		t.Fatal("목록 제목·준비 중 안내 없음")
	}
}

func TestProjectDiaryFigmaLayout(t *testing.T) {
	e, _, _ := newProjectServer(t)
	page := doGet(t, e, "/projects/diary")
	if page.Code != http.StatusOK {
		t.Fatalf("status=%d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{
		"업무일지·투입관리",
		"이 화면은 준비 중입니다",
		"사업 등록 관리",
		"/projects/registry",
		"data-project-pending",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
	for _, leak := range []string{"김현우", "A동 골조공사", "I-2026-1012", "data-node-id"} {
		if strings.Contains(body, leak) {
			t.Fatalf("샘플/원본 흔적 %q", leak)
		}
	}
}
