package handler

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_60_MntCalendarScheduleShow(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "m60.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("adminadmin")); err != nil {
		t.Fatal(err)
	}
	oa := &model.User{Username: "oa60", PasswordHash: HashPassword("password10"), FullName: "도서관장60", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c60a','계획사이트','계획사이트',1,?),('c60b','조치사이트','조치사이트',1,?)`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	mnt := repository.NewMaintenanceRepo(db)
	_ = mnt.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "c60a", ShortName: "계획사이트", InspectionCycle: "monthly", OrgID: model.OrgIDLibrary})
	_ = mnt.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "c60b", ShortName: "조치사이트", InspectionCycle: "monthly", OrgID: model.OrgIDLibrary})
	p, err := mnt.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := mnt.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-10-08", CustomerID: "c60a", ProductType: "KLAS", Assignee: oa.FullName,
	}); err != nil {
		t.Fatal(err)
	}
	if err := mnt.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-09-01", CustomerID: "c60b", ProductType: "KLAS", Assignee: oa.FullName,
		Completed: true, CompletedDate: "2026-10-08",
	}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/:id", h.Maintenance.ShowPlan)
	ck := jwtCookieUser(t, oa)

	def := doGetWith(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10", ck)
	if def.Code != http.StatusOK {
		t.Fatalf("cal %d %s", def.Code, def.Body.String())
	}
	body := def.Body.String()
	if !strings.Contains(body, "접수(계획)") || !strings.Contains(body, "완료") {
		t.Fatal("정기점검 토글 없음")
	}
	days := mntDayVisitsJSON(body)
	if !strings.Contains(days, "계획사이트") {
		t.Fatal("기본 접수에 방문일 사이트 없음")
	}
	if strings.Contains(days, "조치사이트") {
		t.Fatal("기본 접수에 완료일만 사이트가 보임")
	}
	if !strings.Contains(body, "show=") {
		t.Fatal("이전/다음·보기 링크에 show 없음")
	}

	act := doGetWith(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10&show=act", ck)
	ab := act.Body.String()
	ad := mntDayVisitsJSON(ab)
	if strings.Contains(ad, "계획사이트") {
		t.Fatal("조치만인데 미완료 방문이 보임")
	}
	if !strings.Contains(ad, "조치사이트") {
		t.Fatal("조치만인데 완료 사이트 없음")
	}
	if !strings.Contains(ab, "접수(계획)") || !strings.Contains(ab, "완료") {
		t.Fatal("꺼진 쪽 토글이 사라짐")
	}

	both := doGetWith(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10&show=plan,act", ck)
	bb := both.Body.String()
	bd := mntDayVisitsJSON(bb)
	if !strings.Contains(bd, "계획사이트") || !strings.Contains(bd, "조치사이트") {
		t.Fatal("둘 다 켜기 누락")
	}

	list := doGetWith(t, e, "/maintenance/"+p.PlanID+"?view=list&month=10&show=plan", ck)
	if !strings.Contains(list.Body.String(), "id=\"schedule-show\"") {
		t.Fatal("목록에도 토글 없음")
	}
}

func mntDayVisitsJSON(body string) string {
	i := strings.Index(body, `id="mnt-day-visits"`)
	if i < 0 {
		return ""
	}
	rest := body[i:]
	a := strings.Index(rest, ">")
	b := strings.Index(rest, "</script>")
	if a < 0 || b < 0 || b <= a {
		return ""
	}
	return rest[a+1 : b]
}

func Test95_60_FilterVisitsByScheduleShow(t *testing.T) {
	vs := []model.MaintenanceVisit{
		{VisitID: "a", VisitDate: "2026-10-08", ShortName: "계획"},
		{VisitID: "b", VisitDate: "2026-09-01", ShortName: "조치", Completed: true, CompletedDate: "2026-10-08"},
	}
	plan := filterVisitsByMonthShow(vs, 2026, 10, true, false)
	if len(plan) != 1 || plan[0].ShortName != "계획" {
		t.Fatalf("10월 접수만 %+v", plan)
	}
	act := filterVisitsByMonthShow(vs, 2026, 10, false, true)
	if len(act) != 1 || act[0].ShortName != "조치" {
		t.Fatalf("10월 조치만 %+v", act)
	}
	both := filterVisitsByMonthShow(vs, 2026, 10, true, true)
	if len(both) != 2 {
		t.Fatalf("둘 다 %d", len(both))
	}
}
