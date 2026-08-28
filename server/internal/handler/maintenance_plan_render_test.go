package handler

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func planViewData(view string, month int, visits []model.MaintenanceVisit) map[string]interface{} {
	const today = "2026-08-05"
	return map[string]interface{}{
		"Title": "정기점검 2026년", "Active": NavMaintenance, "UserRole": "admin",
		"Plan":      &model.MaintenancePlan{PlanID: "mpl_1", PlanYear: 2026, Status: "draft"},
		"Plans":     []model.MaintenancePlan{{PlanID: "mpl_1", PlanYear: 2026, Status: "draft"}},
		"YearQuery": "",
		"Visits":    visits,
		"Customers": nil,
		"Assignees": nil,
		"View":      view,
		"Month":     month,
		"MonthVisits": visits,
		"Weeks":       buildVisitCalendar(2026, month, visits, today),
		"Board":       buildVisitBoard(visits, today),
		"DoneCount":   1, "OpenCount": 1,
		"FlashDone": "", "FlashOK": "", "Today": today,
		"CanEdit": true, "IsAdmin": true,
		"ScopeAll": true, "ShowScopeToggle": false, "ScopeNote": "전체 일정",
		"Projects": nil, "Unassigned": nil, "QuotaMonth": month,
		"DupCount": 0, "DeleteAfter": "2026-01-01", "FlashErr": "",
	}
}

func sampleVisits() []model.MaintenanceVisit {
	return []model.MaintenanceVisit{
		{
			VisitID: "mvs_1", PlanID: "mpl_1", VisitDate: "2026-08-03",
			ShortName: "새롬동도서관", ProductType: "KLAS", Assignee: "최혜영",
			Completed: true, CompletedDate: "2026-08-03", EntryCategory: "normal",
		},
		{
			// 같은 사이트·다른 점검 대상 — 별도 방문으로 보여야 한다.
			VisitID: "mvs_2", PlanID: "mpl_1", VisitDate: "2026-08-20",
			ShortName: "새롬동도서관", ProductType: "앤로보틱스", Assignee: "양기헌",
			EntryCategory: "normal",
		},
	}
}

// 계획 화면의 세 가지 보기가 모두 렌더링되고, 점검 대상이 함께 보여야 한다.
func TestMaintenancePlanViewsRender(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		filepath.Join(root, "layout", "base.html"),
		filepath.Join(root, "maintenance", "plan_show.html"),
	}
	visits := sampleVisits()

	for _, view := range []string{mntViewCalendar, mntViewKanban, mntViewList} {
		tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "content", planViewData(view, 8, visits)); err != nil {
			t.Fatalf("%s 렌더링 실패: %v", view, err)
		}
		out := buf.String()
		for _, want := range []string{"KLAS", "앤로보틱스", "새롬동도서관", "방문 추가", "지난 방문 일괄 완료", "방문 일정 수정",
			"관리", "bbf7d0", "bfdbfe", "세종 K-LAS", "mntProductStyle"} {
			if want == "mntProductStyle" {
				if !strings.Contains(out, "background-color:#bbf7d0") && !strings.Contains(out, "background-color:#bfdbfe") {
					t.Errorf("%s 화면에 점검 대상 색 style 이 없다", view)
				}
				continue
			}
			if !strings.Contains(out, want) {
				t.Errorf("%s 화면에 %q 가 없다", view, want)
			}
		}
		if !strings.Contains(out, "/maintenance/visits/mvs_2/complete") {
			t.Errorf("%s 화면에 완료 처리 버튼이 없다", view)
		}
		if !strings.Contains(out, "/maintenance/visits/mvs_2/delete") {
			t.Errorf("%s 화면에 삭제 버튼이 없다", view)
		}
		if !strings.Contains(out, "openEdit") {
			t.Errorf("%s 화면에 수정 버튼이 없다", view)
		}
		if err := tmpl.ExecuteTemplate(&buf, "base.html", planViewData(view, 8, visits)); err != nil {
			t.Fatalf("%s base 렌더링 실패: %v", view, err)
		}
	}
}

// 기술담당 화면에는 전체 일정 보기 토글이 있어야 한다.
func TestMaintenancePlanScopeToggleRender(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		filepath.Join(root, "layout", "base.html"),
		filepath.Join(root, "maintenance", "plan_show.html"),
	}
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}
	data := planViewData(mntViewCalendar, 8, sampleVisits())
	data["IsAdmin"] = false
	data["ShowScopeToggle"] = true
	data["ScopeAll"] = false
	data["ScopeNote"] = "내 점검 일정"
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "전체 일정 보기") || !strings.Contains(out, "all=1") {
		t.Fatal("기술담당 기본 화면에 전체 일정 보기 버튼이 없다")
	}
	if !strings.Contains(out, "내 점검 일정") {
		t.Fatal("범위 안내가 없다")
	}
}

