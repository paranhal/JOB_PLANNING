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

func TestMnt95_16CalendarAssigneeOrgIsolation(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "c16.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	vs := &model.User{Username: "vs16", PasswordHash: "x", FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa := &model.User{Username: "oa16", PasswordHash: "x", FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	si := &model.User{Username: "si16", PasswordHash: "x", FullName: "에스아이", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	libU := &model.User{Username: "lu16", PasswordHash: "x", FullName: "도서관원", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	siU := &model.User{Username: "su16", PasswordHash: "x", FullName: "에스원", Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true}
	for _, u := range []*model.User{vs, oa, si, libU, siU} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c16a','도서관','도서관',1,?),('c16b','SI학교','SI학교',1,'O03')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	wg := repository.NewWorkGroupRepo(db)
	gLib := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "도서관점검팀"}
	if err := wg.CreateWithMembers(model.WorkGroupActor{UserID: oa.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}, gLib, []string{libU.UserID}, libU.UserID); err != nil {
		t.Fatal(err)
	}
	gSI := &model.WorkGroup{OrgID: "O03", Name: "SI점검팀"}
	if err := wg.CreateWithMembers(model.WorkGroupActor{UserID: si.UserID, OrgID: "O03", Role: model.RoleOrgAdmin}, gSI, []string{siU.UserID}, siU.UserID); err != nil {
		t.Fatal(err)
	}
	mnt := repository.NewMaintenanceRepo(db)
	_ = mnt.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "c16a", ShortName: "도서관", InspectionCycle: "monthly", OrgID: model.OrgIDLibrary})
	_ = mnt.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "c16b", ShortName: "SI학교", InspectionCycle: "monthly", OrgID: "O03"})
	p, err := mnt.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := mnt.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-10-08", CustomerID: "c16a", ProductType: "KLAS", Assignee: libU.FullName,
	}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/:id", h.Maintenance.ShowPlan)

	oaC := jwtCookieUserClaims(t, oa)
	rec := doGetWith(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10", oaC)
	if rec.Code != http.StatusOK {
		t.Fatalf("oa %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "도서관점검팀") {
		t.Fatal("자기 조직 그룹")
	}
	if strings.Contains(body, "SI점검팀") {
		t.Fatal("타 조직 그룹이 드롭다운에 있음")
	}

	vsC := jwtCookieUserClaims(t, vs)
	rec = doGetWith(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10", vsC)
	if rec.Code != http.StatusOK {
		t.Fatalf("vs %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "도서관점검팀") || !strings.Contains(body, "SI점검팀") {
		t.Fatal("비젼관리자 전체 그룹")
	}
}
