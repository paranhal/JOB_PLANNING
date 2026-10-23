package handler

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestMnt95_16AWaitingCollapsedCalendarFirst(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		filepath.Join(root, "layout", "base.html"),
		filepath.Join(root, "layout", "_nav_tree.html"),
		filepath.Join(root, "maintenance", "plan_show.html"),
	}
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}
	visits := make([]model.MaintenanceVisit, 0, 7)
	for i := 0; i < 7; i++ {
		visits = append(visits, model.MaintenanceVisit{
			VisitID: fmt.Sprintf("mvs16a_%d", i+1), PlanID: "mpl_1",
			VisitDate: fmt.Sprintf("2026-10-%02d", i+1),
			ShortName: "방문사이트", ProductType: "KLAS", Assignee: "김",
			Completed: i < 6, EntryCategory: "normal",
		})
	}
	slots := make([]model.UnassignedMonthSlot, 0, 300)
	for i := 0; i < 300; i++ {
		slots = append(slots, model.UnassignedMonthSlot{
			CustomerID:  fmt.Sprintf("c16a_%d", i+1),
			ShortName:   fmt.Sprintf("대기학교%03d", i+1),
			ProductType: "KLAS",
			Cycle:       "semi",
			Region:      "충남",
		})
	}
	data := planViewData(mntViewCalendar, 10, visits)
	data["PeriodGroups"] = []model.MaintenancePeriodGroup{{
		Cycle: "semi", Index: 2, EndMonth: 12, LastMonth: false,
		LabelShort: "하반기", From: "2026-07-01", To: "2026-12-31", Slots: slots,
	}}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	cal := strings.Index(out, `id="mnt_calendar"`)
	wait := strings.Index(out, "하반기 점검 대기")
	if cal < 0 || wait < 0 {
		t.Fatalf("캘린더/대기 위치 cal=%d wait=%d", cal, wait)
	}
	if cal > wait {
		t.Fatal("대기 목록이 캘린더보다 위에 있다")
	}
	if !strings.Contains(out, "하반기 점검 대기 300곳 · 12월까지 배정") {
		t.Fatalf("요약 N 불일치")
	}
	if strings.Contains(out, "하반기 점검 대기:") {
		t.Fatal("사이트명 · 나열 문단이 남아 있다")
	}
	nForm := strings.Count(out, "/quarter-confirm")
	if nForm > 50 {
		t.Fatalf("접힘 HTML form %d > 50", nForm)
	}
	if !strings.Contains(out, `id="mnt-day-panel"`) {
		t.Fatal("일자 패널 없음")
	}
	if !strings.Contains(out, `id="mnt-month-select"`) || !strings.Contains(out, `value="10" selected`) {
		t.Fatal("보정된 월 선택 없음")
	}
}

func TestMnt95_16AWaitingAssigneeFilterCount(t *testing.T) {
	e, db, h, _ := newMntProtectApp(t)
	var b strings.Builder
	b.WriteString(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES `)
	for i := 0; i < 8; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "('c16af_%d','필터학교%d','필터학교%d',1,'O01')", i+1, i+1, i+1)
	}
	if _, err := db.Exec(b.String()); err != nil {
		t.Fatal(err)
	}
	repo := h.Maintenance.repo
	for i := 0; i < 8; i++ {
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: fmt.Sprintf("c16af_%d", i+1), ShortName: fmt.Sprintf("필터학교%d", i+1),
			HasKlas: true, InspectionCycle: "semi", OrgID: "O01", Region: "세종",
		}); err != nil {
			t.Fatal(err)
		}
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := repo.InsertVisitFull(model.MaintenanceVisit{
			PlanID: p.PlanID, VisitDate: "2026-01-08", CustomerID: fmt.Sprintf("c16af_%d", i+1),
			ProductType: "KLAS", Assignee: "관리자",
		}); err != nil {
			t.Fatal(err)
		}
	}
	all := mntGet(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10")
	if all.Code != http.StatusOK {
		t.Fatalf("status=%d", all.Code)
	}
	body := all.Body.String()
	if !strings.Contains(body, "하반기 점검 대기 8곳") {
		t.Fatalf("전체 N: %s", snippetWaiting(body))
	}
	none := mntGet(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10&assignee=none")
	if !strings.Contains(none.Body.String(), "하반기 점검 대기 5곳") {
		t.Fatalf("none N: %s", snippetWaiting(none.Body.String()))
	}
}

func snippetWaiting(s string) string {
	i := strings.Index(s, "점검 대기")
	if i < 0 {
		return s[:min(len(s), 400)]
	}
	from := i - 20
	if from < 0 {
		from = 0
	}
	to := i + 40
	if to > len(s) {
		to = len(s)
	}
	return s[from:to]
}
