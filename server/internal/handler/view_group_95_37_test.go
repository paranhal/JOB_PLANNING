package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func Test95_37ViewGroupCookieAndScope(t *testing.T) {
	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/work?scope=personal", nil)
	c := e.NewContext(req, rec)
	if viewGroupInclude(c) {
		t.Fatal("?scope=personal 은 개인만")
	}
	persistViewGroupCookie(c)
	ck := rec.Result().Cookies()
	found := false
	for _, x := range ck {
		if x.Name == viewGroupCookie && x.Value == viewGroupMine {
			found = true
		}
	}
	if !found {
		t.Fatal("쿠키 personal")
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/work?scope=with", nil)
	req2.AddCookie(&http.Cookie{Name: viewGroupCookie, Value: viewGroupMine})
	c2 := e.NewContext(req2, rec2)
	if !viewGroupInclude(c2) {
		t.Fatal("?scope=with 이 쿠키보다 우선")
	}

	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/work", nil)
	req3.AddCookie(&http.Cookie{Name: viewGroupCookie, Value: viewGroupMine})
	c3 := e.NewContext(req3, rec3)
	if viewGroupInclude(c3) {
		t.Fatal("쿠키 personal")
	}

	items := []model.WorkListItem{
		{RefID: "1", Assignee: "홍길동", WorkGroupID: "G1"},
		{RefID: "2", Assignee: "홍길동"},
	}
	got := filterPersonalOnlyWork(items, "u1", []string{"홍길동"})
	if len(got) != 1 || got[0].RefID != "2" {
		t.Fatalf("개인만 %+v", got)
	}
}

func Test95_37ToggleOnRequiredScreens(t *testing.T) {
	root := filepath.Join("..", "..", "web", "templates")
	if _, err := os.Stat(root); err != nil {
		root = findTemplateRoot(t)
	}
	need := map[string]string{
		"as/list.html":                    "AS 목록",
		"work/list.html":                  "오늘·전체 업무",
		"plan/unplanned.html":             "미계획 업무함",
		"dashboard.html":                  "대시보드",
		"meeting/weekly.html":             "주간 회의",
		"stats/overview.html":             "통계",
		"stats/reports.html":              "보고서(예전)",
		"stats/company_weekly.html":       "주간업무보고",
		"maintenance/plan_show.html":      "정기점검 캘린더",
		"maintenance/plan_manage.html":    "정기점검 관리·대기",
		"work_status/timeline.html":       "업무처리현황",
	}
	for rel, label := range need {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s 읽기 %v", label, err)
		}
		if !strings.Contains(string(b), `view_group_toggle`) {
			t.Errorf("%s (%s) 토글 없음", label, rel)
		}
	}
	for _, rel := range []string{"workboard/board_list.html", "workboard/_board_head.html", "workboard/board_kanban.html"} {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s 읽기 %v", rel, err)
		}
		if !strings.Contains(string(b), `board_top_controls`) {
			t.Errorf("%s 상단 드롭다운 묶음 없음", rel)
		}
		if strings.Contains(string(b), `view_group_toggle`) {
			t.Errorf("%s 가 공용 view_group_toggle 을 쓰면 안 된다", rel)
		}
	}
}

func Test95_37LeaderCanActForGroupWork(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("role", model.RoleTech)
	c.Set("user_id", "u-lead")
	c.Set("leader_group_ids", map[string]bool{"G1": true})
	if !canActForGroupWork(c, "G1") {
		t.Fatal("자기 그룹 업무 대리")
	}
	if canActForGroupWork(c, "G2") {
		t.Fatal("다른 그룹")
	}
	if canEditTask(c, "다른사람", "", "G1") != true {
		t.Fatal("그룹 업무 수정")
	}
	if canEditTask(c, "다른사람", "", "") {
		t.Fatal("그룹 외 업무")
	}
}
