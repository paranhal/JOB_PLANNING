package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newSpaceApp(t *testing.T) *echo.Echo {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "space-edit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	repository.NewUserRepo(db).EnsureObserver(HashPassword("1234"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, org_id, is_active)
		VALUES ('C1','테스트기관','테스트기관',?,1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customer_buildings (building_id,customer_id,building_name,is_active)
		VALUES ('B1','C1','본관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customer_floors (floor_id,building_id,floor_name,sort_order)
		VALUES ('F1','B1','1층',1)`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/spaces", h.Space.List, h.Auth.RequireMasterView)
	g.GET("/spaces/edit", h.Space.Edit, h.Auth.RequireMasterWrite)
	g.POST("/spaces/buildings", h.Space.CreateBuilding, h.Auth.RequireMasterWrite)
	return e
}

func TestSpaceOldBuildingLinkRedirectsToEdit(t *testing.T) {
	e := newSpaceApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/spaces?customer_id=C1&building_id=B1", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/spaces/edit?") || !strings.Contains(loc, "building_id=B1") {
		t.Fatalf("location=%s", loc)
	}
}

func TestSpaceCustomerOnlyIsFilteredList(t *testing.T) {
	e := newSpaceApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/spaces?customer_id=C1", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "← 공간 목록") {
		t.Fatal("customer_id 만 왔는데 편집 화면이다")
	}
	if !strings.Contains(body, "공간 관리") || !strings.Contains(body, "본관") {
		t.Fatal("필터된 리스트가 아니다")
	}
}

func TestSpaceEditScreenHasBackLink(t *testing.T) {
	e := newSpaceApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/spaces/edit?customer_id=C1&building_id=B1&floor_id=F1", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "← 공간 목록") {
		t.Fatal("← 공간 목록 없음")
	}
	if !strings.Contains(body, `href="/spaces"`) {
		t.Fatal("목록 링크가 /spaces 가 아니다")
	}
}

func TestSpaceEditRequiresMasterEdit(t *testing.T) {
	e := newSpaceApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/spaces/buildings", strings.NewReader("customer_id=C1&building_name=별관"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieReadOnly(t, model.RoleTech))
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("읽기전용이 건물을 만들었다 status=%d", rec.Code)
	}
}

func TestSpaceListAllowsMasterView(t *testing.T) {
	e := newSpaceApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/spaces?customer_id=C1", nil)
	req.AddCookie(jwtCookieReadOnly(t, model.RoleTech))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("observer list status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSpaceCreateBuildingReturnsToEdit(t *testing.T) {
	e := newSpaceApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/spaces/buildings", strings.NewReader("customer_id=C1&building_name=별관"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/spaces/edit?") {
		t.Fatalf("location=%s", loc)
	}
}
