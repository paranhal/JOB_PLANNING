package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestProjectWorkMenusAndDiaryStats(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PROJECT_EXAMPLE_DATA", "1")
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
	if !strings.Contains(list.Body.String(), "사업·업무관리") || !strings.Contains(list.Body.String(), "단계별 일정표") {
		t.Fatal("목록 제목·단계표 없음")
	}
}

func TestProjectDiaryFigmaLayout(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PROJECT_EXAMPLE_DATA", "1")
	e, _, _ := newProjectServer(t)
	page := doGet(t, e, "/projects/diary")
	if page.Code != http.StatusOK {
		t.Fatalf("status=%d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{
		"업무일지·투입관리",
		"임시저장",
		"검토 요청",
		"I-2026-1012",
		"사업 선택",
		"A동 골조공사",
		"데이터센터 네트워크 구축",
		"ERP 연동 플랫폼 개발",
		"충남세종 RFID 자동화 장비 유지보수",
		"금일 수행 업무 및 다음 계획",
		"전날 데이터 불러오기",
		"슬래브 타설 120㎡",
		"참여자·투입 시간",
		"현장소장",
		"합계 6명 · 금일 36시간",
		"자재·장비 / 시스템 자원",
		"레미콘",
		"코어 스위치",
		"작업 결과 및 산출물 첨부",
		"A동_8층_타설기록_1012.jpg",
		"화면 설계를 위한 예시",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
	if strings.Contains(body, "data-node-id") {
		t.Fatal("Figma 원본 흔적이 렌더에 남았다")
	}
}
