package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestProjectDashboardHasFigmaItems(t *testing.T) {
	e, _, _ := newProjectServer(t)
	page := doGet(t, e, "/projects/dashboard")
	if page.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", page.Code, clipBody(page.Body.String()))
	}
	body := page.Body.String()
	for _, want := range []string{
		"사업 대시보드",
		"업무 리포트",
		"사업 필터",
		"공정→기성→준공",
		"구축→개통→검수",
		"접수→조치→완료",
		"사업 운영 현황",
		`tab=other`,
		"오늘 처리할 일",
		"담당 업무",
		"승인 대기",
		"다가오는 일정",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
	for _, leak := range []string{"김현우", "A동 골조공사", "data-node-id", "화면 설계를 위한 예시"} {
		if strings.Contains(body, leak) {
			t.Fatalf("샘플/원본 흔적 %q", leak)
		}
	}
}

func TestProjectsSectorVisibleOnLocalProductionEnv(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SHOW_PROJECTS_SECTOR", "")
	e, _, _ := newProjectServer(t)
	home := doGet(t, e, "/projects")
	if home.Code != http.StatusOK {
		t.Fatalf("list=%d", home.Code)
	}
	if !strings.Contains(home.Body.String(), "사업 대시보드") {
		t.Fatal("로컬에서는 APP_ENV=production 이어도 사업관리 메뉴가 있어야 한다")
	}
	dash := doGet(t, e, "/projects/dashboard")
	if dash.Code != http.StatusOK {
		t.Fatalf("로컬 대시보드 status=%d", dash.Code)
	}
	if strings.Contains(dash.Body.String(), "김현우") || strings.Contains(dash.Body.String(), "A동 골조공사") {
		t.Fatal("예시 데이터가 나왔다")
	}
}

func TestProjectsSectorHiddenWhenProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SHOW_PROJECTS_SECTOR", "0")
	e, _, _ := newProjectServer(t)
	home := doGet(t, e, "/projects")
	if home.Code != http.StatusOK {
		t.Fatalf("list=%d", home.Code)
	}
	body := home.Body.String()
	if strings.Contains(body, ">사업관리<") {
		t.Fatal("운영 홈에 사업관리 초안 메뉴가 보인다")
	}
	if !strings.Contains(body, "수행관계") {
		t.Fatal("운영에서도 수행관계는 남아야 한다")
	}
	dash := doGet(t, e, "/projects/dashboard")
	if dash.Code != http.StatusOK {
		t.Fatalf("운영에서도 URL은 열려야 한다 status=%d", dash.Code)
	}
	if strings.Contains(dash.Body.String(), "김현우") {
		t.Fatal("운영 URL에 예시 데이터가 있다")
	}
}

func TestProjectExampleDataBlankOnProductionSector(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SHOW_PROJECTS_SECTOR", "1")
	e, _, _ := newProjectServer(t)
	for _, path := range []string{"/projects", "/projects/dashboard", "/projects/docs", "/projects/diary"} {
		page := doGet(t, e, path)
		if page.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, page.Code)
		}
		body := page.Body.String()
		for _, leak := range []string{"김현우", "A동 골조공사", "충남세종 RFID", "I-2026-1012"} {
			if strings.Contains(body, leak) {
				t.Fatalf("운영 화면 %s 에 예시 데이터 %q 가 있다", path, leak)
			}
		}
	}
}