// 캘린더 격자 — 방문이 해당 날짜 칸에 들어가야 한다.
func TestBuildVisitCalendar(t *testing.T) {
	weeks := buildVisitCalendar(2026, 8, sampleVisits(), "2026-08-05")
	if len(weeks) == 0 {
		t.Fatal("주가 만들어지지 않았다")
	}
	for _, w := range weeks {
		if len(w) != 7 {
			t.Fatalf("한 주는 7칸이어야 한다: %d", len(w))
		}
	}
	// 2026-08-01은 토요일 → 첫 주의 마지막 칸이 1일
	if weeks[0][6].Day != 1 {
		t.Fatalf("8월 1일 위치: %+v", weeks[0][6])
	}

	found := map[string]int{}
	todayMarked := 0
	for _, w := range weeks {
		for _, d := range w {
			found[d.Date] = len(d.Visits)
			if d.Today {
				todayMarked++
				if d.Date != "2026-08-05" {
					t.Fatalf("오늘 표시가 잘못됐다: %s", d.Date)
				}
			}
		}
	}
	if found["2026-08-03"] != 1 || found["2026-08-20"] != 1 {
		t.Fatalf("방문이 날짜 칸에 들어가지 않았다: %v", found)
	}
	if todayMarked != 1 {
		t.Fatalf("오늘 칸 수: %d", todayMarked)
	}
}

// 칸반 열 분류 — 완료 / 경과 / 오늘 / 예정
func TestBuildVisitBoard(t *testing.T) {
	visits := []model.MaintenanceVisit{
		{VisitID: "a", VisitDate: "2026-08-03", Completed: true},
		{VisitID: "b", VisitDate: "2026-08-03"},
		{VisitID: "c", VisitDate: "2026-08-05"},
		{VisitID: "d", VisitDate: "2026-08-20"},
	}
	b := buildVisitBoard(visits, "2026-08-05")
	if len(b.Done) != 1 || b.Done[0].VisitID != "a" {
		t.Fatalf("완료 열: %+v", b.Done)
	}
	if len(b.Overdue) != 1 || b.Overdue[0].VisitID != "b" {
		t.Fatalf("경과 열: %+v", b.Overdue)
	}
	if len(b.Today) != 1 || b.Today[0].VisitID != "c" {
		t.Fatalf("오늘 열: %+v", b.Today)
	}
	if len(b.Upcoming) != 1 || b.Upcoming[0].VisitID != "d" {
		t.Fatalf("예정 열: %+v", b.Upcoming)
	}
}

func TestMntProductKind(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"앤로보틱스", "anrobotics"},
		{"KLAS", "klas"},
		{"K-LAS", "klas"},
		{"세종 K-LAS", "sejong_klas"},
		{"세종 KLAS", "sejong_klas"},
		{"RFID", ""},
	}
	for _, tc := range cases {
		if got := mntProductKind(tc.in); got != tc.want {
			t.Errorf("%q → %q (want %q)", tc.in, got, tc.want)
		}
	}
	if !strings.Contains(mntProductStyle("앤로보틱스"), "#bbf7d0") {
		t.Fatal("앤로보틱스 초록 style")
	}
	if !strings.Contains(mntProductStyle("KLAS"), "#bfdbfe") {
		t.Fatal("K-LAS 파랑 style")
	}
	if !strings.Contains(mntProductStyle("세종 K-LAS"), "#e9d5ff") {
		t.Fatal("세종 K-LAS 보라 style")
	}
}

func TestFilterVisitsByAssignee(t *testing.T) {
	visits := sampleVisits()
	mine := filterVisitsByAssignee(visits, []string{"최혜영"})
	if len(mine) != 1 || mine[0].Assignee != "최혜영" {
		t.Fatalf("본인 필터: %+v", mine)
	}
	all := filterVisitsByAssignee(visits, []string{"최혜영", "양기헌"})
	if len(all) != 2 {
		t.Fatalf("복수 키: %d", len(all))
	}
	if n := filterVisitsByAssignee(visits, nil); n != nil && len(n) != 0 {
		t.Fatalf("키 없으면 비어야 한다: %+v", n)
	}
}

func TestMaintenanceDupBannerRender(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		filepath.Join(root, "layout", "base.html"),
		filepath.Join(root, "maintenance", "plan_show.html"),
	}
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}
	data := planViewData(mntViewList, 0, sampleVisits())
	data["DupCount"] = 3
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "중복 3건 발견") {
		t.Fatalf("중복 안내 없음: %s", out)
	}
	if !strings.Contains(out, "정리하기") {
		t.Fatal("정리하기 링크 없음")
	}
}

func TestVisitLabelAndDefaultMonth(t *testing.T) {
	got := visitLabel(model.MaintenanceVisit{ShortName: "공주도서관", ProductType: "KLAS"})
	if got != "공주도서관 · KLAS" {
		t.Fatalf("표시명: %q", got)
	}
	if got := visitLabel(model.MaintenanceVisit{OrgName: "국방대학교"}); got != "국방대학교" {
		t.Fatalf("점검 대상이 없으면 기관명만: %q", got)
	}

	visits := sampleVisits()
	if m := defaultPlanMonth(2026, visits, time.Date(2026, 8, 5, 0, 0, 0, 0, time.Local)); m != 8 {
		t.Fatalf("올해 계획은 이번 달: %d", m)
	}
	// 다른 해의 계획이면 방문이 있는 첫 달
	if m := defaultPlanMonth(2026, visits, time.Date(2027, 3, 1, 0, 0, 0, 0, time.Local)); m != 8 {
		t.Fatalf("방문이 있는 첫 달: %d", m)
	}
}
