package handler

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newMnt9514Server(t *testing.T) (*echo.Echo, *repository.MaintenanceRepo, *repository.UserRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "h14.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c14h','해미도서관','해미도서관',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewMaintenanceRepo(db)
	_ = repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14h", ShortName: "해미", HasKlas: true, InspectionCycle: "monthly", RoundsPerPeriod: 2, Region: "세종",
	})
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/maintenance/:id/visits", h.Maintenance.AddVisit)
	g.GET("/maintenance/regions", h.Maintenance.RegionAssignees)
	g.POST("/maintenance/regions", h.Maintenance.SaveRegionAssignees)
	g.POST("/maintenance/regions/reapply", h.Maintenance.ReapplyRegionAssignees)
	return e, repo, users
}

func TestMnt95_14HTTPAddSecondRoundSameMonth(t *testing.T) {
	e, repo, _ := newMnt9514Server(t)
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	rec1 := doForm(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date":   {"2026-08-13"},
		"customer_id":  {"c14h"},
		"product_type": {"KLAS"},
		"assignee":     {"관리자"},
		"round_no":     {"1"},
	})
	if rec1.Code != http.StatusSeeOther {
		t.Fatalf("1회차 %d %s", rec1.Code, rec1.Body.String())
	}
	rec2 := doForm(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date":   {"2026-08-20"},
		"customer_id":  {"c14h"},
		"product_type": {"KLAS"},
		"assignee":     {"관리자"},
		"round_no":     {"2"},
	})
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("2회차 같은 달 %d loc=%s body=%s", rec2.Code, rec2.Header().Get("Location"), rec2.Body.String())
	}
	visits, _ := repo.ListVisits(p.PlanID)
	if len(visits) != 2 {
		t.Fatalf("visits %d", len(visits))
	}
}

func TestMnt95_14HTTPSameRoundFlash(t *testing.T) {
	e, repo, _ := newMnt9514Server(t)
	p, _ := repo.CreatePlan(2026, "t")
	_ = doForm(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date": {"2026-08-13"}, "customer_id": {"c14h"}, "product_type": {"KLAS"}, "round_no": {"1"},
	})
	rec := doForm(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date": {"2026-08-20"}, "customer_id": {"c14h"}, "product_type": {"KLAS"}, "round_no": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("flash 303 got %d", rec.Code)
	}
	ck := rec.Result().Cookies()
	found := false
	for _, c := range ck {
		if c.Name != "jp_flash_err" {
			continue
		}
		raw, _ := url.QueryUnescape(c.Value)
		if strings.Contains(raw, "월") || strings.Contains(raw, "회차") {
			found = true
		}
	}
	if !found {
		t.Fatalf("중복 안내 쿠키 없음 %+v loc=%s", ck, rec.Header().Get("Location"))
	}
}

func TestMnt95_14RegionPageForbiddenForStaff(t *testing.T) {
	e, _, users := newMnt9514Server(t)
	tech := &model.User{Username: "tech14", PasswordHash: HashPassword("x"), FullName: "기술", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(tech); err != nil {
		t.Fatal(err)
	}
	post := doFormUser(t, e, "/maintenance/regions", url.Values{"region": {"세종"}, "owner_id": {tech.UserID}}, jwtCookieUserClaims(t, tech))
	if post.Code == http.StatusOK {
		t.Fatal("조직원 편집은 거부되어야 함")
	}
}
