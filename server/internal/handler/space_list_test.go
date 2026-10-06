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

func seedSpaceListDB(t *testing.T) (*echo.Echo, *repository.SpaceRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "space-list.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	org := model.OrgIDLibrary
	must := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO customers (customer_id, org_name, official_name, org_id, is_active)
		VALUES ('C1','가도서관','가도서관',?,1)`, org)
	must(`INSERT INTO customer_buildings (building_id,customer_id,building_name,is_active)
		VALUES ('B1','C1','본관',1), ('B2','C1','층만관',1), ('B3','C1','빈건물',1), ('B4','C1','숨김관',0)`)
	must(`INSERT INTO customer_floors (floor_id,building_id,floor_name,sort_order)
		VALUES ('F2','B1','2층',2), ('F3','B1','3층',3), ('Fempty','B2','1층',1), ('F4','B4','지하',1)`)
	must(`INSERT INTO customer_rooms (room_id,floor_id,room_name)
		VALUES ('R10','F2','10호'), ('R2','F2','2호'), ('Rhid','F4','99호')`)

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/spaces", h.Space.List, h.Auth.RequireMasterView)
	return e, repository.NewSpaceRepo(db)
}

func getSpaces(t *testing.T, e *echo.Echo, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestSpaceListRoomNumericOrder(t *testing.T) {
	e, _ := seedSpaceListDB(t)
	body := getSpaces(t, e, "/spaces")
	i2 := strings.Index(body, ">2호<")
	i10 := strings.Index(body, ">10호<")
	if i2 < 0 || i10 < 0 || i2 > i10 {
		t.Fatalf("2호가 10호 앞에 와야 한다 i2=%d i10=%d", i2, i10)
	}
}

func TestSpaceListEmptyFloorAndBuildingRows(t *testing.T) {
	e, repo := seedSpaceListDB(t)
	rows, _, _, err := repo.ListSpaceRows(model.OrgIDLibrary, "", "", "", "", false, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var floorOnly, bldOnly bool
	for _, r := range rows {
		if r.BuildingID == "B2" && r.FloorName == "1층" && r.RoomName == "" {
			floorOnly = true
		}
		if r.BuildingID == "B3" && r.FloorName == "" && r.RoomName == "" {
			bldOnly = true
		}
	}
	if !floorOnly {
		t.Fatal("층만 있는 건물이 줄로 안 나온다")
	}
	if !bldOnly {
		t.Fatal("층 0개인 건물이 줄로 안 나온다")
	}
	body := getSpaces(t, e, "/spaces")
	if !strings.Contains(body, "층만관") || !strings.Contains(body, "빈건물") {
		t.Fatal("빈 건물·빈 층이 화면에 없다")
	}
}

func TestSpaceListSearchFloorAndOrg(t *testing.T) {
	e, _ := seedSpaceListDB(t)
	body := getSpaces(t, e, "/spaces?q=3층")
	if !strings.Contains(body, "3층") || strings.Contains(body, ">2호<") {
		t.Fatal("q=3층 이 층명으로 안 걸린다")
	}
	body = getSpaces(t, e, "/spaces?q=가도서관")
	if !strings.Contains(body, "가도서관") {
		t.Fatal("q=기관이름이 안 걸린다")
	}
}

func TestSpaceListHidesInactiveUnlessIncluded(t *testing.T) {
	e, repo := seedSpaceListDB(t)
	rows, _, _, err := repo.ListSpaceRows(model.OrgIDLibrary, "", "", "", "", false, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.BuildingID == "B4" {
			t.Fatal("비활성 건물이 나왔다")
		}
	}
	body := getSpaces(t, e, "/spaces")
	if strings.Contains(body, "숨김관") {
		t.Fatal("비활성 건물이 화면에 있다")
	}
	rows, _, _, err = repo.ListSpaceRows(model.OrgIDLibrary, "", "", "", "", true, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.BuildingID == "B4" {
			found = true
		}
	}
	if !found {
		t.Fatal("include_inactive=1 인데 비활성 건물이 없다")
	}
	body = getSpaces(t, e, "/spaces?include_inactive=1")
	if !strings.Contains(body, "숨김관") {
		t.Fatal("include_inactive=1 화면에 숨김관이 없다")
	}
}

func TestSpaceListSortRoomDesc(t *testing.T) {
	e, _ := seedSpaceListDB(t)
	body := getSpaces(t, e, "/spaces?sort=room&dir=desc")
	i2 := strings.Index(body, ">2호<")
	i10 := strings.Index(body, ">10호<")
	if i2 < 0 || i10 < 0 || i10 > i2 {
		t.Fatalf("room desc 에서 10호가 2호 앞이어야 한다 i10=%d i2=%d", i10, i2)
	}
}

func TestSpaceListEmptyOrgIsError(t *testing.T) {
	_, repo := seedSpaceListDB(t)
	if _, _, _, err := repo.ListSpaceRows("", "", "", "", "", false, 0, 0); err == nil {
		t.Fatal("빈 orgID 가 통과했다")
	}
}
