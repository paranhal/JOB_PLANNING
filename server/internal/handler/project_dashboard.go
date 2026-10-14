package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/config"
)

type dashTodo struct {
	Track   string
	Urgency string
	Title   string
	Person  string
	When    string
	Due     bool
}

type dashUpcoming struct {
	Track string
	Title string
	When  string
}

type dashProgram struct {
	Track    string
	Kind     string
	Name     string
	Progress string
}

type dashDomain struct {
	Track     string
	Title     string
	Count     string
	Programs  []dashProgram
	WorkTitle string
	WorkNote  string
}

type dashSeed struct {
	DateLabel   string
	AsOf        string
	TrackCounts string
	StageCounts string
	TodayN      int
	PeopleN     int
	ApproveN    int
	SoonN       int
	TodayBreak  string
	PeopleBreak string
	ApproveBreak string
	SoonBreak   string
	Todos       []dashTodo
	Approvals   []dashTodo
	Upcoming    []dashUpcoming
	Domains     []dashDomain
	BriefItems  string
}

func sampleProjectDashboard() dashSeed {
	return dashSeed{
		DateLabel:    "2026.10.12",
		AsOf:         "화면 설계용 · 예시",
		TrackCounts:  "공사 2개 · 구축 1개 · 개발 1개 · 유지보수 2개 · 총 6개 사업",
		StageCounts:  "공정 1건 · 기성 1건 · 준공 1건 · 구축 1건 · 개통 1건 · 검수 1건 · AS 1건 · 점검 1건 · 총 8개 단계 운영",
		TodayN:       16,
		PeopleN:      10,
		ApproveN:     15,
		SoonN:        9,
		TodayBreak:   "공정 2건 · 기성 2건 · 준공 1건 · 구축 2건 · 개통 1건 · 검수 4건 · AS 2건 · 점검 2건",
		PeopleBreak:  "공사 1명 · 구축 1명 · 개발 2명 · 유지보수 2명 · PMO 3명",
		ApproveBreak: "공정 2건 · 기성 2건 · 준공 1건 · 구축 2건 · 개통 1건 · 검수 4건 · AS 2건 · 점검 1건",
		SoonBreak:    "공정 1건 · 기성 1건 · 준공 1건 · 구축 1건 · 개통 1건 · 검수 2건 · AS 1건 · 점검 1건",
		Todos: []dashTodo{
			{Track: "construction", Urgency: "긴급", Title: "A동 8층 슬래브 타설 공정 확인서 작성", Person: "김현우", When: "오늘 17:00", Due: true},
			{Track: "network", Urgency: "확인", Title: "네트워크 점검 보고서 검토", Person: "박준석", When: "오늘 18:00", Due: true},
			{Track: "maintenance", Urgency: "긴급", Title: "RFID 리더 오류 AS 현장 조치 보고서", Person: "최은지", When: "오늘 16:30", Due: true},
			{Track: "dev", Urgency: "확인", Title: "ERP 연동 API 테스트 결과 확인", Person: "이미진", When: "내일 10:00", Due: false},
			{Track: "construction", Urgency: "긴급", Title: "B동 6층 철근 배근 변경 승인사 제출", Person: "김현우", When: "내일 09:00", Due: false},
			{Track: "maintenance", Urgency: "확인", Title: "세종시 10월 정기점검 일정 확정", Person: "정우성", When: "내일 11:00", Due: false},
		},
		Approvals: []dashTodo{
			{Track: "construction", Urgency: "긴급", Title: "A동 8층 슬래브 타설 공정 확인서", Person: "김현우", When: "오늘", Due: true},
			{Track: "network", Urgency: "확인", Title: "네트워크 점검 보고서", Person: "박준석", When: "오늘", Due: true},
			{Track: "maintenance", Urgency: "긴급", Title: "RFID AS 조치 완료 보고서", Person: "최은지", When: "오늘", Due: true},
			{Track: "dev", Urgency: "확인", Title: "ERP 연동 API 테스트 결과 확인", Person: "이미진", When: "내일", Due: false},
			{Track: "construction", Urgency: "긴급", Title: "B동 6층 철근 배근 변경 승인사", Person: "김현우", When: "내일", Due: false},
			{Track: "maintenance", Urgency: "확인", Title: "정기점검 결과서", Person: "정우성", When: "내일", Due: false},
		},
		Upcoming: []dashUpcoming{
			{Track: "construction", Title: "A동 8층 슬래브 타설", When: "10/12 09:00"},
			{Track: "network", Title: "네트워크 구축 점검", When: "10/12 14:00"},
			{Track: "maintenance", Title: "RFID AS 현장 방문", When: "10/12 16:00"},
			{Track: "dev", Title: "ERP 연동 테스트", When: "10/13 10:00"},
			{Track: "maintenance", Title: "세종시 정기점검", When: "10/13 13:00"},
		},
		Domains: []dashDomain{
			{
				Track: "construction", Title: "공사 업무", Count: "2개 사업",
				Programs: []dashProgram{
					{Track: "construction", Kind: "공사", Name: "A동 골조공사", Progress: "공정 62%"},
					{Track: "construction", Kind: "공사", Name: "B동 외장공사", Progress: "기성 48%"},
				},
				WorkTitle: "A동 8층 슬래브 타설 공정 확인 · B동 6층 철근 배근 변경 승인",
				WorkNote:  "금일 공정 확인서 작성 · 내일 기성 확인서 제출 · 준공 준비 서류 미작성",
			},
			{
				Track: "network", Title: "구축 업무", Count: "1개 사업",
				Programs: []dashProgram{
					{Track: "network", Kind: "구축", Name: "데이터센터 네트워크 구축", Progress: "구축 75%"},
				},
				WorkTitle: "네트워크 점검 보고서 검토 · 장비 설치 확인",
				WorkNote:  "금일 점검 보고서 검토 · 내주 개통 준비 서류 작성 · 검수 계획 미등록",
			},
			{
				Track: "dev", Title: "개발 업무", Count: "1개 사업",
				Programs: []dashProgram{
					{Track: "dev", Kind: "개발", Name: "ERP 연동 플랫폼 개발", Progress: "구축 58%"},
				},
				WorkTitle: "API 테스트 결과 확인 · 결재 화면 구현",
				WorkNote:  "금일 테스트 결과 검토 · 내주 QA 시작 · 개통 준비 서류 작성 예정",
			},
			{
				Track: "maintenance", Title: "유지보수 업무", Count: "2개 사업",
				Programs: []dashProgram{
					{Track: "maintenance", Kind: "유지보수", Name: "충남세종 RFID 자동화 장비 유지보수", Progress: "조치 3건"},
					{Track: "maintenance", Kind: "유지보수", Name: "세종시 도서관 ICT 유지관리", Progress: "10월 40%"},
				},
				WorkTitle: "RFID 리더 오류 현장 조치 · 10월 정기점검 일정 확정",
				WorkNote:  "금일 AS 조치 보고서 · 내일 점검 체크리스트 작성 · 완료 확인서 미제출",
			},
		},
		BriefItems: "공정 확인서 작성 · 네트워크 점검 보고서 검토 · API 테스트 결과 확인 · AS 조치 보고서 · 정기점검 일정 확정",
	}
}

func dashboardTab(raw string) string {
	switch strings.TrimSpace(raw) {
	case "construction", "network", "dev", "maintenance":
		return strings.TrimSpace(raw)
	default:
		return "ops"
	}
}

func (h *ProjectHandler) Dashboard(c echo.Context) error {
	if !config.Load().ShowProjectsSector() {
		return echo.ErrNotFound
	}
	if !canViewProjects(c) {
		return echo.ErrForbidden
	}
	tab := dashboardTab(c.QueryParam("tab"))
	example := config.Load().ShowProjectExampleData()
	seed := dashSeed{}
	if example {
		seed = sampleProjectDashboard()
	}
	return c.Render(http.StatusOK, "project/dashboard.html", map[string]interface{}{
		"Title":       "사업 대시보드",
		"Active":      NavProjectsDashboard,
		"Tab":         tab,
		"Seed":        seed,
		"Example":     example,
		"ReportURL":   "/projects/diary",
		"ProjectsURL": "/projects",
		"DocsURL":     "/projects/docs",
		"FilterURL":   "/projects",
	})
}
