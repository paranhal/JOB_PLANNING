package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestProjectDashboardHasFigmaItems(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PROJECT_EXAMPLE_DATA", "1")
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
		"유지보수",
		"사업 운영 현황",
		"주의",
		"공사 2개",
		"구축 1개",
		"개발 1개",
		"유지보수 2개",
		"총 6개 사업",
		"총 8개 단계 운영",
		"오늘 처리할 일",
		"담당 업무",
		"승인 대기",
		"다가오는 일정",
		"할 일 및 담당 업무",
		"오늘 처리",
		"승인 대기 및 일정",
		"A동 8층 슬래브 타설 공정 확인서 작성",
		"네트워크 점검 보고서 검토",
		"ERP 연동 API 테스트 결과 확인",
		"B동 6층 철근 배근 변경 승인사 제출",
		"공사 업무",
		"구축 업무",
		"개발 업무",
		"유지보수 업무",
		"RFID 리더 오류 AS 현장 조치 보고서",
		"세종시 10월 정기점검 일정 확정",
		"충남세종 RFID 자동화 장비 유지보수",
		"세종시 도서관 ICT 유지관리",
		"A동 골조공사",
		"B동 외장공사",
		"데이터센터 네트워크 구축",
		"ERP 연동 플랫폼 개발",
		"금일 업무 브리핑",
		"15건의 승인 대기 확인 필요",
		"화면 설계를 위한 예시",
		"기준일자 2026.10.12",
		"data-node-id",
	} {
		if want == "data-node-id" {
			if strings.Contains(body, want) {
				t.Fatal("Figma 원본 흔적이 렌더에 남았다")
			}
			continue
		}
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
	if strings.Contains(body, "진행 사업과 오늘 일일 업무 투입") {
		t.Fatal("이전 요약 대시보드 문구가 남아 있다")
	}
	if strings.Contains(body, "시스템/네트워크") || strings.Contains(body, "유지보수(AS") {
		t.Fatal("사업 유형 이름이 공사·구축·개발·유지보수 네 가지로 통일되지 않았다")
	}

	constOnly := doGet(t, e, "/projects/dashboard?tab=construction")
	cbody := constOnly.Body.String()
	if !strings.Contains(cbody, "A동 골조공사") || strings.Contains(cbody, "ERP 연동 플랫폼 개발") {
		t.Fatal("공사 필터가 유형 카드를 가리지 못했다")
	}
	mnt := doGet(t, e, "/projects/dashboard?tab=maintenance")
	mbody := mnt.Body.String()
	if !strings.Contains(mbody, "유지보수 업무") || strings.Contains(mbody, "A동 골조공사") {
		t.Fatal("유지보수 필터가 유형 카드를 가리지 못했다")
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
		t.Fatal("APP_ENV=production 인데 설계용 예시 데이터가 나왔다")
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
	if strings.Contains(body, ">사업관리<") || strings.Contains(body, "사업 대시보드") {
		t.Fatal("운영 홈에 사업관리 초안 메뉴가 보인다")
	}
	if !strings.Contains(body, "수행관계") {
		t.Fatal("운영에서도 수행관계는 남아야 한다")
	}
	dash := doGet(t, e, "/projects/dashboard")
	if dash.Code != http.StatusNotFound {
		t.Fatalf("운영 대시보드 status=%d want 404", dash.Code)
	}
}

func TestProjectExampleDataBlankOnProductionSector(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SHOW_PROJECTS_SECTOR", "1")
	t.Setenv("PROJECT_EXAMPLE_DATA", "0")
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
